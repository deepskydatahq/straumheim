package pipeline

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/deepsky-data/straumheim/internal/buffer"
	"github.com/deepsky-data/straumheim/internal/record"
	"github.com/deepsky-data/straumheim/internal/sink"
)

// mockSink records all writes for verification.
type mockSink struct {
	mu      sync.Mutex
	records []record.Record
	closed  bool
}

func (m *mockSink) Init(_ context.Context) error { return nil }

func (m *mockSink) Write(_ context.Context, records []record.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, records...)
	return nil
}

func (m *mockSink) Flush(_ context.Context) error { return nil }

func (m *mockSink) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockSink) Mode() sink.SinkMode { return sink.SinkModeBatch }

func (m *mockSink) getRecords() []record.Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.records
}

func (m *mockSink) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func makeRecords(n int) []record.Record {
	recs := make([]record.Record, n)
	for i := range recs {
		recs[i] = record.NewRecord()
	}
	return recs
}

func TestEngine_ImplementsPipeline(t *testing.T) {
	var _ Pipeline = (*Engine)(nil)
}

func TestEngine_IngestDispatchesToSinks(t *testing.T) {
	s1 := &mockSink{}
	s2 := &mockSink{}
	buf := buffer.NewMemoryBuffer(100, 5, 10*time.Second)
	eng := NewEngine(buf, []sink.Sink{s1, s2})

	ctx := context.Background()
	if err := eng.Ingest(ctx, makeRecords(5)); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// Wait for flush-by-count to trigger.
	time.Sleep(50 * time.Millisecond)

	if err := eng.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Both sinks should have received all 5 records (fan-out).
	if got := len(s1.getRecords()); got != 5 {
		t.Errorf("sink1: expected 5 records, got %d", got)
	}
	if got := len(s2.getRecords()); got != 5 {
		t.Errorf("sink2: expected 5 records, got %d", got)
	}
}

func TestEngine_CloseFlushesAndClosesSinks(t *testing.T) {
	s1 := &mockSink{}
	buf := buffer.NewMemoryBuffer(100, 100, 10*time.Second)
	eng := NewEngine(buf, []sink.Sink{s1})

	ctx := context.Background()
	// Push fewer than flushCount — won't trigger count-based flush.
	if err := eng.Ingest(ctx, makeRecords(3)); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	if err := eng.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := len(s1.getRecords()); got != 3 {
		t.Errorf("expected 3 records after close, got %d", got)
	}
	if !s1.isClosed() {
		t.Error("expected sink to be closed")
	}
}

func TestEngine_FanOutNotRoundRobin(t *testing.T) {
	sinks := make([]*mockSink, 3)
	sinkIfaces := make([]sink.Sink, 3)
	for i := range sinks {
		sinks[i] = &mockSink{}
		sinkIfaces[i] = sinks[i]
	}

	buf := buffer.NewMemoryBuffer(100, 2, 10*time.Second)
	eng := NewEngine(buf, sinkIfaces)

	ctx := context.Background()
	if err := eng.Ingest(ctx, makeRecords(4)); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if err := eng.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Every sink should get all 4 records.
	for i, s := range sinks {
		if got := len(s.getRecords()); got != 4 {
			t.Errorf("sink%d: expected 4 records, got %d", i, got)
		}
	}
}
