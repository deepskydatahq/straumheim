package input

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupPixelRouter() (chi.Router, *mockPipeline) {
	r := chi.NewRouter()
	mp := &mockPipeline{}
	px := NewPixel()
	px.Register(r, mp)
	return r, mp
}

func TestPixel_Protocol(t *testing.T) {
	px := NewPixel()
	if px.Protocol() != "pixel" {
		t.Fatalf("expected protocol 'pixel', got %q", px.Protocol())
	}
}

func TestPixel_ReturnsGIF(t *testing.T) {
	router, _ := setupPixelRouter()
	req := httptest.NewRequest(http.MethodGet, "/px?event=signup&user_id=abc", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/gif" {
		t.Errorf("expected Content-Type 'image/gif', got %q", ct)
	}
	// 1x1 transparent GIF is 43 bytes.
	if rr.Body.Len() != 43 {
		t.Errorf("expected 43-byte GIF body, got %d bytes", rr.Body.Len())
	}
}

func TestPixel_QueryParamsBecomePayload(t *testing.T) {
	router, mp := setupPixelRouter()
	req := httptest.NewRequest(http.MethodGet, "/px?event=signup&user_id=abc", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	recs := mp.getRecords()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	rec := recs[0]
	if rec.Payload["event"] != "signup" {
		t.Errorf("expected payload event 'signup', got %v", rec.Payload["event"])
	}
	if rec.Payload["user_id"] != "abc" {
		t.Errorf("expected payload user_id 'abc', got %v", rec.Payload["user_id"])
	}
}

func TestPixel_NoCacheHeaders(t *testing.T) {
	router, _ := setupPixelRouter()
	req := httptest.NewRequest(http.MethodGet, "/px?event=test", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store, no-cache" {
		t.Errorf("expected Cache-Control 'no-store, no-cache', got %q", cc)
	}
	if p := rr.Header().Get("Pragma"); p != "no-cache" {
		t.Errorf("expected Pragma 'no-cache', got %q", p)
	}
}

func TestPixel_SchemaRoute(t *testing.T) {
	router, mp := setupPixelRouter()
	req := httptest.NewRequest(http.MethodGet, "/px/com.acme/page_view/1-0-0?event=test", nil)
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

func TestPixel_RecordMetadata(t *testing.T) {
	router, mp := setupPixelRouter()
	req := httptest.NewRequest(http.MethodGet, "/px?event=test", nil)
	req.Header.Set("User-Agent", "PixelBot/1.0")
	req.Header.Set("Referer", "https://email.example.com")
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	recs := mp.getRecords()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	rec := recs[0]
	if rec.Protocol != "pixel" {
		t.Errorf("expected protocol 'pixel', got %q", rec.Protocol)
	}
	if rec.IP != "10.0.0.1" {
		t.Errorf("expected IP '10.0.0.1', got %q", rec.IP)
	}
	if rec.UserAgent != "PixelBot/1.0" {
		t.Errorf("expected user-agent 'PixelBot/1.0', got %q", rec.UserAgent)
	}
	if rec.Referer != "https://email.example.com" {
		t.Errorf("expected referer 'https://email.example.com', got %q", rec.Referer)
	}
}

func TestPixel_FlattenedPopulated(t *testing.T) {
	router, mp := setupPixelRouter()
	req := httptest.NewRequest(http.MethodGet, "/px?event=signup&user_id=abc", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	recs := mp.getRecords()
	rec := recs[0]
	if rec.Flattened == nil {
		t.Fatal("expected flattened to be populated")
	}
	if rec.Flattened["event"] != "signup" {
		t.Errorf("expected flattened event 'signup', got %v", rec.Flattened["event"])
	}
	if rec.Flattened["user_id"] != "abc" {
		t.Errorf("expected flattened user_id 'abc', got %v", rec.Flattened["user_id"])
	}
}

func TestPixel_EmptyQueryParams(t *testing.T) {
	router, mp := setupPixelRouter()
	req := httptest.NewRequest(http.MethodGet, "/px", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	recs := mp.getRecords()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	rec := recs[0]
	if len(rec.Payload) != 0 {
		t.Errorf("expected empty payload, got %v", rec.Payload)
	}
}

func TestPixel_ImplementsInterface(t *testing.T) {
	var _ Input = (*Pixel)(nil)
}
