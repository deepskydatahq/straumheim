package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_ParsesYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	data := `
server:
  host: 0.0.0.0
  port: 9090
inputs:
  webhook:
    enabled: true
    path: /webhook
buffer:
  type: memory
  capacity: 5000
  flush_interval: 3s
  flush_count: 100
sinks:
  - name: debug
    type: stdout
    mode: stream
`
	if err := os.WriteFile(cfgPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if !cfg.Inputs["webhook"].Enabled {
		t.Error("expected webhook enabled")
	}
	if cfg.Buffer.Capacity != 5000 {
		t.Errorf("expected capacity 5000, got %d", cfg.Buffer.Capacity)
	}
	if cfg.Buffer.FlushInterval != 3*time.Second {
		t.Errorf("expected flush_interval 3s, got %v", cfg.Buffer.FlushInterval)
	}
	if len(cfg.Sinks) != 1 || cfg.Sinks[0].Name != "debug" {
		t.Errorf("unexpected sinks: %+v", cfg.Sinks)
	}
}

func TestLoadConfig_EnvVarSubstitution(t *testing.T) {
	t.Setenv("TEST_DSN", "postgres://localhost/test")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	data := `
server:
  port: 8080
sinks:
  - name: warehouse
    type: postgres
    dsn: ${TEST_DSN}
`
	if err := os.WriteFile(cfgPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Sinks[0].DSN != "postgres://localhost/test" {
		t.Errorf("expected DSN substitution, got %s", cfg.Sinks[0].DSN)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	data := `
server:
  host: 0.0.0.0
`
	if err := os.WriteFile(cfgPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Buffer.Type != "memory" {
		t.Errorf("expected default buffer type memory, got %s", cfg.Buffer.Type)
	}
	if cfg.Buffer.Capacity != 10000 {
		t.Errorf("expected default capacity 10000, got %d", cfg.Buffer.Capacity)
	}
	if cfg.Buffer.FlushInterval != 5*time.Second {
		t.Errorf("expected default flush_interval 5s, got %v", cfg.Buffer.FlushInterval)
	}
	if cfg.Buffer.FlushCount != 500 {
		t.Errorf("expected default flush_count 500, got %d", cfg.Buffer.FlushCount)
	}
}

func TestLoadConfig_CORSDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	data := `
server:
  host: 0.0.0.0
`
	if err := os.WriteFile(cfgPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.Server.CORS.AllowedOrigins) != 1 || cfg.Server.CORS.AllowedOrigins[0] != "*" {
		t.Errorf("expected default CORS allowed_origins [*], got %v", cfg.Server.CORS.AllowedOrigins)
	}
}

func TestLoadConfig_CORSConfigured(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	data := `
server:
  host: 0.0.0.0
  cors:
    allowed_origins:
      - https://example.com
      - https://app.example.com
`
	if err := os.WriteFile(cfgPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.Server.CORS.AllowedOrigins) != 2 {
		t.Errorf("expected 2 CORS allowed_origins, got %d", len(cfg.Server.CORS.AllowedOrigins))
	}
	if cfg.Server.CORS.AllowedOrigins[0] != "https://example.com" {
		t.Errorf("expected first origin https://example.com, got %s", cfg.Server.CORS.AllowedOrigins[0])
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadConfig_GCPRuntime(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	data := `
runtime:
  mode: collector
  pubsub:
    project: test-project
    topic: events
`
	if err := os.WriteFile(cfgPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Runtime.Mode != "collector" || cfg.Runtime.PubSub.Project != "test-project" || cfg.Runtime.PubSub.Topic != "events" {
		t.Fatalf("unexpected runtime config: %+v", cfg.Runtime)
	}
	if cfg.Runtime.PubSub.PushPath != "/internal/pubsub/push" {
		t.Fatalf("push_path = %q, want default", cfg.Runtime.PubSub.PushPath)
	}
}

func TestLoadConfig_BigQuerySink(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	data := `
sinks:
  - name: warehouse
    type: bigquery
    project: test-project
    dataset: analytics
    table: events
    location: EU
    max_inflight_requests: 2
`
	if err := os.WriteFile(cfgPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.Sinks) != 1 {
		t.Fatalf("got %d sinks, want 1", len(cfg.Sinks))
	}
	sink := cfg.Sinks[0]
	if sink.Project != "test-project" || sink.Dataset != "analytics" || sink.Table != "events" || sink.Location != "EU" {
		t.Fatalf("unexpected BigQuery destination: %+v", sink)
	}
	if sink.MaxInflightRequests != 2 {
		t.Fatalf("max_inflight_requests = %d, want 2", sink.MaxInflightRequests)
	}
}
