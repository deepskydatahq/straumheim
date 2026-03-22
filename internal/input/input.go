// Package input defines the Input interface for event ingestion endpoints.
package input

import (
	"github.com/go-chi/chi/v5"

	"github.com/deepskydatahq/straumheim/internal/pipeline"
)

// Input represents an event ingestion endpoint.
type Input interface {
	// Register attaches the input's HTTP handlers to the router.
	Register(router chi.Router, pipeline pipeline.Pipeline)

	// Protocol returns the protocol identifier for this input (e.g., "webhook", "snowplow").
	Protocol() string
}
