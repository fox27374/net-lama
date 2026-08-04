package probe

import (
	"context"
	"testing"
)

// TestDNSQueryAbandonedRun pins the one thing DNSQuery reports as an error.
// A lookup that fails is a measurement (Success=false); a run that was
// abandoned is not, and must not be sent as a result claiming the server
// took 0 ms to not answer.
func TestDNSQueryAbandonedRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := DNSQuery(ctx, "example.com", "1.1.1.1")
	if err == nil {
		t.Fatalf("expected an error for a cancelled run, got result %+v", res)
	}
	if res != nil {
		t.Errorf("expected no result for a cancelled run, got %+v", res)
	}
}
