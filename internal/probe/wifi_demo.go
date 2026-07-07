package probe

import (
	"math/rand"
)

// Synthetic WLAN data for pipeline testing on hosts without a radio,
// enabled with NETLAMA_WLAN_DEMO. Remove reliance on this once real
// monitor-capable hardware is in place.

func demoInterfaces() []WirelessInterface {
	return []WirelessInterface{
		{Name: "wlan0", PHY: "phy0", SupportsMonitor: false},
		{Name: "wlan1", PHY: "phy1", SupportsMonitor: true},
	}
}

func demoScan(iface string) (string, []AccessPoint, error) {
	base := []AccessPoint{
		{BSSID: "a0:f8:49:74:8b:20", SSID: "corp-wifi", FreqMHz: 2412, Security: "WPA2"},
		{BSSID: "a0:f8:49:74:8b:22", SSID: "corp-guest", FreqMHz: 2437, Security: "WPA2"},
		{BSSID: "c0:25:5c:ec:bb:40", SSID: "corp-wifi", FreqMHz: 5180, Security: "WPA2"},
		{BSSID: "2c:3a:fd:8b:1e:56", SSID: "IoT-Net", FreqMHz: 5240, Security: "WPA3"},
		{BSSID: "f4:92:bf:2d:40:45", SSID: "", FreqMHz: 5500, Security: "WPA2"},
		{BSSID: "30:8d:99:b6:85:84", SSID: "Guest-Open", FreqMHz: 2462, Security: "Open"},
	}
	for i := range base {
		base[i].Channel, base[i].Band = channelAndBand(base[i].FreqMHz)
		// Jitter the signal a little so the values move between scans.
		base[i].RSSIdBm = float64(-40 - rand.Intn(50))
	}
	return iface, base, nil
}
