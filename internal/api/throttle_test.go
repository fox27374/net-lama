package api

import (
	"testing"
	"time"
)

func TestThrottle(t *testing.T) {
	th := newThrottle()

	for i := 0; i < maxAuthFailures-1; i++ {
		if !th.allow("alice|10.0.0.1") {
			t.Fatalf("blocked after %d failures, budget is %d", i, maxAuthFailures)
		}
		if th.fail("alice|10.0.0.1") {
			t.Fatalf("failure %d reported as the one that tripped the limit", i+1)
		}
	}
	if !th.fail("alice|10.0.0.1") {
		t.Fatal("the last failure should report that the limit tripped")
	}
	if th.allow("alice|10.0.0.1") {
		t.Fatal("still allowed after using up the budget")
	}

	// Same user from another address, and another user from the same one,
	// keep their own budgets — one noisy IP must not lock anybody out.
	if !th.allow("alice|10.0.0.2") || !th.allow("bob|10.0.0.1") {
		t.Fatal("the block leaked to a different key")
	}

	// A success clears the budget.
	th.success("alice|10.0.0.1")
	if !th.allow("alice|10.0.0.1") {
		t.Fatal("still blocked after a successful authentication")
	}

	// An expired window is pruned rather than remembered forever.
	th.fail("carol|10.0.0.3")
	th.mu.Lock()
	th.entries["carol|10.0.0.3"].first = time.Now().Add(-2 * authWindow)
	th.mu.Unlock()
	th.allow("anyone|10.0.0.9")
	th.mu.Lock()
	_, kept := th.entries["carol|10.0.0.3"]
	th.mu.Unlock()
	if kept {
		t.Fatal("an entry outside the window survived a prune")
	}
}
