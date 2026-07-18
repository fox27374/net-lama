package probe

import "math/rand"

// demoWlanActive synthesizes a plausible active-test outcome for
// NETLAMA_WLAN_DEMO pipelines.
func demoWlanActive(iface string, opts WlanActiveOpts) *WlanActiveOutcome {
	out := &WlanActiveOutcome{
		Interface:      iface,
		SSID:           opts.SSID,
		BSSID:          "a0:f8:49:74:8b:20",
		Success:        true,
		ScanMs:         float64(1500 + rand.Intn(2500)),
		AssociateMs:    float64(20 + rand.Intn(40)),
		AuthenticateMs: float64(30 + rand.Intn(80)),
		DHCPMs:         float64(50 + rand.Intn(200)),
		IP:             "192.168.77.42",
		Netmask:        "255.255.255.0",
		Gateway:        "192.168.77.1",
		RSSIdBm:        int32(-45 - rand.Intn(15)),
	}
	if opts.ThroughputURL != "" {
		out.ThroughputMbps = float64(120 + rand.Intn(180))
		out.ThroughputMs = float64(3000 + rand.Intn(2000))
	}
	out.TotalMs = out.ScanMs + out.AssociateMs + out.AuthenticateMs + out.DHCPMs + out.ThroughputMs
	return out
}
