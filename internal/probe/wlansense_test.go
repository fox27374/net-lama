package probe

import (
	"context"
	"os"
	"testing"
)

func TestWlanSenseDemoMode(t *testing.T) {
	// Enable demo mode
	os.Setenv("NETLAMA_WLAN_DEMO", "1")
	defer os.Unsetenv("NETLAMA_WLAN_DEMO")

	ctx := context.Background()
	iface, stations, channels, networks, sweepMs, err := Sense(ctx, "wlan0", nil, 0)

	if err != nil {
		t.Fatalf("Sense failed: %v", err)
	}

	if iface != "wlan0" {
		t.Errorf("expected interface wlan0, got %s", iface)
	}

	if len(stations) == 0 {
		t.Error("expected stations in demo mode, got 0")
	}

	if len(channels) == 0 {
		t.Error("expected channels in demo mode, got 0")
	}

	if len(networks) == 0 {
		t.Error("expected networks in demo mode, got 0")
	}

	if sweepMs == 0 {
		t.Error("expected non-zero sweep time")
	}

	// Verify station data looks reasonable
	for _, station := range stations {
		if station.MAC == "" {
			t.Error("station MAC should not be empty")
		}
		if station.RSSIdBm > 0 || station.RSSIdBm < -100 {
			t.Errorf("station RSSI out of reasonable range: %d dBm", station.RSSIdBm)
		}
	}

	// Verify channel data looks reasonable
	for _, channel := range channels {
		if channel.Channel == 0 {
			t.Error("channel number should not be zero")
		}
		if channel.FreqMHz == 0 {
			t.Error("channel frequency should not be zero")
		}
		if channel.UtilizationPct < 0 || channel.UtilizationPct > 100 {
			t.Errorf("channel utilization out of range: %.1f%%", channel.UtilizationPct)
		}
	}

	t.Logf("WLAN sense demo: %d stations, %d channels, %d ms", len(stations), len(channels), sweepMs)
}

func TestIWSurveyDumpParser(t *testing.T) {
	// Test parsing of `iw dev <if> survey dump` output
	// Note: the parser expects lowercase "frequency:" prefix
	sample := `frequency:	2412 MHz [1]
	channel active time:	1000 ms
	channel busy time:	250 ms
frequency:	2437 MHz [6]
	channel active time:	1000 ms
	channel busy time:	500 ms
frequency:	5180 MHz [36]
	channel active time:	1000 ms
	channel busy time:	200 ms
`

	result := parseIWSurveyDump(sample)

	if len(result) != 3 {
		t.Errorf("expected 3 entries, got %d", len(result))
	}

	// Check 2.4 GHz channel
	if stat, ok := result[2412]; ok {
		if stat.Channel != 1 {
			t.Errorf("channel 2412 MHz: expected ch 1, got %d", stat.Channel)
		}
		if stat.UtilizationPct != 25.0 {
			t.Errorf("channel 2412 MHz: expected 25%% utilization, got %.1f%%", stat.UtilizationPct)
		}
	} else {
		t.Error("missing 2412 MHz entry")
	}

	// Check 5 GHz channel
	if stat, ok := result[5180]; ok {
		if stat.Channel != 36 {
			t.Errorf("channel 5180 MHz: expected ch 36, got %d", stat.Channel)
		}
		if stat.UtilizationPct != 20.0 {
			t.Errorf("channel 5180 MHz: expected 20%% utilization, got %.1f%%", stat.UtilizationPct)
		}
	} else {
		t.Error("missing 5180 MHz entry")
	}
}

func TestChannelAndBand(t *testing.T) {
	tests := []struct {
		freq     uint32
		expectCh uint32
		expectBand string
	}{
		{2412, 1, "2.4 GHz"},
		{2437, 6, "2.4 GHz"},
		{2484, 14, "2.4 GHz"},
		{5180, 36, "5 GHz"},
		{5240, 48, "5 GHz"},
		{5885, 177, "5 GHz"},
		{5955, 1, "6 GHz"},
		{6425, 95, "6 GHz"},
	}

	for _, test := range tests {
		ch, band := channelAndBand(test.freq)
		if ch != test.expectCh {
			t.Errorf("freq %d MHz: expected channel %d, got %d", test.freq, test.expectCh, ch)
		}
		if band != test.expectBand {
			t.Errorf("freq %d MHz: expected band %s, got %s", test.freq, test.expectBand, band)
		}
	}
}

// Note: Frame parsing tests are complex with gopacket serialization.
// The critical cross-platform component is parseSSIDFromElements,
// which is tested below. Full frame parsing is tested via the Linux
// captureOnChannel function in E2E scenarios with real packets.

// fixedBody builds the 12-byte fixed beacon header (timestamp 8, interval 2,
// capability 2) followed by the given TLV elements. privacy sets capability
// bit 4 (0x0010).
func fixedBody(privacy bool, elements ...byte) []byte {
	cap0 := byte(0)
	if privacy {
		cap0 = 0x10
	}
	body := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, cap0, 0}
	return append(body, elements...)
}

func ssidElem(ssid string) []byte {
	return append([]byte{0, byte(len(ssid))}, []byte(ssid)...)
}

func TestParseBeaconBodySSID(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected string
	}{
		{"simple SSID", fixedBody(false, ssidElem("test")...), "test"},
		{"empty SSID", fixedBody(false, 0, 0), ""},
		{"SSID with other elements", fixedBody(false, append([]byte{1, 2, 0x11, 0x22}, ssidElem("netwk")...)...), "netwk"},
		{"truncated", fixedBody(false, 0, 5, 't', 'e', 's'), ""},
		{"short body", []byte{0, 0, 0}, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parseBeaconBody(test.body).SSID
			if result != test.expected {
				t.Errorf("expected '%s', got '%s'", test.expected, result)
			}
		})
	}
}

// rsnElem builds an RSN element body: version, group cipher (4), pairwise
// count+suites, akm count+suites (each AKM suite is 00-0F-AC-<type>).
func rsnElem(akmTypes ...byte) []byte {
	v := []byte{1, 0} // version
	v = append(v, 0x00, 0x0F, 0xAC, 4)                       // group cipher: CCMP
	v = append(v, 1, 0, 0x00, 0x0F, 0xAC, 4)                 // 1 pairwise: CCMP
	v = append(v, byte(len(akmTypes)), 0)
	for _, t := range akmTypes {
		v = append(v, 0x00, 0x0F, 0xAC, t)
	}
	return append([]byte{48, byte(len(v))}, v...)
}

func TestParseBeaconBodySecurity(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected string
	}{
		{"open", fixedBody(false), "Open"},
		{"wep", fixedBody(true), "WEP"},
		{"wpa2 psk", fixedBody(true, rsnElem(2)...), "WPA2"},
		{"wpa3 sae", fixedBody(true, rsnElem(8)...), "WPA3"},
		{"wpa2/wpa3 transition", fixedBody(true, rsnElem(2, 8)...), "WPA2/WPA3"},
		{"wpa2 enterprise", fixedBody(true, rsnElem(1)...), "WPA2-Ent"},
		{"owe", fixedBody(true, rsnElem(18)...), "OWE"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parseBeaconBody(test.body).Security
			if result != test.expected {
				t.Errorf("expected '%s', got '%s'", test.expected, result)
			}
		})
	}
}

func TestParseBeaconBodyStandards(t *testing.T) {
	htElem := []byte{45, 2, 0, 0}
	vhtElem := []byte{191, 2, 0, 0}
	heElem := []byte{255, 1, 35}
	body := fixedBody(false, append(append(append([]byte{}, htElem...), vhtElem...), heElem...)...)

	result := parseBeaconBody(body).Standards
	if result != "n/ac/ax" {
		t.Errorf("expected 'n/ac/ax', got '%s'", result)
	}
}

// --- processFrame tests: build raw radiotap+802.11 frames by hand and assert
// the station/SSID aggregation. gopacket has no Dot11 serializer, so we
// construct the bytes directly (this is what afpacket hands us on the wire). ---

// rtHeader builds a radiotap header carrying Flags, Rate (500kbps units) and
// DBMAntennaSignal (all 1-byte, 1-byte aligned so no padding needed).
func rtHeader(flags byte, rate byte, sig int8) []byte {
	present := uint32(0x02 | 0x04 | 0x20) // Flags | Rate | DBMAntennaSignal
	h := []byte{0x00, 0x00, 0x0b, 0x00} // version, pad, len=11 (LE), then present
	h = append(h, byte(present), byte(present>>8), byte(present>>16), byte(present>>24))
	h = append(h, flags, rate, byte(sig))
	return h
}

func mac(b byte) []byte { return []byte{0x02, 0, 0, 0, 0, b} }

// dot11 builds a MAC header: fc0 (type/subtype), fcFlags (ToDS/FromDS),
// three addresses, plus an optional body.
func dot11(fc0, fcFlags byte, a1, a2, a3 []byte, body ...byte) []byte {
	f := []byte{fc0, fcFlags, 0, 0}
	f = append(f, a1...)
	f = append(f, a2...)
	f = append(f, a3...)
	f = append(f, 0, 0) // sequence control
	return append(f, body...)
}

func TestProcessFrameDataToDS(t *testing.T) {
	stations := map[string]*WlanStation{}
	ssid := map[string]*WlanNetwork{}
	// ToDS (0x01): STA transmits to AP. STA=Address2, BSSID=Address1.
	frame := append(rtHeader(0x00, 48, -57), dot11(0x08, 0x01, mac(0xAA), mac(0xBB), mac(0xAA))...)
	processFrame(frame, stations, ssid, 1000)

	sta := stations["02:00:00:00:00:bb"]
	if sta == nil {
		t.Fatalf("expected station 02:00:00:00:00:bb, got %v", stations)
	}
	if sta.BSSID != "02:00:00:00:00:aa" {
		t.Errorf("BSSID = %q, want 02:00:00:00:00:aa", sta.BSSID)
	}
	if sta.RSSIdBm != -57 {
		t.Errorf("RSSI = %d, want -57", sta.RSSIdBm)
	}
	if sta.RateMbps != 24 {
		t.Errorf("rate = %v, want 24", sta.RateMbps)
	}
	if sta.ProbeOnly {
		t.Error("data-frame station must not be probe-only")
	}
	// A second frame from the same station increments the frame count.
	processFrame(frame, stations, ssid, 1001)
	if stations["02:00:00:00:00:bb"].Frames != 2 {
		t.Errorf("frames = %d, want 2", stations["02:00:00:00:00:bb"].Frames)
	}
}

func TestProcessFrameDataFromDS(t *testing.T) {
	stations := map[string]*WlanStation{}
	// FromDS (0x02): AP transmits to STA. STA=Address1, BSSID=Address2.
	frame := append(rtHeader(0x00, 0, -60), dot11(0x08, 0x02, mac(0x11), mac(0x22), mac(0x22))...)
	processFrame(frame, stations, map[string]*WlanNetwork{}, 1000)
	if _, ok := stations["02:00:00:00:00:11"]; !ok {
		t.Fatalf("expected STA at Address1 (02:00:00:00:00:11), got %v", stations)
	}
}

func TestProcessFrameBeaconSSID(t *testing.T) {
	networks := map[string]*WlanNetwork{}
	// Beacon body: 8 timestamp + 2 interval + 2 capability, then SSID IE.
	body := make([]byte, 12)
	body = append(body, 0x00, 0x04, 'c', 'o', 'r', 'p') // IE tag 0, len 4, "corp"
	frame := append(rtHeader(0x00, 0, -40), dot11(0x80, 0x00, mac(0xFF), mac(0x01), mac(0x01), body...)...)
	processFrame(frame, map[string]*WlanStation{}, networks, 1000)
	n := networks["02:00:00:00:00:01"]
	if n == nil || n.SSID != "corp" {
		t.Fatalf("networks = %v, want BSSID 02:00:00:00:00:01 -> corp", networks)
	}
	if n.Beacons != 1 {
		t.Errorf("beacons = %d, want 1", n.Beacons)
	}
}

func TestProcessFrameProbeRequest(t *testing.T) {
	stations := map[string]*WlanStation{}
	frame := append(rtHeader(0x00, 0, -70), dot11(0x40, 0x00, mac(0xFF), mac(0x33), mac(0xFF))...)
	processFrame(frame, stations, map[string]*WlanNetwork{}, 1000)
	sta := stations["02:00:00:00:00:33"]
	if sta == nil || !sta.ProbeOnly {
		t.Fatalf("expected probe-only station 02:00:00:00:00:33, got %v", stations)
	}
}

func TestProcessFrameBadFCSSkipped(t *testing.T) {
	stations := map[string]*WlanStation{}
	// Flags byte 0x40 = BadFCS: the frame must be dropped entirely.
	frame := append(rtHeader(0x40, 48, -50), dot11(0x08, 0x01, mac(0xAA), mac(0xBB), mac(0xAA))...)
	processFrame(frame, stations, map[string]*WlanNetwork{}, 1000)
	if len(stations) != 0 {
		t.Fatalf("BadFCS frame must be skipped, got stations %v", stations)
	}
}

func TestIsUnicastMAC(t *testing.T) {
	cases := map[string]bool{
		"02:00:00:00:00:bb": true,  // locally-administered unicast
		"a0:f8:49:74:8b:20": true,  // normal unicast
		"ff:ff:ff:ff:ff:ff": false, // broadcast
		"01:00:5e:00:00:fb": false, // IPv4 multicast
		"33:33:00:00:00:01": false, // IPv6 multicast
		"00:00:00:00:00:00": false, // all-zero
		"":                  false,
	}
	for mac, want := range cases {
		if got := isUnicastMAC(mac); got != want {
			t.Errorf("isUnicastMAC(%q) = %v, want %v", mac, got, want)
		}
	}
}

func TestProcessFrameDropsBroadcastStation(t *testing.T) {
	stations := map[string]*WlanStation{}
	// FromDS broadcast data frame from an AP: Address1 (destination) is the
	// broadcast address — must NOT be recorded as a station.
	bcast := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	frame := append(rtHeader(0x00, 0, -62), dot11(0x08, 0x02, bcast, mac(0xAA), mac(0xAA))...)
	processFrame(frame, stations, map[string]*WlanNetwork{}, 1000)
	if len(stations) != 0 {
		t.Fatalf("broadcast destination must not be a station, got %v", stations)
	}
}

func TestProcessFrameNetworkRSSIandBeacons(t *testing.T) {
	networks := map[string]*WlanNetwork{}
	body := make([]byte, 12)
	body = append(body, 0x00, 0x05, 'a', 't', 'a', 'l', 't') // SSID "atalt"
	// Two beacons from the same BSSID at different RSSI; keep the strongest.
	weak := append(rtHeader(0x00, 0, -70), dot11(0x80, 0x00, mac(0xFF), mac(0x0A), mac(0x0A), body...)...)
	strong := append(rtHeader(0x00, 0, -42), dot11(0x80, 0x00, mac(0xFF), mac(0x0A), mac(0x0A), body...)...)
	processFrame(weak, map[string]*WlanStation{}, networks, 1000)
	processFrame(strong, map[string]*WlanStation{}, networks, 1001)
	n := networks["02:00:00:00:00:0a"]
	if n == nil {
		t.Fatal("expected network 02:00:00:00:00:0a")
	}
	if n.SSID != "atalt" {
		t.Errorf("ssid = %q, want atalt", n.SSID)
	}
	if n.RSSIdBm != -42 {
		t.Errorf("rssi = %d, want -42 (strongest)", n.RSSIdBm)
	}
	if n.Beacons != 2 {
		t.Errorf("beacons = %d, want 2", n.Beacons)
	}
}

func TestRecordNetworkSkipsBroadcastBSSID(t *testing.T) {
	networks := map[string]*WlanNetwork{}
	recordNetwork(networks, "ff:ff:ff:ff:ff:ff", beaconInfo{SSID: "x"}, -50, 0)
	if len(networks) != 0 {
		t.Fatalf("broadcast BSSID must be skipped, got %v", networks)
	}
}

func TestParseIWPhyChannels(t *testing.T) {
	// Real `iw phy <phy> channels` format: leading "*", DFS/radar and
	// "No IR" channels are usable for passive monitoring; "disabled" is not.
	out := `Band 1:
	* 2412 MHz [1] 
	  Maximum TX power: 20.0 dBm
	* 2437 MHz [6] 
	* 2462 MHz [11] 
Band 2:
	* 5180 MHz [36] 
	  5500 MHz [100] (radar detection)
	  5560 MHz [112] (radar detection)
	  5300 MHz [60] (disabled)
	* 5745 MHz [149] `
	got := parseIWPhyChannels(out)
	want := map[uint32]bool{1: true, 6: true, 11: true, 36: true, 100: true, 112: true, 149: true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want channels %v", got, want)
	}
	for _, ch := range got {
		if !want[ch] {
			t.Errorf("unexpected channel %d (disabled ch 60 must be excluded)", ch)
		}
	}
	// 2.4 GHz must sort before 5 GHz.
	if got[0] > 14 {
		t.Errorf("expected 2.4 GHz channels first, got %v", got)
	}
}

func TestParsePhyName(t *testing.T) {
	// `iw dev <iface> info` format (what getPhyChannels feeds it)
	devInfo := `Interface wlan1
	ifindex 5
	wdev 0x100000001
	addr 96:fe:ac:54:10:ac
	wiphy 1
	type managed`
	if got := parsePhyName(devInfo); got != "phy1" {
		t.Errorf("wiphy form: got %q, want phy1", got)
	}
	// bare `iw dev` format
	bare := "phy#0\n\tInterface wlan0"
	if got := parsePhyName(bare); got != "phy0" {
		t.Errorf("phy# form: got %q, want phy0", got)
	}
	if got := parsePhyName("no phy here"); got != "" {
		t.Errorf("no match: got %q, want empty", got)
	}
}

func TestParseBeaconBodyDetails(t *testing.T) {
	elems := []byte{}
	elems = append(elems, 7, 6, 'A', 'T', ' ', 1, 11, 20)        // Country AT
	elems = append(elems, 11, 5, 7, 0, 128, 0, 0)                // BSS Load: 7 stations, util 128/255
	elems = append(elems, 61, 3, 6, 0x01, 0)                     // HT op: secondary above → 40 MHz
	elems = append(elems, 54, 3, 0x12, 0x34, 1)                  // Mobility Domain → r
	elems = append(elems, 70, 5, 0, 0, 0, 0, 0)                  // RM Enabled → k
	elems = append(elems, 127, 3, 0, 0, 0x08)                    // ExtCap bit 19 → v
	elems = append(elems, rsnElem(2, 8)...)                      // PSK+SAE, CCMP pairwise

	body := fixedBody(true, elems...)
	body[8], body[9] = 100, 0 // beacon interval 100 TU
	info := parseBeaconBody(body)

	if info.BeaconIntervalTU != 100 {
		t.Errorf("interval: expected 100, got %d", info.BeaconIntervalTU)
	}
	if info.Country != "AT" {
		t.Errorf("country: expected AT, got %q", info.Country)
	}
	if !info.LoadPresent || info.LoadStations != 7 {
		t.Errorf("load: expected present with 7 stations, got %+v", info)
	}
	if info.LoadChannelUtilPct < 50 || info.LoadChannelUtilPct > 51 {
		t.Errorf("load util: expected ~50.2, got %f", info.LoadChannelUtilPct)
	}
	if info.WidthMHz != 40 {
		t.Errorf("width: expected 40, got %d", info.WidthMHz)
	}
	if info.Roaming != "k/r/v" {
		t.Errorf("roaming: expected k/r/v, got %q", info.Roaming)
	}
	if info.SecurityDetail != "PSK+SAE · CCMP" {
		t.Errorf("security detail: expected 'PSK+SAE · CCMP', got %q", info.SecurityDetail)
	}
}

func TestParseBeaconBodyVHTWidth(t *testing.T) {
	vht80 := fixedBody(false, 192, 5, 1, 42, 0, 0, 0)
	if w := parseBeaconBody(vht80).WidthMHz; w != 80 {
		t.Errorf("vht80: expected 80, got %d", w)
	}
	vht160 := fixedBody(false, 192, 5, 1, 42, 50, 0, 0) // CCFS1 set → 160
	if w := parseBeaconBody(vht160).WidthMHz; w != 160 {
		t.Errorf("vht160: expected 160, got %d", w)
	}
	plain := fixedBody(false, ssidElem("x")...)
	if w := parseBeaconBody(plain).WidthMHz; w != 20 {
		t.Errorf("plain: expected 20, got %d", w)
	}
}

func TestParseBeaconBodyProDetails(t *testing.T) {
	elems := []byte{}
	elems = append(elems, 1, 4, 0x82, 0x84, 0x0B, 0x6C)  // rates: 1/2/5.5/54(basic)
	elems = append(elems, 5, 4, 0, 3, 0, 0)              // TIM: DTIM period 3
	// RSN with group TKIP, pairwise CCMP, AKM PSK, caps MFPC|MFPR
	rsn := []byte{1, 0, 0x00, 0x0F, 0xAC, 2, 1, 0, 0x00, 0x0F, 0xAC, 4, 1, 0, 0x00, 0x0F, 0xAC, 2, 0xC0, 0x00}
	elems = append(elems, append([]byte{48, byte(len(rsn))}, rsn...)...)
	// HT cap: 2 RX MCS bitmask bytes set → 2 streams
	htcap := make([]byte, 26)
	htcap[3], htcap[4] = 0xFF, 0xFF
	elems = append(elems, append([]byte{45, byte(len(htcap))}, htcap...)...)
	elems = append(elems, 61, 3, 6, 0x01, 0)             // HT op → 40 MHz
	elems = append(elems, 221, 5, 0x00, 0x50, 0xF2, 0x04, 0x10) // WPS

	info := parseBeaconBody(fixedBody(true, elems...))
	if info.MFP != "required" {
		t.Errorf("mfp: expected required, got %q", info.MFP)
	}
	if info.GroupCipher != "TKIP" {
		t.Errorf("group cipher: expected TKIP, got %q", info.GroupCipher)
	}
	if info.DTIMPeriod != 3 {
		t.Errorf("dtim: expected 3, got %d", info.DTIMPeriod)
	}
	if !info.WPS {
		t.Error("wps: expected true")
	}
	if info.Streams != 2 {
		t.Errorf("streams: expected 2, got %d", info.Streams)
	}
	if info.MaxRateMbps != 300 { // n, 40 MHz, 2 streams = 150*2
		t.Errorf("max rate: expected 300, got %f", info.MaxRateMbps)
	}
}

func TestParseBeaconBodyVHTStreamsAndLegacyRate(t *testing.T) {
	// VHT cap with Rx MCS map: 4 streams supported (0xFFAA → pairs 2,2,2,2,3,3,3,3... check)
	vhtcap := make([]byte, 12)
	vhtcap[4], vhtcap[5] = 0xAA, 0xFF // streams 1-4 = MCS0-9 (0b10), 5-8 unsupported (0b11)
	body := fixedBody(false, append(append([]byte{191, byte(len(vhtcap))}, vhtcap...), 192, 5, 1, 42, 0, 0, 0)...)
	info := parseBeaconBody(body)
	if info.Streams != 4 {
		t.Errorf("vht streams: expected 4, got %d", info.Streams)
	}
	if info.MaxRateMbps != 433.3*4 {
		t.Errorf("max rate: expected %f, got %f", 433.3*4, info.MaxRateMbps)
	}

	// Legacy-only AP: max from supported rates
	legacy := fixedBody(false, 1, 4, 0x82, 0x84, 0x0B, 0x6C)
	if r := parseBeaconBody(legacy).MaxRateMbps; r != 54 {
		t.Errorf("legacy rate: expected 54, got %f", r)
	}
}
