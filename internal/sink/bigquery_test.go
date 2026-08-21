package sink

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/deepskydatahq/straumheim/internal/record"
)

type fakeBigQueryAdmin struct {
	dataset   string
	table     string
	location  string
	metadata  *bigquery.TableMetadata
	ensureErr error
	closeErr  error
	closed    bool
}

func (f *fakeBigQueryAdmin) EnsureTable(
	_ context.Context,
	dataset, table, location string,
	metadata *bigquery.TableMetadata,
) error {
	f.dataset = dataset
	f.table = table
	f.location = location
	f.metadata = metadata
	return f.ensureErr
}

func (f *fakeBigQueryAdmin) Close() error {
	f.closed = true
	return f.closeErr
}

type fakeBigQueryAppender struct {
	calls     int
	rows      [][]byte
	appendErr error
	closeErr  error
	closed    bool
}

func (f *fakeBigQueryAppender) Append(_ context.Context, rows [][]byte) error {
	f.calls++
	f.rows = append(f.rows[:0], rows...)
	return f.appendErr
}

func (f *fakeBigQueryAppender) Close() error {
	f.closed = true
	return f.closeErr
}

func newTestBigQuerySink(
	admin *fakeBigQueryAdmin,
	appender *fakeBigQueryAppender,
) *BigQuerySink {
	return newBigQuerySinkWithDependencies(
		BigQueryOptions{
			Project:  "test-project",
			Dataset:  "analytics",
			Table:    "events",
			Location: "EU",
		},
		func(context.Context, string) (bigQueryTableAdmin, error) { return admin, nil },
		func(context.Context, BigQueryOptions, *descriptorpb.DescriptorProto) (bigQueryAppender, error) {
			return appender, nil
		},
	)
}

func TestBigQuerySinkValidate(t *testing.T) {
	tests := []struct {
		name    string
		options BigQueryOptions
		want    string
	}{
		{name: "project", options: BigQueryOptions{Dataset: "d", Table: "t", Location: "EU"}, want: "project is required"},
		{name: "dataset", options: BigQueryOptions{Project: "p", Table: "t", Location: "EU"}, want: "dataset is required"},
		{name: "table", options: BigQueryOptions{Project: "p", Dataset: "d", Location: "EU"}, want: "table is required"},
		{name: "location", options: BigQueryOptions{Project: "p", Dataset: "d", Table: "t"}, want: "location is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewBigQuerySink(tt.options)
			err := s.Init(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Init() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestBigQuerySinkInitUsesStableTableMetadata(t *testing.T) {
	admin := &fakeBigQueryAdmin{}
	appender := &fakeBigQueryAppender{}
	s := newTestBigQuerySink(admin, appender)

	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if admin.dataset != "analytics" || admin.table != "events" || admin.location != "EU" {
		t.Fatalf("unexpected destination: %s.%s (%s)", admin.dataset, admin.table, admin.location)
	}
	if admin.metadata == nil || admin.metadata.TimePartitioning == nil {
		t.Fatal("expected table metadata with partitioning")
	}
	if admin.metadata.TimePartitioning.Field != "timestamp" || admin.metadata.TimePartitioning.Type != bigquery.DayPartitioningType {
		t.Fatalf("unexpected partitioning: %+v", admin.metadata.TimePartitioning)
	}
	if got := admin.metadata.Clustering.Fields; len(got) != 2 || got[0] != "protocol" || got[1] != "source" {
		t.Fatalf("unexpected clustering fields: %v", got)
	}

	fields := make(map[string]*bigquery.FieldSchema)
	for _, field := range admin.metadata.Schema {
		fields[field.Name] = field
	}
	if fields["id"].Type != bigquery.StringFieldType || !fields["id"].Required {
		t.Fatalf("unexpected id field: %+v", fields["id"])
	}
	if fields["payload"].Type != bigquery.JSONFieldType || fields["flattened"].Type != bigquery.JSONFieldType {
		t.Fatalf("payload fields must be JSON: payload=%+v flattened=%+v", fields["payload"], fields["flattened"])
	}
}

func TestValidateBigQueryTableMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*bigquery.TableMetadata)
		want   string
	}{
		{name: "valid", mutate: func(*bigquery.TableMetadata) {}},
		{
			name: "extra nullable field",
			mutate: func(metadata *bigquery.TableMetadata) {
				metadata.Schema = append(metadata.Schema, &bigquery.FieldSchema{Name: "future", Type: bigquery.StringFieldType})
			},
		},
		{
			name: "extra required field",
			mutate: func(metadata *bigquery.TableMetadata) {
				metadata.Schema = append(metadata.Schema, &bigquery.FieldSchema{Name: "future", Type: bigquery.StringFieldType, Required: true})
			},
			want: "unsupported required field",
		},
		{
			name: "missing field",
			mutate: func(metadata *bigquery.TableMetadata) {
				metadata.Schema = metadata.Schema[:len(metadata.Schema)-1]
			},
			want: "missing field",
		},
		{
			name: "wrong type",
			mutate: func(metadata *bigquery.TableMetadata) {
				metadata.Schema[0].Type = bigquery.IntegerFieldType
			},
			want: "want STRING",
		},
		{
			name:   "partitioning",
			mutate: func(metadata *bigquery.TableMetadata) { metadata.TimePartitioning = nil },
			want:   "partitioning",
		},
		{
			name: "clustering",
			mutate: func(metadata *bigquery.TableMetadata) {
				metadata.Clustering = &bigquery.Clustering{Fields: []string{"source", "protocol"}}
			},
			want: "clustering",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := bigQueryTableMetadata()
			tt.mutate(actual)
			err := validateBigQueryTableMetadata(actual, bigQueryTableMetadata())
			if tt.want == "" {
				if err != nil {
					t.Fatalf("validateBigQueryTableMetadata() error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestBigQuerySinkInitFailureClosesAdmin(t *testing.T) {
	wantErr := errors.New("location mismatch")
	admin := &fakeBigQueryAdmin{ensureErr: wantErr}
	s := newTestBigQuerySink(admin, &fakeBigQueryAppender{})

	err := s.Init(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Init() error = %v, want wrapped %v", err, wantErr)
	}
	if !admin.closed {
		t.Fatal("expected metadata client to close after init failure")
	}
}

func TestBigQuerySinkAppenderFactoryFailureClosesAdmin(t *testing.T) {
	wantErr := errors.New("writer unavailable")
	admin := &fakeBigQueryAdmin{}
	s := newBigQuerySinkWithDependencies(
		BigQueryOptions{Project: "p", Dataset: "d", Table: "t", Location: "EU"},
		func(context.Context, string) (bigQueryTableAdmin, error) { return admin, nil },
		func(context.Context, BigQueryOptions, *descriptorpb.DescriptorProto) (bigQueryAppender, error) {
			return nil, wantErr
		},
	)

	err := s.Init(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Init() error = %v, want wrapped %v", err, wantErr)
	}
	if !admin.closed {
		t.Fatal("expected metadata client to close after writer failure")
	}
}

func TestBigQuerySinkWriteMapsRecord(t *testing.T) {
	admin := &fakeBigQueryAdmin{}
	appender := &fakeBigQueryAppender{}
	s := newTestBigQuerySink(admin, appender)
	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	timestamp := time.Date(2026, 8, 21, 5, 0, 1, 234567000, time.UTC)
	receivedAt := timestamp.Add(time.Second)
	deviceTime := timestamp.Add(-time.Minute)
	valid := true
	r := record.Record{
		ID:            "event-1",
		Timestamp:     timestamp,
		ReceivedAt:    receivedAt,
		DeviceTime:    &deviceTime,
		Protocol:      "webhook",
		Source:        "site",
		Schema:        "signup",
		Vendor:        "example",
		SchemaVersion: "1-0-0",
		IsValid:       &valid,
		Payload:       map[string]any{"event": "signup", "nested": map[string]any{"count": float64(2)}},
		Flattened:     map[string]any{"event": "signup", "nested_count": float64(2)},
		IP:            "192.0.2.1",
		UserAgent:     "test-agent",
		Referer:       "https://example.com",
	}

	if err := s.Write(context.Background(), []record.Record{r}); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if appender.calls != 1 || len(appender.rows) != 1 {
		t.Fatalf("append calls=%d rows=%d, want 1 and 1", appender.calls, len(appender.rows))
	}

	message := dynamicpb.NewMessage(s.descriptor)
	if err := proto.Unmarshal(appender.rows[0], message); err != nil {
		t.Fatalf("unmarshal row: %v", err)
	}
	assertProtoString(t, message, "id", "event-1")
	assertProtoString(t, message, "protocol", "webhook")
	assertProtoString(t, message, "source", "site")
	assertProtoString(t, message, "payload", `{"event":"signup","nested":{"count":2}}`)
	assertProtoString(t, message, "flattened", `{"event":"signup","nested_count":2}`)
	assertProtoInt64(t, message, "timestamp", timestamp.UnixMicro())
	assertProtoInt64(t, message, "received_at", receivedAt.UnixMicro())
	assertProtoInt64(t, message, "device_time", deviceTime.UnixMicro())

	field := message.Descriptor().Fields().ByName("is_valid")
	if !message.Has(field) || !message.Get(field).Bool() {
		t.Fatal("expected is_valid=true")
	}
}

func TestBigQuerySinkWriteBatchAndEmpty(t *testing.T) {
	appender := &fakeBigQueryAppender{}
	s := newTestBigQuerySink(&fakeBigQueryAdmin{}, appender)

	if err := s.Write(context.Background(), nil); err != nil {
		t.Fatalf("empty Write before Init error: %v", err)
	}
	if appender.calls != 0 {
		t.Fatalf("empty Write append calls = %d, want 0", appender.calls)
	}

	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	records := []record.Record{
		{ID: "one", Timestamp: time.Now(), Payload: map[string]any{"n": 1}},
		{ID: "two", Timestamp: time.Now(), Payload: map[string]any{"n": 2}},
	}
	if err := s.Write(context.Background(), records); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if appender.calls != 1 || len(appender.rows) != 2 {
		t.Fatalf("append calls=%d rows=%d, want 1 and 2", appender.calls, len(appender.rows))
	}
}

func TestBigQuerySinkWriteErrors(t *testing.T) {
	t.Run("not initialized", func(t *testing.T) {
		s := NewBigQuerySink(BigQueryOptions{})
		err := s.Write(context.Background(), []record.Record{{ID: "event-1"}})
		if err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("Write() error = %v", err)
		}
	})

	t.Run("JSON serialization", func(t *testing.T) {
		s := newTestBigQuerySink(&fakeBigQueryAdmin{}, &fakeBigQueryAppender{})
		if err := s.Init(context.Background()); err != nil {
			t.Fatal(err)
		}
		err := s.Write(context.Background(), []record.Record{
			{ID: "bad-event", Timestamp: time.Now(), Payload: map[string]any{"bad": make(chan int)}},
		})
		if err == nil || !strings.Contains(err.Error(), `serialize record "bad-event"`) || !strings.Contains(err.Error(), "marshal payload") {
			t.Fatalf("Write() error = %v", err)
		}
	})

	t.Run("append", func(t *testing.T) {
		wantErr := errors.New("append rejected")
		appender := &fakeBigQueryAppender{appendErr: wantErr}
		s := newTestBigQuerySink(&fakeBigQueryAdmin{}, appender)
		if err := s.Init(context.Background()); err != nil {
			t.Fatal(err)
		}
		err := s.Write(context.Background(), []record.Record{{ID: "event-1", Timestamp: time.Now()}})
		if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "append 1 records") {
			t.Fatalf("Write() error = %v, want wrapped %v", err, wantErr)
		}
	})
}

func TestBigQuerySinkCloseJoinsErrors(t *testing.T) {
	appendErr := errors.New("stream close")
	adminErr := errors.New("metadata close")
	admin := &fakeBigQueryAdmin{closeErr: adminErr}
	appender := &fakeBigQueryAppender{closeErr: appendErr}
	s := newTestBigQuerySink(admin, appender)
	if err := s.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	err := s.Close()
	if !errors.Is(err, appendErr) || !errors.Is(err, adminErr) {
		t.Fatalf("Close() error = %v, want both close errors", err)
	}
	if !appender.closed || !admin.closed {
		t.Fatal("expected both resources to close")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close() error: %v", err)
	}
}

type fakeAppendResult struct {
	offset int64
	err    error
}

func (f *fakeAppendResult) GetResult(context.Context) (int64, error) {
	return f.offset, f.err
}

func TestManagedBigQueryAppenderCloseTreatsEOFAsClean(t *testing.T) {
	closedClient := false
	a := &managedBigQueryAppender{
		closeStream: func() error { return io.EOF },
		closeClient: func() error {
			closedClient = true
			return nil
		},
	}

	if err := a.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if !closedClient {
		t.Fatal("expected client to close")
	}
}

func TestManagedBigQueryAppenderErrors(t *testing.T) {
	t.Run("immediate", func(t *testing.T) {
		wantErr := errors.New("send failed")
		a := &managedBigQueryAppender{
			appendRows: func(context.Context, [][]byte) (bigQueryAppendResult, error) {
				return nil, wantErr
			},
			closeStream: func() error { return nil },
			closeClient: func() error { return nil },
		}
		err := a.Append(context.Background(), [][]byte{{1}})
		if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "append rows") {
			t.Fatalf("Append() error = %v", err)
		}
	})

	t.Run("result", func(t *testing.T) {
		wantErr := errors.New("row rejected")
		a := &managedBigQueryAppender{
			appendRows: func(context.Context, [][]byte) (bigQueryAppendResult, error) {
				return &fakeAppendResult{err: wantErr}, nil
			},
			closeStream: func() error { return nil },
			closeClient: func() error { return nil },
		}
		err := a.Append(context.Background(), [][]byte{{1}})
		if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "append result") {
			t.Fatalf("Append() error = %v", err)
		}
	})
}

func assertProtoString(t *testing.T, message *dynamicpb.Message, name protoreflect.Name, want string) {
	t.Helper()
	field := message.Descriptor().Fields().ByName(name)
	if field == nil || !message.Has(field) {
		t.Fatalf("missing field %s", name)
	}
	if got := message.Get(field).String(); got != want {
		t.Fatalf("field %s = %q, want %q", name, got, want)
	}
}

func assertProtoInt64(t *testing.T, message *dynamicpb.Message, name protoreflect.Name, want int64) {
	t.Helper()
	field := message.Descriptor().Fields().ByName(name)
	if field == nil || !message.Has(field) {
		t.Fatalf("missing field %s", name)
	}
	if got := message.Get(field).Int(); got != want {
		t.Fatalf("field %s = %d, want %d", name, got, want)
	}
}
