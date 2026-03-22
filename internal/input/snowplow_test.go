package input

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/deepskydatahq/straumheim/internal/record"
)

// fakePipeline captures ingested records for test assertions.
type fakePipeline struct {
	records []record.Record
}

func (f *fakePipeline) Ingest(_ context.Context, recs []record.Record) error {
	f.records = append(f.records, recs...)
	return nil
}

func newTestRouter(sp *SnowplowInput, fp *fakePipeline) *chi.Mux {
	r := chi.NewRouter()
	sp.Register(r, fp)
	return r
}

func TestSnowplowGET_ReturnsGIF(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	req := httptest.NewRequest(http.MethodGet, "/sp/i?e=pv&url=https://example.com", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/gif" {
		t.Fatalf("expected image/gif, got %s", ct)
	}
	// 1x1 transparent GIF is 43 bytes.
	if w.Body.Len() != 43 {
		t.Fatalf("expected 43-byte GIF body, got %d bytes", w.Body.Len())
	}
}

func TestSnowplowGET_CreatesRecord(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	req := httptest.NewRequest(http.MethodGet, "/sp/i?e=pv&url=https://example.com&page=Home&referer=https://ref.com&aid=my-app&p=web", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	req.Header.Set("Referer", "https://header-referer.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if len(fp.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fp.records))
	}

	rec := fp.records[0]
	if rec.Protocol != "snowplow" {
		t.Fatalf("expected protocol snowplow, got %s", rec.Protocol)
	}

	// Snowplow fields in payload.
	tests := []struct {
		key  string
		want string
	}{
		{"e", "pv"},
		{"url", "https://example.com"},
		{"page", "Home"},
		{"referer", "https://ref.com"},
		{"aid", "my-app"},
		{"p", "web"},
	}
	for _, tt := range tests {
		v, ok := rec.Payload[tt.key]
		if !ok {
			t.Errorf("expected payload key %q", tt.key)
			continue
		}
		if v != tt.want {
			t.Errorf("payload[%q] = %q, want %q", tt.key, v, tt.want)
		}
	}
}

func TestSnowplowGET_IPUserAgentReferer(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	req := httptest.NewRequest(http.MethodGet, "/sp/i?e=pv", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	req.Header.Set("Referer", "https://header-referer.com")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if len(fp.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fp.records))
	}
	rec := fp.records[0]
	if rec.IP != "1.2.3.4" {
		t.Errorf("expected IP 1.2.3.4, got %s", rec.IP)
	}
	if rec.UserAgent != "TestAgent/1.0" {
		t.Errorf("expected UserAgent TestAgent/1.0, got %s", rec.UserAgent)
	}
	if rec.Referer != "https://header-referer.com" {
		t.Errorf("expected Referer https://header-referer.com, got %s", rec.Referer)
	}
}

func TestSnowplowGET_SourceFromAid(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	req := httptest.NewRequest(http.MethodGet, "/sp/i?e=pv&aid=my-app", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if len(fp.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fp.records))
	}
	if fp.records[0].Source != "my-app" {
		t.Errorf("expected Source my-app, got %s", fp.records[0].Source)
	}
}

func TestSnowplowGET_FlattenedPopulated(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	req := httptest.NewRequest(http.MethodGet, "/sp/i?e=pv&url=https://example.com", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if len(fp.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fp.records))
	}
	rec := fp.records[0]
	if rec.Flattened == nil {
		t.Fatal("expected Flattened to be populated")
	}
	if v, ok := rec.Flattened["e"]; !ok || v != "pv" {
		t.Errorf("expected Flattened[e]=pv, got %v", v)
	}
}

func TestSnowplowGET_DeviceTimeFromDtm(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	// 1609459200000 = 2021-01-01T00:00:00Z in Unix millis.
	req := httptest.NewRequest(http.MethodGet, "/sp/i?e=pv&dtm=1609459200000", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if len(fp.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fp.records))
	}
	rec := fp.records[0]
	if rec.DeviceTime == nil {
		t.Fatal("expected DeviceTime to be set")
	}
	expected := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	if !rec.DeviceTime.Equal(expected) {
		t.Errorf("expected DeviceTime %v, got %v", expected, *rec.DeviceTime)
	}
}

// --- Story 2: POST /sp/tp2 tests ---

func TestSnowplowPOST_MultipleEvents(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	body := `{"schema":"iglu:com.snowplowanalytics.snowplow/payload_data/jsonschema/1-0-4","data":[{"e":"pv","url":"https://example.com","aid":"my-app"},{"e":"se","se_ca":"video","aid":"my-app"}]}`
	req := httptest.NewRequest(http.MethodPost, "/sp/tp2", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if len(fp.records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(fp.records))
	}
	for _, rec := range fp.records {
		if rec.Protocol != "snowplow" {
			t.Errorf("expected protocol snowplow, got %s", rec.Protocol)
		}
	}
	if fp.records[0].Payload["e"] != "pv" {
		t.Errorf("expected first event e=pv, got %v", fp.records[0].Payload["e"])
	}
	if fp.records[1].Payload["e"] != "se" {
		t.Errorf("expected second event e=se, got %v", fp.records[1].Payload["e"])
	}
}

func TestSnowplowPOST_FullPathVariant(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	body := `{"schema":"iglu:com.snowplowanalytics.snowplow/payload_data/jsonschema/1-0-4","data":[{"e":"pv"}]}`
	req := httptest.NewRequest(http.MethodPost, "/sp/com.snowplowanalytics.snowplow/tp2", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if len(fp.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fp.records))
	}
	if fp.records[0].Protocol != "snowplow" {
		t.Errorf("expected protocol snowplow, got %s", fp.records[0].Protocol)
	}
}

func TestSnowplowPOST_RecordHasSnowplowFields(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	body := `{"schema":"...","data":[{"e":"pv","url":"https://example.com","aid":"my-app","p":"web"}]}`
	req := httptest.NewRequest(http.MethodPost, "/sp/tp2", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TestAgent/1.0")
	req.Header.Set("X-Forwarded-For", "5.6.7.8")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if len(fp.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fp.records))
	}
	rec := fp.records[0]
	if rec.Source != "my-app" {
		t.Errorf("expected Source my-app, got %s", rec.Source)
	}
	if rec.IP != "5.6.7.8" {
		t.Errorf("expected IP 5.6.7.8, got %s", rec.IP)
	}
	if rec.UserAgent != "TestAgent/1.0" {
		t.Errorf("expected UserAgent TestAgent/1.0, got %s", rec.UserAgent)
	}
	if rec.Flattened == nil {
		t.Error("expected Flattened to be populated")
	}
}

func TestSnowplowPOST_BodySizeLimit(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	// Create a body larger than 1MB.
	bigPayload := `{"schema":"...","data":[{"e":"` + strings.Repeat("x", 1<<20+1) + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/sp/tp2", strings.NewReader(bigPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d", w.Code)
	}
}

func TestSnowplowPOST_InvalidJSON(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	req := httptest.NewRequest(http.MethodPost, "/sp/tp2", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSnowplowPOST_EmptyDataArray(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	body := `{"schema":"...","data":[]}`
	req := httptest.NewRequest(http.MethodPost, "/sp/tp2", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if len(fp.records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(fp.records))
	}
}

// --- Story 3: Cookie handling tests ---

func TestSnowplowCookie_FirstRequestSetsCookie(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{
		Cookie: CookieConfig{
			Enabled: true,
			Name:    "sp",
			TTL:     365 * 24 * time.Hour,
		},
	})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	req := httptest.NewRequest(http.MethodGet, "/sp/i?e=pv", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Check Set-Cookie header.
	cookies := w.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "sp" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected Set-Cookie header with name 'sp'")
	}
	if found.Path != "/" {
		t.Errorf("expected cookie Path=/, got %s", found.Path)
	}
	if found.Value == "" {
		t.Error("expected cookie to have a UUID value")
	}

	// Check that network_userid is in the record payload.
	if len(fp.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fp.records))
	}
	nuid, ok := fp.records[0].Payload["network_userid"]
	if !ok {
		t.Fatal("expected network_userid in payload")
	}
	if nuid != found.Value {
		t.Errorf("expected network_userid=%s, got %v", found.Value, nuid)
	}
}

func TestSnowplowCookie_SubsequentRequestReadsCookie(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{
		Cookie: CookieConfig{
			Enabled: true,
			Name:    "sp",
			TTL:     365 * 24 * time.Hour,
		},
	})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	req := httptest.NewRequest(http.MethodGet, "/sp/i?e=pv", nil)
	req.AddCookie(&http.Cookie{Name: "sp", Value: "existing-uuid-123"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if len(fp.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fp.records))
	}
	nuid, ok := fp.records[0].Payload["network_userid"]
	if !ok {
		t.Fatal("expected network_userid in payload")
	}
	if nuid != "existing-uuid-123" {
		t.Errorf("expected network_userid=existing-uuid-123, got %v", nuid)
	}
}

func TestSnowplowCookie_ConfigurableDomain(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{
		Cookie: CookieConfig{
			Enabled: true,
			Name:    "sp",
			Domain:  "example.com",
			TTL:     365 * 24 * time.Hour,
		},
	})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	req := httptest.NewRequest(http.MethodGet, "/sp/i?e=pv", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	cookies := w.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "sp" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected Set-Cookie header")
	}
	if found.Domain != "example.com" {
		t.Errorf("expected cookie Domain=example.com, got %s", found.Domain)
	}
}

func TestSnowplowCookie_DisabledByDefault(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	req := httptest.NewRequest(http.MethodGet, "/sp/i?e=pv", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "sp" {
			t.Fatal("expected no cookie when cookie is disabled")
		}
	}
}

func TestSnowplowCookie_CustomName(t *testing.T) {
	sp := NewSnowplowInput(SnowplowConfig{
		Cookie: CookieConfig{
			Enabled: true,
			Name:    "my_tracker",
			TTL:     365 * 24 * time.Hour,
		},
	})
	fp := &fakePipeline{}
	r := newTestRouter(sp, fp)

	req := httptest.NewRequest(http.MethodGet, "/sp/i?e=pv", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	cookies := w.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "my_tracker" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected Set-Cookie header with name 'my_tracker'")
	}
}
