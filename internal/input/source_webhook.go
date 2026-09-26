package input

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/deepskydatahq/straumheim/internal/pipeline"
	"github.com/deepskydatahq/straumheim/internal/record"
)

const sourceWebhookBodyLimit int64 = 64 * 1024

// SourceWebhookConfig binds one authenticated route to one server-owned source
// and schema. Payload fields cannot change this identity.
type SourceWebhookConfig struct {
	Path                 string
	Source               string
	KeyID                string
	Secret               string
	Vendor               string
	Schema               string
	SchemaVersion        string
	PayloadSchemaVersion string
	MaxClockSkew         time.Duration
}

// SourceWebhook accepts privacy-minimized, HMAC-authenticated JSON events.
type SourceWebhook struct {
	config SourceWebhookConfig
	now    func() time.Time
}

func NewSourceWebhook(config SourceWebhookConfig) (*SourceWebhook, error) {
	if !strings.HasPrefix(config.Path, "/source/") || strings.Contains(config.Path[len("/source/"):], "/") {
		return nil, fmt.Errorf("source webhook path must be one fixed /source/<name> route")
	}
	for name, value := range map[string]string{"source": config.Source, "key id": config.KeyID, "secret": config.Secret, "vendor": config.Vendor, "schema": config.Schema, "schema version": config.SchemaVersion, "payload schema version": config.PayloadSchemaVersion} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("source webhook %s is required", name)
		}
	}
	if config.MaxClockSkew <= 0 || config.MaxClockSkew > 15*time.Minute {
		return nil, fmt.Errorf("source webhook max clock skew must be between zero and 15 minutes")
	}
	return &SourceWebhook{config: config, now: time.Now}, nil
}

func (w *SourceWebhook) Protocol() string { return "webhook" }

func (w *SourceWebhook) Register(router chi.Router, p pipeline.Pipeline) {
	router.Post(w.config.Path, w.handler(p))
}

func (w *SourceWebhook) signature(timestamp, eventID string, body []byte) string {
	bodyDigest := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(w.config.Secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(eventID))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(hex.EncodeToString(bodyDigest[:])))
	return hex.EncodeToString(mac.Sum(nil))
}

func (w *SourceWebhook) authorized(r *http.Request, body []byte) (string, bool) {
	if r.Header.Get("X-Straumheim-Key-Id") != w.config.KeyID {
		return "", false
	}
	timestamp := r.Header.Get("X-Straumheim-Timestamp")
	observedAt, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil || w.now().Sub(observedAt) > w.config.MaxClockSkew || observedAt.Sub(w.now()) > w.config.MaxClockSkew {
		return "", false
	}
	eventID := r.Header.Get("X-Straumheim-Event-Id")
	if _, err := uuid.Parse(eventID); err != nil {
		return "", false
	}
	provided := strings.TrimPrefix(r.Header.Get("X-Straumheim-Signature"), "sha256=")
	expected := w.signature(timestamp, eventID, body)
	return eventID, hmac.Equal([]byte(provided), []byte(expected))
}

var applicationVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$`)

func stringIn(value any, accepted ...string) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	for _, candidate := range accepted {
		if text == candidate {
			return true
		}
	}
	return false
}

func nullOrStringIn(value any, accepted ...string) bool {
	return value == nil || stringIn(value, accepted...)
}

func boundedInteger(value any, maximum float64) bool {
	if value == nil {
		return true
	}
	number, ok := value.(float64)
	return ok && number >= 0 && number <= maximum && math.Trunc(number) == number
}

func validateSourceEventPayload(payload map[string]any, eventID, expectedSchemaVersion string) error {
	required := map[string]bool{
		"schema_version": true, "event_id": true, "run_id": true, "occurred_at": true, "emitted_at": true,
		"environment": true, "application_version": true, "event_name": true, "workflow_family": true,
		"outcome": true, "reason_code": true, "duration_ms": true, "counts": true, "availability_reason": true,
	}
	if len(payload) != len(required) {
		return fmt.Errorf("source event has unknown or missing fields")
	}
	for key := range payload {
		if !required[key] {
			return fmt.Errorf("source event has unknown field")
		}
	}
	if payload["schema_version"] != expectedSchemaVersion || payload["event_id"] != eventID {
		return fmt.Errorf("source event identity mismatch")
	}
	runID, ok := payload["run_id"].(string)
	if !ok {
		return fmt.Errorf("source event run id is invalid")
	}
	if _, err := uuid.Parse(runID); err != nil {
		return fmt.Errorf("source event run id is invalid")
	}
	occurredText, occurredOK := payload["occurred_at"].(string)
	emittedText, emittedOK := payload["emitted_at"].(string)
	occurredAt, occurredErr := time.Parse(time.RFC3339Nano, occurredText)
	emittedAt, emittedErr := time.Parse(time.RFC3339Nano, emittedText)
	if !occurredOK || !emittedOK || occurredErr != nil || emittedErr != nil || emittedAt.Before(occurredAt) {
		return fmt.Errorf("source event timestamps are invalid")
	}
	version, versionOK := payload["application_version"].(string)
	if !versionOK || len(version) > 64 || !applicationVersionPattern.MatchString(version) {
		return fmt.Errorf("source event application version is invalid")
	}
	if !stringIn(payload["environment"], "development", "staging", "production") ||
		!stringIn(payload["event_name"], "application_started", "request_routed", "plan_proposed", "approval_recorded", "execution_started", "validation_completed", "review_commit_created", "run_blocked", "run_completed") ||
		!stringIn(payload["workflow_family"], "terminal_review", "semantic_mapping", "small_change", "touchpoint_validation", "pilot", "blueprint", "generation", "unknown") ||
		!nullOrStringIn(payload["outcome"], "started", "routed", "proposed", "approved", "denied", "passed", "failed", "blocked", "cancelled", "completed") ||
		!nullOrStringIn(payload["reason_code"], "policy_denied", "approval_withheld", "invalid_input", "stale_state", "validation_failed", "timeout", "cancelled", "unsupported", "internal_error", "unavailable") ||
		!nullOrStringIn(payload["availability_reason"], "not_applicable", "not_recorded", "provider_unavailable", "validation_unavailable") ||
		!boundedInteger(payload["duration_ms"], 86400000) {
		return fmt.Errorf("source event contains an invalid bounded value")
	}
	counts, ok := payload["counts"].(map[string]any)
	if !ok || len(counts) > 4 {
		return fmt.Errorf("source event counts are invalid")
	}
	limits := map[string]float64{"files": 10000, "tests_passed": 1000000, "tests_failed": 1000000, "tool_calls": 10000}
	for key, value := range counts {
		maximum, exists := limits[key]
		if !exists || !boundedInteger(value, maximum) {
			return fmt.Errorf("source event counts are invalid")
		}
	}
	return nil
}

func (w *SourceWebhook) handler(p pipeline.Pipeline) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(rw, "Unsupported Media Type", http.StatusUnsupportedMediaType)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(rw, r.Body, sourceWebhookBodyLimit))
		if err != nil {
			http.Error(rw, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		eventID, ok := w.authorized(r, body)
		if !ok {
			http.Error(rw, "Unauthorized", http.StatusUnauthorized)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(rw, "Bad Request", http.StatusBadRequest)
			return
		}
		if err := validateSourceEventPayload(payload, eventID, w.config.PayloadSchemaVersion); err != nil {
			http.Error(rw, "Bad Request", http.StatusBadRequest)
			return
		}
		rec := record.NewRecord()
		rec.ID = eventID
		rec.Protocol = w.Protocol()
		rec.Source = w.config.Source
		rec.Vendor = w.config.Vendor
		rec.Schema = w.config.Schema
		rec.SchemaVersion = w.config.SchemaVersion
		rec.Payload = payload
		rec.Flattened = record.Flatten(payload)
		// Intentionally do not persist request IP, user-agent, or referrer.
		if err := p.Ingest(r.Context(), []record.Record{rec}); err != nil {
			http.Error(rw, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(rw).Encode(map[string]string{"id": rec.ID})
	}
}
