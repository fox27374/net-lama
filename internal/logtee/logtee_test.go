package logtee

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestLevelFiltering verifies only Info-and-above records reach the sink,
// while every record (regardless of level) still reaches the wrapped
// handler.
func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	next := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	var got []Entry
	h := New(next, func(e Entry) { got = append(got, e) })
	logger := slog.New(h)

	logger.Debug("debug line")
	logger.Info("info line")
	logger.Warn("warn line")
	logger.Error("error line")

	if len(got) != 3 {
		t.Fatalf("expected 3 sink entries (info/warn/error), got %d: %+v", len(got), got)
	}
	for i, want := range []string{"info line", "warn line", "error line"} {
		if got[i].Message != want {
			t.Errorf("entry %d: message = %q, want %q", i, got[i].Message, want)
		}
	}

	// All four lines, including debug, must still reach the wrapped handler.
	out := buf.String()
	for _, want := range []string{"debug line", "info line", "warn line", "error line"} {
		if !strings.Contains(out, want) {
			t.Errorf("wrapped handler output missing %q:\n%s", want, out)
		}
	}
}

// TestBoundAttrsInMessage verifies attributes bound via Logger.With(...)
// are folded into the sunk message, matching the compact (time, level,
// message) log schema.
func TestBoundAttrsInMessage(t *testing.T) {
	next := slog.NewTextHandler(&bytes.Buffer{}, nil)

	var got []Entry
	h := New(next, func(e Entry) { got = append(got, e) })
	logger := slog.New(h).With(slog.String("agent", "pi-1"))

	logger.Info("connected", slog.Int("tests", 3))

	if len(got) != 1 {
		t.Fatalf("expected 1 sink entry, got %d", len(got))
	}
	msg := got[0].Message
	if !strings.Contains(msg, "connected") || !strings.Contains(msg, "agent=pi-1") || !strings.Contains(msg, "tests=3") {
		t.Errorf("message = %q, want it to contain the base message plus bound and call attrs", msg)
	}
}

// TestSinkNeverBlocksCaller verifies Handle does not itself block: any
// backpressure handling is the Sink's responsibility (e.g. a non-blocking
// channel send), and a Sink that never returns must not hang the logger
// forever. This exercises that Handle simply calls Sink synchronously and
// returns once Sink returns, with no internal buffering of its own that
// could mask a slow Sink implementation done wrong elsewhere.
func TestSinkCalledSynchronouslyOncePerRecord(t *testing.T) {
	next := slog.NewTextHandler(&bytes.Buffer{}, nil)

	calls := 0
	h := New(next, func(e Entry) { calls++ })
	logger := slog.New(h)

	for i := 0; i < 5; i++ {
		logger.Info("line")
	}
	if calls != 5 {
		t.Fatalf("sink called %d times, want 5", calls)
	}
}

// TestNonBlockingSinkDropsUnderLoad simulates the real usage pattern (a
// bounded channel with a select/default in the Sink) and checks that
// pushing past capacity drops entries instead of blocking Handle.
func TestNonBlockingSinkDropsUnderLoad(t *testing.T) {
	next := slog.NewTextHandler(&bytes.Buffer{}, nil)

	ch := make(chan Entry, 2)
	drops := 0
	h := New(next, func(e Entry) {
		select {
		case ch <- e:
		default:
			drops++
		}
	})
	logger := slog.New(h)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			logger.Info("line")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Handle blocked instead of dropping entries when the sink's buffer was full")
	}
	if drops == 0 {
		t.Errorf("expected some drops once the 2-entry channel filled up, got 0")
	}
}
