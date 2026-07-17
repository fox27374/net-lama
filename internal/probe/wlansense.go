package probe

import (
	"bufio"
	"context"
	"strconv"
	"strings"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// WlanSenseDemo holds synthetic WLAN monitor-mode sensing data for testing.
type WlanSenseDemo struct {
	Interface string
	Stations  []WlanStation
	Channels  []WlanChannelStat
	SweepMs   uint32
}

type WlanStation struct {
	MAC       string  // station MAC address
	BSSID     string  // access point MAC, empty for probe-only
	SSID      string  // resolved network name when known
	RSSIdBm   int32   // last observed signal strength
	RSSIAvgdBm int32  // average RSSI over the sweep
	RateMbps  float64 // last observed data rate (0 = unknown)
	MCS       int32   // -1 = unknown/legacy
	Frames    uint32  // frame count
	ProbeOnly bool    // true if only seen probing
	LastSeenMs int64  // unix milliseconds
}

type WlanChannelStat struct {
	Channel       uint32  // channel number (1-177)
	FreqMHz       uint32  // center frequency
	ActiveMs      uint64  // time the channel was active
	BusyMs        uint64  // time the channel was busy
	UtilizationPct float64 // busy/active*100, 0 if unavailable
	Frames        uint32  // frame count on this channel
}

// WlanNetwork is an access point heard from its beacons/probe responses.
type WlanNetwork struct {
	BSSID     string // access point MAC
	SSID      string // network name, empty = hidden
	Channel   uint32 // channel it was heard on (stamped by the sweep)
	FreqMHz   uint32
	RSSIdBm   int32  // strongest beacon RSSI
	Beacons   uint32 // beacon/probe-response frames seen
	Security  string // "Open", "WEP", "WPA2", "WPA2/WPA3", "WPA3", ...
	Standards string // PHY generations from IEs, e.g. "n/ac/ax"
}

// wlanSenseDemo reports whether to emit synthetic WLAN sense data.
func wlanSenseDemo() bool {
	return envEnabled("NETLAMA_WLAN_SENSE_DEMO")
}

// DemoModeWlanSense reports whether WLAN sense results are synthetic.
func DemoModeWlanSense() bool {
	return wlanSenseDemo()
}

// Sense performs a monitor-mode sweep, capturing stations, per-channel
// utilization, and the networks (APs) heard from beacons. Returns interface
// name, stations, channel stats, networks, total sweep time, and error.
// Requires monitor-capable interface, NET_ADMIN + NET_RAW.
func Sense(ctx context.Context, iface string, channels []uint32, dwellMs uint32) (string, []WlanStation, []WlanChannelStat, []WlanNetwork, uint32, error) {
	if wlanSenseDemo() {
		return demoSense(iface)
	}
	// Real implementation is in wlansense_linux.go or wlansense_other.go
	return senseImpl(ctx, iface, channels, dwellMs)
}

// parsePhyName finds the wiphy a interface belongs to. `iw dev <iface> info`
// reports "wiphy N" (→ "phyN"); bare `iw dev` uses "phy#N". Handle both.
func parsePhyName(iwDevInfo string) string {
	scanner := bufio.NewScanner(strings.NewReader(iwDevInfo))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if v, ok := strings.CutPrefix(line, "wiphy "); ok {
			return "phy" + strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "phy#"); ok {
			return "phy" + strings.TrimSpace(v)
		}
	}
	return ""
}

// parseIWPhyChannels extracts available channels from `iw phy <phy> channels` output.
// Returns sorted list (2.4 GHz first, then 5 GHz) and omits disabled/radar-only channels.
func parseIWPhyChannels(out string) []uint32 {
	var channels []uint32
	seen := make(map[uint32]bool)

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Frequency lines from `iw phy <phy> channels` look like:
		//   "* 2412 MHz [1] "  or  "5500 MHz [100] (radar detection)".
		// The frequency is the token immediately before "MHz" (a leading
		// "*" marks a usable channel). Skip only explicitly disabled ones;
		// DFS/"No IR"/radar channels are fine for passive monitor capture.
		if !strings.Contains(line, "MHz [") || strings.Contains(line, "disabled") {
			continue
		}
		fields := strings.Fields(line)
		for i, f := range fields {
			if f != "MHz" || i == 0 {
				continue
			}
			freq, err := strconv.ParseUint(fields[i-1], 10, 32)
			if err != nil {
				break
			}
			if ch, _ := channelAndBand(uint32(freq)); ch > 0 && !seen[ch] {
				channels = append(channels, ch)
				seen[ch] = true
			}
			break
		}
	}

	// Sort 2.4 GHz first, then 5 GHz
	var sorted24, sorted5 []uint32
	for _, ch := range channels {
		if ch <= 14 {
			sorted24 = append(sorted24, ch)
		} else {
			sorted5 = append(sorted5, ch)
		}
	}
	return append(sorted24, sorted5...)
}

// parseIWSurveyDump extracts per-frequency busy/active time from `iw dev <if> survey dump`.
// Returns a map of frequency -> (activems, busyms, utilization%).
func parseIWSurveyDump(out string) map[uint32]WlanChannelStat {
	result := make(map[uint32]WlanChannelStat)

	var freq uint32
	var activems, busyms uint64

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "frequency:") {
			// Parse "frequency:        2412 MHz [1]"
			parts := strings.Fields(strings.TrimPrefix(line, "frequency:"))
			if len(parts) > 0 {
				if f, err := strconv.ParseUint(parts[0], 10, 32); err == nil {
					freq = uint32(f)
				}
			}
		} else if strings.HasPrefix(line, "channel active time:") {
			// Parse "channel active time: 12345 ms"
			parts := strings.Fields(strings.TrimPrefix(line, "channel active time:"))
			if len(parts) > 0 {
				if a, err := strconv.ParseUint(parts[0], 10, 64); err == nil {
					activems = a
				}
			}
		} else if strings.HasPrefix(line, "channel busy time:") {
			// Parse "channel busy time: 1234 ms"
			parts := strings.Fields(strings.TrimPrefix(line, "channel busy time:"))
			if len(parts) > 0 {
				if b, err := strconv.ParseUint(parts[0], 10, 64); err == nil {
					busyms = b
					// When we see busy time, we have a complete entry
					if freq > 0 && activems > 0 {
						ch, _ := channelAndBand(freq)
						util := 0.0
						if activems > 0 {
							util = float64(busyms) / float64(activems) * 100.0
						}
						result[freq] = WlanChannelStat{
							Channel:        ch,
							FreqMHz:        freq,
							ActiveMs:       activems,
							BusyMs:         busyms,
							UtilizationPct: util,
							Frames:         0,
						}
						freq, activems, busyms = 0, 0, 0
					}
				}
			}
		}
	}

	return result
}

// processFrame parses a single 802.11 frame (RadioTap + Dot11) and updates
// the station and SSID maps. This is the core frame-parsing logic, extracted
// for testability (no build tags, works cross-platform with gopacket).
// isUnicastMAC reports whether a MAC string is a real per-station address:
// not empty, not all-zero, and not group-addressed (the multicast/broadcast
// bit — the low bit of the first octet — must be clear).
func isUnicastMAC(mac string) bool {
	if len(mac) < 2 || mac == "00:00:00:00:00:00" {
		return false
	}
	first, err := strconv.ParseUint(mac[0:2], 16, 8)
	if err != nil {
		return false
	}
	return first&0x01 == 0
}

// recordNetwork upserts an AP heard from a beacon/probe-response, keeping the
// strongest RSSI and non-empty SSID/security/standards once seen.
func recordNetwork(networks map[string]*WlanNetwork, bssid string, info beaconInfo, rssi int32) {
	if !isUnicastMAC(bssid) {
		return // broadcast/multicast BSSID isn't a real AP
	}
	n := networks[bssid]
	if n == nil {
		n = &WlanNetwork{BSSID: bssid, RSSIdBm: rssi}
		networks[bssid] = n
	}
	if info.SSID != "" {
		n.SSID = info.SSID
	}
	if info.Security != "" {
		n.Security = info.Security
	}
	if info.Standards != "" {
		n.Standards = info.Standards
	}
	if rssi > n.RSSIdBm {
		n.RSSIdBm = rssi
	}
	n.Beacons++
}

func processFrame(data []byte, stations map[string]*WlanStation, networks map[string]*WlanNetwork, nowMs int64) {
	// Parse RadioTap + Dot11
	packet := gopacket.NewPacket(data, layers.LayerTypeRadioTap, gopacket.NoCopy)

	// Get RadioTap layer for metadata (RSSI, rate, etc.)
	radiotapLayer := packet.Layer(layers.LayerTypeRadioTap)
	if radiotapLayer == nil {
		return
	}
	radiotap := radiotapLayer.(*layers.RadioTap)

	// Skip FCS-failed frames
	for _, rt := range radiotap.RadioTapValues {
		if rt.Flags.BadFCS() {
			return
		}
	}

	// Extract RSSI, rate, MCS from RadioTap values
	var rssi int32 = -50 // Default fallback
	var rateMbps float64
	var mcs int32 = -1

	for _, rt := range radiotap.RadioTapValues {
		if rssi == -50 && rt.DBMAntennaSignal != 0 {
			rssi = int32(rt.DBMAntennaSignal)
		}
		if rt.Rate != 0 {
			// Rate is in 500 kbps units
			rateMbps = float64(rt.Rate) * 0.5
		}
		if rt.MCS.Known != 0 {
			mcs = int32(rt.MCS.MCS)
		}
	}

	// Get Dot11 layer for frame details
	dot11Layer := packet.Layer(layers.LayerTypeDot11)
	if dot11Layer == nil {
		return
	}
	dot11 := dot11Layer.(*layers.Dot11)

	// Process based on frame type
	switch dot11.Type {
	case layers.Dot11TypeMgmtBeacon:
		// Beacon frame - Address3 is the BSSID; the fixed body (timestamp,
		// interval, capability = 12 bytes) precedes the tagged elements.
		recordNetwork(networks, dot11.Address3.String(), parseBeaconBody(dot11Layer.LayerPayload()), rssi)

	case layers.Dot11TypeMgmtProbeResp:
		// Probe response - same fixed-body layout as a beacon.
		recordNetwork(networks, dot11.Address3.String(), parseBeaconBody(dot11Layer.LayerPayload()), rssi)

	case layers.Dot11TypeMgmtProbeReq:
		// Probe request - station is transmitter (Address2)
		mac := dot11.Address2.String()
		if !isUnicastMAC(mac) {
			return
		}
		if _, ok := stations[mac]; !ok {
			stations[mac] = &WlanStation{
				MAC:        mac,
				ProbeOnly:  true,
				RSSIdBm:    rssi,
				LastSeenMs: nowMs,
			}
		}

	case layers.Dot11TypeData:
		// Data frame - station is transmitter, BSSID is receiver/AP
		// Handle ToDS/FromDS address ordering per 802.11 spec
		if len(dot11.Address1) > 0 && len(dot11.Address2) > 0 && len(dot11.Address3) > 0 {
			var stationMAC, bssidMAC string

			// Frame Control flags for addressing (Flags is the single
			// FC flags byte; ToDS/FromDS are its low two bits).
			toDS := dot11.Flags.ToDS()
			fromDS := dot11.Flags.FromDS()

			if toDS && !fromDS {
				// STA -> AP: Address2 = STA, Address1 = AP
				stationMAC = dot11.Address2.String()
				bssidMAC = dot11.Address1.String()
			} else if !toDS && fromDS {
				// AP -> STA: Address1 = STA, Address2 = AP
				stationMAC = dot11.Address1.String()
				bssidMAC = dot11.Address2.String()
			} else if toDS && fromDS {
				// AP -> AP (WDS): Address2 = STA (or repeater)
				stationMAC = dot11.Address2.String()
				bssidMAC = dot11.Address1.String()
			} else {
				// Ad-hoc or unknown: Address2 = transmitter
				stationMAC = dot11.Address2.String()
				bssidMAC = dot11.Address3.String()
			}

			// Broadcast/multicast destinations (e.g. an AP's broadcast data
			// frame in FromDS) are not client stations.
			if !isUnicastMAC(stationMAC) {
				return
			}
			if _, ok := stations[stationMAC]; !ok {
				stations[stationMAC] = &WlanStation{
					MAC:        stationMAC,
					BSSID:      bssidMAC,
					RSSIdBm:    rssi,
					RateMbps:   rateMbps,
					MCS:        mcs,
					Frames:     1,
					LastSeenMs: nowMs,
				}
			} else {
				existing := stations[stationMAC]
				if existing.RSSIdBm < rssi {
					existing.RSSIdBm = rssi
				}
				if rateMbps > 0 {
					existing.RateMbps = rateMbps
				}
				if mcs >= 0 {
					existing.MCS = mcs
				}
				existing.Frames++
				existing.LastSeenMs = nowMs
			}
		}
	}
}

// beaconInfo is what parseBeaconBody extracts from a beacon/probe-response body.
type beaconInfo struct {
	SSID      string
	Security  string // derived from RSN/WPA elements + privacy capability bit
	Standards string // PHY generations from HT/VHT/HE/EHT elements, "n/ac/ax"
}

// parseBeaconBody parses a beacon/probe-response frame body: 12 fixed bytes
// (timestamp 8, interval 2, capability 2), then TLV information elements
// (Tag 1 byte, Length 1 byte, Value).
func parseBeaconBody(body []byte) beaconInfo {
	var info beaconInfo
	if len(body) < 12 {
		return info
	}
	privacy := body[10]&0x10 != 0 // capability bit 4
	data := body[12:]

	var hasRSN, hasWPA bool
	var akms []byte
	var ht, vht, he, eht bool

	for i := 0; i+2 <= len(data); {
		tag, length := data[i], int(data[i+1])
		if i+2+length > len(data) {
			break
		}
		v := data[i+2 : i+2+length]
		switch tag {
		case 0: // SSID
			if length > 0 {
				info.SSID = string(v)
			}
		case 45: // HT capabilities → 802.11n
			ht = true
		case 48: // RSN
			hasRSN = true
			akms = append(akms, parseRSNAKMs(v)...)
		case 191: // VHT capabilities → 802.11ac
			vht = true
		case 221: // vendor: Microsoft WPA1 OUI 00:50:F2 type 1
			if length >= 4 && v[0] == 0x00 && v[1] == 0x50 && v[2] == 0xF2 && v[3] == 0x01 {
				hasWPA = true
			}
		case 255: // element ID extension
			if length >= 1 {
				switch v[0] {
				case 35: // HE capabilities → 802.11ax
					he = true
				case 108: // EHT capabilities → 802.11be
					eht = true
				}
			}
		}
		i += 2 + length
	}

	info.Security = securityLabel(privacy, hasWPA, hasRSN, akms)
	var gens []string
	if ht {
		gens = append(gens, "n")
	}
	if vht {
		gens = append(gens, "ac")
	}
	if he {
		gens = append(gens, "ax")
	}
	if eht {
		gens = append(gens, "be")
	}
	info.Standards = strings.Join(gens, "/")
	return info
}

// parseRSNAKMs pulls the AKM suite-type bytes out of an RSN element body:
// version(2) group-cipher(4) pairwise-count(2) pairwise(4n) akm-count(2)
// akm(4m); the type is the 4th byte of each 00-0F-AC-XX suite.
func parseRSNAKMs(v []byte) []byte {
	off := 2 + 4
	if off+2 > len(v) {
		return nil
	}
	pairwise := int(v[off]) | int(v[off+1])<<8
	off += 2 + 4*pairwise
	if off+2 > len(v) {
		return nil
	}
	akmCount := int(v[off]) | int(v[off+1])<<8
	off += 2
	var out []byte
	for j := 0; j < akmCount && off+4 <= len(v); j++ {
		out = append(out, v[off+3])
		off += 4
	}
	return out
}

// securityLabel maps parsed security signals to a display string.
func securityLabel(privacy, hasWPA, hasRSN bool, akms []byte) string {
	if !hasRSN && !hasWPA {
		if privacy {
			return "WEP"
		}
		return "Open"
	}
	var psk, sae, dot1x, suiteB, owe bool
	for _, a := range akms {
		switch a {
		case 1, 5: // 802.1X, 802.1X-SHA256
			dot1x = true
		case 2, 6: // PSK, PSK-SHA256
			psk = true
		case 8, 9: // SAE, FT-SAE
			sae = true
		case 12, 13: // Suite-B-192
			suiteB = true
		case 18: // OWE
			owe = true
		}
	}
	switch {
	case owe:
		return "OWE"
	case sae && psk:
		return "WPA2/WPA3"
	case sae:
		return "WPA3"
	case suiteB:
		return "WPA3-Ent"
	case dot1x:
		return "WPA2-Ent"
	case psk && hasWPA:
		return "WPA/WPA2"
	case hasRSN:
		return "WPA2"
	default:
		return "WPA"
	}
}
