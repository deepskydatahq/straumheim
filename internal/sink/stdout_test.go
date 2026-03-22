package sink

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/deepsky-data/straumheim/internal/record"
)

func TestStdoutSinkMode(t *testing.T) {
	s := NewStdoutSink(nil)
	if s.Mode() != SinkModeStream {
		t.Fatalf("expected SinkModeStream, got %s", s.Mode())
	}
}

func TestStdoutSinkNoOps(t *testing.T) {
	s := NewStdoutSink(nil)
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStdoutSinkWrite(t *testing.T) {
	var buf bytes.Buffer
	s := NewStdoutSink(&buf)

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	isValid := true
	records := []record.Record{
		{
			ID:        "id-1",
			Timestamp: now,
			Protocol:  "http",
			Source:    "web",
			IsValid:   &isValid,
			Payload:   map[string]any{"key": "value"},
		},
		{
			ID:        "id-2",
			Timestamp: now,
			Protocol:  "ws",
			Source:    "mobile",
		},
	}

	if err := s.Write(context.Background(), records); err != nil {
		t.Fatal(err)
	}

	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Verify first line is valid JSON with expected fields.
	var parsed map[string]any
	if err := json.Unmarshal(lines[0], &parsed); err != nil {
		t.Fatalf("line 0 is not valid JSON: %v", err)
	}
	if parsed["id"] != "id-1" {
		t.Errorf("expected id id-1, got %v", parsed["id"])
	}
	if parsed["protocol"] != "http" {
		t.Errorf("expected protocol http, got %v", parsed["protocol"])
	}
}

func TestStdoutSinkWriteEmpty(t *testing.T) {
	var buf bytes.Buffer
	s := NewStdoutSink(&buf)
	if err := s.Write(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty output, got %d bytes", buf.Len())
	}
}

// Verify StdoutSink implements Sink interface.
var _ Sink = (*StdoutSink)(nil)
