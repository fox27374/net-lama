package agent

import (
	"sync"

	"github.com/fox27374/net-lama/internal/logtee"
)

// logRingBuffer holds recent Info+ log lines while the control stream is
// disconnected (or between drains), dropping the oldest entry once full
// so pushing never blocks the logger. Drain removes and returns
// everything currently buffered, for shipping once the stream is
// connected.
type logRingBuffer struct {
	mu       sync.Mutex
	entries  []logtee.Entry
	capacity int
}

func newLogRingBuffer(capacity int) *logRingBuffer {
	return &logRingBuffer{capacity: capacity}
}

// Push appends an entry, dropping the oldest one if the buffer is full.
func (b *logRingBuffer) Push(e logtee.Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) >= b.capacity {
		b.entries = b.entries[1:]
	}
	b.entries = append(b.entries, e)
}

// Drain removes and returns everything currently buffered, oldest first.
// Returns nil if the buffer is empty.
func (b *logRingBuffer) Drain() []logtee.Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) == 0 {
		return nil
	}
	out := b.entries
	b.entries = nil
	return out
}
