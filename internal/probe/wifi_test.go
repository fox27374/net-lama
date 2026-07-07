package probe

import "testing"

const iwDevSample = `phy#1
	Interface wlan1
		ifindex 5
		wdev 0x100000001
		addr 00:c0:ca:11:22:33
		type monitor
		txpower 20.00 dBm
phy#0
	Interface wlan0
		ifindex 3
		wdev 0x1
		addr dc:a6:32:aa:bb:cc
		ssid corp-wifi
		type managed
		channel 36 (5180 MHz), width: 80 MHz, center1: 5210 MHz
		txpower 31.00 dBm
`

func TestParseIWDev(t *testing.T) {
	ifaces := parseIWDev(iwDevSample)
	if len(ifaces) != 2 {
		t.Fatalf("expected 2 interfaces, got %d: %+v", len(ifaces), ifaces)
	}
	if ifaces[0].Name != "wlan1" || ifaces[0].PHY != "phy1" || !ifaces[0].SupportsMonitor {
		t.Errorf("wlan1 parsed wrong: %+v", ifaces[0])
	}
	if ifaces[1].Name != "wlan0" || ifaces[1].PHY != "phy0" || ifaces[1].SupportsMonitor {
		t.Errorf("wlan0 parsed wrong: %+v", ifaces[1])
	}
}

const iwScanSample = `BSS a0:f8:49:74:8b:20(on wlan0)
	last seen: 100 ms ago
	freq: 2412
	beacon interval: 100 TUs
	capability: ESS Privacy (0x0411)
	signal: -42.00 dBm
	SSID: corp-wifi
	RSN:	 * Version: 1
		 * Group cipher: CCMP
		 * Authentication suites: PSK
BSS c0:25:5c:ec:bb:40(on wlan0)
	freq: 5180
	capability: ESS Privacy (0x0411)
	signal: -67.00 dBm
	SSID: IoT-Net
	RSN:	 * Version: 1
		 * Authentication suites: SAE
BSS 30:8d:99:b6:85:84(on wlan0)
	freq: 2462
	capability: ESS (0x0401)
	signal: -80.00 dBm
	SSID: Guest-Open
BSS f4:92:bf:2d:40:45(on wlan0)
	freq: 5500
	capability: ESS Privacy (0x0411)
	signal: -75.00 dBm
	SSID:
	RSN:	 * Version: 1
`

func TestParseIWScan(t *testing.T) {
	aps := parseIWScan(iwScanSample)
	if len(aps) != 4 {
		t.Fatalf("expected 4 APs, got %d", len(aps))
	}

	if aps[0].BSSID != "a0:f8:49:74:8b:20" || aps[0].SSID != "corp-wifi" {
		t.Errorf("AP0 bssid/ssid wrong: %+v", aps[0])
	}
	if aps[0].Channel != 1 || aps[0].Band != "2.4 GHz" {
		t.Errorf("AP0 channel/band wrong: ch=%d band=%q", aps[0].Channel, aps[0].Band)
	}
	if aps[0].RSSIdBm != -42 {
		t.Errorf("AP0 rssi wrong: %v", aps[0].RSSIdBm)
	}
	if aps[0].Security != "WPA2" {
		t.Errorf("AP0 security wrong: %q", aps[0].Security)
	}

	// SAE -> WPA3, 5 GHz channel 36
	if aps[1].Security != "WPA3" {
		t.Errorf("AP1 security wrong: %q", aps[1].Security)
	}
	if aps[1].Channel != 36 || aps[1].Band != "5 GHz" {
		t.Errorf("AP1 channel/band wrong: ch=%d band=%q", aps[1].Channel, aps[1].Band)
	}

	// Open network (no RSN/WPA, no Privacy)
	if aps[2].Security != "Open" {
		t.Errorf("AP2 security wrong: %q", aps[2].Security)
	}

	// Hidden SSID still recorded
	if aps[3].SSID != "" || aps[3].BSSID != "f4:92:bf:2d:40:45" {
		t.Errorf("AP3 (hidden) wrong: %+v", aps[3])
	}
}
