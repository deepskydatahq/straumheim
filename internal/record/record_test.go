package record

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewRecord_HasValidUUIDv7(t *testing.T) {
	r := NewRecord()
	parsed, err := uuid.Parse(r.ID)
	if err != nil {
		t.Fatalf("ID is not a valid UUID: %v", err)
	}
	if parsed.Version() != 7 {
		t.Fatalf("expected UUID version 7, got %d", parsed.Version())
	}
}

func TestNewRecord_SetsTimestamp(t *testing.T) {
	before := time.Now()
	r := NewRecord()
	after := time.Now()

	if r.Timestamp.Before(before) || r.Timestamp.After(after) {
		t.Fatalf("Timestamp %v not between %v and %v", r.Timestamp, before, after)
	}
}

func TestNewRecord_SetsReceivedAt(t *testing.T) {
	r := NewRecord()
	if r.ReceivedAt.IsZero() {
		t.Fatal("ReceivedAt should not be zero")
	}
}

func TestNewRecord_HasAllFields(t *testing.T) {
	r := NewRecord()
	// Just verify the struct is usable with all fields
	r.DeviceTime = nil
	r.Protocol = "webhook"
	r.Source = "test"
	r.Schema = "event"
	r.Vendor = "acme"
	r.SchemaVersion = "1.0"
	valid := true
	r.IsValid = &valid
	r.Payload = map[string]any{"key": "value"}
	r.Flattened = map[string]any{"key": "value"}
	r.IP = "127.0.0.1"
	r.UserAgent = "test-agent"
	r.Referer = "https://example.com"

	if r.Protocol != "webhook" {
		t.Fatal("field assignment failed")
	}
}

func TestRecord_JSONSerialization_ExcludesReceivedAt(t *testing.T) {
	r := NewRecord()
	r.ReceivedAt = time.Now()

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if _, ok := m["received_at"]; ok {
		t.Fatal("ReceivedAt should not appear in JSON output")
	}
}

func TestRecord_JSONSerialization_IncludesAllPublicFields(t *testing.T) {
	r := NewRecord()
	dt := time.Now()
	r.DeviceTime = &dt
	r.Protocol = "webhook"
	r.Source = "test"
	r.Schema = "event"
	r.Vendor = "acme"
	r.SchemaVersion = "1.0"
	valid := true
	r.IsValid = &valid
	r.Payload = map[string]any{"key": "value"}
	r.Flattened = map[string]any{"flat": "value"}
	r.IP = "127.0.0.1"
	r.UserAgent = "agent"
	r.Referer = "ref"

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	expected := []string{
		"id", "timestamp", "device_time", "protocol", "source",
		"schema", "vendor", "schema_version", "is_valid",
		"payload", "flattened", "ip", "user_agent", "referer",
	}
	for _, field := range expected {
		if _, ok := m[field]; !ok {
			t.Errorf("expected field %q in JSON output", field)
		}
	}
}
