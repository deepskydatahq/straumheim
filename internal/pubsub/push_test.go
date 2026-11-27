package pubsub

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deepskydatahq/straumheim/internal/record"
)

type fakeRecordWriter struct {
	records []record.Record
	err     error
}

func (w *fakeRecordWriter) Write(_ context.Context, records []record.Record) error {
	w.records = append(w.records, records...)
	return w.err
}

func pushRequest(t *testing.T, r record.Record, attributes map[string]string) *http.Request {
	t.Helper()
	data, err := marshalRecord(r)
	if err != nil {
		t.Fatal(err)
	}
	envelope := pushEnvelope{
		Message: pushMessage{
			Data:       base64.StdEncoding.EncodeToString(data),
			Attributes: attributes,
			MessageID:  "message-1",
		},
		Subscription:    "projects/p/subscriptions/s",
		DeliveryAttempt: 2,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewRequest(http.MethodPost, "/internal/pubsub/push", bytes.NewReader(body))
}

func TestPushHandlerWritesBeforeAcknowledgement(t *testing.T) {
	writer := &fakeRecordWriter{}
	handler := NewPushHandler(writer)
	want := testRecord("event-1")
	req := pushRequest(t, want, map[string]string{"record_id": want.ID})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", response.Code, response.Body.String())
	}
	if len(writer.records) != 1 {
		t.Fatalf("writer records = %d, want 1", len(writer.records))
	}
	got := writer.records[0]
	if got.ID != want.ID || !got.Timestamp.Equal(want.Timestamp) || !got.ReceivedAt.Equal(want.ReceivedAt) {
		t.Fatalf("written record = %+v, want identity/timestamps from %+v", got, want)
	}
}

func TestPushHandlerReturnsRetryableFailure(t *testing.T) {
	wantErr := errors.New("BigQuery unavailable")
	writer := &fakeRecordWriter{err: wantErr}
	handler := NewPushHandler(writer)
	req := pushRequest(t, testRecord("event-1"), nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	if len(writer.records) != 1 || writer.records[0].ID != "event-1" {
		t.Fatalf("writer records = %+v", writer.records)
	}
	if strings.Contains(response.Body.String(), wantErr.Error()) {
		t.Fatal("response must not expose destination details")
	}
}

func TestPushHandlerRejectsMalformedMessages(t *testing.T) {
	valid := testRecord("event-1")
	validData, err := marshalRecord(valid)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		method string
		body   string
	}{
		{name: "method", method: http.MethodGet, body: `{}`},
		{name: "invalid envelope", method: http.MethodPost, body: `{`},
		{name: "multiple values", method: http.MethodPost, body: `{}` + `{}`},
		{name: "missing message ID", method: http.MethodPost, body: `{"message":{"data":"e30="}}`},
		{name: "missing data", method: http.MethodPost, body: `{"message":{"messageId":"m"}}`},
		{name: "invalid base64", method: http.MethodPost, body: `{"message":{"messageId":"m","data":"%%%"}}`},
		{name: "invalid record", method: http.MethodPost, body: `{"message":{"messageId":"m","data":"e30="}}`},
		{
			name:   "attribute mismatch",
			method: http.MethodPost,
			body: `{"message":{"messageId":"m","data":"` + base64.StdEncoding.EncodeToString(validData) +
				`","attributes":{"record_id":"different"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := &fakeRecordWriter{}
			handler := NewPushHandler(writer)
			request := httptest.NewRequest(tt.method, "/internal/pubsub/push", strings.NewReader(tt.body))
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
			}
			if len(writer.records) != 0 {
				t.Fatalf("writer received malformed records: %+v", writer.records)
			}
		})
	}
}

func TestPushHandlerUnavailableWriter(t *testing.T) {
	handler := NewPushHandler(nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, pushRequest(t, testRecord("event-1"), nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}
