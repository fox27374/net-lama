package probe

import (
	"context"
	"fmt"
	"strings"
)

// WlanActiveOpts configures one active WLAN connection test.
type WlanActiveOpts struct {
	SSID               string
	Security           string // "psk", "eap-peap", "open"
	Password           string // PSK passphrase or EAP password
	Identity           string // EAP identity
	CACertPEM          string // PEM CA certificate for EAP server validation
	InsecureSkipVerify bool   // EAP: skip server certificate validation
	ThroughputURL      string // optional download URL for the throughput step
	MACMode            string // "permanent" (default) or "random"
}

// WlanActiveOutcome is the timed result of one active connection test.
type WlanActiveOutcome struct {
	Interface      string
	SSID           string
	BSSID          string
	Success        bool
	FailedStep     string // "associate", "authenticate", "dhcp", "throughput"
	ScanMs         float64 // supplicant start → SSID found
	AssociateMs    float64
	AuthenticateMs float64
	DHCPMs         float64
	IP             string
	Netmask        string
	Gateway        string
	DNSServers     []string
	ThroughputMbps float64
	ThroughputMs   float64
	RSSIdBm        int32
	NoiseDBm       int32
	SNRdB          float64
	TxRetryPct     float64
	TxPackets      uint32
	TxRetries      uint32
	MAC            string // client MAC actually used
	TotalMs        float64
}

// WlanActive runs an active connection test: associate + authenticate via
// wpa_supplicant, DHCP, and optionally a throughput download, timing each
// step. The interface is switched to managed mode and restored afterwards.
// Requires wpa_supplicant, iproute2 and NET_ADMIN/NET_RAW.
func WlanActive(ctx context.Context, iface string, opts WlanActiveOpts) (*WlanActiveOutcome, error) {
	if wlanDemo() {
		return demoWlanActive(iface, opts), nil
	}
	return wlanActiveImpl(ctx, iface, opts)
}

// wpaSupplicantConf renders a wpa_supplicant network block for the test.
// caCertPath is the on-disk path the CA PEM was written to ("" = none —
// wpa_supplicant then accepts any EAP server certificate).
func wpaSupplicantConf(opts WlanActiveOpts, caCertPath string) string {
	var b strings.Builder
	b.WriteString("ctrl_interface=/tmp/netlama-wpa\n")
	// MAC policy. Default "permanent" (mac_addr=0): the adapter's real MAC for
	// both scans and association — one stable identity, reused DHCP lease, one
	// entry in the AP's client table. "random" (mac_addr=1) uses a fresh MAC
	// per run: each test looks like a new device (consumes a DHCP lease and
	// clutters the AP's client table), which wpa_supplicant does by default.
	macAddr := "0"
	if opts.MACMode == "random" {
		macAddr = "1"
	}
	fmt.Fprintf(&b, "mac_addr=%s\n", macAddr)
	fmt.Fprintf(&b, "preassoc_mac_addr=%s\n", macAddr)
	b.WriteString("network={\n")
	fmt.Fprintf(&b, "  mac_addr=%s\n", macAddr)
	fmt.Fprintf(&b, "  ssid=\"%s\"\n", wpaEscape(opts.SSID))
	// Directed probe requests: finds the SSID faster and more reliably than
	// passive listening (and is required for hidden SSIDs)
	b.WriteString("  scan_ssid=1\n")
	switch opts.Security {
	case "eap-peap":
		b.WriteString("  key_mgmt=WPA-EAP\n")
		b.WriteString("  eap=PEAP\n")
		fmt.Fprintf(&b, "  identity=\"%s\"\n", wpaEscape(opts.Identity))
		fmt.Fprintf(&b, "  password=\"%s\"\n", wpaEscape(opts.Password))
		b.WriteString("  phase2=\"auth=MSCHAPV2\"\n")
		if caCertPath != "" {
			fmt.Fprintf(&b, "  ca_cert=\"%s\"\n", caCertPath)
		}
	case "open":
		b.WriteString("  key_mgmt=NONE\n")
	default: // "psk"
		// key_mgmt WPA-PSK + SAE covers WPA2, WPA3 and transition-mode APs
		b.WriteString("  key_mgmt=WPA-PSK SAE\n")
		b.WriteString("  ieee80211w=1\n")
		fmt.Fprintf(&b, "  psk=\"%s\"\n", wpaEscape(opts.Password))
	}
	b.WriteString("}\n")
	return b.String()
}

// wpaEscape neutralizes quote/backslash so values cannot break out of the
// quoted wpa_supplicant string.
func wpaEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// wpaEvent classifies one wpa_supplicant output line for step timing.
type wpaEvent int

const (
	wpaEventNone       wpaEvent = iota
	wpaEventTrying              // SSID found, starting to authenticate/associate
	wpaEventAssociated          // 802.11 association done
	wpaEventConnected           // key handshake / EAP completed
	wpaEventAssocFail           // association rejected / not found
	wpaEventAuthFail            // wrong credentials / EAP failure
)

// parseWpaEvent classifies a wpa_supplicant stdout line and extracts the
// BSSID when present.
func parseWpaEvent(line string) (wpaEvent, string) {
	switch {
	case strings.Contains(line, "Trying to associate"),
		strings.Contains(line, "Trying to authenticate"):
		return wpaEventTrying, ""
	case strings.Contains(line, "Associated with "):
		bssid := ""
		if i := strings.Index(line, "Associated with "); i >= 0 {
			rest := line[i+len("Associated with "):]
			if len(rest) >= 17 {
				bssid = rest[:17]
			}
		}
		return wpaEventAssociated, bssid
	case strings.Contains(line, "CTRL-EVENT-CONNECTED"):
		return wpaEventConnected, ""
	case strings.Contains(line, "CTRL-EVENT-ASSOC-REJECT"),
		strings.Contains(line, "CTRL-EVENT-AUTH-REJECT"),
		strings.Contains(line, "CTRL-EVENT-NETWORK-NOT-FOUND"):
		return wpaEventAssocFail, ""
	case strings.Contains(line, "CTRL-EVENT-SSID-TEMP-DISABLED"),
		strings.Contains(line, "CTRL-EVENT-EAP-FAILURE"),
		strings.Contains(line, "WRONG_KEY"),
		strings.Contains(line, "4WAY_HANDSHAKE_TIMEOUT"):
		return wpaEventAuthFail, ""
	}
	return wpaEventNone, ""
}
