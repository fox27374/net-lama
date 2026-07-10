package agent

import (
	"testing"
	"time"

	"github.com/fox27374/net-lama/internal/logtee"
)

func TestLogRingBufferDropsOldest(t *testing.T) {
	b := newLogRingBuffer(3)

	for i := 0; i < 5; i++ {
		b.Push(logtee.Entry{Time: time.Now(), Level: "INFO", Message: string(rune('a' + i))})
	}

	got := b.Drain()
	if len(got) != 3 {
		t.Fatalf("expected 3 entries after dropping the oldest 2, got %d", len(got))
	}
	// The oldest two ("a", "b") should have been dropped; "c", "d", "e" remain.
	want := []string{"c", "d", "e"}
	for i, w := range want {
		if got[i].Message != w {
			t.Errorf("entry %d = %q, want %q", i, got[i].Message, w)
		}
	}
}

func TestLogRingBufferDrainEmpties(t *testing.T) {
	b := newLogRingBuffer(10)
	b.Push(logtee.Entry{Message: "only"})

	first := b.Drain()
	if len(first) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(first))
	}

	second := b.Drain()
	if second != nil {
		t.Errorf("expected nil after draining an empty buffer, got %v", second)
	}
}
