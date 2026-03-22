package sink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/deepskydatahq/straumheim/internal/record"
)

// chCoreColumns defines the columns always present in the ClickHouse events table.
var chCoreColumns = []chColumnDef{
	{name: "id", sqlType: "String"},
	{name: "timestamp", sqlType: "DateTime64(3)"},
	{name: "device_time", sqlType: "Nullable(DateTime64(3))"},
	{name: "protocol", sqlType: "String"},
	{name: "source", sqlType: "String"},
	{name: "ip", sqlType: "String"},
	{name: "user_agent", sqlType: "String"},
	{name: "referer", sqlType: "String"},
	{name: "schema", sqlType: "String"},
	{name: "vendor", sqlType: "String"},
	{name: "schema_version", sqlType: "String"},
	{name: "is_valid", sqlType: "Nullable(UInt8)"},
	{name: "payload", sqlType: "String"},
}

type chColumnDef struct {
	name    string
	sqlType string
}

// chCoreColumnSet is the set of core column names for quick lookup.
var chCoreColumnSet = func() map[string]bool {
	m := make(map[string]bool, len(chCoreColumns))
	for _, c := range chCoreColumns {
		m[c.name] = true
	}
	return m
}()

// ClickHouseSink writes records to ClickHouse via the HTTP interface using JSONEachRow format.
type ClickHouseSink struct {
	endpoint string
	database string
	table    string
	username string
	password string
	client   *http.Client
	mu       sync.Mutex
	knownCols map[string]bool
}

// NewClickHouseSink creates a new ClickHouseSink.
func NewClickHouseSink(endpoint, database, table, username, password string) *ClickHouseSink {
	known := make(map[string]bool, len(chCoreColumns))
	for _, c := range chCoreColumns {
		known[c.name] = true
	}
	return &ClickHouseSink{
		endpoint:  endpoint,
		database:  database,
		table:     table,
		username:  username,
		password:  password,
		client:    &http.Client{},
		knownCols: known,
	}
}

// Init creates the events table with core columns using MergeTree engine.
func (s *ClickHouseSink) Init(ctx context.Context) error {
	var colDefs []string
	for _, c := range chCoreColumns {
		colDefs = append(colDefs, fmt.Sprintf("%s %s", c.name, c.sqlType))
	}
	query := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s.%s (%s) ENGINE = MergeTree() ORDER BY (timestamp, id)",
		s.database, s.table, strings.Join(colDefs, ", "),
	)
	return s.execQuery(ctx, query, nil)
}

// Write sends records to ClickHouse as JSONEachRow via HTTP POST.
// It auto-adds columns for new flattened keys before inserting.
func (s *ClickHouseSink) Write(ctx context.Context, records []record.Record) error {
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
		query := fmt.Sprintf(
			"ALTER TABLE %s.%s ADD COLUMN IF NOT EXISTS %s String",
			s.database, s.table, chQuoteIdent(col),
		)
		if err := s.execQuery(ctx, query, nil); err != nil {
			return fmt.Errorf("clickhouse sink: alter table add %s: %w", col, err)
		}
	}

	// Build JSONEachRow body.
	flatCols := s.flattenedColumns()
	var buf bytes.Buffer
	for _, r := range records {
		row := buildCHRow(r, flatCols)
		data, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("clickhouse sink: marshal record %s: %w", r.ID, err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}

	// POST insert.
	query := fmt.Sprintf("INSERT INTO %s.%s FORMAT JSONEachRow", s.database, s.table)
	return s.execQuery(ctx, query, &buf)
}

// Flush is a no-op for ClickHouse since Write sends data immediately.
func (s *ClickHouseSink) Flush(_ context.Context) error { return nil }

// Close is a no-op — the HTTP client has no persistent connections to close.
func (s *ClickHouseSink) Close() error { return nil }

// Mode returns SinkModeBatch.
func (s *ClickHouseSink) Mode() SinkMode { return SinkModeBatch }

// execQuery sends a query to ClickHouse via the HTTP interface.
func (s *ClickHouseSink) execQuery(ctx context.Context, query string, body io.Reader) error {
	if body == nil {
		body = http.NoBody
	}

	u, err := url.Parse(s.endpoint)
	if err != nil {
		return fmt.Errorf("clickhouse sink: parse endpoint: %w", err)
	}
	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), body)
	if err != nil {
		return fmt.Errorf("clickhouse sink: create request: %w", err)
	}

	if s.username != "" || s.password != "" {
		req.SetBasicAuth(s.username, s.password)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("clickhouse sink: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clickhouse sink: http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}

// flattenedColumns returns sorted non-core known columns.
func (s *ClickHouseSink) flattenedColumns() []string {
	var cols []string
	for col := range s.knownCols {
		if !chCoreColumnSet[col] {
			cols = append(cols, col)
		}
	}
	sort.Strings(cols)
	return cols
}

// buildCHRow builds a flat map for a single record suitable for JSONEachRow.
func buildCHRow(r record.Record, flatCols []string) map[string]any {
	row := map[string]any{
		"id":             r.ID,
		"timestamp":      r.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
		"protocol":       r.Protocol,
		"source":         r.Source,
		"ip":             r.IP,
		"user_agent":     r.UserAgent,
		"referer":        r.Referer,
		"schema":         r.Schema,
		"vendor":         r.Vendor,
		"schema_version": r.SchemaVersion,
	}

	// device_time: nullable.
	if r.DeviceTime != nil {
		row["device_time"] = r.DeviceTime.UTC().Format("2006-01-02T15:04:05Z")
	} else {
		row["device_time"] = nil
	}

	// is_valid: nullable UInt8 (1=true, 0=false, null=unknown).
	if r.IsValid != nil {
		if *r.IsValid {
			row["is_valid"] = 1
		} else {
			row["is_valid"] = 0
		}
	} else {
		row["is_valid"] = nil
	}

	// payload: JSON string.
	if r.Payload != nil {
		data, _ := json.Marshal(r.Payload)
		row["payload"] = string(data)
	} else {
		row["payload"] = ""
	}

	// Flattened dynamic columns.
	for _, col := range flatCols {
		if r.Flattened != nil {
			if v, ok := r.Flattened[col]; ok {
				row[col] = fmt.Sprintf("%v", v)
				continue
			}
		}
		row[col] = ""
	}

	return row
}

// chQuoteIdent quotes a ClickHouse identifier with backticks.
func chQuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
