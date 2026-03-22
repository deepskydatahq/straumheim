package input

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/deepsky-data/straumheim/internal/record"
)

// mockPipeline implements pipeline.Pipeline for testing.
type mockPipeline struct {
	mu      sync.Mutex
	records []record.Record
}

func (m *mockPipeline) Ingest(_ context.Context, records []record.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, records...)
	return nil
}

func (m *mockPipeline) getRecords() []record.Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.records
}

func setupRouter() (chi.Router, *mockPipeline) {
	r := chi.NewRouter()
	mp := &mockPipeline{}
	wh := NewWebhook()
	wh.Register(r, mp)
	return r, mp
}

func TestWebhook_Protocol(t *testing.T) {
	wh := NewWebhook()
	if wh.Protocol() != "webhook" {
		t.Fatalf("expected protocol 'webhook', got %q", wh.Protocol())
	}
}

func TestWebhook_ValidJSON(t *testing.T) {
	router, mp := setupRouter()
	body := `{"event":"click","value":42}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TestAgent/1.0")
	req.Header.Set("Referer", "https://example.com")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["id"] == "" {
		t.Fatal("expected non-empty id in response")
	}

	recs := mp.getRecords()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	rec := recs[0]
	if rec.Protocol != "webhook" {
		t.Errorf("expected protocol 'webhook', got %q", rec.Protocol)
	}
	if rec.UserAgent != "TestAgent/1.0" {
		t.Errorf("expected user-agent 'TestAgent/1.0', got %q", rec.UserAgent)
	}
	if rec.Referer != "https://example.com" {
		t.Errorf("expected referer 'https://example.com', got %q", rec.Referer)
	}
	if rec.Payload["event"] != "click" {
		t.Errorf("expected payload event 'click', got %v", rec.Payload["event"])
	}
}

func TestWebhook_NonJSONContentType(t *testing.T) {
	router, _ := setupRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString("plain text"))
	req.Header.Set("Content-Type", "text/plain")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rr.Code)
	}
}

func TestWebhook_NoContentType(t *testing.T) {
	router, _ := setupRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(`{}`))
	// No Content-Type header set.

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rr.Code)
	}
}

func TestWebhook_InvalidJSON(t *testing.T) {
	router, _ := setupRouter()
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestWebhook_SchemaRoute(t *testing.T) {
	router, mp := setupRouter()
	body := `{"data":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/com.acme/page_view/1-0-0", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	recs := mp.getRecords()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	rec := recs[0]
	if rec.Vendor != "com.acme" {
		t.Errorf("expected vendor 'com.acme', got %q", rec.Vendor)
	}
	if rec.Schema != "page_view" {
		t.Errorf("expected schema 'page_view', got %q", rec.Schema)
	}
	if rec.SchemaVersion != "1-0-0" {
		t.Errorf("expected schema_version '1-0-0', got %q", rec.SchemaVersion)
	}
}

func TestWebhook_ExtractsIP(t *testing.T) {
	router, mp := setupRouter()
	body := `{"x":1}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	recs := mp.getRecords()
	if recs[0].IP != "1.2.3.4" {
		t.Errorf("expected IP '1.2.3.4', got %q", recs[0].IP)
	}
}

func TestWebhook_ImplementsInterface(t *testing.T) {
	var _ Input = (*Webhook)(nil)
}
