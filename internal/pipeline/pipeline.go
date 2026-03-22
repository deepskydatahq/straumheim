// Package pipeline defines the Pipeline interface for event ingestion.
package pipeline

import (
	"context"

	"github.com/deepsky-data/straumheim/internal/record"
)

// Pipeline accepts records from inputs and routes them through the system.
type Pipeline interface {
	// Ingest accepts a batch of records for processing.
	Ingest(ctx context.Context, records []record.Record) error
}
