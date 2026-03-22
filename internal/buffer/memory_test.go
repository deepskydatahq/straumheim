package buffer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/deepskydatahq/straumheim/internal/record"
)

func makeRecords(n int) []record.Record {
	recs := make([]record.Record, n)
	for i := range recs {
		recs[i] = record.NewRecord()
	}
	return recs
}

func TestMemoryBuffer_FlushByCount(t *testing.T) {
	buf := NewMemoryBuffer(100, 5, 10*time.Second)

	var mu sync.Mutex
	var batches [][]record.Record

	ctx := context.Background()
	buf.Consume(ctx, func(_ context.Context, records []record.Record) {
		mu.Lock()
		defer mu.Unlock()
		cp := make([]record.Record, len(records))
		copy(cp, records)
		batches = append(batches, cp)
	})

	// Push exactly flushCount records — should trigger one flush.
	if err := buf.Push(ctx, makeRecords(5)); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Give the consumer goroutine time to process.
	time.Sleep(50 * time.Millisecond)

	if err := buf.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(batches) == 0 {
		t.Fatal("expected at least one batch, got none")
	}
	if len(batches[0]) != 5 {
		t.Fatalf("expected batch of 5, got %d", len(batches[0]))
	}
}

func TestMemoryBuffer_FlushByInterval(t *testing.T) {
	buf := NewMemoryBuffer(100, 100, 50*time.Millisecond)

	var mu sync.Mutex
	var batches [][]record.Record

	ctx := context.Background()
	buf.Consume(ctx, func(_ context.Context, records []record.Record) {
		mu.Lock()
		defer mu.Unlock()
		cp := make([]record.Record, len(records))
		copy(cp, records)
		batches = append(batches, cp)
	})

	// Push fewer than flushCount — should flush on interval.
	if err := buf.Push(ctx, makeRecords(3)); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Wait longer than flushInterval.
	time.Sleep(150 * time.Millisecond)

	if err := buf.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(batches) == 0 {
		t.Fatal("expected at least one batch from interval flush, got none")
	}
	total := 0
	for _, b := range batches {
		total += len(b)
	}
	if total != 3 {
		t.Fatalf("expected 3 total records, got %d", total)
	}
}

func TestMemoryBuffer_CloseFlushesRemaining(t *testing.T) {
	buf := NewMemoryBuffer(100, 100, 10*time.Second)

	var mu sync.Mutex
	var batches [][]record.Record

	ctx := context.Background()
	buf.Consume(ctx, func(_ context.Context, records []record.Record) {
		mu.Lock()
		defer mu.Unlock()
		cp := make([]record.Record, len(records))
		copy(cp, records)
		batches = append(batches, cp)
	})

	// Push records that won't reach flushCount and won't hit the interval.
	if err := buf.Push(ctx, makeRecords(7)); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Close should drain remaining records.
	if err := buf.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	total := 0
	for _, b := range batches {
		total += len(b)
	}
	if total != 7 {
		t.Fatalf("expected 7 records after Close, got %d", total)
	}
}

func TestMemoryBuffer_PushFullReturnsError(t *testing.T) {
	buf := NewMemoryBuffer(3, 100, 10*time.Second)

	ctx := context.Background()
	buf.Consume(ctx, func(_ context.Context, _ []record.Record) {})

	// Fill the buffer exactly.
	if err := buf.Push(ctx, makeRecords(3)); err != nil {
		t.Fatalf("Push to capacity: %v", err)
	}

	// Next push should fail.
	err := buf.Push(ctx, makeRecords(1))
	if err != ErrBufferFull {
		t.Fatalf("expected ErrBufferFull, got %v", err)
	}

	if err := buf.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestMemoryBuffer_ImplementsInterface(t *testing.T) {
	var _ Buffer = (*MemoryBuffer)(nil)
}
