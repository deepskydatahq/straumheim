package input

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/deepsky-data/straumheim/internal/pipeline"
	"github.com/deepsky-data/straumheim/internal/record"
)

const maxBodySize = 1 << 20 // 1MB

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
	router.Post("/sp/tp2", s.postHandler(p))
	router.Post("/sp/com.snowplowanalytics.snowplow/tp2", s.postHandler(p))
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

// trackerPayload is the Snowplow tracker protocol v2 POST body format.
type trackerPayload struct {
	Schema string           `json:"schema"`
	Data   []map[string]any `json:"data"`
}

func (s *SnowplowInput) postHandler(p pipeline.Pipeline) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(rw, r.Body, maxBodySize)

		var tp trackerPayload
		if err := json.NewDecoder(r.Body).Decode(&tp); err != nil {
			http.Error(rw, "Bad Request", http.StatusBadRequest)
			return
		}

		if len(tp.Data) == 0 {
			rw.WriteHeader(http.StatusOK)
			return
		}

		records := make([]record.Record, 0, len(tp.Data))
		for _, fields := range tp.Data {
			rec := s.buildRecordFromFields(r, fields)
			records = append(records, rec)
		}

		if err := p.Ingest(r.Context(), records); err != nil {
			http.Error(rw, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		rw.WriteHeader(http.StatusOK)
	}
}

// buildRecordFromFields creates a Record from a Snowplow field map (used by POST).
func (s *SnowplowInput) buildRecordFromFields(r *http.Request, fields map[string]any) record.Record {
	rec := record.NewRecord()
	rec.Protocol = s.Protocol()
	rec.IP = extractIP(r)
	rec.UserAgent = r.UserAgent()
	rec.Referer = r.Referer()
	rec.Payload = fields
	rec.Flattened = record.Flatten(fields)

	// Set Source from aid (app_id).
	if aid, ok := fields["aid"]; ok {
		if aidStr, ok := aid.(string); ok {
			rec.Source = aidStr
		}
	}

	// Set DeviceTime from dtm (device timestamp in Unix milliseconds).
	if dtm, ok := fields["dtm"]; ok {
		if dtmStr, ok := dtm.(string); ok {
			if ms, err := strconv.ParseInt(dtmStr, 10, 64); err == nil {
				t := time.UnixMilli(ms).UTC()
				rec.DeviceTime = &t
			}
		}
	}

	return rec
}

// buildRecordFromQuery creates a Record from Snowplow GET query parameters.
func (s *SnowplowInput) buildRecordFromQuery(r *http.Request) record.Record {
	fields := make(map[string]any)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			fields[key] = values[0]
		}
	}
	return s.buildRecordFromFields(r, fields)
}
