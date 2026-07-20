package store

import (
	"encoding/json"
	"sort"
	"time"
)

// WlanRoamingSummary is the aggregated roaming picture over a time window:
// network-wide counts (for summary tiles) plus the flattened event log.
// Client timelines (for the swimlane chart) are derived client-side from
// Events, since the UI already needs the full list for the table.
type WlanRoamingSummary struct {
	GoodRoams       int             `json:"goodRoams"`
	SuboptimalRoams int             `json:"suboptimalRoams"`
	BadRoams        int             `json:"badRoams"`
	PingPongClients int             `json:"pingPongClients"`
	StickyClients   int             `json:"stickyClients"`
	Disconnects     int             `json:"disconnects"`
	Events          []WlanRoamEntry `json:"events"` // newest first
}

// WlanRoamEntry is one roam or disconnect event, classified and with the
// client's dwell time on ToBSSID filled in (until its next event, or the
// window end if still the latest).
type WlanRoamEntry struct {
	ClientMAC      string  `json:"clientMac"`
	SSID           string  `json:"ssid"`
	FromBSSID      string  `json:"fromBssid"`
	ToBSSID        string  `json:"toBssid"` // "" = disconnect
	FromChannel    uint32  `json:"fromChannel"`
	ToChannel      uint32  `json:"toChannel"`
	FromRSSIdBm    int32   `json:"fromRssiDbm"`
	ToRSSIdBm      int32   `json:"toRssiDbm"`
	RoamTimeMs     float64 `json:"roamTimeMs"`
	DetectedAtMs   int64   `json:"detectedAtMs"`
	Classification string  `json:"classification"` // "good" | "suboptimal" | "bad" | "disconnect"
	DurationMs     float64 `json:"durationMs"`     // time on ToBSSID until the next event (0 for disconnects)
}

// pingPongWindowMs: an A->B->A bounce within this window flags the client
// as ping-ponging.
const pingPongWindowMs = 5 * 60 * 1000

// stickyRSSIGapDB: a client is "sticky" if a sibling BSSID of the same SSID
// was heard this much stronger than the one it's on.
const stickyRSSIGapDB = 10

// stickyMinDwellMs: minimum time on the current (weak) BSSID before it
// counts as "sticky" rather than a roam that just happened to land there.
const stickyMinDwellMs = 5 * 60 * 1000

// WlanRoaming aggregates wlan_passive roam_events for the filter's window
// into network-wide counts and a flattened, classified event log.
func (s *Store) WlanRoaming(f ResultFilter) (*WlanRoamingSummary, error) {
	f.TestType = "wlan_passive"
	if f.Limit <= 0 {
		f.Limit = 2000
	}
	results, err := s.ListResults(f)
	if err != nil {
		return nil, err
	}

	// DetectedAtMs is a proto int64, which protojson serializes as a JSON
	// string (not all int64 values fit safely in a JSON number) — the
	// ",string" tag tells encoding/json to unquote it.
	type rawEvent struct {
		ClientMac    string  `json:"clientMac"`
		Ssid         string  `json:"ssid"`
		FromBssid    string  `json:"fromBssid"`
		ToBssid      string  `json:"toBssid"`
		FromChannel  uint32  `json:"fromChannel"`
		ToChannel    uint32  `json:"toChannel"`
		FromRssiDbm  int32   `json:"fromRssiDbm"`
		ToRssiDbm    int32   `json:"toRssiDbm"`
		RoamTimeMs   float64 `json:"roamTimeMs"`
		DetectedAtMs int64   `json:"detectedAtMs,string"`
	}

	var all []rawEvent
	var latestNetworks []struct {
		Bssid   string `json:"bssid"`
		Ssid    string `json:"ssid"`
		RssiDbm int32  `json:"rssiDbm"`
	}
	var latestNetworksAtMs int64
	for _, r := range results {
		var payload struct {
			WlanPassive struct {
				RoamEvents []rawEvent `json:"roamEvents"`
				Networks   []struct {
					Bssid   string `json:"bssid"`
					Ssid    string `json:"ssid"`
					RssiDbm int32  `json:"rssiDbm"`
				} `json:"networks"`
			} `json:"wlanPassive"`
		}
		if err := json.Unmarshal(r.Payload, &payload); err != nil {
			continue
		}
		all = append(all, payload.WlanPassive.RoamEvents...)
		// results are newest-first; the first one with networks is the latest sweep
		if latestNetworksAtMs == 0 && len(payload.WlanPassive.Networks) > 0 {
			latestNetworksAtMs = r.Time.UnixMilli()
			latestNetworks = payload.WlanPassive.Networks
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].DetectedAtMs < all[j].DetectedAtMs })

	// Best RSSI per SSID from the latest sweep, for sticky-client detection.
	bestRSSIBySSID := map[string]int32{}
	for _, n := range latestNetworks {
		if n.Ssid == "" {
			continue
		}
		if cur, ok := bestRSSIBySSID[n.Ssid]; !ok || n.RssiDbm > cur {
			bestRSSIBySSID[n.Ssid] = n.RssiDbm
		}
	}

	// Index events per client (in chronological order) to compute duration
	// (time until the client's next event) and detect ping-pong bounces.
	byClient := map[string][]rawEvent{}
	for _, e := range all {
		byClient[e.ClientMac] = append(byClient[e.ClientMac], e)
	}

	summary := &WlanRoamingSummary{}
	pingPong := map[string]bool{}
	for mac, events := range byClient {
		for i := 1; i < len(events); i++ {
			prev, cur := events[i-1], events[i]
			if prev.FromBssid != "" && cur.ToBssid == prev.FromBssid && cur.FromBssid == prev.ToBssid &&
				cur.DetectedAtMs-prev.DetectedAtMs <= pingPongWindowMs {
				pingPong[mac] = true
			}
		}
	}
	summary.PingPongClients = len(pingPong)

	// Build the classified, duration-filled event log (newest first).
	nextEventMs := map[string]int64{} // client -> its next event's time, filled walking backwards
	entries := make([]WlanRoamEntry, 0, len(all))
	for i := len(all) - 1; i >= 0; i-- {
		e := all[i]
		entry := WlanRoamEntry{
			ClientMAC: e.ClientMac, SSID: e.Ssid,
			FromBSSID: e.FromBssid, ToBSSID: e.ToBssid,
			FromChannel: e.FromChannel, ToChannel: e.ToChannel,
			FromRSSIdBm: e.FromRssiDbm, ToRSSIdBm: e.ToRssiDbm,
			RoamTimeMs: e.RoamTimeMs, DetectedAtMs: e.DetectedAtMs,
		}
		end, ok := nextEventMs[e.ClientMac]
		if !ok {
			end = time.Now().UnixMilli() // this is the client's latest event: still ongoing
		}
		if e.ToBssid != "" {
			entry.DurationMs = float64(end - e.DetectedAtMs)
		}
		nextEventMs[e.ClientMac] = e.DetectedAtMs

		switch {
		case e.ToBssid == "":
			entry.Classification = "disconnect"
			summary.Disconnects++
		case e.FromRssiDbm == 0 || e.ToRssiDbm == 0:
			entry.Classification = "" // signal unknown, leave unclassified
		default:
			delta := e.ToRssiDbm - e.FromRssiDbm
			switch {
			case delta >= 0:
				entry.Classification = "good"
				summary.GoodRoams++
			case delta > -5:
				entry.Classification = "suboptimal"
				summary.SuboptimalRoams++
			default:
				entry.Classification = "bad"
				summary.BadRoams++
			}
		}
		entries = append(entries, entry)
	}
	summary.Events = entries

	// Sticky clients: for each client's current (newest) position — entries
	// is newest-first, so that's the first entry seen per MAC — flag it if
	// it's dwelled >= stickyMinDwellMs on a BSSID meaningfully weaker than a
	// same-SSID sibling from the latest sweep. Requiring dwell time excludes
	// a client that simply just bad-roamed there a moment ago.
	seenClient := map[string]bool{}
	sticky := 0
	for _, e := range entries {
		if seenClient[e.ClientMAC] {
			continue
		}
		seenClient[e.ClientMAC] = true
		if e.ToBSSID == "" {
			continue // currently disconnected, not sticky
		}
		best, ok := bestRSSIBySSID[e.SSID]
		if !ok {
			continue
		}
		if e.DurationMs >= stickyMinDwellMs && best-e.ToRSSIdBm >= stickyRSSIGapDB {
			sticky++
		}
	}
	summary.StickyClients = sticky

	return summary, nil
}
