package sink

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deepskydatahq/straumheim/internal/record"
)

// mockDB implements PGDB for testing.
type mockDB struct {
	execCalls    []string
	copyFromArgs []copyFromCall
	closed       bool
}

type copyFromCall struct {
	table   pgx.Identifier
	columns []string
	rows    [][]any
}

func (m *mockDB) Exec(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	m.execCalls = append(m.execCalls, sql)
	return nil, nil
}

func (m *mockDB) CopyFrom(_ context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	var rows [][]any
	for rowSrc.Next() {
		vals, err := rowSrc.Values()
		if err != nil {
			return 0, err
		}
		row := make([]any, len(vals))
		copy(row, vals)
		rows = append(rows, row)
	}
	if err := rowSrc.Err(); err != nil {
		return 0, err
	}
	m.copyFromArgs = append(m.copyFromArgs, copyFromCall{
		table:   tableName,
		columns: columnNames,
		rows:    rows,
	})
	return int64(len(rows)), nil
}

func (m *mockDB) Close() {
	m.closed = true
}

// Verify PostgresSink implements Sink interface.
var _ Sink = (*PostgresSink)(nil)

func TestPostgresSinkMode(t *testing.T) {
	s := NewPostgresSink("postgres://localhost/test")
	if s.Mode() != SinkModeBatch {
		t.Fatalf("expected SinkModeBatch, got %s", s.Mode())
	}
}

func TestPostgresSinkInit(t *testing.T) {
	db := &mockDB{}
	s := NewPostgresSinkWithDB(db)
	if err := s.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(db.execCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(db.execCalls))
	}
	sql := db.execCalls[0]
	if !strings.HasPrefix(sql, "CREATE TABLE IF NOT EXISTS events") {
		t.Fatalf("unexpected SQL: %s", sql)
	}
	// Verify core columns are present.
	for _, col := range []string{"id", "timestamp", "protocol", "source", "payload", "ip", "user_agent", "referer", "is_valid", "device_time", "schema", "vendor", "schema_version"} {
		if !strings.Contains(sql, col) {
			t.Errorf("missing column %s in CREATE TABLE", col)
		}
	}
}

func TestPostgresSinkWriteBasic(t *testing.T) {
	db := &mockDB{}
	s := NewPostgresSinkWithDB(db)

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	isValid := true
	records := []record.Record{
		{
			ID:        "id-1",
			Timestamp: now,
			Protocol:  "http",
			Source:    "web",
			IsValid:   &isValid,
			Payload:   map[string]any{"key": "value"},
		},
	}

	if err := s.Write(context.Background(), records); err != nil {
		t.Fatal(err)
	}

	if len(db.copyFromArgs) != 1 {
		t.Fatalf("expected 1 CopyFrom call, got %d", len(db.copyFromArgs))
	}

	call := db.copyFromArgs[0]
	if call.table[0] != "events" {
		t.Errorf("expected table 'events', got %v", call.table)
	}

	// Should have core columns + flattened "key" column.
	expectedCols := len(coreColumns) + 1 // +1 for flattened "key"
	if len(call.columns) != expectedCols {
		t.Errorf("expected %d columns, got %d: %v", expectedCols, len(call.columns), call.columns)
	}

	// Check that ALTER TABLE was called for the new "key" column.
	if len(db.execCalls) != 1 {
		t.Fatalf("expected 1 exec call (ALTER), got %d", len(db.execCalls))
	}
	if !strings.Contains(db.execCalls[0], "ALTER TABLE") {
		t.Errorf("expected ALTER TABLE call, got: %s", db.execCalls[0])
	}

	// Verify row data.
	if len(call.rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(call.rows))
	}
	row := call.rows[0]
	if row[0] != "id-1" {
		t.Errorf("expected id 'id-1', got %v", row[0])
	}
}

func TestPostgresSinkWriteEmpty(t *testing.T) {
	db := &mockDB{}
	s := NewPostgresSinkWithDB(db)

	if err := s.Write(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(db.copyFromArgs) != 0 {
		t.Fatal("expected no CopyFrom calls for empty write")
	}
}

func TestPostgresSinkNewColumnsAddedDynamically(t *testing.T) {
	db := &mockDB{}
	s := NewPostgresSinkWithDB(db)

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// First write with field "a".
	records1 := []record.Record{
		{
			ID:        "id-1",
			Timestamp: now,
			Payload:   map[string]any{"a": "1"},
		},
	}
	if err := s.Write(context.Background(), records1); err != nil {
		t.Fatal(err)
	}

	// Second write with field "b" (new) and "a" (existing).
	records2 := []record.Record{
		{
			ID:        "id-2",
			Timestamp: now,
			Payload:   map[string]any{"a": "2", "b": "3"},
		},
	}
	if err := s.Write(context.Background(), records2); err != nil {
		t.Fatal(err)
	}

	// Should have 2 ALTER TABLE calls total: one for "a", one for "b".
	alterCount := 0
	for _, sql := range db.execCalls {
		if strings.Contains(sql, "ALTER TABLE") {
			alterCount++
		}
	}
	if alterCount != 2 {
		t.Errorf("expected 2 ALTER TABLE calls, got %d", alterCount)
	}

	// Second CopyFrom should include both "a" and "b" columns.
	call := db.copyFromArgs[1]
	found := map[string]bool{}
	for _, col := range call.columns {
		found[col] = true
	}
	if !found["a"] || !found["b"] {
		t.Errorf("expected columns a and b, got %v", call.columns)
	}
}

func TestPostgresSinkClose(t *testing.T) {
	db := &mockDB{}
	s := NewPostgresSinkWithDB(db)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if !db.closed {
		t.Fatal("expected db to be closed")
	}
}

func TestPgQuoteIdent(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"simple", `"simple"`},
		{`has"quote`, `"has""quote"`},
		{"with space", `"with space"`},
	}
	for _, tt := range tests {
		got := pgQuoteIdent(tt.input)
		if got != tt.want {
			t.Errorf("pgQuoteIdent(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
