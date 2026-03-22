package input

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/deepskydatahq/straumheim/internal/pipeline"
	"github.com/deepskydatahq/straumheim/internal/record"
)

// transparentGIF is defined in snowplow.go and shared across inputs.

// Pixel is an HTTP input that serves a 1x1 transparent GIF and captures
// query parameters as event payload fields.
type Pixel struct{}

// NewPixel creates a new Pixel input.
func NewPixel() *Pixel {
	return &Pixel{}
}

// Protocol returns the protocol identifier for the pixel input.
func (p *Pixel) Protocol() string {
	return "pixel"
}

// Register attaches the pixel HTTP handlers to the router.
func (p *Pixel) Register(router chi.Router, pl pipeline.Pipeline) {
	router.Get("/px", p.handler(pl))
	router.Get("/px/{vendor}/{name}/{version}", p.handler(pl))
}

func (p *Pixel) handler(pl pipeline.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Build payload from query parameters.
		payload := make(map[string]any)
		for key, values := range r.URL.Query() {
			if len(values) > 0 {
				payload[key] = values[0]
			}
		}

		rec := record.NewRecord()
		rec.Protocol = p.Protocol()
		rec.Payload = payload
		rec.Flattened = record.Flatten(payload)
		rec.IP = extractIP(r)
		rec.UserAgent = r.UserAgent()
		rec.Referer = r.Referer()

		// Extract schema params from URL if present.
		if vendor := chi.URLParam(r, "vendor"); vendor != "" {
			rec.Vendor = vendor
		}
		if name := chi.URLParam(r, "name"); name != "" {
			rec.Schema = name
		}
		if version := chi.URLParam(r, "version"); version != "" {
			rec.SchemaVersion = version
		}

		if err := pl.Ingest(r.Context(), []record.Record{rec}); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "image/gif")
		w.Header().Set("Cache-Control", "no-store, no-cache")
		w.Header().Set("Pragma", "no-cache")
		w.WriteHeader(http.StatusOK)
		w.Write(transparentGIF)
	}
}
