package probe

import (
	"context"
	"os"
	"testing"
)

func TestWlanSenseDemoMode(t *testing.T) {
	// Enable demo mode
	os.Setenv("NETLAMA_WLAN_SENSE_DEMO", "1")
	defer os.Unsetenv("NETLAMA_WLAN_SENSE_DEMO")

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

func TestParseSSIDFromElements(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "simple SSID",
			data:     []byte{0, 4, 't', 'e', 's', 't'},
			expected: "test",
		},
		{
			name:     "empty SSID",
			data:     []byte{0, 0},
			expected: "",
		},
		{
			name:     "SSID with other elements",
			data:     []byte{1, 2, 0x11, 0x22, 0, 5, 'n', 'e', 't', 'w', 'k'},
			expected: "netwk",
		},
		{
			name:     "truncated",
			data:     []byte{0, 5, 't', 'e', 's'},
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parseSSIDFromElements(test.data)
			if result != test.expected {
				t.Errorf("expected '%s', got '%s'", test.expected, result)
			}
		})
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
	recordNetwork(networks, "ff:ff:ff:ff:ff:ff", "x", -50)
	if len(networks) != 0 {
		t.Fatalf("broadcast BSSID must be skipped, got %v", networks)
	}
}
