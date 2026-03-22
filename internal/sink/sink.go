// Package sink defines the Sink interface for event output destinations.
package sink

import (
	"context"

	"github.com/deepsky-data/straumheim/internal/record"
)

// SinkMode indicates whether a sink processes records as a stream or in batches.
type SinkMode string

const (
	// SinkModeStream processes records one at a time as they arrive.
	SinkModeStream SinkMode = "stream"

	// SinkModeBatch processes records in batches.
	SinkModeBatch SinkMode = "batch"
)

// Sink is an output destination for processed records.
type Sink interface {
	// Init initializes the sink with its configuration.
	Init(ctx context.Context) error

	// Write sends records to the sink.
	Write(ctx context.Context, records []record.Record) error

	// Flush forces any buffered records to be written.
	Flush(ctx context.Context) error

	// Close shuts down the sink gracefully.
	Close() error

	// Mode returns whether this sink operates in stream or batch mode.
	Mode() SinkMode
}
