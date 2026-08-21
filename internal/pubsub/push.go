package pubsub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/deepskydatahq/straumheim/internal/record"
)

const maxPushBodySize = 10 << 20 // 10 MiB, below Pub/Sub's maximum request size.

// RecordWriter is the request-scoped destination boundary used by PushHandler.
type RecordWriter interface {
	Write(context.Context, []record.Record) error
}

// PushHandler handles wrapped Pub/Sub push requests. Cloud Run IAM is
// responsible for authenticating the configured push service account.
type PushHandler struct {
	writer RecordWriter
}

// NewPushHandler creates a Pub/Sub push handler for a request-scoped writer.
func NewPushHandler(writer RecordWriter) *PushHandler {
	return &PushHandler{writer: writer}
}

type pushEnvelope struct {
	Message         pushMessage `json:"message"`
	Subscription    string      `json:"subscription"`
	DeliveryAttempt int         `json:"deliveryAttempt"`
}

type pushMessage struct {
	Data        string            `json:"data"`
	Attributes  map[string]string `json:"attributes"`
	MessageID   string            `json:"messageId"`
	PublishTime string            `json:"publishTime"`
	OrderingKey string            `json:"orderingKey"`
}

// ServeHTTP returns 204 only after the writer confirms delivery. Every
// non-success response leaves acknowledgement and bounded retries to Pub/Sub.
func (h *PushHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.writer == nil {
		slog.Error("pubsub push writer unavailable")
		http.Error(w, "writer unavailable", http.StatusServiceUnavailable)
		return
	}

	recordValue, envelope, err := decodePushRequest(w, r)
	if err != nil {
		slog.Error("pubsub push rejected",
			"message_id", envelope.Message.MessageID,
			"delivery_attempt", envelope.DeliveryAttempt,
			"error", err,
		)
		http.Error(w, "invalid Pub/Sub message", http.StatusBadRequest)
		return
	}
	if err := h.writer.Write(r.Context(), []record.Record{recordValue}); err != nil {
		slog.Error("pubsub push delivery failed",
			"message_id", envelope.Message.MessageID,
			"record_id", recordValue.ID,
			"delivery_attempt", envelope.DeliveryAttempt,
			"error", err,
		)
		http.Error(w, "delivery failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func decodePushRequest(w http.ResponseWriter, r *http.Request) (record.Record, pushEnvelope, error) {
	var envelope pushEnvelope
	if r.Method != http.MethodPost {
		return record.Record{}, envelope, fmt.Errorf("method %s is not POST", r.Method)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPushBodySize)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&envelope); err != nil {
		return record.Record{}, envelope, fmt.Errorf("decode envelope: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return record.Record{}, envelope, err
	}
	if envelope.Message.MessageID == "" {
		return record.Record{}, envelope, fmt.Errorf("messageId is required")
	}
	if envelope.Message.Data == "" {
		return record.Record{}, envelope, fmt.Errorf("message data is required")
	}
	data, err := base64.StdEncoding.DecodeString(envelope.Message.Data)
	if err != nil {
		return record.Record{}, envelope, fmt.Errorf("decode message data: %w", err)
	}
	recordValue, err := unmarshalRecord(data)
	if err != nil {
		return record.Record{}, envelope, err
	}
	if attributeID := envelope.Message.Attributes["record_id"]; attributeID != "" && attributeID != recordValue.ID {
		return record.Record{}, envelope, fmt.Errorf("record_id attribute %q does not match record %q", attributeID, recordValue.ID)
	}
	return recordValue, envelope, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing envelope data: %w", err)
	}
	return fmt.Errorf("decode envelope: multiple JSON values")
}
