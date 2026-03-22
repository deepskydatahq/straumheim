// Package pipeline defines the Pipeline interface and its Engine implementation.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/deepsky-data/straumheim/internal/buffer"
	"github.com/deepsky-data/straumheim/internal/metrics"
	"github.com/deepsky-data/straumheim/internal/record"
	"github.com/deepsky-data/straumheim/internal/sink"
)

// Engine is the concrete Pipeline implementation that wires a buffer to sinks.
type Engine struct {
	buf       buffer.Buffer
	sinks     []sink.Sink
	sinkNames []string
	metrics   *metrics.Metrics
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewEngine creates a new pipeline engine and starts the buffer consumer.
// The consumer dispatches batches to all configured sinks (fan-out).
// sinkNames provides labels for metrics; pass nil to skip metrics.
// m is optional — if nil, no instrumentation is performed.
func NewEngine(buf buffer.Buffer, sinks []sink.Sink, sinkNames []string, m *metrics.Metrics) *Engine {
	ctx, cancel := context.WithCancel(context.Background())

	// Default sink names to index-based names if not provided.
	if sinkNames == nil {
		sinkNames = make([]string, len(sinks))
		for i := range sinks {
			sinkNames[i] = fmt.Sprintf("sink_%d", i)
		}
	}

	e := &Engine{
		buf:       buf,
		sinks:     sinks,
		sinkNames: sinkNames,
		metrics:   m,
		ctx:       ctx,
		cancel:    cancel,
	}

	buf.Consume(ctx, func(ctx context.Context, records []record.Record) {
		for i, s := range e.sinks {
			name := e.sinkNames[i]
			start := time.Now()

			err := s.Write(ctx, records)

			if e.metrics != nil {
				e.metrics.ObserveFlushDuration(name, time.Since(start))
				if err != nil {
					for range records {
						e.metrics.RecordFailed(name)
					}
				} else {
					for range records {
						e.metrics.RecordDelivered(name)
					}
				}
			}

			if err != nil {
				slog.Error("sink write failed", "sink", name, "error", err)
			}
		}
	})

	return e
}

// Ingest pushes records into the buffer for processing.
func (e *Engine) Ingest(ctx context.Context, records []record.Record) error {
	if e.metrics != nil {
		for _, r := range records {
			protocol := r.Protocol
			if protocol == "" {
				protocol = "unknown"
			}
			e.metrics.RecordReceived(protocol)
		}
	}

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
