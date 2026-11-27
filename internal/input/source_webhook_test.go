package input

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func sourceWebhookForTest(t *testing.T) (*SourceWebhook, chi.Router, *mockPipeline) {
	t.Helper()
	webhook, err := NewSourceWebhook(SourceWebhookConfig{
		Path: "/source/propel-harness", Source: "propel-harness", KeyID: "test-key", Secret: "test-secret",
		Vendor: "propel", Schema: "harness-run", SchemaVersion: "1-0-0", PayloadSchemaVersion: "propel-harness-run-event/1.0", MaxClockSkew: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("create source webhook: %v", err)
	}
	webhook.now = func() time.Time { return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) }
	router := chi.NewRouter()
	pipeline := &mockPipeline{}
	webhook.Register(router, pipeline)
	return webhook, router, pipeline
}

func signedSourceRequest(t *testing.T, webhook *SourceWebhook, payload map[string]any) *http.Request {
	t.Helper()
	timestamp := "2026-09-26T12:00:00Z"
	eventID := uuid.NewString()
	fullPayload := map[string]any{
		"schema_version": "propel-harness-run-event/1.0", "event_id": eventID, "run_id": uuid.NewString(),
		"occurred_at": "2026-09-26T11:59:59Z", "emitted_at": "2026-09-26T12:00:00Z",
		"environment": "development", "application_version": "0.7.0", "event_name": "request_routed", "workflow_family": "small_change",
		"outcome": "routed", "reason_code": nil, "duration_ms": nil, "counts": map[string]any{}, "availability_reason": nil,
	}
	for key, value := range payload {
		fullPayload[key] = value
	}
	body, err := json.Marshal(fullPayload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/source/propel-harness", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Straumheim-Key-Id", "test-key")
	req.Header.Set("X-Straumheim-Timestamp", timestamp)
	req.Header.Set("X-Straumheim-Event-Id", eventID)
	req.Header.Set("X-Straumheim-Signature", "sha256="+webhook.signature(timestamp, eventID, body))
	return req
}

func TestSourceWebhookSignatureFixture(t *testing.T) {
	webhook, _, _ := sourceWebhookForTest(t)
	got := webhook.signature("2026-09-26T12:00:00Z", "00000000-0000-4000-8000-000000000017", []byte(`{"event":"test"}`))
	want := "dff1bdf171d5140ac23b418d992735e333c4aaeb7f3bf36efb5c2218af3d0a3f"
	if got != want {
		t.Fatalf("signature = %s, want %s", got, want)
	}
}

func TestSourceWebhookBindsSourceAndMinimizesRequestMetadata(t *testing.T) {
	webhook, router, pipeline := sourceWebhookForTest(t)
	req := signedSourceRequest(t, webhook, map[string]any{"event_name": "run_completed", "outcome": "completed"})
	req.Header.Set("User-Agent", "private-agent")
	req.Header.Set("Referer", "https://private.invalid/path")
	req.Header.Set("X-Forwarded-For", "192.0.2.20")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	records := pipeline.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected one record, got %d", len(records))
	}
	rec := records[0]
	if rec.Source != "propel-harness" || rec.Vendor != "propel" || rec.Schema != "harness-run" || rec.SchemaVersion != "1-0-0" {
		t.Fatalf("unexpected server-bound identity: %#v", rec)
	}
	if rec.IP != "" || rec.UserAgent != "" || rec.Referer != "" {
		t.Fatalf("source webhook retained request metadata: ip=%q user_agent=%q referer=%q", rec.IP, rec.UserAgent, rec.Referer)
	}
	if rec.ID != req.Header.Get("X-Straumheim-Event-Id") {
		t.Fatalf("record id %q does not preserve producer event id", rec.ID)
	}
}

func TestSourceWebhookRejectsWrongSignatureAndStaleTimestamp(t *testing.T) {
	webhook, router, pipeline := sourceWebhookForTest(t)
	wrong := signedSourceRequest(t, webhook, map[string]any{})
	wrong.Header.Set("X-Straumheim-Signature", "sha256=wrong")
	wrongRecorder := httptest.NewRecorder()
	router.ServeHTTP(wrongRecorder, wrong)
	if wrongRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("wrong signature status = %d", wrongRecorder.Code)
	}

	stale := signedSourceRequest(t, webhook, map[string]any{})
	stale.Header.Set("X-Straumheim-Timestamp", "2026-09-26T11:00:00Z")
	staleRecorder := httptest.NewRecorder()
	router.ServeHTTP(staleRecorder, stale)
	if staleRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("stale timestamp status = %d", staleRecorder.Code)
	}
	if len(pipeline.getRecords()) != 0 {
		t.Fatal("unauthorized request reached pipeline")
	}
}

func TestSourceWebhookRejectsUnknownOrSensitivePayloadFields(t *testing.T) {
	webhook, router, pipeline := sourceWebhookForTest(t)
	req := signedSourceRequest(t, webhook, map[string]any{"prompt": "private", "client": "customer", "source": "forged"})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown-field status = %d", recorder.Code)
	}
	if len(pipeline.getRecords()) != 0 {
		t.Fatal("unknown-field payload reached pipeline")
	}
}

func TestSourceWebhookRejectsOversizedBody(t *testing.T) {
	webhook, router, _ := sourceWebhookForTest(t)
	req := signedSourceRequest(t, webhook, map[string]any{"value": strings.Repeat("x", int(sourceWebhookBodyLimit))})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d", recorder.Code)
	}
}

func TestSourceWebhookConfigurationFailsClosed(t *testing.T) {
	cases := []SourceWebhookConfig{
		{},
		{Path: "/webhook", Source: "propel-harness", KeyID: "key", Secret: "secret", Vendor: "propel", Schema: "harness-run", SchemaVersion: "1-0-0", PayloadSchemaVersion: "propel-harness-run-event/1.0", MaxClockSkew: time.Minute},
		{Path: "/source/propel-harness", Source: "propel-harness", KeyID: "key", Secret: "", Vendor: "propel", Schema: "harness-run", SchemaVersion: "1-0-0", PayloadSchemaVersion: "propel-harness-run-event/1.0", MaxClockSkew: time.Minute},
	}
	for index, config := range cases {
		if _, err := NewSourceWebhook(config); err == nil {
			t.Fatalf("case %d unexpectedly accepted", index)
		}
	}
}

func TestSourceWebhookImplementsInput(t *testing.T) {
	webhook, _, _ := sourceWebhookForTest(t)
	var _ Input = webhook
}
