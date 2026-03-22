package sink

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepsky-data/straumheim/internal/record"
)

// coreColumns defines the columns always present in the events table, in order.
var coreColumns = []coreColumnDef{
	{name: "id", sqlType: "TEXT PRIMARY KEY"},
	{name: "timestamp", sqlType: "TIMESTAMPTZ NOT NULL"},
	{name: "protocol", sqlType: "TEXT"},
	{name: "source", sqlType: "TEXT"},
	{name: "payload", sqlType: "JSONB"},
	{name: "ip", sqlType: "TEXT"},
	{name: "user_agent", sqlType: "TEXT"},
	{name: "referer", sqlType: "TEXT"},
	{name: "is_valid", sqlType: "BOOLEAN"},
	{name: "device_time", sqlType: "TIMESTAMPTZ"},
	{name: "schema", sqlType: "TEXT"},
	{name: "vendor", sqlType: "TEXT"},
	{name: "schema_version", sqlType: "TEXT"},
}

type coreColumnDef struct {
	name    string
	sqlType string
}

// coreColumnSet is the set of core column names for quick lookup.
var coreColumnSet = func() map[string]bool {
	m := make(map[string]bool, len(coreColumns))
	for _, c := range coreColumns {
		m[c.name] = true
	}
	return m
}()

// PGDB is the interface for database operations used by PostgresSink.
// This allows mocking in tests.
type PGDB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
	Close()
}

// pgxPoolAdapter wraps pgxpool.Pool to satisfy the PGDB interface.
type pgxPoolAdapter struct {
	pool *pgxpool.Pool
}

func (a *pgxPoolAdapter) Exec(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error) {
	return a.pool.Query(ctx, sql, arguments...)
}

func (a *pgxPoolAdapter) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return a.pool.CopyFrom(ctx, tableName, columnNames, rowSrc)
}

func (a *pgxPoolAdapter) Close() {
	a.pool.Close()
}

// PostgresSink writes records to a Postgres database using COPY FROM STDIN.
type PostgresSink struct {
	dsn       string
	db        PGDB
	mu        sync.Mutex
	knownCols map[string]bool // tracks all known columns (core + flattened)
}

// NewPostgresSink creates a new PostgresSink with the given DSN.
func NewPostgresSink(dsn string) *PostgresSink {
	known := make(map[string]bool, len(coreColumns))
	for _, c := range coreColumns {
		known[c.name] = true
	}
	return &PostgresSink{
		dsn:       dsn,
		knownCols: known,
	}
}

// NewPostgresSinkWithDB creates a PostgresSink with a pre-configured DB (for testing).
func NewPostgresSinkWithDB(db PGDB) *PostgresSink {
	known := make(map[string]bool, len(coreColumns))
	for _, c := range coreColumns {
		known[c.name] = true
	}
	return &PostgresSink{
		db:        db,
		knownCols: known,
	}
}

// Init connects to Postgres and creates the events table with core columns.
func (s *PostgresSink) Init(ctx context.Context) error {
	if s.db == nil {
		pool, err := pgxpool.New(ctx, s.dsn)
		if err != nil {
			return fmt.Errorf("postgres sink: connect: %w", err)
		}
		s.db = &pgxPoolAdapter{pool: pool}
	}

	var colDefs []string
	for _, c := range coreColumns {
		colDefs = append(colDefs, fmt.Sprintf("%s %s", c.name, c.sqlType))
	}
	sql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS events (%s)", strings.Join(colDefs, ", "))
	rows, err := s.db.Exec(ctx, sql)
	if err != nil {
		return fmt.Errorf("postgres sink: create table: %w", err)
	}
	if rows != nil {
		rows.Close()
	}
	return nil
}

// Write sends records to Postgres via CopyFrom. It auto-adds columns for new flattened keys.
func (s *PostgresSink) Write(ctx context.Context, records []record.Record) error {
	if len(records) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure all records are flattened and discover new columns.
	var newCols []string
	for i := range records {
		records[i].EnsureFlattened()
		for k := range records[i].Flattened {
			if !s.knownCols[k] {
				newCols = append(newCols, k)
				s.knownCols[k] = true
			}
		}
	}

	// ALTER TABLE to add new columns.
	for _, col := range newCols {
		sql := fmt.Sprintf("ALTER TABLE events ADD COLUMN IF NOT EXISTS %s TEXT", pgQuoteIdent(col))
		rows, err := s.db.Exec(ctx, sql)
		if err != nil {
			return fmt.Errorf("postgres sink: alter table add %s: %w", col, err)
		}
		if rows != nil {
			rows.Close()
		}
	}

	// Build column list: core columns + sorted flattened columns.
	flatCols := s.flattenedColumns()
	allCols := make([]string, 0, len(coreColumns)+len(flatCols))
	for _, c := range coreColumns {
		allCols = append(allCols, c.name)
	}
	allCols = append(allCols, flatCols...)

	// Build rows for CopyFrom.
	rows := make([][]any, 0, len(records))
	for _, r := range records {
		row := buildRow(r, flatCols)
		rows = append(rows, row)
	}

	_, err := s.db.CopyFrom(ctx, pgx.Identifier{"events"}, allCols, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("postgres sink: copy: %w", err)
	}
	return nil
}

// flattenedColumns returns sorted non-core known columns.
func (s *PostgresSink) flattenedColumns() []string {
	var cols []string
	for col := range s.knownCols {
		if !coreColumnSet[col] {
			cols = append(cols, col)
		}
	}
	sort.Strings(cols)
	return cols
}

// buildRow builds a row of values for the given record, matching the column order.
func buildRow(r record.Record, flatCols []string) []any {
	var payloadJSON []byte
	if r.Payload != nil {
		payloadJSON, _ = json.Marshal(r.Payload)
	}

	row := []any{
		r.ID,
		r.Timestamp,
		r.Protocol,
		r.Source,
		nilIfEmpty(payloadJSON),
		r.IP,
		r.UserAgent,
		r.Referer,
		r.IsValid,
		r.DeviceTime,
		r.Schema,
		r.Vendor,
		r.SchemaVersion,
	}

	// Append flattened values in sorted order.
	for _, col := range flatCols {
		if r.Flattened != nil {
			if v, ok := r.Flattened[col]; ok {
				row = append(row, fmt.Sprintf("%v", v))
				continue
			}
		}
		row = append(row, nil)
	}
	return row
}

func nilIfEmpty(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

// Flush is a no-op for the Postgres sink since Write already commits via COPY.
func (s *PostgresSink) Flush(_ context.Context) error { return nil }

// Close closes the database connection pool.
func (s *PostgresSink) Close() error {
	if s.db != nil {
		s.db.Close()
	}
	return nil
}

// Mode returns SinkModeBatch.
func (s *PostgresSink) Mode() SinkMode { return SinkModeBatch }

// pgQuoteIdent quotes an identifier for safe use in SQL.
func pgQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
