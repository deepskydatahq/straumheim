package input

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/deepsky-data/straumheim/internal/pipeline"
	"github.com/deepsky-data/straumheim/internal/record"
)

// transparentGIF is a 1x1 transparent GIF image (43 bytes).
var transparentGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00,
	0x80, 0x00, 0x00, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21,
	0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00,
	0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44,
	0x01, 0x00, 0x3b,
}

// SnowplowConfig holds configuration for the Snowplow input.
type SnowplowConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
	Cookie  CookieConfig `yaml:"cookie"`
}

// CookieConfig holds cookie settings for network_userid tracking.
type CookieConfig struct {
	Enabled bool          `yaml:"enabled"`
	Name    string        `yaml:"name"`
	Domain  string        `yaml:"domain"`
	TTL     time.Duration `yaml:"ttl"`
}

// SnowplowInput is an HTTP input that implements Snowplow tracker protocol.
type SnowplowInput struct {
	cfg SnowplowConfig
}

// NewSnowplowInput creates a new SnowplowInput.
func NewSnowplowInput(cfg SnowplowConfig) *SnowplowInput {
	return &SnowplowInput{cfg: cfg}
}

// Protocol returns the protocol identifier.
func (s *SnowplowInput) Protocol() string {
	return "snowplow"
}

// Register attaches the Snowplow HTTP handlers to the router.
func (s *SnowplowInput) Register(router chi.Router, p pipeline.Pipeline) {
	router.Get("/sp/i", s.getHandler(p))
}

func (s *SnowplowInput) getHandler(p pipeline.Pipeline) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		rec := s.buildRecordFromQuery(r)

		if err := p.Ingest(r.Context(), []record.Record{rec}); err != nil {
			http.Error(rw, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		rw.Header().Set("Content-Type", "image/gif")
		rw.Header().Set("Cache-Control", "no-store")
		rw.WriteHeader(http.StatusOK)
		rw.Write(transparentGIF)
	}
}

// buildRecordFromQuery creates a Record from Snowplow GET query parameters.
func (s *SnowplowInput) buildRecordFromQuery(r *http.Request) record.Record {
	rec := record.NewRecord()
	rec.Protocol = s.Protocol()
	rec.IP = extractIP(r)
	rec.UserAgent = r.UserAgent()
	rec.Referer = r.Referer()

	payload := make(map[string]any)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			payload[key] = values[0]
		}
	}
	rec.Payload = payload
	rec.Flattened = record.Flatten(payload)

	// Set Source from aid (app_id).
	if aid, ok := payload["aid"]; ok {
		if s, ok := aid.(string); ok {
			rec.Source = s
		}
	}

	// Set DeviceTime from dtm (device timestamp in Unix milliseconds).
	if dtm, ok := payload["dtm"]; ok {
		if dtmStr, ok := dtm.(string); ok {
			if ms, err := strconv.ParseInt(dtmStr, 10, 64); err == nil {
				t := time.UnixMilli(ms).UTC()
				rec.DeviceTime = &t
			}
		}
	}

	return rec
}
