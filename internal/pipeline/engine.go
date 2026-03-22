package pipeline

import (
	"context"

	"github.com/deepsky-data/straumheim/internal/buffer"
	"github.com/deepsky-data/straumheim/internal/record"
	"github.com/deepsky-data/straumheim/internal/sink"
)

// Engine is the concrete Pipeline implementation that wires a buffer to sinks.
type Engine struct {
	buf    buffer.Buffer
	sinks  []sink.Sink
	ctx    context.Context
	cancel context.CancelFunc
}

// NewEngine creates a new pipeline engine and starts the buffer consumer.
// The consumer dispatches batches to all configured sinks (fan-out).
func NewEngine(buf buffer.Buffer, sinks []sink.Sink) *Engine {
	ctx, cancel := context.WithCancel(context.Background())

	e := &Engine{
		buf:    buf,
		sinks:  sinks,
		ctx:    ctx,
		cancel: cancel,
	}

	buf.Consume(ctx, func(ctx context.Context, records []record.Record) {
		for _, s := range e.sinks {
			// Fan-out: every sink receives every record.
			_ = s.Write(ctx, records)
		}
	})

	return e
}

// Ingest pushes records into the buffer for processing.
func (e *Engine) Ingest(ctx context.Context, records []record.Record) error {
	return e.buf.Push(ctx, records)
}

// Close stops the consumer, flushes the buffer, and closes all sinks.
func (e *Engine) Close() error {
	// Cancel context triggers buffer drain.
	e.cancel()

	// Wait for buffer to finish flushing.
	if err := e.buf.Close(); err != nil {
		return err
	}

	// Close all sinks.
	for _, s := range e.sinks {
		if err := s.Close(); err != nil {
			return err
		}
	}

	return nil
}
