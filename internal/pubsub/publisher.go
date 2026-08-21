package pubsub

import (
	"context"
	"fmt"

	gcppubsub "cloud.google.com/go/pubsub/v2"

	"github.com/deepskydatahq/straumheim/internal/record"
)

// PublishResult is the confirmed result of one asynchronous client publish.
type PublishResult interface {
	Get(context.Context) (string, error)
}

// MessagePublisher is the small publish boundary used by PublisherPipeline.
type MessagePublisher interface {
	Publish(context.Context, []byte, map[string]string) PublishResult
	Stop()
}

// PublisherPipeline durably publishes canonical Records before Ingest returns.
type PublisherPipeline struct {
	publisher   MessagePublisher
	closeClient func() error
	observer    Observer
}

// NewPublisherPipeline constructs a publisher pipeline from testable dependencies.
func NewPublisherPipeline(publisher MessagePublisher, closeClient func() error) *PublisherPipeline {
	if closeClient == nil {
		closeClient = func() error { return nil }
	}
	return &PublisherPipeline{publisher: publisher, closeClient: closeClient}
}

// SetObserver attaches delivery metrics before the pipeline serves requests.
func (p *PublisherPipeline) SetObserver(observer Observer) { p.observer = observer }

// NewGooglePublisherPipeline creates a Pub/Sub publisher using Application Default Credentials.
func NewGooglePublisherPipeline(ctx context.Context, project, topic string) (*PublisherPipeline, error) {
	if project == "" {
		return nil, fmt.Errorf("pubsub publisher: project is required")
	}
	if topic == "" {
		return nil, fmt.Errorf("pubsub publisher: topic is required")
	}
	client, err := gcppubsub.NewClient(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("pubsub publisher: create client: %w", err)
	}
	publisher := client.Publisher(topic)
	return NewPublisherPipeline(
		&googleMessagePublisher{publisher: publisher},
		client.Close,
	), nil
}

// Ingest publishes every Record and waits for every server result. Publishing a
// batch is not atomic; callers retain Record IDs when retrying after an error.
func (p *PublisherPipeline) Ingest(ctx context.Context, records []record.Record) error {
	if len(records) == 0 {
		return nil
	}
	if p.publisher == nil {
		return fmt.Errorf("pubsub publisher: not initialized")
	}

	type pendingPublish struct {
		recordID string
		result   PublishResult
	}
	pending := make([]pendingPublish, 0, len(records))
	messages := make([][]byte, len(records))
	for i, r := range records {
		data, err := marshalRecord(r)
		if err != nil {
			p.observePublish(r.Protocol, resultFailure)
			return err
		}
		messages[i] = data
	}
	for i, r := range records {
		result := p.publisher.Publish(ctx, messages[i], map[string]string{
			"record_id": r.ID,
			"protocol":  r.Protocol,
		})
		if result == nil {
			p.observePublish(r.Protocol, resultFailure)
			return fmt.Errorf("pubsub publisher: publish record %q returned no result", r.ID)
		}
		pending = append(pending, pendingPublish{recordID: r.ID, result: result})
	}
	for i, publish := range pending {
		if _, err := publish.result.Get(ctx); err != nil {
			p.observePublish(records[i].Protocol, resultFailure)
			return fmt.Errorf("pubsub publisher: confirm record %q: %w", publish.recordID, err)
		}
		p.observePublish(records[i].Protocol, resultSuccess)
	}
	return nil
}

// Close stops the topic publisher and closes the Pub/Sub client.
func (p *PublisherPipeline) Close() error {
	if p.publisher != nil {
		p.publisher.Stop()
	}
	return p.closeClient()
}

func (p *PublisherPipeline) observePublish(protocol, result string) {
	if p.observer != nil {
		p.observer.RecordPubSubPublish(protocol, result)
	}
}

type googleMessagePublisher struct {
	publisher *gcppubsub.Publisher
}

func (p *googleMessagePublisher) Publish(ctx context.Context, data []byte, attributes map[string]string) PublishResult {
	return p.publisher.Publish(ctx, &gcppubsub.Message{Data: data, Attributes: attributes})
}

func (p *googleMessagePublisher) Stop() { p.publisher.Stop() }

var _ interface {
	Ingest(context.Context, []record.Record) error
} = (*PublisherPipeline)(nil)
