package probe

import "testing"

const mtrReached = `{
  "report": {
    "mtr": {"src": "10.0.0.5", "dst": "8.8.8.8", "tests": 5},
    "hubs": [
      {"count": 1, "host": "192.168.1.1", "Loss%": 0.0, "Snt": 5, "Avg": 0.7, "Best": 0.5, "Wrst": 1.2},
      {"count": 2, "host": "???", "Loss%": 100.0, "Snt": 5, "Avg": 0.0, "Best": 0.0, "Wrst": 0.0},
      {"count": 3, "host": "72.14.204.68", "Loss%": 0.0, "Snt": 5, "Avg": 12.4, "Best": 11.9, "Wrst": 14.1},
      {"count": 4, "host": "8.8.8.8", "Loss%": 0.0, "Snt": 5, "Avg": 15.1, "Best": 14.8, "Wrst": 16.0}
    ]
  }
}`

func TestParseMTRReached(t *testing.T) {
	res, err := parseMTR([]byte(mtrReached), "dns.google", 30)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Reached || res.Status != "reached" {
		t.Errorf("expected reached, got %q reached=%v", res.Status, res.Reached)
	}
	if len(res.Hops) != 4 {
		t.Fatalf("expected 4 hops, got %d", len(res.Hops))
	}
	if res.Hops[1].Host != "" || res.Hops[1].LossPercent != 100 {
		t.Errorf("anonymous hop 2 parsed wrong: %+v", res.Hops[1])
	}
	if res.RttMs != 15.1 {
		t.Errorf("expected destination RTT 15.1, got %v", res.RttMs)
	}
}

const mtrStalled = `{
  "report": {
    "mtr": {"src": "10.0.0.5", "dst": "203.0.113.9", "tests": 5},
    "hubs": [
      {"count": 1, "host": "192.168.1.1", "Loss%": 0.0, "Snt": 5, "Avg": 0.7, "Best": 0.5, "Wrst": 1.2},
      {"count": 2, "host": "84.116.130.1", "Loss%": 0.0, "Snt": 5, "Avg": 8.5, "Best": 8.0, "Wrst": 9.1},
      {"count": 3, "host": "???", "Loss%": 100.0, "Snt": 5, "Avg": 0.0, "Best": 0.0, "Wrst": 0.0},
      {"count": 4, "host": "???", "Loss%": 100.0, "Snt": 5, "Avg": 0.0, "Best": 0.0, "Wrst": 0.0}
    ]
  }
}`

func TestParseMTRStalled(t *testing.T) {
	res, err := parseMTR([]byte(mtrStalled), "203.0.113.9", 4)
	if err != nil {
		t.Fatal(err)
	}
	if res.Reached || res.Status != "stalled" {
		t.Errorf("expected stalled, got %q reached=%v", res.Status, res.Reached)
	}
	if res.FailureHop != 2 {
		t.Errorf("expected failure at hop 2 (last responder), got %d", res.FailureHop)
	}
}
