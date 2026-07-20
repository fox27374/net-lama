package store

import (
	"fmt"
	"testing"
	"time"
)

func TestWlanRoamingAggregation(t *testing.T) {
	s := openTestStore(t)
	now := time.Now().UnixMilli()
	ago := func(d time.Duration) int64 { return now - d.Milliseconds() }

	tenant, _ := s.CreateTenant("t1")
	site, _ := s.CreateSite(tenant.ID, "s1")
	agent, _ := s.CreateAgent(tenant.ID, site.ID, "a1")
	testDef, _ := s.CreateTest(&TestDef{
		TenantID: tenant.ID, Name: "wlan", Type: "wlan_passive", IntervalSeconds: 60,
	})

	insert := func(offsetSec int, payload string) {
		_, err := s.db.Exec(`
			INSERT INTO results (agent_id, test_id, test_name, test_type, time, payload)
			VALUES (?, ?, ?, ?, datetime('now', '+' || ? || ' seconds'), ?)
		`, agent.ID, testDef.ID, testDef.Name, testDef.Type, offsetSec, payload)
		if err != nil {
			t.Fatalf("inserting result: %v", err)
		}
	}

	// Sweep with a network snapshot: SSID "corp" has a strong AP (bb, -40)
	// and a weak one (aa, -70). client1 connected to aa 20 minutes ago and
	// never roamed since — dwelled long enough to count as sticky.
	insert(1, fmt.Sprintf(`{"wlanPassive":{"networks":[
		{"bssid":"aa","ssid":"corp","rssiDbm":-70},
		{"bssid":"bb","ssid":"corp","rssiDbm":-40}
	],"roamEvents":[
		{"clientMac":"client1","ssid":"corp","fromBssid":"","toBssid":"aa","fromRssiDbm":0,"toRssiDbm":-70,"detectedAtMs":"%d"}
	]}}`, ago(20*time.Minute)))

	// Good roam: client2 moves to meaningfully better signal.
	insert(2, fmt.Sprintf(`{"wlanPassive":{"roamEvents":[
		{"clientMac":"client2","ssid":"corp","fromBssid":"aa","toBssid":"bb","fromRssiDbm":-70,"toRssiDbm":-40,"detectedAtMs":"%d"}
	]}}`, ago(9*time.Minute)))

	// Bad roam: client3 moves to meaningfully worse signal, just now — must
	// NOT count as sticky despite ending on the weak AP (too little dwell).
	insert(3, fmt.Sprintf(`{"wlanPassive":{"roamEvents":[
		{"clientMac":"client3","ssid":"corp","fromBssid":"bb","toBssid":"aa","fromRssiDbm":-40,"toRssiDbm":-75,"detectedAtMs":"%d"}
	]}}`, ago(5*time.Second)))

	// Ping-pong: client4 bounces aa->bb->aa within the window (lateral RSSI
	// both ways, so both legs classify as "good" — isolates the ping-pong
	// count from the good/bad tally).
	insert(4, fmt.Sprintf(`{"wlanPassive":{"roamEvents":[
		{"clientMac":"client4","ssid":"corp","fromBssid":"aa","toBssid":"bb","fromRssiDbm":-55,"toRssiDbm":-55,"detectedAtMs":"%d"},
		{"clientMac":"client4","ssid":"corp","fromBssid":"bb","toBssid":"aa","fromRssiDbm":-55,"toRssiDbm":-55,"detectedAtMs":"%d"}
	]}}`, ago(4*time.Minute), ago(3*time.Minute)))

	// Disconnect: client5 ages out.
	insert(5, fmt.Sprintf(`{"wlanPassive":{"roamEvents":[
		{"clientMac":"client5","ssid":"corp","fromBssid":"bb","toBssid":"","fromRssiDbm":-50,"toRssiDbm":0,"detectedAtMs":"%d"}
	]}}`, ago(2*time.Minute)))

	summary, err := s.WlanRoaming(ResultFilter{TenantID: tenant.ID})
	if err != nil {
		t.Fatalf("WlanRoaming: %v", err)
	}

	if summary.GoodRoams != 3 { // client2 + client4's two lateral legs
		t.Errorf("GoodRoams = %d, want 3", summary.GoodRoams)
	}
	if summary.BadRoams != 1 { // client3
		t.Errorf("BadRoams = %d, want 1", summary.BadRoams)
	}
	if summary.Disconnects != 1 { // client5
		t.Errorf("Disconnects = %d, want 1", summary.Disconnects)
	}
	if summary.PingPongClients != 1 { // client4
		t.Errorf("PingPongClients = %d, want 1", summary.PingPongClients)
	}
	if summary.StickyClients != 1 { // client1, stuck on aa (-70) vs bb (-40), gap 30 >= 10
		t.Errorf("StickyClients = %d, want 1", summary.StickyClients)
	}
	if len(summary.Events) != 6 {
		t.Errorf("Events count = %d, want 6", len(summary.Events))
	}
	// Newest first (client3's bad roam was 5s ago, the most recent event)
	if summary.Events[0].ClientMAC != "client3" {
		t.Errorf("Events[0].ClientMAC = %q, want client3 (newest)", summary.Events[0].ClientMAC)
	}
}
