// Package buffer defines the Buffer interface for event buffering.
package buffer

import (
	"context"

	"github.com/deepsky-data/straumheim/internal/record"
)

// Buffer stores records temporarily before flushing to sinks.
type Buffer interface {
	// Push adds records to the buffer.
	Push(ctx context.Context, records []record.Record) error

	// Consume retrieves and removes a batch of records from the buffer.
	Consume(ctx context.Context, count int) ([]record.Record, error)

	// Close shuts down the buffer, flushing any remaining records.
	Close() error
}
