package pubsub

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deepskydatahq/straumheim/internal/record"
)

type fakePublishResult struct {
	id     string
	err    error
	called bool
}

func (r *fakePublishResult) Get(context.Context) (string, error) {
	r.called = true
	return r.id, r.err
}

type publishedMessage struct {
	data       []byte
	attributes map[string]string
}

type fakeMessagePublisher struct {
	messages []publishedMessage
	results  []PublishResult
	stopped  bool
}

func (p *fakeMessagePublisher) Publish(_ context.Context, data []byte, attributes map[string]string) PublishResult {
	p.messages = append(p.messages, publishedMessage{data: data, attributes: attributes})
	if len(p.results) == 0 {
		return &fakePublishResult{id: "message-id"}
	}
	result := p.results[0]
	p.results = p.results[1:]
	return result
}

func (p *fakeMessagePublisher) Stop() { p.stopped = true }

func testRecord(id string) record.Record {
	timestamp := time.Date(2026, 8, 21, 8, 0, 0, 123456000, time.UTC)
	return record.Record{
		ID:         id,
		Timestamp:  timestamp,
		ReceivedAt: timestamp.Add(time.Second),
		Protocol:   "webhook",
		Payload:    map[string]any{"event": "signup", "nested": map[string]any{"count": float64(2)}},
		Flattened:  map[string]any{"event": "signup", "nested_count": float64(2)},
	}
}

func TestPublisherPipelineIngestConfirmsCanonicalRecords(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	pipeline := NewPublisherPipeline(publisher, nil)
	records := []record.Record{testRecord("event-1"), testRecord("event-2")}

	if err := pipeline.Ingest(context.Background(), records); err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(publisher.messages) != 2 {
		t.Fatalf("published %d messages, want 2", len(publisher.messages))
	}
	for i, message := range publisher.messages {
		want := records[i]
		if message.attributes["record_id"] != want.ID || message.attributes["protocol"] != want.Protocol {
			t.Fatalf("attributes = %v, want record ID and protocol", message.attributes)
		}
		got, err := unmarshalRecord(message.data)
		if err != nil {
			t.Fatalf("unmarshal published record: %v", err)
		}
		if got.ID != want.ID || !got.Timestamp.Equal(want.Timestamp) || !got.ReceivedAt.Equal(want.ReceivedAt) {
			t.Fatalf("published record = %+v, want identity/timestamps from %+v", got, want)
		}
		if got.Payload["event"] != "signup" || got.Flattened["nested_count"] != float64(2) {
			t.Fatalf("published JSON fields = payload %v flattened %v", got.Payload, got.Flattened)
		}
	}
}

func TestPublisherPipelineIngestErrors(t *testing.T) {
	t.Run("not initialized", func(t *testing.T) {
		pipeline := NewPublisherPipeline(nil, nil)
		err := pipeline.Ingest(context.Background(), []record.Record{testRecord("event-1")})
		if err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("Ingest() error = %v", err)
		}
	})

	t.Run("serialization before publish", func(t *testing.T) {
		publisher := &fakeMessagePublisher{}
		pipeline := NewPublisherPipeline(publisher, nil)
		bad := testRecord("bad-event")
		bad.Payload = map[string]any{"unsupported": make(chan int)}
		err := pipeline.Ingest(context.Background(), []record.Record{testRecord("good-event"), bad})
		if err == nil || !strings.Contains(err.Error(), `marshal record "bad-event"`) {
			t.Fatalf("Ingest() error = %v", err)
		}
		if len(publisher.messages) != 0 {
			t.Fatalf("published %d messages before serialization completed", len(publisher.messages))
		}
	})

	t.Run("confirmation", func(t *testing.T) {
		wantErr := errors.New("quota unavailable")
		publisher := &fakeMessagePublisher{results: []PublishResult{&fakePublishResult{err: wantErr}}}
		pipeline := NewPublisherPipeline(publisher, nil)
		err := pipeline.Ingest(context.Background(), []record.Record{testRecord("event-1")})
		if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), `confirm record "event-1"`) {
			t.Fatalf("Ingest() error = %v, want wrapped %v", err, wantErr)
		}
	})

	t.Run("waits for all results after failure", func(t *testing.T) {
		wantErr := errors.New("first failed")
		first := &fakePublishResult{err: wantErr}
		second := &fakePublishResult{id: "second"}
		publisher := &fakeMessagePublisher{results: []PublishResult{first, second}}
		pipeline := NewPublisherPipeline(publisher, nil)
		err := pipeline.Ingest(context.Background(), []record.Record{testRecord("event-1"), testRecord("event-2")})
		if !errors.Is(err, wantErr) {
			t.Fatalf("Ingest() error = %v, want wrapped %v", err, wantErr)
		}
		if !first.called || !second.called {
			t.Fatalf("result calls: first=%t second=%t, want both", first.called, second.called)
		}
	})

	t.Run("nil result", func(t *testing.T) {
		publisher := &fakeMessagePublisher{results: []PublishResult{nil}}
		pipeline := NewPublisherPipeline(publisher, nil)
		err := pipeline.Ingest(context.Background(), []record.Record{testRecord("event-1")})
		if err == nil || !strings.Contains(err.Error(), "returned no result") {
			t.Fatalf("Ingest() error = %v", err)
		}
	})
}

func TestPublisherPipelineEmptyIngestAndClose(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	wantErr := errors.New("client close")
	pipeline := NewPublisherPipeline(publisher, func() error { return wantErr })

	if err := pipeline.Ingest(context.Background(), nil); err != nil {
		t.Fatalf("empty Ingest() error: %v", err)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("empty Ingest() published %d messages", len(publisher.messages))
	}
	if err := pipeline.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("Close() error = %v, want %v", err, wantErr)
	}
	if !publisher.stopped {
		t.Fatal("expected publisher to stop")
	}
}

func TestUnmarshalRecordValidation(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "invalid JSON", data: `{`, want: "unmarshal record"},
		{name: "ID", data: `{"timestamp":"2026-08-21T08:00:00Z","received_at":"2026-08-21T08:00:00Z"}`, want: "id is required"},
		{name: "timestamp", data: `{"id":"event-1","received_at":"2026-08-21T08:00:00Z"}`, want: "timestamp is required"},
		{name: "received_at", data: `{"id":"event-1","timestamp":"2026-08-21T08:00:00Z"}`, want: "received_at is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := unmarshalRecord([]byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unmarshalRecord() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
