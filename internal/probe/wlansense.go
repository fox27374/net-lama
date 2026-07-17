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
	BSSID   string // access point MAC
	SSID    string // network name, empty = hidden
	Channel uint32 // channel it was heard on (stamped by the sweep)
	FreqMHz uint32
	RSSIdBm int32  // strongest beacon RSSI
	Beacons uint32 // beacon/probe-response frames seen
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
// strongest RSSI and a non-empty SSID once seen.
func recordNetwork(networks map[string]*WlanNetwork, bssid, ssid string, rssi int32) {
	if !isUnicastMAC(bssid) {
		return // broadcast/multicast BSSID isn't a real AP
	}
	n := networks[bssid]
	if n == nil {
		n = &WlanNetwork{BSSID: bssid, RSSIdBm: rssi}
		networks[bssid] = n
	}
	if ssid != "" {
		n.SSID = ssid
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
		// interval, capability = 12 bytes) precedes the tagged SSID element.
		bssid := dot11.Address3.String()
		ssid := ""
		if payload := dot11Layer.LayerPayload(); len(payload) > 12 {
			ssid = parseSSIDFromElements(payload[12:])
		}
		recordNetwork(networks, bssid, ssid, rssi)

	case layers.Dot11TypeMgmtProbeResp:
		// Probe response - same fixed-body layout as a beacon.
		bssid := dot11.Address3.String()
		ssid := ""
		if payload := dot11Layer.LayerPayload(); len(payload) > 12 {
			ssid = parseSSIDFromElements(payload[12:])
		}
		recordNetwork(networks, bssid, ssid, rssi)

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

// parseSSIDFromElements parses 802.11 information elements to extract SSID.
// Elements are TLV encoded: Tag (1 byte), Length (1 byte), Value (Length bytes).
func parseSSIDFromElements(data []byte) string {
	for i := 0; i < len(data)-1; {
		tag := data[i]
		length := int(data[i+1])
		if i+2+length > len(data) {
			break
		}

		// Tag 0 = SSID
		if tag == 0 && length > 0 {
			return string(data[i+2 : i+2+length])
		}

		i += 2 + length
	}
	return ""
}
