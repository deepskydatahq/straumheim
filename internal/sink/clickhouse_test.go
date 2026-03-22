package sink

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deepskydatahq/straumheim/internal/record"
)

// Verify ClickHouseSink implements Sink interface.
var _ Sink = (*ClickHouseSink)(nil)

func TestClickHouseSinkMode(t *testing.T) {
	s := NewClickHouseSink("http://localhost:8123", "default", "events", "", "")
	if s.Mode() != SinkModeBatch {
		t.Fatalf("expected SinkModeBatch, got %s", s.Mode())
	}
}

func TestClickHouseSinkInit_CreateTable(t *testing.T) {
	var receivedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query().Get("query")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewClickHouseSink(srv.URL, "testdb", "events", "", "")
	if err := s.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(receivedQuery, "CREATE TABLE IF NOT EXISTS testdb.events") {
		t.Fatalf("expected CREATE TABLE, got: %s", receivedQuery)
	}
	if !strings.Contains(receivedQuery, "MergeTree()") {
		t.Fatalf("expected MergeTree engine, got: %s", receivedQuery)
	}
	if !strings.Contains(receivedQuery, "ORDER BY (timestamp, id)") {
		t.Fatalf("expected ORDER BY (timestamp, id), got: %s", receivedQuery)
	}

	// Verify core columns are present.
	for _, col := range []string{
		"id String",
		"timestamp DateTime64(3)",
		"device_time Nullable(DateTime64(3))",
		"protocol String",
		"source String",
		"ip String",
		"user_agent String",
		"referer String",
		"schema String",
		"vendor String",
		"schema_version String",
		"is_valid Nullable(UInt8)",
		"payload String",
	} {
		if !strings.Contains(receivedQuery, col) {
			t.Errorf("missing column %q in CREATE TABLE: %s", col, receivedQuery)
		}
	}
}

func TestClickHouseSinkWrite_JSONEachRow(t *testing.T) {
	var (
		receivedQuery string
		receivedBody  string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query().Get("query")
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewClickHouseSink(srv.URL, "testdb", "events", "", "")

	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	isValid := true
	records := []record.Record{
		{
			ID:        "id-1",
			Timestamp: now,
			Protocol:  "webhook",
			Source:    "web",
			IsValid:   &isValid,
			Payload:   map[string]any{"key": "value"},
			IP:        "1.2.3.4",
			UserAgent: "TestAgent",
			Referer:   "https://example.com",
			Schema:    "test-schema",
			Vendor:    "test-vendor",
			SchemaVersion: "1.0",
		},
	}

	if err := s.Write(context.Background(), records); err != nil {
		t.Fatal(err)
	}

	// Verify INSERT query format.
	expectedQuery := "INSERT INTO testdb.events FORMAT JSONEachRow"
	if receivedQuery != expectedQuery {
		t.Fatalf("expected query %q, got %q", expectedQuery, receivedQuery)
	}

	// Verify body is valid JSON.
	lines := strings.Split(strings.TrimSpace(receivedBody), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 JSON line, got %d", len(lines))
	}

	var row map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &row); err != nil {
		t.Fatalf("invalid JSON line: %v", err)
	}

	// Verify core fields.
	if row["id"] != "id-1" {
		t.Errorf("expected id 'id-1', got %v", row["id"])
	}
	if row["timestamp"] != "2025-01-15 10:30:00.000" {
		t.Errorf("expected timestamp '2025-01-15T10:30:00Z', got %v", row["timestamp"])
	}
	if row["protocol"] != "webhook" {
		t.Errorf("expected protocol 'webhook', got %v", row["protocol"])
	}
	if row["is_valid"] != float64(1) {
		t.Errorf("expected is_valid 1, got %v", row["is_valid"])
	}
	if row["ip"] != "1.2.3.4" {
		t.Errorf("expected ip '1.2.3.4', got %v", row["ip"])
	}

	// Verify flattened field appears.
	if row["key"] != "value" {
		t.Errorf("expected flattened key 'value', got %v", row["key"])
	}

	// Verify payload is JSON string.
	payloadStr, ok := row["payload"].(string)
	if !ok {
		t.Fatalf("expected payload to be a string, got %T", row["payload"])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
}

func TestClickHouseSinkWrite_BatchProducesSingleHTTPPost(t *testing.T) {
	postCount := 0
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "" && strings.HasPrefix(r.URL.Query().Get("query"), "INSERT") {
			postCount++
			body, _ := io.ReadAll(r.Body)
			receivedBody = string(body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewClickHouseSink(srv.URL, "default", "events", "", "")
	now := time.Now()
	records := []record.Record{
		{ID: "id-1", Timestamp: now, Payload: map[string]any{"a": "1"}},
		{ID: "id-2", Timestamp: now, Payload: map[string]any{"a": "2"}},
		{ID: "id-3", Timestamp: now, Payload: map[string]any{"a": "3"}},
	}

	if err := s.Write(context.Background(), records); err != nil {
		t.Fatal(err)
	}

	if postCount != 1 {
		t.Fatalf("expected 1 INSERT POST, got %d", postCount)
	}

	lines := strings.Split(strings.TrimSpace(receivedBody), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 JSON lines, got %d", len(lines))
	}
}

func TestClickHouseSinkWrite_NewColumnsAlterTable(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if q != "" {
			queries = append(queries, q)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewClickHouseSink(srv.URL, "testdb", "events", "", "")
	now := time.Now()

	// First write with field "a".
	records1 := []record.Record{
		{ID: "id-1", Timestamp: now, Payload: map[string]any{"a": "1"}},
	}
	if err := s.Write(context.Background(), records1); err != nil {
		t.Fatal(err)
	}

	// Second write with field "b" (new).
	records2 := []record.Record{
		{ID: "id-2", Timestamp: now, Payload: map[string]any{"a": "2", "b": "3"}},
	}
	if err := s.Write(context.Background(), records2); err != nil {
		t.Fatal(err)
	}

	// Count ALTER TABLE calls.
	alterCount := 0
	for _, q := range queries {
		if strings.Contains(q, "ALTER TABLE") {
			alterCount++
			if !strings.Contains(q, "ADD COLUMN IF NOT EXISTS") {
				t.Errorf("ALTER TABLE should use IF NOT EXISTS: %s", q)
			}
			if !strings.Contains(q, "String") {
				t.Errorf("dynamic columns should be String type: %s", q)
			}
		}
	}

	// Should have 2 ALTER calls: one for "a", one for "b".
	if alterCount != 2 {
		t.Errorf("expected 2 ALTER TABLE calls, got %d", alterCount)
	}
}

func TestClickHouseSinkWrite_HTTPErrorReturnsWrappedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("DB error: something broke"))
	}))
	defer srv.Close()

	s := NewClickHouseSink(srv.URL, "default", "events", "", "")
	now := time.Now()
	records := []record.Record{
		{ID: "id-1", Timestamp: now},
	}

	err := s.Write(context.Background(), records)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "clickhouse") {
		t.Errorf("error should mention clickhouse: %v", err)
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "DB error") {
		t.Errorf("error should include status code and body: %v", err)
	}
}

func TestClickHouseSinkInit_HTTPErrorReturnsWrappedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("syntax error"))
	}))
	defer srv.Close()

	s := NewClickHouseSink(srv.URL, "default", "events", "", "")
	err := s.Init(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "clickhouse") {
		t.Errorf("error should mention clickhouse: %v", err)
	}
}

func TestClickHouseSinkAuth(t *testing.T) {
	var receivedUser, receivedPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUser, receivedPass, _ = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewClickHouseSink(srv.URL, "default", "events", "myuser", "mypass")
	now := time.Now()
	records := []record.Record{
		{ID: "id-1", Timestamp: now},
	}
	if err := s.Write(context.Background(), records); err != nil {
		t.Fatal(err)
	}

	if receivedUser != "myuser" {
		t.Errorf("expected user 'myuser', got %q", receivedUser)
	}
	if receivedPass != "mypass" {
		t.Errorf("expected password 'mypass', got %q", receivedPass)
	}
}

func TestClickHouseSinkAuth_NoCredentials(t *testing.T) {
	var hasAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, hasAuth = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewClickHouseSink(srv.URL, "default", "events", "", "")
	now := time.Now()
	records := []record.Record{
		{ID: "id-1", Timestamp: now},
	}
	if err := s.Write(context.Background(), records); err != nil {
		t.Fatal(err)
	}

	if hasAuth {
		t.Error("expected no auth header when credentials are empty")
	}
}

func TestClickHouseSinkClose(t *testing.T) {
	s := NewClickHouseSink("http://localhost:8123", "default", "events", "", "")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClickHouseSinkWrite_Empty(t *testing.T) {
	s := NewClickHouseSink("http://localhost:8123", "default", "events", "", "")
	if err := s.Write(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestClickHouseSinkWrite_DeviceTime(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewClickHouseSink(srv.URL, "default", "events", "", "")
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	dt := time.Date(2025, 6, 1, 11, 59, 0, 0, time.UTC)
	records := []record.Record{
		{ID: "id-1", Timestamp: now, DeviceTime: &dt},
	}
	if err := s.Write(context.Background(), records); err != nil {
		t.Fatal(err)
	}

	var row map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(receivedBody)), &row); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if row["device_time"] != "2025-06-01 11:59:00.000" {
		t.Errorf("expected device_time ClickHouse format, got %v", row["device_time"])
	}
}

func TestClickHouseSinkWrite_IsValidFalse(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewClickHouseSink(srv.URL, "default", "events", "", "")
	now := time.Now()
	isValid := false
	records := []record.Record{
		{ID: "id-1", Timestamp: now, IsValid: &isValid},
	}
	if err := s.Write(context.Background(), records); err != nil {
		t.Fatal(err)
	}

	var row map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(receivedBody)), &row); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if row["is_valid"] != float64(0) {
		t.Errorf("expected is_valid 0 for false, got %v", row["is_valid"])
	}
}

func TestClickHouseSinkWrite_IsValidNil(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewClickHouseSink(srv.URL, "default", "events", "", "")
	now := time.Now()
	records := []record.Record{
		{ID: "id-1", Timestamp: now},
	}
	if err := s.Write(context.Background(), records); err != nil {
		t.Fatal(err)
	}

	var row map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(receivedBody)), &row); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// is_valid should be null when nil.
	if row["is_valid"] != nil {
		t.Errorf("expected is_valid nil, got %v", row["is_valid"])
	}
}
