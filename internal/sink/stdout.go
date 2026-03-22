package sink

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/deepsky-data/straumheim/internal/record"
)

// StdoutSink writes each record as a JSON line to an io.Writer.
type StdoutSink struct {
	w io.Writer
}

// NewStdoutSink creates a new StdoutSink that writes to the given writer.
// If w is nil, os.Stdout is used.
func NewStdoutSink(w io.Writer) *StdoutSink {
	if w == nil {
		w = os.Stdout
	}
	return &StdoutSink{w: w}
}

func (s *StdoutSink) Init(_ context.Context) error  { return nil }
func (s *StdoutSink) Flush(_ context.Context) error  { return nil }
func (s *StdoutSink) Close() error                   { return nil }
func (s *StdoutSink) Mode() SinkMode                 { return SinkModeStream }

// Write outputs each record as a single JSON line.
func (s *StdoutSink) Write(_ context.Context, records []record.Record) error {
	for _, r := range records {
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if _, err := s.w.Write(data); err != nil {
			return err
		}
	}
	return nil
}
