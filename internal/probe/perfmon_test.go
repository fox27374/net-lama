package probe

import (
	"context"
	"testing"
	"time"
)

func TestPerfmonLoopback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	allowed, err := ParseCIDRs([]string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("ParseCIDRs: %v", err)
	}
	ln, err := Reflector(ctx, "127.0.0.1:0", allowed)
	if err != nil {
		t.Fatalf("starting reflector: %v", err)
	}
	defer ln.Close()

	res, err := RunClient(ctx, ln.Addr().String(), 1)
	if err != nil {
		t.Fatalf("RunClient returned an error (should report failures via the result): %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got failedStep=%q", res.FailedStep)
	}
	if res.LatencyMs <= 0 {
		t.Errorf("expected positive latency, got %v", res.LatencyMs)
	}
	if res.UploadMbps <= 0 {
		t.Errorf("expected positive upload throughput, got %v", res.UploadMbps)
	}
	if res.DownloadMbps <= 0 {
		t.Errorf("expected positive download throughput, got %v", res.DownloadMbps)
	}
	if res.DurationSeconds != 1 {
		t.Errorf("DurationSeconds = %d, want 1", res.DurationSeconds)
	}
}

func TestPerfmonConnectFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Nothing listening on this port — connect should fail cleanly.
	res, err := RunClient(ctx, "127.0.0.1:1", 1)
	if err != nil {
		t.Fatalf("RunClient returned an error (should report failures via the result): %v", err)
	}
	if res.Success {
		t.Fatal("expected failure connecting to a closed port")
	}
	if res.FailedStep != "connect" {
		t.Errorf("FailedStep = %q, want connect", res.FailedStep)
	}
}

func TestPerfmonReflectorACL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Empty allowlist: reject everyone, even loopback.
	ln, err := Reflector(ctx, "127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("starting reflector: %v", err)
	}
	defer ln.Close()
	res, err := RunClient(ctx, ln.Addr().String(), 1)
	if err != nil {
		t.Fatalf("RunClient returned an error (should report failures via the result): %v", err)
	}
	if res.Success {
		t.Fatal("expected the reflector to reject a connection with an empty allowlist")
	}

	// Bare IP entries get an implicit /32 and admit the matching peer.
	allowed, err := ParseCIDRs([]string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("ParseCIDRs: %v", err)
	}
	ln2, err := Reflector(ctx, "127.0.0.1:0", allowed)
	if err != nil {
		t.Fatalf("starting reflector: %v", err)
	}
	defer ln2.Close()
	res, err = RunClient(ctx, ln2.Addr().String(), 1)
	if err != nil {
		t.Fatalf("RunClient returned an error (should report failures via the result): %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success with a matching allowlist entry, got failedStep=%q", res.FailedStep)
	}
}

func TestPerfmonDuration(t *testing.T) {
	if got := perfmonDuration(0); got != 5 {
		t.Errorf("perfmonDuration(0) = %d, want 5 (default)", got)
	}
	if got := perfmonDuration(20); got != 20 {
		t.Errorf("perfmonDuration(20) = %d, want 20 (unchanged)", got)
	}
}
