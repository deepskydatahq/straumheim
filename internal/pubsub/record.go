// Package pubsub implements the optional GCP request-scoped delivery profile.
package pubsub

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/deepskydatahq/straumheim/internal/record"
)

type wireRecord struct {
	record.Record
	ReceivedAt time.Time `json:"received_at"`
}

func marshalRecord(r record.Record) ([]byte, error) {
	data, err := json.Marshal(wireRecord{Record: r, ReceivedAt: r.ReceivedAt})
	if err != nil {
		return nil, fmt.Errorf("marshal record %q: %w", r.ID, err)
	}
	return data, nil
}

func unmarshalRecord(data []byte) (record.Record, error) {
	var wire wireRecord
	if err := json.Unmarshal(data, &wire); err != nil {
		return record.Record{}, fmt.Errorf("unmarshal record: %w", err)
	}
	wire.Record.ReceivedAt = wire.ReceivedAt
	if wire.ID == "" {
		return record.Record{}, fmt.Errorf("validate record: id is required")
	}
	if wire.Timestamp.IsZero() {
		return record.Record{}, fmt.Errorf("validate record %q: timestamp is required", wire.ID)
	}
	if wire.ReceivedAt.IsZero() {
		return record.Record{}, fmt.Errorf("validate record %q: received_at is required", wire.ID)
	}
	return wire.Record, nil
}
