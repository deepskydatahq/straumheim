package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/deepskydatahq/straumheim/internal/buffer"
	"github.com/deepskydatahq/straumheim/internal/metrics"
	"github.com/deepskydatahq/straumheim/internal/record"
	"github.com/deepskydatahq/straumheim/internal/sink"
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
	eng := NewEngine(buf, []sink.Sink{s1, s2}, nil, nil)

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
	eng := NewEngine(buf, []sink.Sink{s1}, nil, nil)

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
	eng := NewEngine(buf, sinkIfaces, nil, nil)

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

// failingSink always returns an error from Write.
type failingSink struct {
	mockSink
}

func (f *failingSink) Write(_ context.Context, _ []record.Record) error {
	return errors.New("sink error")
}

func gatherCounter(reg *prometheus.Registry, name string, labels map[string]string) float64 {
	families, _ := reg.Gather()
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			match := true
			for _, lp := range m.GetLabel() {
				if v, ok := labels[lp.GetName()]; ok && v != lp.GetValue() {
					match = false
				}
			}
			if match {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func gatherHistogramCount(reg *prometheus.Registry, name string, labels map[string]string) uint64 {
	families, _ := reg.Gather()
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			match := true
			for _, lp := range m.GetLabel() {
				if v, ok := labels[lp.GetName()]; ok && v != lp.GetValue() {
					match = false
				}
			}
			if match {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func makeRecordsWithProtocol(n int, protocol string) []record.Record {
	recs := make([]record.Record, n)
	for i := range recs {
		r := record.NewRecord()
		r.Protocol = protocol
		recs[i] = r
	}
	return recs
}

func TestEngine_IngestIncrementsRecordsReceived(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	s := &mockSink{}
	buf := buffer.NewMemoryBuffer(100, 100, 10*time.Second)
	eng := NewEngine(buf, []sink.Sink{s}, []string{"test"}, m)

	ctx := context.Background()
	if err := eng.Ingest(ctx, makeRecordsWithProtocol(3, "webhook")); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	val := gatherCounter(reg, "straumheim_records_received_total", map[string]string{"protocol": "webhook"})
	if val != 3 {
		t.Errorf("expected 3 records_received, got %f", val)
	}

	eng.Close()
}

func TestEngine_FlushIncrementsRecordsDelivered(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	s := &mockSink{}
	buf := buffer.NewMemoryBuffer(100, 3, 10*time.Second)
	eng := NewEngine(buf, []sink.Sink{s}, []string{"warehouse"}, m)

	ctx := context.Background()
	if err := eng.Ingest(ctx, makeRecordsWithProtocol(3, "webhook")); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// Wait for flush-by-count.
	time.Sleep(50 * time.Millisecond)

	val := gatherCounter(reg, "straumheim_records_delivered_total", map[string]string{"sink": "warehouse"})
	if val != 3 {
		t.Errorf("expected 3 records_delivered, got %f", val)
	}

	eng.Close()
}

func TestEngine_FlushIncrementsRecordsFailed(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	s := &failingSink{}
	buf := buffer.NewMemoryBuffer(100, 3, 10*time.Second)
	eng := NewEngine(buf, []sink.Sink{s}, []string{"broken"}, m)

	ctx := context.Background()
	if err := eng.Ingest(ctx, makeRecordsWithProtocol(3, "webhook")); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// Wait for flush-by-count.
	time.Sleep(50 * time.Millisecond)

	val := gatherCounter(reg, "straumheim_records_failed_total", map[string]string{"sink": "broken"})
	if val != 3 {
		t.Errorf("expected 3 records_failed, got %f", val)
	}

	eng.Close()
}

func TestEngine_FlushObservesFlushDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	s := &mockSink{}
	buf := buffer.NewMemoryBuffer(100, 3, 10*time.Second)
	eng := NewEngine(buf, []sink.Sink{s}, []string{"warehouse"}, m)

	ctx := context.Background()
	if err := eng.Ingest(ctx, makeRecordsWithProtocol(3, "webhook")); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// Wait for flush-by-count.
	time.Sleep(50 * time.Millisecond)

	count := gatherHistogramCount(reg, "straumheim_flush_duration_seconds", map[string]string{"sink": "warehouse"})
	if count < 1 {
		t.Errorf("expected at least 1 flush duration observation, got %d", count)
	}

	eng.Close()
}

func TestEngine_NilMetricsSafe(t *testing.T) {
	// Ensure engine works fine without metrics (nil-safe).
	s := &mockSink{}
	buf := buffer.NewMemoryBuffer(100, 3, 10*time.Second)
	eng := NewEngine(buf, []sink.Sink{s}, nil, nil)

	ctx := context.Background()
	if err := eng.Ingest(ctx, makeRecordsWithProtocol(3, "webhook")); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if err := eng.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := len(s.getRecords()); got != 3 {
		t.Errorf("expected 3 records, got %d", got)
	}
}

// Suppress unused import warnings.
var _ = dto.Metric{}
