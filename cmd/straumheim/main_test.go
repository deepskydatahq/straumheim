package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/deepskydatahq/straumheim/internal/buffer"
	"github.com/deepskydatahq/straumheim/internal/config"
	"github.com/deepskydatahq/straumheim/internal/metrics"
	"github.com/deepskydatahq/straumheim/internal/pipeline"
	"github.com/deepskydatahq/straumheim/internal/sink"
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

func TestCreateSinksFile(t *testing.T) {
	dir := t.TempDir()
	configs := []config.SinkConfig{
		{Name: "archive", Type: "file", OutputDir: dir, RotationInterval: 10 * time.Minute},
	}
	sinks, err := createSinks(configs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sinks) != 1 {
		t.Fatalf("expected 1 sink, got %d", len(sinks))
	}
}

func TestCreateSinksFileDefaultRotation(t *testing.T) {
	dir := t.TempDir()
	configs := []config.SinkConfig{
		{Name: "archive", Type: "file", OutputDir: dir},
	}
	sinks, err := createSinks(configs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sinks) != 1 {
		t.Fatalf("expected 1 sink, got %d", len(sinks))
	}
}

func TestCreateSinksFileNoOutputDir(t *testing.T) {
	configs := []config.SinkConfig{
		{Name: "archive", Type: "file"},
	}
	_, err := createSinks(configs)
	if err == nil {
		t.Fatal("expected error for file sink without output_dir")
	}
}

func TestMetricsEndpoint(t *testing.T) {
	reg := prometheus.NewRegistry()
	met := metrics.NewMetrics(reg)

	// Trigger a metric so there's something to scrape.
	met.RecordReceived("webhook")

	r := chi.NewRouter()
	r.Get("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("expected text/plain content type, got %s", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "straumheim_records_received_total") {
		t.Fatalf("expected straumheim_ prefixed metrics in body, got:\n%s", body)
	}
}

func TestRegisterInputs(t *testing.T) {
	r := chi.NewRouter()
	buf := buffer.NewMemoryBuffer(100, 10, 1000000000)
	engine := pipeline.NewEngine(buf, []sink.Sink{}, nil, nil)

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

func TestRegisterInputsSnowplow(t *testing.T) {
	r := chi.NewRouter()
	buf := buffer.NewMemoryBuffer(100, 10, 1000000000)
	engine := pipeline.NewEngine(buf, []sink.Sink{}, nil, nil)

	inputs := map[string]config.InputConfig{
		"snowplow": {Enabled: true},
	}
	registerInputs(r, inputs, engine)

	// Send a GET request to verify the snowplow GET endpoint is registered.
	req := httptest.NewRequest(http.MethodGet, "/sp/i?e=pv", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/gif" {
		t.Fatalf("expected image/gif, got %s", ct)
	}

	// Send a POST request to verify the snowplow POST endpoint is registered.
	body := strings.NewReader(`{"schema":"...","data":[{"e":"pv"}]}`)
	req2 := httptest.NewRequest(http.MethodPost, "/sp/tp2", body)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for POST, got %d", w2.Code)
	}

	engine.Close()
}

func TestCreateSinksClickHouse(t *testing.T) {
	configs := []config.SinkConfig{
		{Name: "ch", Type: "clickhouse", Endpoint: "http://localhost:8123", Database: "mydb", Table: "events"},
	}
	sinks, err := createSinks(configs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sinks) != 1 {
		t.Fatalf("expected 1 sink, got %d", len(sinks))
	}
}

func TestCreateSinksClickHouseDefaultDatabase(t *testing.T) {
	configs := []config.SinkConfig{
		{Name: "ch", Type: "clickhouse", Endpoint: "http://localhost:8123"},
	}
	sinks, err := createSinks(configs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sinks) != 1 {
		t.Fatalf("expected 1 sink, got %d", len(sinks))
	}
}

func TestCreateSinksClickHouseNoEndpoint(t *testing.T) {
	configs := []config.SinkConfig{
		{Name: "ch", Type: "clickhouse"},
	}
	_, err := createSinks(configs)
	if err == nil {
		t.Fatal("expected error for clickhouse sink without endpoint")
	}
}

func TestCreateSinksClickHousePassword(t *testing.T) {
	t.Setenv("CH_PASS", "secret123")
	configs := []config.SinkConfig{
		{Name: "ch", Type: "clickhouse", Endpoint: "http://localhost:8123", Username: "user", Password: "${CH_PASS}"},
	}
	// Note: env var substitution happens during config loading, not in createSinks.
	// This test verifies the fields are accepted.
	sinks, err := createSinks(configs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sinks) != 1 {
		t.Fatalf("expected 1 sink, got %d", len(sinks))
	}
}

func TestRegisterInputsPixel(t *testing.T) {
	r := chi.NewRouter()
	buf := buffer.NewMemoryBuffer(100, 10, 1000000000)
	engine := pipeline.NewEngine(buf, []sink.Sink{}, nil, nil)

	inputs := map[string]config.InputConfig{
		"pixel": {Enabled: true, Path: "/px"},
	}
	registerInputs(r, inputs, engine)

	// Send a pixel request to verify it's registered.
	req := httptest.NewRequest(http.MethodGet, "/px?event=test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/gif" {
		t.Errorf("expected Content-Type 'image/gif', got %q", ct)
	}

	engine.Close()
}

// newTestRouterWithCORS creates a Chi router with CORS middleware configured with the given origins.
func newTestRouterWithCORS(origins []string) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: origins,
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type"},
	}))
	r.Post("/webhook", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/health", healthHandler)
	return r
}

func TestCORS_OptionsReturnsAllowOrigin(t *testing.T) {
	r := newTestRouterWithCORS([]string{"https://example.com"})

	req := httptest.NewRequest(http.MethodOptions, "/webhook", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != "https://example.com" {
		t.Fatalf("expected Access-Control-Allow-Origin 'https://example.com', got %q", acao)
	}
}

func TestCORS_OptionsReturnsAllowMethods(t *testing.T) {
	r := newTestRouterWithCORS([]string{"https://example.com"})

	// Verify each allowed method is accepted in a preflight request.
	for _, method := range []string{"GET", "POST", "OPTIONS"} {
		req := httptest.NewRequest(http.MethodOptions, "/webhook", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", method)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		acam := w.Header().Get("Access-Control-Allow-Methods")
		if acam == "" {
			t.Errorf("expected Access-Control-Allow-Methods for request method %s, got empty", method)
		}
	}
}

func TestCORS_OptionsReturnsAllowHeaders(t *testing.T) {
	r := newTestRouterWithCORS([]string{"https://example.com"})

	req := httptest.NewRequest(http.MethodOptions, "/webhook", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	acah := w.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(strings.ToLower(acah), "content-type") {
		t.Errorf("expected Access-Control-Allow-Headers to contain Content-Type, got %q", acah)
	}
}

func TestCORS_WildcardAllowsAllOrigins(t *testing.T) {
	r := newTestRouterWithCORS([]string{"*"})

	req := httptest.NewRequest(http.MethodOptions, "/webhook", nil)
	req.Header.Set("Origin", "https://any-site.example.org")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin '*', got %q", acao)
	}
}

func TestRecovery_PanicReturns500(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Get("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// captureHandler is a slog.Handler that captures log records for testing.
type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) findAttr(rec slog.Record, key string) (slog.Value, bool) {
	var val slog.Value
	var found bool
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			val = a.Value
			found = true
			return false
		}
		return true
	})
	return val, found
}

func TestRequestLogger_LogsRequestFields(t *testing.T) {
	ch := &captureHandler{}
	logger := slog.New(ch)
	slog.SetDefault(logger)

	r := chi.NewRouter()
	r.Use(requestLogger)
	r.Post("/webhook", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if len(ch.records) != 1 {
		t.Fatalf("expected 1 log record, got %d", len(ch.records))
	}

	rec := ch.records[0]
	if rec.Level != slog.LevelInfo {
		t.Errorf("expected INFO level, got %v", rec.Level)
	}

	// Check method field.
	if v, ok := ch.findAttr(rec, "method"); !ok || v.String() != "POST" {
		t.Errorf("expected method=POST, got %v (found=%v)", v, ok)
	}

	// Check path field.
	if v, ok := ch.findAttr(rec, "path"); !ok || v.String() != "/webhook" {
		t.Errorf("expected path=/webhook, got %v (found=%v)", v, ok)
	}

	// Check status field.
	if v, ok := ch.findAttr(rec, "status"); !ok || v.Int64() != 200 {
		t.Errorf("expected status=200, got %v (found=%v)", v, ok)
	}

	// Check duration_ms field exists.
	if _, ok := ch.findAttr(rec, "duration_ms"); !ok {
		t.Error("expected duration_ms field in log record")
	}

	// Check remote_addr field.
	if v, ok := ch.findAttr(rec, "remote_addr"); !ok || v.String() != "192.168.1.1:12345" {
		t.Errorf("expected remote_addr=192.168.1.1:12345, got %v (found=%v)", v, ok)
	}
}

func TestRequestLogger_SkipsHealthEndpoint(t *testing.T) {
	ch := &captureHandler{}
	logger := slog.New(ch)
	slog.SetDefault(logger)

	r := chi.NewRouter()
	r.Use(requestLogger)
	r.Get("/health", healthHandler)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if len(ch.records) != 0 {
		t.Fatalf("expected no log records for /health, got %d", len(ch.records))
	}
}

func TestRequestLogger_CapturesNon200Status(t *testing.T) {
	ch := &captureHandler{}
	logger := slog.New(ch)
	slog.SetDefault(logger)

	r := chi.NewRouter()
	r.Use(requestLogger)
	r.Get("/bad", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if len(ch.records) != 1 {
		t.Fatalf("expected 1 log record, got %d", len(ch.records))
	}

	if v, ok := ch.findAttr(ch.records[0], "status"); !ok || v.Int64() != 400 {
		t.Errorf("expected status=400, got %v", v)
	}
}
