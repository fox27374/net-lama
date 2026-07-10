// Package logtee provides an slog.Handler wrapper that tees Info-and-above
// records to a caller-provided sink, in addition to passing every record
// through to the wrapped handler unchanged. It is shared by the server
// (sink writes to the log store) and the agent (sink pushes into a ring
// buffer that is drained onto the control stream).
package logtee

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Entry is one captured Info+ log line.
type Entry struct {
	Time    time.Time
	Level   string
	Message string
}

// Sink receives one Entry per qualifying record. It is called
// synchronously from Handle and therefore MUST NOT block or panic — do
// any buffering/backpressure handling (e.g. a non-blocking channel send
// or a ring buffer) inside the Sink itself.
type Sink func(Entry)

// Handler wraps an slog.Handler and calls Sink for every record at
// slog.LevelInfo or above, then always forwards the record to Next
// unchanged. Attributes bound via Logger.With(...) are folded into
// Entry.Message (as "key=value" pairs) since the log store only tracks a
// single message string per line, not structured attributes.
type Handler struct {
	next  slog.Handler
	sink  Sink
	attrs []slog.Attr
}

// New wraps next; sink is invoked for records at LevelInfo or above.
func New(next slog.Handler, sink Sink) *Handler {
	return &Handler{next: next, sink: sink}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if h.sink != nil && r.Level >= slog.LevelInfo {
		h.sink(Entry{
			Time:    r.Time,
			Level:   r.Level.String(),
			Message: formatMessage(h.attrs, r),
		})
	}
	return h.next.Handle(ctx, r)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &Handler{next: h.next.WithAttrs(attrs), sink: h.sink, attrs: merged}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name), sink: h.sink, attrs: h.attrs}
}

// formatMessage renders the record message plus any bound and per-call
// attributes as a single "msg key=value key=value" line.
func formatMessage(bound []slog.Attr, r slog.Record) string {
	var b strings.Builder
	b.WriteString(r.Message)
	for _, a := range bound {
		fmt.Fprintf(&b, " %s=%s", a.Key, a.Value.String())
	}
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%s", a.Key, a.Value.String())
		return true
	})
	return b.String()
}
