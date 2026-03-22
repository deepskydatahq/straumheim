// Package buffer defines the Buffer interface for event buffering.
package buffer

import (
	"context"

	"github.com/deepskydatahq/straumheim/internal/record"
)

// HandlerFunc processes a batch of records flushed from the buffer.
type HandlerFunc func(ctx context.Context, records []record.Record)

// Buffer stores records temporarily before flushing to sinks.
type Buffer interface {
	// Push adds records to the buffer. Returns an error if the buffer is full.
	Push(ctx context.Context, records []record.Record) error

	// Consume starts a goroutine that batches records and calls handler
	// when flushCount is reached or flushInterval elapses. Cancelling the
	// context triggers a final drain of remaining records.
	Consume(ctx context.Context, handler HandlerFunc)

	// Close flushes any remaining buffered records then returns.
	Close() error
}
