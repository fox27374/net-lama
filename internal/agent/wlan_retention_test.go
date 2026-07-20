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
	nets, stas, _ := mergeWlanRetained(state,
		[]probe.WlanNetwork{{BSSID: "aa", LastSeenMs: now.UnixMilli()}, {BSSID: "bb", LastSeenMs: now.UnixMilli()}},
		[]probe.WlanStation{{MAC: "s1", LastSeenMs: now.UnixMilli()}}, now)
	if len(nets) != 2 || len(stas) != 1 {
		t.Fatalf("first sweep: got %d nets, %d stations", len(nets), len(stas))
	}

	// Second sweep 5 min later: only aa heard again — bb retained
	later := now.Add(5 * time.Minute)
	nets, _, _ = mergeWlanRetained(state,
		[]probe.WlanNetwork{{BSSID: "aa", RSSIdBm: -40, LastSeenMs: later.UnixMilli()}}, nil, later)
	if len(nets) != 2 {
		t.Fatalf("retention: expected 2 nets, got %d", len(nets))
	}

	// Third sweep 11 min after start: bb and s1 expired, aa refreshed at 5 min stays
	expiry := now.Add(11 * time.Minute)
	nets, stas, _ = mergeWlanRetained(state, nil, nil, expiry)
	if len(nets) != 1 || nets[0].BSSID != "aa" {
		t.Fatalf("expiry: expected only aa, got %+v", nets)
	}
	if len(stas) != 0 {
		t.Fatalf("expiry: expected no stations, got %d", len(stas))
	}

	// Entries without LastSeenMs get stamped with the sweep time
	nets, _, _ = mergeWlanRetained(state, []probe.WlanNetwork{{BSSID: "cc"}}, nil, expiry)
	for _, n := range nets {
		if n.BSSID == "cc" && n.LastSeenMs != expiry.UnixMilli() {
			t.Errorf("stamp: cc LastSeenMs = %d, want %d", n.LastSeenMs, expiry.UnixMilli())
		}
	}
}

func TestMergeWlanRetainedRoamDetection(t *testing.T) {
	now := time.Now()
	state := &wlanPassiveState{}

	// AP "aa" known on channel 6, AP "bb" on channel 11 — needed to resolve
	// FromChannel/ToChannel in the roam event.
	_, _, _ = mergeWlanRetained(state,
		[]probe.WlanNetwork{{BSSID: "aa", Channel: 6, LastSeenMs: now.UnixMilli()}, {BSSID: "bb", Channel: 11, LastSeenMs: now.UnixMilli()}},
		[]probe.WlanStation{{MAC: "client1", BSSID: "aa", SSID: "corp", RSSIdBm: -50, LastSeenMs: now.UnixMilli()}}, now)

	// Client roams from aa to bb 2s later
	later := now.Add(2 * time.Second)
	_, _, roams := mergeWlanRetained(state, nil,
		[]probe.WlanStation{{MAC: "client1", BSSID: "bb", SSID: "corp", RSSIdBm: -60, LastSeenMs: later.UnixMilli()}}, later)
	if len(roams) != 1 {
		t.Fatalf("expected 1 roam event, got %d: %+v", len(roams), roams)
	}
	r := roams[0]
	if r.ClientMAC != "client1" || r.FromBSSID != "aa" || r.ToBSSID != "bb" {
		t.Errorf("roam identity: got %+v", r)
	}
	if r.FromChannel != 6 || r.ToChannel != 11 {
		t.Errorf("roam channels: got from=%d to=%d, want 6/11", r.FromChannel, r.ToChannel)
	}
	if r.FromRSSIdBm != -50 || r.ToRSSIdBm != -60 {
		t.Errorf("roam rssi: got from=%d to=%d, want -50/-60", r.FromRSSIdBm, r.ToRSSIdBm)
	}
	if r.RoamTimeMs != 2000 {
		t.Errorf("roam time: got %v, want 2000", r.RoamTimeMs)
	}

	// A repeat sighting on the SAME bssid is not a roam
	_, _, roams = mergeWlanRetained(state, nil,
		[]probe.WlanStation{{MAC: "client1", BSSID: "bb", SSID: "corp", RSSIdBm: -58, LastSeenMs: later.Add(time.Second).UnixMilli()}}, later.Add(time.Second))
	if len(roams) != 0 {
		t.Errorf("same-BSSID resighting should not roam, got %+v", roams)
	}

	// Client ages out past retention -> disconnect event (ToBSSID empty)
	expiry := later.Add(wlanRetention + time.Minute)
	_, _, roams = mergeWlanRetained(state, nil, nil, expiry)
	if len(roams) != 1 || roams[0].ToBSSID != "" || roams[0].FromBSSID != "bb" {
		t.Fatalf("expected 1 disconnect event from bb, got %+v", roams)
	}
}
