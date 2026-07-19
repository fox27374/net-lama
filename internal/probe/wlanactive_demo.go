package probe

import (
	"fmt"
	"math/rand"
)

// demoMAC returns the adapter's stable demo MAC, or a random one when the
// test is configured for per-run randomization.
func demoMAC(mode string) string {
	if mode == "random" {
		return fmt.Sprintf("9a:%02x:%02x:%02x:%02x:%02x", rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(256))
	}
	return "40:a5:ef:5c:61:b4"
}

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
		DNSServers:     []string{"192.168.77.1", "1.1.1.1"},
		RSSIdBm:        int32(-45 - rand.Intn(15)),
		NoiseDBm:       -95,
		MAC:            demoMAC(opts.MACMode),
		TxPackets:      uint32(800 + rand.Intn(400)),
		TxRetries:      uint32(20 + rand.Intn(60)),
	}
	out.SNRdB = float64(out.RSSIdBm - out.NoiseDBm)
	if out.TxPackets > 0 {
		out.TxRetryPct = float64(out.TxRetries) / float64(out.TxPackets) * 100
	}
	if opts.ThroughputURL != "" {
		out.ThroughputMbps = float64(120 + rand.Intn(180))
		out.ThroughputMs = float64(3000 + rand.Intn(2000))
	}
	out.TotalMs = out.ScanMs + out.AssociateMs + out.AuthenticateMs + out.DHCPMs + out.ThroughputMs
	return out
}
