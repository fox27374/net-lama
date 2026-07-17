//go:build linux

package probe

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/gopacket/gopacket/afpacket"
)

// senseImpl performs a monitor-mode sweep on Linux using afpacket + gopacket
// for frame capture and parsing (stations, rates, RSSI) via RadioTap + Dot11.
func senseImpl(ctx context.Context, iface string, channels []uint32, dwellMs uint32) (string, []WlanStation, []WlanChannelStat, []WlanNetwork, uint32, error) {
	if dwellMs == 0 {
		dwellMs = 400
	}

	// Get the interface's current type so we can restore it
	originalType, err := getInterfaceType(ctx, iface)
	if err != nil {
		return iface, nil, nil, nil, 0, fmt.Errorf("failed to detect interface type: %w", err)
	}

	// Set to monitor mode
	if originalType != "monitor" {
		if err := setInterfaceType(ctx, iface, "monitor"); err != nil {
			return iface, nil, nil, nil, 0, fmt.Errorf("failed to set monitor mode: %w", err)
		}
	}

	// Defer restoration of original type
	defer func() {
		if originalType != "monitor" {
			_ = setInterfaceType(context.Background(), iface, originalType)
		}
	}()

	// If channels is empty, derive from phy
	if len(channels) == 0 {
		var err error
		channels, err = getPhyChannels(ctx, iface)
		if err != nil {
			slog.Warn("Failed to get phy channels, using common defaults", "error", err)
			channels = []uint32{1, 6, 11, 36, 40, 44, 48, 149, 153, 157, 161}
		}
	}

	startTime := time.Now()
	stations := make(map[string]*WlanStation) // key: MAC address
	networks := make(map[string]*WlanNetwork)  // BSSID -> AP heard from beacons
	channelStats := make(map[uint32]*WlanChannelStat)

	// Initialize channel stats
	for _, ch := range channels {
		_, freq := channelToFreq(ch)
		if freq > 0 {
			channelStats[ch] = &WlanChannelStat{
				Channel: ch,
				FreqMHz: freq,
			}
		}
	}

	// Per-channel capture
	for _, ch := range channels {
		select {
		case <-ctx.Done():
			return iface, nil, nil, nil, 0, ctx.Err()
		default:
		}

		// Set channel
		if err := setChannel(ctx, iface, ch); err != nil {
			slog.Warn("Failed to set channel", "channel", ch, "error", err)
			continue
		}

		// Capture for dwell_ms
		capturedStations, capturedFrames, capturedNetworks, err := captureOnChannel(ctx, iface, time.Duration(dwellMs)*time.Millisecond)
		if err != nil {
			slog.Warn("Failed to capture on channel", "channel", ch, "error", err)
			continue
		}

		// Merge networks, stamping the channel they were heard on.
		_, chFreq := channelToFreq(ch)
		for bssid, n := range capturedNetworks {
			existing := networks[bssid]
			if existing == nil {
				n.Channel = ch
				n.FreqMHz = chFreq
				networks[bssid] = n
				continue
			}
			existing.Beacons += n.Beacons
			if n.SSID != "" {
				existing.SSID = n.SSID
			}
			if n.RSSIdBm > existing.RSSIdBm {
				existing.RSSIdBm = n.RSSIdBm
				existing.Channel = ch // strongest beacon wins the channel
				existing.FreqMHz = chFreq
			}
		}

		// Merge stations
		for mac, station := range capturedStations {
			if existing, ok := stations[mac]; ok {
				// Update existing station
				if station.RSSIdBm > existing.RSSIdBm {
					existing.RSSIdBm = station.RSSIdBm
				}
				// Incremental average RSSI
				if existing.RSSIAvgdBm == 0 {
					existing.RSSIAvgdBm = station.RSSIdBm
				} else {
					existing.RSSIAvgdBm = (existing.RSSIAvgdBm + station.RSSIdBm) / 2
				}
				if station.RateMbps > 0 {
					existing.RateMbps = station.RateMbps
				}
				if station.MCS >= 0 {
					existing.MCS = station.MCS
				}
				existing.Frames += station.Frames
				existing.LastSeenMs = station.LastSeenMs
				if station.BSSID != "" {
					existing.BSSID = station.BSSID
				}
			} else {
				stations[mac] = station
			}
		}

		// Update channel stats
		if stat, ok := channelStats[ch]; ok {
			stat.Frames += capturedFrames
		}
	}

	// Resolve SSIDs for stations
	for mac, station := range stations {
		if station.BSSID != "" {
			if n, ok := networks[station.BSSID]; ok {
				station.SSID = n.SSID
			}
		}
		stations[mac] = station
	}

	// Fetch survey data
	surveyData := getSurveyStats(ctx, iface)
	for ch, stat := range surveyData {
		if existing, ok := channelStats[ch]; ok {
			existing.ActiveMs = stat.ActiveMs
			existing.BusyMs = stat.BusyMs
			existing.UtilizationPct = stat.UtilizationPct
		}
	}

	// Convert map to slice and cap at ~256 stations
	stationList := make([]WlanStation, 0, len(stations))
	for _, s := range stations {
		stationList = append(stationList, *s)
		if len(stationList) >= 256 {
			break
		}
	}

	// Convert channel map to slice
	channelList := make([]WlanChannelStat, 0, len(channelStats))
	for _, ch := range channels {
		if stat, ok := channelStats[ch]; ok {
			channelList = append(channelList, *stat)
		}
	}

	networkList := make([]WlanNetwork, 0, len(networks))
	for _, n := range networks {
		networkList = append(networkList, *n)
		if len(networkList) >= 256 {
			break
		}
	}

	sweepTime := uint32(time.Since(startTime).Milliseconds())
	return iface, stationList, channelList, networkList, sweepTime, nil
}

// captureOnChannel captures frames on the current channel for a given duration
// using afpacket. Returns a map of MAC -> station data, total frame count,
// and BSSID -> SSID mappings.
func captureOnChannel(ctx context.Context, iface string, duration time.Duration) (map[string]*WlanStation, uint32, map[string]*WlanNetwork, error) {
	// Open packet handle with AF_PACKET socket (zero-copy)
	handle, err := afpacket.NewTPacket(
		afpacket.OptInterface(iface),
		afpacket.OptPollTimeout(time.Duration(100)*time.Millisecond),
		afpacket.OptFrameSize(65536),
		afpacket.OptBlockSize(1024 * 1024),
	)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to open afpacket: %w", err)
	}
	defer handle.Close()

	stations := make(map[string]*WlanStation)
	networks := make(map[string]*WlanNetwork) // BSSID -> AP
	frameCount := uint32(0)
	now := time.Now().UnixMilli()

	timeoutCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	// Capture loop
	for {
		select {
		case <-timeoutCtx.Done():
			return stations, frameCount, networks, nil
		default:
		}

		// Read frame (zero-copy first, fallback to normal read)
		data, _, err := handle.ZeroCopyReadPacketData()
		if err != nil {
			if strings.Contains(err.Error(), "not available") {
				// Fallback to regular read if zero-copy fails
				data, _, err = handle.ReadPacketData()
				if err != nil {
					select {
					case <-timeoutCtx.Done():
						return stations, frameCount, networks, nil
					default:
						continue
					}
				}
			} else {
				select {
				case <-timeoutCtx.Done():
					return stations, frameCount, networks, nil
				default:
					continue
				}
			}
		}

		frameCount++

		// Process frame using the cross-platform testable function
		processFrame(data, stations, networks, now)
	}
}

// getInterfaceType retrieves the current type (e.g. "managed", "monitor").
func getInterfaceType(ctx context.Context, iface string) (string, error) {
	out, err := exec.CommandContext(ctx, "iw", "dev", iface, "info").Output()
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "type ") {
			return strings.TrimPrefix(line, "type "), nil
		}
	}
	return "", fmt.Errorf("could not determine interface type")
}

// setInterfaceType sets the interface to the given type (monitor, managed, etc).
func setInterfaceType(ctx context.Context, iface, typeStr string) error {
	// Bring interface down first
	if err := exec.CommandContext(ctx, "ip", "link", "set", iface, "down").Run(); err != nil {
		// Ignore errors - interface might already be down
	}

	// Set the type
	if err := exec.CommandContext(ctx, "iw", "dev", iface, "set", "type", typeStr).Run(); err != nil {
		return err
	}

	// Bring interface up
	if err := exec.CommandContext(ctx, "ip", "link", "set", iface, "up").Run(); err != nil {
		return err
	}

	return nil
}

// setChannel sets the interface to a specific channel.
func setChannel(ctx context.Context, iface string, channel uint32) error {
	return exec.CommandContext(ctx, "iw", "dev", iface, "set", "channel", fmt.Sprintf("%d", channel)).Run()
}

// getPhyChannels retrieves available channels for the interface's phy.
func getPhyChannels(ctx context.Context, iface string) ([]uint32, error) {
	// First, get the phy name
	out, err := exec.CommandContext(ctx, "iw", "dev", iface, "info").Output()
	if err != nil {
		return nil, err
	}

	phy := ""
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "phy#") {
			phy = "phy" + strings.TrimPrefix(line, "phy#")
			break
		}
	}

	if phy == "" {
		return nil, fmt.Errorf("could not determine phy")
	}

	// Get channels for this phy
	out, err = exec.CommandContext(ctx, "iw", "phy", phy, "channels").Output()
	if err != nil {
		return nil, err
	}

	return parseIWPhyChannels(string(out)), nil
}

// getSurveyStats fetches per-frequency busy/active stats from `iw dev <if> survey dump`.
func getSurveyStats(ctx context.Context, iface string) map[uint32]WlanChannelStat {
	out, err := exec.CommandContext(ctx, "iw", "dev", iface, "survey", "dump").Output()
	if err != nil {
		// Survey data is optional - return empty map on error
		return make(map[uint32]WlanChannelStat)
	}

	surveyData := parseIWSurveyDump(string(out))

	// Convert frequency map to channel map
	result := make(map[uint32]WlanChannelStat)
	for _, stat := range surveyData {
		if stat.Channel > 0 {
			result[stat.Channel] = stat
		}
	}

	return result
}

// channelToFreq converts a channel number to a frequency in MHz.
func channelToFreq(channel uint32) (uint32, uint32) {
	switch {
	case channel == 14:
		return 14, 2484
	case channel >= 1 && channel <= 13:
		return channel, 2407 + channel*5
	case channel >= 36 && channel <= 165:
		return channel, 5000 + channel*5
	case channel >= 169 && channel <= 177:
		return channel, 5950 + channel*5
	default:
		return channel, 0
	}
}
