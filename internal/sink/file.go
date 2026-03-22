package sink

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/deepsky-data/straumheim/internal/record"
)

// FileSink writes records as JSONL (one JSON object per line) to local
// filesystem files, rotating by time interval.
type FileSink struct {
	dir              string
	rotationInterval time.Duration

	// now returns the current time. Overridable for testing.
	now func() time.Time

	mu           sync.Mutex
	currentFile  *os.File
	fileOpenedAt time.Time
}

// NewFileSink creates a new FileSink that writes JSONL files to dir,
// rotating after rotationInterval.
func NewFileSink(dir string, rotationInterval time.Duration) *FileSink {
	return &FileSink{
		dir:              dir,
		rotationInterval: rotationInterval,
		now:              time.Now,
	}
}

func (f *FileSink) Init(_ context.Context) error {
	return os.MkdirAll(f.dir, 0o755)
}

func (f *FileSink) Write(_ context.Context, records []record.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.ensureFile(); err != nil {
		return fmt.Errorf("opening file: %w", err)
	}

	for _, r := range records {
		data, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("marshaling record: %w", err)
		}
		data = append(data, '\n')
		if _, err := f.currentFile.Write(data); err != nil {
			return fmt.Errorf("writing record: %w", err)
		}
	}
	return nil
}

func (f *FileSink) Flush(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.currentFile != nil {
		return f.currentFile.Sync()
	}
	return nil
}

func (f *FileSink) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.currentFile != nil {
		err := f.currentFile.Close()
		f.currentFile = nil
		return err
	}
	return nil
}

func (f *FileSink) Mode() SinkMode { return SinkModeBatch }

// ensureFile opens a new file if none is open or if the rotation interval has
// elapsed. Must be called with f.mu held.
func (f *FileSink) ensureFile() error {
	now := f.now()
	if f.currentFile != nil && now.Sub(f.fileOpenedAt) < f.rotationInterval {
		return nil
	}

	// Close the current file before rotating.
	if f.currentFile != nil {
		if err := f.currentFile.Close(); err != nil {
			return fmt.Errorf("closing rotated file: %w", err)
		}
		f.currentFile = nil
	}

	name := fmt.Sprintf("events-%s.jsonl", now.UTC().Format("20060102T150405Z"))
	path := filepath.Join(f.dir, name)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}

	f.currentFile = file
	f.fileOpenedAt = now
	return nil
}
