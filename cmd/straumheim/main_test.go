package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/deepsky-data/straumheim/internal/buffer"
	"github.com/deepsky-data/straumheim/internal/config"
	"github.com/deepsky-data/straumheim/internal/pipeline"
	"github.com/deepsky-data/straumheim/internal/sink"
)

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %s", body["status"])
	}
}

func TestCreateSinks(t *testing.T) {
	configs := []config.SinkConfig{
		{Name: "debug", Type: "stdout"},
	}
	sinks, err := createSinks(configs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sinks) != 1 {
		t.Fatalf("expected 1 sink, got %d", len(sinks))
	}
}

func TestCreateSinksUnknownType(t *testing.T) {
	configs := []config.SinkConfig{
		{Name: "bad", Type: "unknown"},
	}
	_, err := createSinks(configs)
	if err == nil {
		t.Fatal("expected error for unknown sink type")
	}
}

func TestCreateSinksPostgresNoDSN(t *testing.T) {
	configs := []config.SinkConfig{
		{Name: "pg", Type: "postgres"},
	}
	_, err := createSinks(configs)
	if err == nil {
		t.Fatal("expected error for postgres sink without DSN")
	}
}

func TestRegisterInputs(t *testing.T) {
	r := chi.NewRouter()
	buf := buffer.NewMemoryBuffer(100, 10, 1000000000)
	engine := pipeline.NewEngine(buf, []sink.Sink{})

	inputs := map[string]config.InputConfig{
		"webhook": {Enabled: true, Path: "/webhook"},
	}
	registerInputs(r, inputs, engine)

	// Send a webhook request to verify it's registered.
	body := strings.NewReader(`{"event":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Clean up engine.
	engine.Close()
}
