package sink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/deepskydatahq/straumheim/internal/record"
)

// BigQueryOptions identifies the destination table for a BigQuery sink.
type BigQueryOptions struct {
	Project             string
	Dataset             string
	Table               string
	Location            string
	MaxInflightRequests int
}

// BigQuerySink writes record batches to a BigQuery Storage Write API default stream.
type BigQuerySink struct {
	options BigQueryOptions

	adminFactory    bigQueryTableAdminFactory
	appenderFactory bigQueryAppenderFactory
	admin           bigQueryTableAdmin
	appender        bigQueryAppender
	descriptor      protoreflect.MessageDescriptor
}

var _ Sink = (*BigQuerySink)(nil)

type bigQueryTableAdmin interface {
	EnsureTable(ctx context.Context, dataset, table, location string, metadata *bigquery.TableMetadata) error
	Close() error
}

type bigQueryAppender interface {
	Append(ctx context.Context, rows [][]byte) error
	Close() error
}

type bigQueryTableAdminFactory func(ctx context.Context, project string) (bigQueryTableAdmin, error)

type bigQueryAppenderFactory func(
	ctx context.Context,
	options BigQueryOptions,
	schema *descriptorpb.DescriptorProto,
) (bigQueryAppender, error)

// NewBigQuerySink creates a batch sink for a BigQuery table.
func NewBigQuerySink(options BigQueryOptions) *BigQuerySink {
	return newBigQuerySinkWithDependencies(options, newBigQueryTableAdmin, newBigQueryAppender)
}

func newBigQuerySinkWithDependencies(
	options BigQueryOptions,
	adminFactory bigQueryTableAdminFactory,
	appenderFactory bigQueryAppenderFactory,
) *BigQuerySink {
	return &BigQuerySink{
		options:         options,
		adminFactory:    adminFactory,
		appenderFactory: appenderFactory,
	}
}

// Init validates the destination, creates the table when absent, and opens the default stream.
func (s *BigQuerySink) Init(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}
	if s.admin != nil || s.appender != nil {
		return errors.New("bigquery sink: already initialized")
	}

	descriptor, normalized, err := bigQueryRowDescriptor()
	if err != nil {
		return fmt.Errorf("bigquery sink: build row descriptor: %w", err)
	}

	admin, err := s.adminFactory(ctx, s.options.Project)
	if err != nil {
		return fmt.Errorf("bigquery sink: create metadata client: %w", err)
	}

	metadata := bigQueryTableMetadata()
	if err := admin.EnsureTable(ctx, s.options.Dataset, s.options.Table, s.options.Location, metadata); err != nil {
		return errors.Join(
			fmt.Errorf("bigquery sink: ensure table %s.%s: %w", s.options.Dataset, s.options.Table, err),
			admin.Close(),
		)
	}

	appender, err := s.appenderFactory(ctx, s.options, normalized)
	if err != nil {
		return errors.Join(
			fmt.Errorf("bigquery sink: open default stream: %w", err),
			admin.Close(),
		)
	}

	s.admin = admin
	s.appender = appender
	s.descriptor = descriptor
	return nil
}

// Write serializes records and appends the entire batch in one Storage Write request.
func (s *BigQuerySink) Write(ctx context.Context, records []record.Record) error {
	if len(records) == 0 {
		return nil
	}
	if s.appender == nil || s.descriptor == nil {
		return errors.New("bigquery sink: not initialized")
	}

	rows := make([][]byte, len(records))
	for i := range records {
		row, err := marshalBigQueryRecord(records[i], s.descriptor)
		if err != nil {
			return fmt.Errorf("bigquery sink: serialize record %q: %w", records[i].ID, err)
		}
		rows[i] = row
	}

	if err := s.appender.Append(ctx, rows); err != nil {
		return fmt.Errorf("bigquery sink: append %d records: %w", len(records), err)
	}
	return nil
}

// Flush is a no-op because Write waits for the append result.
func (s *BigQuerySink) Flush(_ context.Context) error { return nil }

// Close closes the managed stream and metadata clients.
func (s *BigQuerySink) Close() error {
	var errs []error
	if s.appender != nil {
		errs = append(errs, s.appender.Close())
		s.appender = nil
	}
	if s.admin != nil {
		errs = append(errs, s.admin.Close())
		s.admin = nil
	}
	s.descriptor = nil
	return errors.Join(errs...)
}

// Mode returns SinkModeBatch.
func (s *BigQuerySink) Mode() SinkMode { return SinkModeBatch }

func (s *BigQuerySink) validate() error {
	fields := []struct {
		name  string
		value string
	}{
		{name: "project", value: s.options.Project},
		{name: "dataset", value: s.options.Dataset},
		{name: "table", value: s.options.Table},
		{name: "location", value: s.options.Location},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("bigquery sink: %s is required", field.name)
		}
	}
	return nil
}

func bigQuerySchema() bigquery.Schema {
	return bigquery.Schema{
		{Name: "id", Type: bigquery.StringFieldType, Required: true},
		{Name: "timestamp", Type: bigquery.TimestampFieldType, Required: true},
		{Name: "received_at", Type: bigquery.TimestampFieldType},
		{Name: "device_time", Type: bigquery.TimestampFieldType},
		{Name: "protocol", Type: bigquery.StringFieldType},
		{Name: "source", Type: bigquery.StringFieldType},
		{Name: "schema", Type: bigquery.StringFieldType},
		{Name: "vendor", Type: bigquery.StringFieldType},
		{Name: "schema_version", Type: bigquery.StringFieldType},
		{Name: "is_valid", Type: bigquery.BooleanFieldType},
		{Name: "ip", Type: bigquery.StringFieldType},
		{Name: "user_agent", Type: bigquery.StringFieldType},
		{Name: "referer", Type: bigquery.StringFieldType},
		{Name: "payload", Type: bigquery.JSONFieldType},
		{Name: "flattened", Type: bigquery.JSONFieldType},
	}
}

func bigQueryTableMetadata() *bigquery.TableMetadata {
	return &bigquery.TableMetadata{
		Schema: bigQuerySchema(),
		TimePartitioning: &bigquery.TimePartitioning{
			Type:  bigquery.DayPartitioningType,
			Field: "timestamp",
		},
		Clustering: &bigquery.Clustering{Fields: []string{"protocol", "source"}},
	}
}

func bigQueryRowDescriptor() (
	protoreflect.MessageDescriptor,
	*descriptorpb.DescriptorProto,
	error,
) {
	storageSchema, err := adapt.BQSchemaToStorageTableSchema(bigQuerySchema())
	if err != nil {
		return nil, nil, fmt.Errorf("convert table schema: %w", err)
	}
	descriptor, err := adapt.StorageSchemaToProto2Descriptor(storageSchema, "StraumheimEvent")
	if err != nil {
		return nil, nil, fmt.Errorf("convert storage schema: %w", err)
	}
	messageDescriptor, ok := descriptor.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, nil, fmt.Errorf("storage schema produced %T, not a message descriptor", descriptor)
	}
	normalized, err := adapt.NormalizeDescriptor(messageDescriptor)
	if err != nil {
		return nil, nil, fmt.Errorf("normalize descriptor: %w", err)
	}
	return messageDescriptor, normalized, nil
}

func marshalBigQueryRecord(r record.Record, descriptor protoreflect.MessageDescriptor) ([]byte, error) {
	message := dynamicpb.NewMessage(descriptor)
	fields := descriptor.Fields()

	setString := func(name, value string) {
		message.Set(fields.ByName(protoreflect.Name(name)), protoreflect.ValueOfString(value))
	}
	setTimestamp := func(name string, value int64) {
		message.Set(fields.ByName(protoreflect.Name(name)), protoreflect.ValueOfInt64(value))
	}

	setString("id", r.ID)
	setTimestamp("timestamp", r.Timestamp.UnixMicro())
	if !r.ReceivedAt.IsZero() {
		setTimestamp("received_at", r.ReceivedAt.UnixMicro())
	}
	if r.DeviceTime != nil {
		setTimestamp("device_time", r.DeviceTime.UnixMicro())
	}
	setString("protocol", r.Protocol)
	setString("source", r.Source)
	setString("schema", r.Schema)
	setString("vendor", r.Vendor)
	setString("schema_version", r.SchemaVersion)
	if r.IsValid != nil {
		message.Set(fields.ByName("is_valid"), protoreflect.ValueOfBool(*r.IsValid))
	}
	setString("ip", r.IP)
	setString("user_agent", r.UserAgent)
	setString("referer", r.Referer)

	if r.Payload != nil {
		payload, err := json.Marshal(r.Payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		setString("payload", string(payload))
	}

	r.EnsureFlattened()
	if r.Flattened != nil {
		flattened, err := json.Marshal(r.Flattened)
		if err != nil {
			return nil, fmt.Errorf("marshal flattened payload: %w", err)
		}
		setString("flattened", string(flattened))
	}

	row, err := proto.MarshalOptions{Deterministic: true}.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("marshal protobuf: %w", err)
	}
	return row, nil
}

type googleBigQueryTableAdmin struct {
	client *bigquery.Client
}

func newBigQueryTableAdmin(ctx context.Context, project string) (bigQueryTableAdmin, error) {
	client, err := bigquery.NewClient(ctx, project)
	if err != nil {
		return nil, err
	}
	return &googleBigQueryTableAdmin{client: client}, nil
}

func (a *googleBigQueryTableAdmin) EnsureTable(
	ctx context.Context,
	datasetID, tableID, location string,
	metadata *bigquery.TableMetadata,
) error {
	dataset := a.client.Dataset(datasetID)
	datasetMetadata, err := dataset.Metadata(ctx)
	if err != nil {
		return fmt.Errorf("read dataset metadata: %w", err)
	}
	if !strings.EqualFold(datasetMetadata.Location, location) {
		return fmt.Errorf(
			"dataset location %q does not match configured location %q",
			datasetMetadata.Location,
			location,
		)
	}

	table := dataset.Table(tableID)
	tableMetadata, err := table.Metadata(ctx)
	if err == nil {
		return validateBigQueryTableMetadata(tableMetadata, metadata)
	}
	if !isGoogleAPIStatus(err, 404) {
		return fmt.Errorf("read table metadata: %w", err)
	}
	if err := table.Create(ctx, metadata); err == nil {
		return nil
	} else if !isGoogleAPIStatus(err, 409) {
		return fmt.Errorf("create table: %w", err)
	}

	// Another collector instance can create the table between Metadata and Create.
	tableMetadata, err = table.Metadata(ctx)
	if err != nil {
		return fmt.Errorf("read concurrently created table metadata: %w", err)
	}
	return validateBigQueryTableMetadata(tableMetadata, metadata)
}

func (a *googleBigQueryTableAdmin) Close() error { return a.client.Close() }

func validateBigQueryTableMetadata(actual, expected *bigquery.TableMetadata) error {
	actualFields := make(map[string]*bigquery.FieldSchema, len(actual.Schema))
	for _, field := range actual.Schema {
		actualFields[field.Name] = field
	}
	expectedFields := make(map[string]struct{}, len(expected.Schema))
	for _, expectedField := range expected.Schema {
		expectedFields[expectedField.Name] = struct{}{}
	}
	for _, actualField := range actual.Schema {
		if _, ok := expectedFields[actualField.Name]; !ok && actualField.Required {
			return fmt.Errorf("table has unsupported required field %q", actualField.Name)
		}
	}
	for _, expectedField := range expected.Schema {
		actualField, ok := actualFields[expectedField.Name]
		if !ok {
			return fmt.Errorf("table schema is missing field %q", expectedField.Name)
		}
		if actualField.Type != expectedField.Type || actualField.Required != expectedField.Required {
			return fmt.Errorf(
				"table field %q is %s required=%t, want %s required=%t",
				expectedField.Name,
				actualField.Type,
				actualField.Required,
				expectedField.Type,
				expectedField.Required,
			)
		}
	}
	if actual.TimePartitioning == nil ||
		actual.TimePartitioning.Field != expected.TimePartitioning.Field ||
		actual.TimePartitioning.Type != expected.TimePartitioning.Type {
		return fmt.Errorf("table partitioning does not match timestamp DAY contract")
	}
	if actual.Clustering == nil || !equalStrings(actual.Clustering.Fields, expected.Clustering.Fields) {
		return fmt.Errorf("table clustering is %v, want %v", clusteringFields(actual), expected.Clustering.Fields)
	}
	return nil
}

func clusteringFields(metadata *bigquery.TableMetadata) []string {
	if metadata.Clustering == nil {
		return nil
	}
	return metadata.Clustering.Fields
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isGoogleAPIStatus(err error, code int) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == code
}

type bigQueryAppendResult interface {
	GetResult(ctx context.Context) (int64, error)
}

type managedBigQueryAppender struct {
	appendRows  func(ctx context.Context, rows [][]byte) (bigQueryAppendResult, error)
	closeStream func() error
	closeClient func() error
}

func newBigQueryAppender(
	ctx context.Context,
	options BigQueryOptions,
	schema *descriptorpb.DescriptorProto,
) (bigQueryAppender, error) {
	client, err := managedwriter.NewClient(ctx, options.Project, managedwriter.WithMultiplexing())
	if err != nil {
		return nil, err
	}

	writerOptions := []managedwriter.WriterOption{
		managedwriter.WithType(managedwriter.DefaultStream),
		managedwriter.WithDestinationTable(managedwriter.TableParentFromParts(
			options.Project,
			options.Dataset,
			options.Table,
		)),
		managedwriter.WithSchemaDescriptor(schema),
	}
	if options.MaxInflightRequests > 0 {
		writerOptions = append(writerOptions, managedwriter.WithMaxInflightRequests(options.MaxInflightRequests))
	}
	stream, err := client.NewManagedStream(ctx, writerOptions...)
	if err != nil {
		return nil, errors.Join(err, client.Close())
	}
	return &managedBigQueryAppender{
		appendRows: func(ctx context.Context, rows [][]byte) (bigQueryAppendResult, error) {
			return stream.AppendRows(ctx, rows)
		},
		closeStream: stream.Close,
		closeClient: client.Close,
	}, nil
}

func (a *managedBigQueryAppender) Append(ctx context.Context, rows [][]byte) error {
	result, err := a.appendRows(ctx, rows)
	if err != nil {
		return fmt.Errorf("append rows: %w", err)
	}
	if _, err := result.GetResult(ctx); err != nil {
		return fmt.Errorf("wait for append result: %w", err)
	}
	return nil
}

func (a *managedBigQueryAppender) Close() error {
	return errors.Join(a.closeStream(), a.closeClient())
}
