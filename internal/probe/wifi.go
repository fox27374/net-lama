package probe

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type WirelessInterface struct {
	Name            string
	PHY             string
	SupportsMonitor bool
}

type AccessPoint struct {
	BSSID    string
	SSID     string
	Channel  uint32
	FreqMHz  uint32
	Band     string
	RSSIdBm  float64
	Security string
}

// wlanDemo reports whether the agent should emit synthetic WLAN data
// (for pipeline testing on hosts without a wireless radio).
func wlanDemo() bool {
	_, ok := os.LookupEnv("NETLAMA_WLAN_DEMO")
	return ok
}

// DemoMode reports whether WLAN results are synthetic, so they can be
// labelled as such in the UI.
func DemoMode() bool { return wlanDemo() }

// WirelessInterfaces enumerates the host's wireless interfaces via `iw dev`.
// Returns an empty list (no error) when iw is absent or there is no radio.
func WirelessInterfaces(ctx context.Context) []WirelessInterface {
	if wlanDemo() {
		return demoInterfaces()
	}
	if _, err := exec.LookPath("iw"); err != nil {
		return nil
	}
	out, err := exec.CommandContext(ctx, "iw", "dev").Output()
	if err != nil {
		return nil
	}
	return parseIWDev(string(out))
}

// Scan performs a managed-mode scan of nearby access points on iface.
// Requires iw and NET_ADMIN. Needs no monitor mode.
func Scan(ctx context.Context, iface string) (string, []AccessPoint, error) {
	if wlanDemo() {
		return demoScan(iface)
	}
	out, err := exec.CommandContext(ctx, "iw", "dev", iface, "scan").Output()
	if err != nil {
		return iface, nil, err
	}
	return iface, parseIWScan(string(out)), nil
}

// parseIWDev parses `iw dev` output into a list of interfaces.
func parseIWDev(out string) []WirelessInterface {
	var ifaces []WirelessInterface
	var phy string
	var cur *WirelessInterface

	flush := func() {
		if cur != nil {
			ifaces = append(ifaces, *cur)
			cur = nil
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "phy#"):
			phy = "phy" + strings.TrimPrefix(line, "phy#")
		case strings.HasPrefix(line, "Interface "):
			flush()
			cur = &WirelessInterface{Name: strings.TrimPrefix(line, "Interface "), PHY: phy}
		case strings.HasPrefix(line, "type ") && cur != nil:
			// A monitor-typed interface obviously supports monitor mode;
			// full capability detection (iw list) is deferred to phase 2.
			if strings.TrimPrefix(line, "type ") == "monitor" {
				cur.SupportsMonitor = true
			}
		}
	}
	flush()
	return ifaces
}

// parseIWScan parses `iw dev <iface> scan` output into access points.
func parseIWScan(out string) []AccessPoint {
	var aps []AccessPoint
	var cur *AccessPoint
	var hasRSN, hasWPA, hasSAE, privacy bool

	flush := func() {
		if cur == nil {
			return
		}
		cur.Security = deriveSecurity(hasRSN, hasWPA, hasSAE, privacy)
		if cur.Channel == 0 && cur.FreqMHz != 0 {
			cur.Channel, cur.Band = channelAndBand(cur.FreqMHz)
		}
		aps = append(aps, *cur)
		cur = nil
		hasRSN, hasWPA, hasSAE, privacy = false, false, false, false
	}

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "BSS "):
			flush()
			bssid := strings.TrimPrefix(line, "BSS ")
			if i := strings.IndexAny(bssid, "(( "); i >= 0 {
				bssid = bssid[:i]
			}
			cur = &AccessPoint{BSSID: strings.ToLower(bssid)}
		case cur == nil:
			continue
		case strings.HasPrefix(line, "freq:"):
			if f, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, "freq:")), 64); err == nil {
				cur.FreqMHz = uint32(f)
				cur.Channel, cur.Band = channelAndBand(cur.FreqMHz)
			}
		case strings.HasPrefix(line, "signal:"):
			field := strings.Fields(strings.TrimPrefix(line, "signal:"))
			if len(field) > 0 {
				if s, err := strconv.ParseFloat(field[0], 64); err == nil {
					cur.RSSIdBm = s
				}
			}
		case strings.HasPrefix(line, "SSID:"):
			cur.SSID = strings.TrimSpace(strings.TrimPrefix(line, "SSID:"))
		case strings.HasPrefix(line, "RSN:"):
			hasRSN = true
		case strings.HasPrefix(line, "WPA:"):
			hasWPA = true
		case strings.Contains(line, "SAE"):
			hasSAE = true
		case strings.Contains(line, "Privacy"):
			privacy = true
		}
	}
	flush()
	return aps
}

func deriveSecurity(rsn, wpa, sae, privacy bool) string {
	switch {
	case rsn && sae:
		return "WPA3"
	case rsn:
		return "WPA2"
	case wpa:
		return "WPA"
	case privacy:
		return "WEP"
	default:
		return "Open"
	}
}

// channelAndBand converts a frequency in MHz to a channel number and band.
func channelAndBand(freq uint32) (uint32, string) {
	switch {
	case freq == 2484:
		return 14, "2.4 GHz"
	case freq >= 2412 && freq <= 2472:
		return (freq - 2407) / 5, "2.4 GHz"
	case freq >= 5160 && freq <= 5885:
		return (freq - 5000) / 5, "5 GHz"
	case freq >= 5955 && freq <= 7115:
		return (freq - 5950) / 5, "6 GHz"
	default:
		return 0, ""
	}
}
