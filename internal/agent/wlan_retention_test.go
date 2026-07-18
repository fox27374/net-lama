package agent

import (
	"testing"
	"time"

	"github.com/fox27374/net-lama/internal/probe"
)

func TestMergeWlanRetained(t *testing.T) {
	now := time.Now()
	state := &wlanPassiveState{}

	// First sweep: two APs, one station
	nets, stas := mergeWlanRetained(state,
		[]probe.WlanNetwork{{BSSID: "aa", LastSeenMs: now.UnixMilli()}, {BSSID: "bb", LastSeenMs: now.UnixMilli()}},
		[]probe.WlanStation{{MAC: "s1", LastSeenMs: now.UnixMilli()}}, now)
	if len(nets) != 2 || len(stas) != 1 {
		t.Fatalf("first sweep: got %d nets, %d stations", len(nets), len(stas))
	}

	// Second sweep 5 min later: only aa heard again — bb retained
	later := now.Add(5 * time.Minute)
	nets, _ = mergeWlanRetained(state,
		[]probe.WlanNetwork{{BSSID: "aa", RSSIdBm: -40, LastSeenMs: later.UnixMilli()}}, nil, later)
	if len(nets) != 2 {
		t.Fatalf("retention: expected 2 nets, got %d", len(nets))
	}

	// Third sweep 11 min after start: bb and s1 expired, aa refreshed at 5 min stays
	expiry := now.Add(11 * time.Minute)
	nets, stas = mergeWlanRetained(state, nil, nil, expiry)
	if len(nets) != 1 || nets[0].BSSID != "aa" {
		t.Fatalf("expiry: expected only aa, got %+v", nets)
	}
	if len(stas) != 0 {
		t.Fatalf("expiry: expected no stations, got %d", len(stas))
	}

	// Entries without LastSeenMs get stamped with the sweep time
	nets, _ = mergeWlanRetained(state, []probe.WlanNetwork{{BSSID: "cc"}}, nil, expiry)
	for _, n := range nets {
		if n.BSSID == "cc" && n.LastSeenMs != expiry.UnixMilli() {
			t.Errorf("stamp: cc LastSeenMs = %d, want %d", n.LastSeenMs, expiry.UnixMilli())
		}
	}
}
