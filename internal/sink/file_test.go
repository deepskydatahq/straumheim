package sink

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deepsky-data/straumheim/internal/record"
)

// Interface compliance.
var _ Sink = (*FileSink)(nil)

func testRecords(n int) []record.Record {
	recs := make([]record.Record, n)
	for i := range recs {
		recs[i] = record.Record{
			ID:        "test-id-" + string(rune('0'+i)),
			Timestamp: time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC),
			Protocol:  "webhook",
			Source:    "test",
			Payload:   map[string]any{"key": "value"},
		}
	}
	return recs
}

func TestFileSink_WriteAppendsJSONLines(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileSink(dir, 5*time.Minute)

	ctx := context.Background()
	if err := fs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	recs := testRecords(3)
	if err := fs.Write(ctx, recs); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := fs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Find the written file.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// Each line must be valid JSON that unmarshals to a Record.
	for i, line := range lines {
		var r record.Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("line %d: invalid JSON: %v", i, err)
		}
		if r.ID != recs[i].ID {
			t.Fatalf("line %d: expected ID %q, got %q", i, recs[i].ID, r.ID)
		}
	}
}

func TestFileSink_FileNaming(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileSink(dir, 5*time.Minute)

	ctx := context.Background()
	if err := fs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := fs.Write(ctx, testRecords(1)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := fs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}

	name := entries[0].Name()
	if !strings.HasPrefix(name, "events-") {
		t.Fatalf("expected file name starting with 'events-', got %q", name)
	}
	if !strings.HasSuffix(name, ".jsonl") {
		t.Fatalf("expected file name ending with '.jsonl', got %q", name)
	}

	// Verify timestamp portion is RFC3339-compact (e.g., 20260322T100000Z).
	ts := strings.TrimPrefix(name, "events-")
	ts = strings.TrimSuffix(ts, ".jsonl")
	_, err := time.Parse("20060102T150405Z", ts)
	if err != nil {
		t.Fatalf("file timestamp %q does not match expected format: %v", ts, err)
	}
}

func TestFileSink_CreatesOutputDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "subdir")
	fs := NewFileSink(dir, 5*time.Minute)

	ctx := context.Background()
	if err := fs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := fs.Write(ctx, testRecords(1)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := fs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("output directory was not created")
	}
}

func TestFileSink_Rotation(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileSink(dir, 5*time.Minute)

	// Use a fake clock to control rotation without sleeping.
	fakeTime := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	fs.now = func() time.Time { return fakeTime }

	ctx := context.Background()
	if err := fs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Write first batch.
	if err := fs.Write(ctx, testRecords(1)); err != nil {
		t.Fatalf("Write 1: %v", err)
	}

	// Advance time past the rotation interval.
	fakeTime = fakeTime.Add(6 * time.Minute)

	// Write second batch — should rotate to a new file.
	if err := fs.Write(ctx, testRecords(1)); err != nil {
		t.Fatalf("Write 2: %v", err)
	}

	if err := fs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 files after rotation, got %d", len(entries))
	}
}

func TestFileSink_CloseFlushesAndClosesFile(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileSink(dir, 5*time.Minute)

	ctx := context.Background()
	if err := fs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := fs.Write(ctx, testRecords(2)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := fs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify file contents are fully written (not truncated).
	entries, _ := os.ReadDir(dir)
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines after Close, got %d", len(lines))
	}
}

func TestFileSink_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileSink(dir, 5*time.Minute)

	ctx := context.Background()
	if err := fs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = fs.Write(ctx, testRecords(1))
		}()
	}
	wg.Wait()

	if err := fs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify all 10 records were written.
	entries, _ := os.ReadDir(dir)
	total := 0
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		total += len(lines)
	}
	if total != 10 {
		t.Fatalf("expected 10 records from concurrent writes, got %d", total)
	}
}

func TestFileSink_Mode(t *testing.T) {
	fs := NewFileSink(t.TempDir(), 5*time.Minute)
	if fs.Mode() != SinkModeBatch {
		t.Fatalf("expected batch mode, got %s", fs.Mode())
	}
}
