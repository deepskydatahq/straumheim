package pubsub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/deepskydatahq/straumheim/internal/input"
)

func deliverPublishedMessages(t *testing.T, publisher *fakeMessagePublisher) []string {
	t.Helper()
	writer := &fakeRecordWriter{}
	handler := NewPushHandler(writer)
	for i, message := range publisher.messages {
		envelope := map[string]any{"message": map[string]any{
			"messageId":  "message-integration",
			"data":       base64.StdEncoding.EncodeToString(message.data),
			"attributes": message.attributes,
		}}
		body, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/internal/pubsub/push", bytes.NewReader(body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("message %d push status = %d: %s", i, response.Code, response.Body.String())
		}
	}
	protocols := make([]string, len(writer.records))
	for i, recordValue := range writer.records {
		protocols[i] = recordValue.Protocol
		if publisher.messages[i].attributes["record_id"] != recordValue.ID {
			t.Fatalf("message %d attribute ID %q != delivered ID %q", i, publisher.messages[i].attributes["record_id"], recordValue.ID)
		}
	}
	return protocols
}

func TestInputsPublishAndPushCanonicalRecords(t *testing.T) {
	tests := []struct {
		name          string
		register      func(chi.Router, *PublisherPipeline)
		request       func() *http.Request
		wantProtocols []string
	}{
		{
			name: "webhook",
			register: func(r chi.Router, p *PublisherPipeline) {
				input.NewWebhook().Register(r, p)
			},
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(`{"event":"signup"}`))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			wantProtocols: []string{"webhook"},
		},
		{
			name: "pixel",
			register: func(r chi.Router, p *PublisherPipeline) {
				input.NewPixel().Register(r, p)
			},
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/px?event=open", nil)
			},
			wantProtocols: []string{"pixel"},
		},
		{
			name: "snowplow batch",
			register: func(r chi.Router, p *PublisherPipeline) {
				input.NewSnowplowInput(input.SnowplowConfig{}).Register(r, p)
			},
			request: func() *http.Request {
				body := bytes.NewBufferString(`{"schema":"iglu:test","data":[{"e":"pv","aid":"app"},{"e":"pp","aid":"app"}]}`)
				req := httptest.NewRequest(http.MethodPost, "/sp/tp2", body)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			wantProtocols: []string{"snowplow", "snowplow"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publisher := &fakeMessagePublisher{}
			pipeline := NewPublisherPipeline(publisher, nil)
			router := chi.NewRouter()
			tt.register(router, pipeline)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, tt.request())

			if response.Code != http.StatusOK {
				t.Fatalf("input status = %d, want 200: %s", response.Code, response.Body.String())
			}
			protocols := deliverPublishedMessages(t, publisher)
			if len(protocols) != len(tt.wantProtocols) {
				t.Fatalf("delivered protocols = %v, want %v", protocols, tt.wantProtocols)
			}
			for i := range protocols {
				if protocols[i] != tt.wantProtocols[i] {
					t.Fatalf("delivered protocols = %v, want %v", protocols, tt.wantProtocols)
				}
			}
		})
	}
}

func TestInputsFailWhenPublishConfirmationFails(t *testing.T) {
	tests := []struct {
		name     string
		register func(chi.Router, *PublisherPipeline)
		request  func() *http.Request
	}{
		{
			name:     "webhook",
			register: func(r chi.Router, p *PublisherPipeline) { input.NewWebhook().Register(r, p) },
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(`{"event":"signup"}`))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
		},
		{
			name:     "pixel",
			register: func(r chi.Router, p *PublisherPipeline) { input.NewPixel().Register(r, p) },
			request:  func() *http.Request { return httptest.NewRequest(http.MethodGet, "/px?event=open", nil) },
		},
		{
			name: "snowplow",
			register: func(r chi.Router, p *PublisherPipeline) {
				input.NewSnowplowInput(input.SnowplowConfig{}).Register(r, p)
			},
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/sp/i?e=pv", nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publisher := &fakeMessagePublisher{results: []PublishResult{&fakePublishResult{err: errors.New("publish failed")}}}
			pipeline := NewPublisherPipeline(publisher, nil)
			router := chi.NewRouter()
			tt.register(router, pipeline)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, tt.request())

			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", response.Code)
			}
		})
	}
}
