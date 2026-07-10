package server

import (
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/fox27374/net-lama/internal/logtee"
	"github.com/fox27374/net-lama/internal/store"
)

// serverLogBuffer is the channel capacity between the logging goroutine
// (any goroutine calling logger.Info/Warn/Error) and the single writer
// goroutine that persists entries to the store.
const serverLogBuffer = 500

// NewLogTee wraps next with a handler that also stores every Info+ record
// in st, under source "server". It never blocks the caller: entries are
// handed to a buffered channel drained by one background goroutine, and
// dropped (silently, past a full buffer) rather than applying
// backpressure to the hot path. Failures while storing are written
// directly to stderr — never through the teeing logger itself, which
// would recurse back into this same sink.
func NewLogTee(next slog.Handler, st *store.Store) slog.Handler {
	ch := make(chan store.LogEntry, serverLogBuffer)
	var drops atomic.Int64

	go func() {
		for e := range ch {
			if err := st.InsertLog(&e); err != nil {
				fmt.Fprintf(os.Stderr, "server log sink: storing log entry failed: %v\n", err)
			}
		}
	}()

	return logtee.New(next, func(e logtee.Entry) {
		entry := store.LogEntry{Time: e.Time, Source: "server", Level: e.Level, Message: e.Message}
		select {
		case ch <- entry:
		default:
			drops.Add(1)
		}
	})
}
