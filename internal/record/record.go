// Package record defines the core Record struct for the event pipeline.
package record

import (
	"time"

	"github.com/google/uuid"
)

// Record represents a single event flowing through the pipeline.
type Record struct {
	ID            string         `json:"id"`
	Timestamp     time.Time      `json:"timestamp"`
	DeviceTime    *time.Time     `json:"device_time"`
	Protocol      string         `json:"protocol"`
	Source        string         `json:"source"`
	Schema        string         `json:"schema"`
	Vendor        string         `json:"vendor"`
	SchemaVersion string         `json:"schema_version"`
	IsValid       *bool          `json:"is_valid"`
	Payload       map[string]any `json:"payload"`
	Flattened     map[string]any `json:"flattened"`
	IP            string         `json:"ip"`
	UserAgent     string         `json:"user_agent"`
	Referer       string         `json:"referer"`
	ReceivedAt    time.Time      `json:"-"`
}

// NewRecord creates a new Record with a UUIDv7 ID and current timestamps.
func NewRecord() Record {
	now := time.Now()
	return Record{
		ID:         uuid.Must(uuid.NewV7()).String(),
		Timestamp:  now,
		ReceivedAt: now,
	}
}
