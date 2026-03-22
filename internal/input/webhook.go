package input

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/deepskydatahq/straumheim/internal/pipeline"
	"github.com/deepskydatahq/straumheim/internal/record"
)

// Webhook is an HTTP input that accepts JSON POST requests.
type Webhook struct{}

// NewWebhook creates a new Webhook input.
func NewWebhook() *Webhook {
	return &Webhook{}
}

// Protocol returns the protocol identifier for the webhook input.
func (w *Webhook) Protocol() string {
	return "webhook"
}

// Register attaches the webhook HTTP handlers to the router.
func (w *Webhook) Register(router chi.Router, p pipeline.Pipeline) {
	router.Post("/webhook", w.handler(p))
	router.Post("/webhook/{vendor}/{name}/{version}", w.handler(p))
}

func (w *Webhook) handler(p pipeline.Pipeline) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct == "" || !strings.HasPrefix(ct, "application/json") {
			http.Error(rw, "Unsupported Media Type", http.StatusUnsupportedMediaType)
			return
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(rw, "Bad Request", http.StatusBadRequest)
			return
		}

		rec := record.NewRecord()
		rec.Protocol = w.Protocol()
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

		if err := p.Ingest(r.Context(), []record.Record{rec}); err != nil {
			http.Error(rw, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(map[string]string{"id": rec.ID})
	}
}

// extractIP returns the client IP from X-Forwarded-For, X-Real-Ip, or RemoteAddr.
func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain.
		if i := strings.IndexByte(xff, ','); i != -1 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-Ip"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// Strip port from RemoteAddr.
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}
