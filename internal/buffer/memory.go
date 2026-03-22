package buffer

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/deepsky-data/straumheim/internal/record"
)

// ErrBufferFull is returned when Push is called on a buffer at capacity.
var ErrBufferFull = errors.New("buffer is full")

// MemoryBuffer is an in-memory Buffer backed by a Go channel.
type MemoryBuffer struct {
	ch            chan record.Record
	flushCount    int
	flushInterval time.Duration

	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

// NewMemoryBuffer creates a new in-memory buffer with the given capacity,
// flush-by-count threshold, and flush-by-interval duration.
func NewMemoryBuffer(capacity, flushCount int, flushInterval time.Duration) *MemoryBuffer {
	return &MemoryBuffer{
		ch:            make(chan record.Record, capacity),
		flushCount:    flushCount,
		flushInterval: flushInterval,
		done:          make(chan struct{}),
	}
}

// Push adds records to the buffer. Returns ErrBufferFull if the channel is at capacity.
func (m *MemoryBuffer) Push(_ context.Context, records []record.Record) error {
	for _, r := range records {
		select {
		case m.ch <- r:
		default:
			return ErrBufferFull
		}
	}
	return nil
}

// Consume starts a goroutine that reads records from the channel and calls
// handler when flushCount records have been collected or flushInterval elapses.
// Cancelling ctx triggers a final drain of any remaining records.
func (m *MemoryBuffer) Consume(ctx context.Context, handler HandlerFunc) {
	ctx, m.cancel = context.WithCancel(ctx)

	go func() {
		defer close(m.done)

		ticker := time.NewTicker(m.flushInterval)
		defer ticker.Stop()

		batch := make([]record.Record, 0, m.flushCount)

		flush := func() {
			if len(batch) == 0 {
				return
			}
			handler(ctx, batch)
			batch = make([]record.Record, 0, m.flushCount)
		}

		for {
			select {
			case rec, ok := <-m.ch:
				if !ok {
					flush()
					return
				}
				batch = append(batch, rec)
				if len(batch) >= m.flushCount {
					flush()
				}
			case <-ticker.C:
				flush()
			case <-ctx.Done():
				// Drain remaining records from the channel.
				for {
					select {
					case rec, ok := <-m.ch:
						if !ok {
							flush()
							return
						}
						batch = append(batch, rec)
					default:
						flush()
						return
					}
				}
			}
		}
	}()
}

// Close flushes any remaining buffered records then returns.
func (m *MemoryBuffer) Close() error {
	m.once.Do(func() {
		if m.cancel != nil {
			m.cancel()
		}
	})
	<-m.done
	return nil
}
