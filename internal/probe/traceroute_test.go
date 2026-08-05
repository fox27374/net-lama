package probe

import (
	"math"
	"testing"
)

// The mtr JSON parser these tests used to cover is gone: the native engine
// builds hops from its own probes, so what needs pinning now is how probe
// samples become a hop and how the destination's answer is classified.

func TestRttStats(t *testing.T) {
	avg, best, worst, jitter := rttStats([]float64{10, 12, 14})
	if avg != 12 || best != 10 || worst != 14 {
		t.Errorf("avg/best/worst = %v/%v/%v, want 12/10/14", avg, best, worst)
	}
	// Standard deviation, which is what mtr reported as StDev — history
	// spanning the engine change has to stay comparable.
	if want := math.Sqrt(8.0 / 3.0); math.Abs(jitter-want) > 1e-9 {
		t.Errorf("jitter = %v, want %v (population stddev)", jitter, want)
	}

	if avg, best, worst, jitter := rttStats(nil); avg != 0 || best != 0 || worst != 0 || jitter != 0 {
		t.Errorf("no samples gave %v/%v/%v/%v, want zeros", avg, best, worst, jitter)
	}
}

// TestClassifyICMP pins the distinction the native engine exists to make:
// an intermediate router versus the destination answering for itself, and
// which answer it gave.
func TestClassifyICMP(t *testing.T) {
	cases := []struct {
		name      string
		icmpType  uint8
		icmpCode  uint8
		wantKind  replyKind
		wantState string
	}{
		{"time exceeded is a hop", 11, 0, replyTimeExceeded, ""},
		{"port unreachable is the target", 3, 3, replyDestination, DestEchoed},
		{"admin prohibited is filtering", 3, 13, replyDestination, DestFiltered},
		{"host unreachable", 3, 1, replyDestination, DestUnreachable},
		{"echo reply is the target", 0, 0, replyDestination, DestEchoed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kind, state := classifyICMP(c.icmpType, c.icmpCode)
			if kind != c.wantKind || state != c.wantState {
				t.Errorf("classifyICMP(%d,%d) = (%v,%q), want (%v,%q)",
					c.icmpType, c.icmpCode, kind, state, c.wantKind, c.wantState)
			}
		})
	}
}

// TestFlowPortIsStablePerFlow covers the property the whole ECMP story
// rests on: the same test always probes from the same source port, so every
// run follows the same branch, while different tests can differ.
func TestFlowPortIsStablePerFlow(t *testing.T) {
	if flowPort(1234) != flowPort(1234) {
		t.Error("same flow id gave different ports")
	}
	if flowPort(1234) == flowPort(4321) {
		t.Error("different flow ids collided; two tests would share a branch")
	}
	for _, id := range []uint16{0, 1, 65535, 2000, 1999} {
		if p := flowPort(id); p < 33000 || p > 34999 {
			t.Errorf("flowPort(%d) = %d, outside the intended range", id, p)
		}
	}
}

func TestTracerouteDemoModeFillsNewFields(t *testing.T) {
	t.Setenv("NETLAMA_TRACEROUTE_DEMO", "1")
	res := demoTraceroute("example.com", "tcp", 443)
	if res.Engine != "native" {
		t.Errorf("demo engine = %q, want native", res.Engine)
	}
	if res.DestinationState == "" {
		t.Error("demo result has no destination state; the UI would show a blank")
	}
	if res.Reached && res.DestinationState != DestOpen {
		t.Errorf("reached tcp demo has state %q, want %q", res.DestinationState, DestOpen)
	}
	if !res.Reached && res.DestinationState != DestFiltered {
		t.Errorf("stalled demo has state %q, want %q", res.DestinationState, DestFiltered)
	}
}
