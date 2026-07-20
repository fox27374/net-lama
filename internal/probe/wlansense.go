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
	MAC        string  // station MAC address
	BSSID      string  // access point MAC, empty for probe-only
	SSID       string  // resolved network name when known
	RSSIdBm    int32   // last observed signal strength
	RSSIAvgdBm int32   // average RSSI over the sweep
	RateMbps   float64 // last observed data rate (0 = unknown)
	MCS        int32   // -1 = unknown/legacy
	Frames     uint32  // frame count
	ProbeOnly  bool    // true if only seen probing
	LastSeenMs int64   // unix milliseconds
}

// WlanRoamEvent is a client station's BSSID transition (roam) or its
// disappearance (disconnect, ToBSSID empty), detected by the agent from
// consecutive sweeps. RoamTimeMs is bounded by sweep cadence, not
// sub-100ms radio-handoff precision — see proto WlanRoamEvent comment.
type WlanRoamEvent struct {
	ClientMAC    string
	SSID         string
	FromBSSID    string
	ToBSSID      string
	FromChannel  uint32
	ToChannel    uint32
	FromRSSIdBm  int32
	ToRSSIdBm    int32
	RoamTimeMs   float64
	DetectedAtMs int64
}

type WlanChannelStat struct {
	Channel        uint32  // channel number (1-177)
	FreqMHz        uint32  // center frequency
	ActiveMs       uint64  // time the channel was active
	BusyMs         uint64  // time the channel was busy
	UtilizationPct float64 // busy/active*100, 0 if unavailable
	Frames         uint32  // frame count on this channel
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

	WidthMHz           uint32  // channel width from HT/VHT operation IEs (20/40/80/160)
	BeaconIntervalTU   uint32  // beacon interval in time units (1 TU = 1.024 ms)
	Country            string  // country code from the Country IE
	LoadPresent        bool    // BSS Load IE seen
	LoadStations       uint32  // associated station count reported by the AP
	LoadChannelUtilPct float64 // AP-reported channel utilization 0-100
	SecurityDetail     string  // AKM + cipher, e.g. "PSK+SAE · CCMP"
	Roaming            string  // 802.11 roaming amendments seen, e.g. "k/r/v"
	MFP                string  // management frame protection: "", "capable", "required"
	GroupCipher        string  // RSN group cipher, e.g. "CCMP"
	DTIMPeriod         uint32  // DTIM period from the TIM element
	WPS                bool    // WPS vendor element present
	Streams            uint32  // spatial streams from HT/VHT MCS maps (0 = unknown)
	MaxRateMbps        float64 // estimated max PHY rate from gen/width/streams
	LastSeenMs         int64   // unix ms of the last beacon heard
}

// Sense performs a monitor-mode sweep, capturing stations, per-channel
// utilization, and the networks (APs) heard from beacons. Returns interface
// name, stations, channel stats, networks, total sweep time, and error.
// Requires monitor-capable interface, NET_ADMIN + NET_RAW.
func Sense(ctx context.Context, iface string, channels []uint32, dwellMs uint32) (string, []WlanStation, []WlanChannelStat, []WlanNetwork, uint32, error) {
	if wlanDemo() {
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
func recordNetwork(networks map[string]*WlanNetwork, bssid string, info beaconInfo, rssi int32, nowMs int64) {
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
	if info.WidthMHz > n.WidthMHz {
		n.WidthMHz = info.WidthMHz
	}
	if info.BeaconIntervalTU > 0 {
		n.BeaconIntervalTU = info.BeaconIntervalTU
	}
	if info.Country != "" {
		n.Country = info.Country
	}
	if info.LoadPresent {
		n.LoadPresent = true
		n.LoadStations = info.LoadStations
		n.LoadChannelUtilPct = info.LoadChannelUtilPct
	}
	if info.SecurityDetail != "" {
		n.SecurityDetail = info.SecurityDetail
	}
	if info.Roaming != "" {
		n.Roaming = info.Roaming
	}
	if info.MFP != "" {
		n.MFP = info.MFP
	}
	if info.GroupCipher != "" {
		n.GroupCipher = info.GroupCipher
	}
	if info.DTIMPeriod > 0 {
		n.DTIMPeriod = info.DTIMPeriod
	}
	if info.WPS {
		n.WPS = true
	}
	if info.Streams > n.Streams {
		n.Streams = info.Streams
	}
	if info.MaxRateMbps > n.MaxRateMbps {
		n.MaxRateMbps = info.MaxRateMbps
	}
	if rssi > n.RSSIdBm {
		n.RSSIdBm = rssi
	}
	n.LastSeenMs = nowMs
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
		recordNetwork(networks, dot11.Address3.String(), parseBeaconBody(dot11Layer.LayerPayload()), rssi, nowMs)

	case layers.Dot11TypeMgmtProbeResp:
		// Probe response - same fixed-body layout as a beacon.
		recordNetwork(networks, dot11.Address3.String(), parseBeaconBody(dot11Layer.LayerPayload()), rssi, nowMs)

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

	WidthMHz           uint32 // 20/40 from HT operation, 80/160 from VHT operation
	BeaconIntervalTU   uint32 // fixed-body beacon interval
	Country            string // Country IE
	LoadPresent        bool   // BSS Load IE seen
	LoadStations       uint32
	LoadChannelUtilPct float64
	SecurityDetail     string  // AKM + pairwise cipher, e.g. "PSK+SAE · CCMP"
	Roaming            string  // "k/r/v" from RM/Mobility-Domain/BSS-Transition
	MFP                string  // "", "capable", "required" from RSN capabilities
	GroupCipher        string  // RSN group cipher name
	DTIMPeriod         uint32  // from the TIM element
	WPS                bool    // WPS vendor element present
	Streams            uint32  // spatial streams from HT/VHT MCS maps
	MaxRateMbps        float64 // estimated max PHY rate
}

// parseBeaconBody parses a beacon/probe-response frame body: 12 fixed bytes
// (timestamp 8, interval 2, capability 2), then TLV information elements
// (Tag 1 byte, Length 1 byte, Value).
func parseBeaconBody(body []byte) beaconInfo {
	var info beaconInfo
	if len(body) < 12 {
		return info
	}
	info.BeaconIntervalTU = uint32(body[8]) | uint32(body[9])<<8
	privacy := body[10]&0x10 != 0 // capability bit 4
	data := body[12:]

	var hasRSN, hasWPA bool
	var akms, ciphers []byte
	var ht, vht, he, eht bool
	var rm, ft, btm bool // 802.11k / 802.11r / 802.11v
	var htStreams, vhtStreams uint32
	var legacyMax float64
	info.WidthMHz = 20

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
		case 1, 50: // Supported Rates / Extended Supported Rates (500 kbps units)
			for _, r := range v {
				if mbps := float64(r&0x7F) * 0.5; mbps > legacyMax {
					legacyMax = mbps
				}
			}
		case 5: // TIM: DTIM count(1), DTIM period(1), ...
			if length >= 2 {
				info.DTIMPeriod = uint32(v[1])
			}
		case 7: // Country: 2-char code + environment byte
			if length >= 2 && v[0] >= 'A' && v[0] <= 'Z' && v[1] >= 'A' && v[1] <= 'Z' {
				info.Country = string(v[:2])
			}
		case 11: // BSS Load: station count(2 LE), channel utilization(1, /255)
			if length >= 3 {
				info.LoadPresent = true
				info.LoadStations = uint32(v[0]) | uint32(v[1])<<8
				info.LoadChannelUtilPct = float64(v[2]) / 255.0 * 100.0
			}
		case 45: // HT capabilities → 802.11n; MCS set at offset 3, one RX bitmask byte per stream
			ht = true
			for s := 0; s < 4 && 3+s < len(v); s++ {
				if v[3+s] != 0 {
					htStreams = uint32(s + 1)
				}
			}
		case 48: // RSN
			hasRSN = true
			r := parseRSN(v)
			akms = append(akms, r.akms...)
			ciphers = append(ciphers, r.ciphers...)
			if r.group != 0 {
				info.GroupCipher = cipherName(r.group)
			}
			if r.capsOK {
				switch {
				case r.caps&0x40 != 0: // MFPR
					info.MFP = "required"
				case r.caps&0x80 != 0: // MFPC
					info.MFP = "capable"
				}
			}
		case 54: // Mobility Domain → 802.11r
			ft = true
		case 61: // HT operation: secondary channel offset (byte1 bits 0-1) → 40 MHz
			if length >= 2 && v[1]&0x03 != 0 && info.WidthMHz < 40 {
				info.WidthMHz = 40
			}
		case 70: // RM Enabled Capabilities → 802.11k
			rm = true
		case 127: // Extended Capabilities: bit 19 = BSS Transition → 802.11v
			if length >= 3 && v[2]&0x08 != 0 {
				btm = true
			}
		case 191: // VHT capabilities → 802.11ac; Rx MCS map at bytes 4-5 (2 bits/stream, 3=unsupported)
			vht = true
			if length >= 6 {
				rxMap := uint16(v[4]) | uint16(v[5])<<8
				for s := 0; s < 8; s++ {
					if (rxMap>>(2*s))&0x3 != 0x3 {
						vhtStreams = uint32(s + 1)
					}
				}
			}
		case 192: // VHT operation: width byte 1+ → 80, CCFS1 set → 160
			if length >= 3 && v[0] >= 1 {
				if v[2] != 0 || v[0] >= 2 {
					info.WidthMHz = 160
				} else if info.WidthMHz < 80 {
					info.WidthMHz = 80
				}
			}
		case 221: // vendor elements (Microsoft OUI 00:50:F2: type 1 = WPA1, type 4 = WPS)
			if length >= 4 && v[0] == 0x00 && v[1] == 0x50 && v[2] == 0xF2 {
				switch v[3] {
				case 0x01:
					hasWPA = true
				case 0x04:
					info.WPS = true
				}
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
	// ponytail: HE-operation 6 GHz width not parsed; 6 GHz APs report the HT/VHT-derived width

	info.Security = securityLabel(privacy, hasWPA, hasRSN, akms)
	info.SecurityDetail = securityDetail(akms, ciphers)
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

	var roam []string
	if rm {
		roam = append(roam, "k")
	}
	if ft {
		roam = append(roam, "r")
	}
	if btm {
		roam = append(roam, "v")
	}
	info.Roaming = strings.Join(roam, "/")

	info.Streams = max(htStreams, vhtStreams)
	info.MaxRateMbps = estimateMaxRate(ht, vht, he || eht, info.WidthMHz, info.Streams, legacyMax)
	return info
}

// estimateMaxRate estimates the max PHY rate (Mbps) from the newest PHY
// generation, channel width, and spatial streams. Top-MCS short-GI rates per
// stream; legacy APs report the highest advertised supported rate.
// ponytail: HE streams aren't parsed from HE caps — 6 GHz-only ax APs fall
// back to the HT/VHT stream count (or 1); refine when HE cap parsing lands.
func estimateMaxRate(ht, vht, he bool, widthMHz, streams uint32, legacyMax float64) float64 {
	perStream := map[uint32]float64{}
	switch {
	case he: // 802.11ax MCS 11
		perStream = map[uint32]float64{20: 143.4, 40: 287.1, 80: 600.5, 160: 1201.0}
	case vht: // 802.11ac MCS 9
		perStream = map[uint32]float64{20: 86.7, 40: 200.0, 80: 433.3, 160: 866.7}
	case ht: // 802.11n MCS 7 short-GI
		perStream = map[uint32]float64{20: 72.2, 40: 150.0}
	default:
		return legacyMax
	}
	rate, ok := perStream[widthMHz]
	if !ok { // clamp to the widest rate the table knows below the width
		for w, r := range perStream {
			if w <= widthMHz && r > rate {
				rate = r
			}
		}
	}
	if streams == 0 {
		streams = 1
	}
	return rate * float64(streams)
}

// rsnInfo is what parseRSN extracts from an RSN element body.
type rsnInfo struct {
	akms    []byte // AKM suite types
	ciphers []byte // pairwise cipher suite types
	group   byte   // group cipher suite type (0 = absent)
	caps    uint16 // RSN capabilities field
	capsOK  bool   // capabilities field present
}

// parseRSN parses an RSN element body: version(2) group-cipher(4)
// pairwise-count(2) pairwise(4n) akm-count(2) akm(4m) rsn-caps(2);
// the type is the 4th byte of each 00-0F-AC-XX suite.
func parseRSN(v []byte) rsnInfo {
	var r rsnInfo
	if len(v) >= 6 {
		r.group = v[5]
	}
	off := 2 + 4
	if off+2 > len(v) {
		return r
	}
	pairwise := int(v[off]) | int(v[off+1])<<8
	off += 2
	for j := 0; j < pairwise && off+4 <= len(v); j++ {
		r.ciphers = append(r.ciphers, v[off+3])
		off += 4
	}
	if off+2 > len(v) {
		return r
	}
	akmCount := int(v[off]) | int(v[off+1])<<8
	off += 2
	for j := 0; j < akmCount && off+4 <= len(v); j++ {
		r.akms = append(r.akms, v[off+3])
		off += 4
	}
	if off+2 <= len(v) {
		r.caps = uint16(v[off]) | uint16(v[off+1])<<8
		r.capsOK = true
	}
	return r
}

var cipherNames = map[byte]string{
	1: "WEP-40", 2: "TKIP", 4: "CCMP", 5: "WEP-104",
	8: "GCMP", 9: "GCMP-256", 10: "CCMP-256",
}

// cipherName maps an RSN cipher suite type (00-0F-AC-XX) to its display name.
func cipherName(b byte) string {
	if s, ok := cipherNames[b]; ok {
		return s
	}
	return "type-" + strconv.Itoa(int(b))
}

// securityDetail renders AKM and pairwise cipher names, e.g. "PSK+SAE · CCMP".
func securityDetail(akms, ciphers []byte) string {
	akmNames := map[byte]string{
		1: "802.1X", 2: "PSK", 3: "FT-802.1X", 4: "FT-PSK", 5: "802.1X-SHA256",
		6: "PSK-SHA256", 8: "SAE", 9: "FT-SAE", 11: "802.1X-SuiteB", 12: "802.1X-SuiteB-192",
		13: "FT-802.1X-SHA384", 18: "OWE",
	}
	name := func(m map[byte]string, b byte) string {
		if s, ok := m[b]; ok {
			return s
		}
		return "type-" + strconv.Itoa(int(b))
	}
	var aParts, cParts []string
	seen := map[string]bool{}
	for _, a := range akms {
		if s := name(akmNames, a); !seen["a"+s] {
			seen["a"+s] = true
			aParts = append(aParts, s)
		}
	}
	for _, c := range ciphers {
		if s := name(cipherNames, c); !seen["c"+s] {
			seen["c"+s] = true
			cParts = append(cParts, s)
		}
	}
	if len(aParts) == 0 {
		return ""
	}
	out := strings.Join(aParts, "+")
	if len(cParts) > 0 {
		out += " · " + strings.Join(cParts, "/")
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
