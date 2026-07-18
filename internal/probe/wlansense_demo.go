package probe

import (
	"math/rand"
	"time"
)

// Synthetic WLAN monitor-mode sensing data for pipeline testing on hosts without
// a monitor-capable radio, enabled with NETLAMA_WLAN_DEMO.

func demoSense(iface string) (string, []WlanStation, []WlanChannelStat, []WlanNetwork, uint32, error) {
	now := time.Now().UnixMilli()

	// Realistic synthetic stations: ~8-15 across 2-4 BSSs
	stations := []WlanStation{
		{
			MAC:        "aa:bb:cc:dd:ee:01",
			BSSID:      "a0:f8:49:74:8b:20",
			SSID:       "corp-wifi",
			RSSIdBm:    int32(-45 - rand.Intn(15)),
			RSSIAvgdBm: int32(-48 - rand.Intn(15)),
			RateMbps:   float64(120 + rand.Intn(40)),
			MCS:        int32(7 + rand.Intn(5)),
			Frames:     uint32(200 + rand.Intn(300)),
			ProbeOnly:  false,
			LastSeenMs: now,
		},
		{
			MAC:        "aa:bb:cc:dd:ee:02",
			BSSID:      "a0:f8:49:74:8b:20",
			SSID:       "corp-wifi",
			RSSIdBm:    int32(-55 - rand.Intn(20)),
			RSSIAvgdBm: int32(-58 - rand.Intn(20)),
			RateMbps:   float64(50 + rand.Intn(50)),
			MCS:        int32(2 + rand.Intn(6)),
			Frames:     uint32(100 + rand.Intn(150)),
			ProbeOnly:  false,
			LastSeenMs: now,
		},
		{
			MAC:        "aa:bb:cc:dd:ee:03",
			BSSID:      "c0:25:5c:ec:bb:40",
			SSID:       "corp-wifi",
			RSSIdBm:    int32(-50 - rand.Intn(15)),
			RSSIAvgdBm: int32(-53 - rand.Intn(15)),
			RateMbps:   float64(300 + rand.Intn(50)),
			MCS:        int32(8 + rand.Intn(4)),
			Frames:     uint32(400 + rand.Intn(200)),
			ProbeOnly:  false,
			LastSeenMs: now,
		},
		{
			MAC:        "aa:bb:cc:dd:ee:04",
			BSSID:      "c0:25:5c:ec:bb:40",
			SSID:       "corp-wifi",
			RSSIdBm:    int32(-60 - rand.Intn(20)),
			RSSIAvgdBm: int32(-63 - rand.Intn(20)),
			RateMbps:   float64(80 + rand.Intn(40)),
			MCS:        int32(4 + rand.Intn(5)),
			Frames:     uint32(120 + rand.Intn(180)),
			ProbeOnly:  false,
			LastSeenMs: now,
		},
		{
			MAC:        "aa:bb:cc:dd:ee:05",
			BSSID:      "2c:3a:fd:8b:1e:56",
			SSID:       "IoT-Net",
			RSSIdBm:    int32(-70 - rand.Intn(15)),
			RSSIAvgdBm: int32(-72 - rand.Intn(15)),
			RateMbps:   float64(20 + rand.Intn(20)),
			MCS:        int32(-1), // unknown
			Frames:     uint32(50 + rand.Intn(80)),
			ProbeOnly:  false,
			LastSeenMs: now,
		},
		{
			MAC:        "aa:bb:cc:dd:ee:06",
			BSSID:      "a0:f8:49:74:8b:22",
			SSID:       "corp-guest",
			RSSIdBm:    int32(-52 - rand.Intn(18)),
			RSSIAvgdBm: int32(-55 - rand.Intn(18)),
			RateMbps:   float64(90 + rand.Intn(50)),
			MCS:        int32(5 + rand.Intn(6)),
			Frames:     uint32(180 + rand.Intn(120)),
			ProbeOnly:  false,
			LastSeenMs: now,
		},
		{
			MAC:        "aa:bb:cc:dd:ee:07",
			BSSID:      "",
			SSID:       "",
			RSSIdBm:    int32(-75 - rand.Intn(20)),
			RSSIAvgdBm: int32(-78 - rand.Intn(20)),
			RateMbps:   0,
			MCS:        int32(-1),
			Frames:     uint32(5 + rand.Intn(10)),
			ProbeOnly:  true, // probe request only
			LastSeenMs: now - 1000,
		},
		{
			MAC:        "aa:bb:cc:dd:ee:08",
			BSSID:      "",
			SSID:       "",
			RSSIdBm:    int32(-72 - rand.Intn(15)),
			RSSIAvgdBm: int32(-74 - rand.Intn(15)),
			RateMbps:   0,
			MCS:        int32(-1),
			Frames:     uint32(3 + rand.Intn(8)),
			ProbeOnly:  true,
			LastSeenMs: now - 500,
		},
	}

	// Per-channel stats: 2.4 GHz channels 1, 6, 11 and 5 GHz channels 36, 149
	channels := []WlanChannelStat{
		{
			Channel:        1,
			FreqMHz:        2412,
			ActiveMs:       uint64(400),
			BusyMs:         uint64(120 + rand.Intn(100)),
			UtilizationPct: 0,
			Frames:         uint32(50 + rand.Intn(80)),
		},
		{
			Channel:        6,
			FreqMHz:        2437,
			ActiveMs:       uint64(400),
			BusyMs:         uint64(200 + rand.Intn(150)),
			UtilizationPct: 0,
			Frames:         uint32(100 + rand.Intn(150)),
		},
		{
			Channel:        11,
			FreqMHz:        2462,
			ActiveMs:       uint64(400),
			BusyMs:         uint64(80 + rand.Intn(60)),
			UtilizationPct: 0,
			Frames:         uint32(30 + rand.Intn(50)),
		},
		{
			Channel:        36,
			FreqMHz:        5180,
			ActiveMs:       uint64(400),
			BusyMs:         uint64(180 + rand.Intn(100)),
			UtilizationPct: 0,
			Frames:         uint32(80 + rand.Intn(120)),
		},
		{
			Channel:        149,
			FreqMHz:        5745,
			ActiveMs:       uint64(400),
			BusyMs:         uint64(120 + rand.Intn(80)),
			UtilizationPct: 0,
			Frames:         uint32(60 + rand.Intn(100)),
		},
	}

	// Compute utilization percentages
	for i := range channels {
		if channels[i].ActiveMs > 0 {
			channels[i].UtilizationPct = float64(channels[i].BusyMs) / float64(channels[i].ActiveMs) * 100.0
		}
	}

	// Synthetic networks (APs) matching the stations' BSSIDs.
	networks := []WlanNetwork{
		{BSSID: "a0:f8:49:74:8b:20", SSID: "corp-wifi", Channel: 6, FreqMHz: 2437, RSSIdBm: int32(-44 - rand.Intn(8)), Beacons: uint32(30 + rand.Intn(20)), Security: "WPA2/WPA3", Standards: "n/ac/ax",
			WidthMHz: 40, BeaconIntervalTU: 100, Country: "AT", LoadPresent: true, LoadStations: uint32(rand.Intn(12)), LoadChannelUtilPct: float64(20 + rand.Intn(40)), SecurityDetail: "PSK+SAE · CCMP", Roaming: "k/r/v",
			MFP: "capable", GroupCipher: "CCMP", DTIMPeriod: 2, Streams: 2, MaxRateMbps: 574.2},
		{BSSID: "a0:f8:49:74:8b:22", SSID: "corp-guest", Channel: 6, FreqMHz: 2437, RSSIdBm: int32(-48 - rand.Intn(8)), Beacons: uint32(28 + rand.Intn(20)), Security: "WPA2", Standards: "n/ac",
			WidthMHz: 20, BeaconIntervalTU: 100, Country: "AT", LoadPresent: true, LoadStations: uint32(rand.Intn(5)), LoadChannelUtilPct: float64(10 + rand.Intn(30)), SecurityDetail: "PSK · CCMP", Roaming: "k/v",
			MFP: "capable", GroupCipher: "CCMP", DTIMPeriod: 2, Streams: 2, MaxRateMbps: 400},
		{BSSID: "c0:25:5c:ec:bb:40", SSID: "corp-wifi", Channel: 36, FreqMHz: 5180, RSSIdBm: int32(-50 - rand.Intn(10)), Beacons: uint32(25 + rand.Intn(20)), Security: "WPA2/WPA3", Standards: "ac/ax",
			WidthMHz: 80, BeaconIntervalTU: 100, Country: "AT", LoadPresent: true, LoadStations: uint32(rand.Intn(20)), LoadChannelUtilPct: float64(15 + rand.Intn(35)), SecurityDetail: "PSK+SAE · CCMP", Roaming: "k/r/v",
			MFP: "required", GroupCipher: "CCMP", DTIMPeriod: 3, Streams: 4, MaxRateMbps: 2402},
		{BSSID: "2c:3a:fd:8b:1e:56", SSID: "IoT-Net", Channel: 11, FreqMHz: 2462, RSSIdBm: int32(-68 - rand.Intn(10)), Beacons: uint32(20 + rand.Intn(15)), Security: "WPA2", Standards: "n",
			WidthMHz: 20, BeaconIntervalTU: 100, Country: "AT", SecurityDetail: "PSK · CCMP/TKIP",
			GroupCipher: "TKIP", DTIMPeriod: 1, WPS: true, Streams: 1, MaxRateMbps: 72.2},
		{BSSID: "e8:9f:80:11:22:33", SSID: "", Channel: 1, FreqMHz: 2412, RSSIdBm: int32(-72 - rand.Intn(10)), Beacons: uint32(10 + rand.Intn(10)), Security: "WPA2", Standards: "n",
			WidthMHz: 20, BeaconIntervalTU: 100, SecurityDetail: "PSK · CCMP",
			GroupCipher: "CCMP", DTIMPeriod: 1, Streams: 1, MaxRateMbps: 72.2},
	}

	for i := range networks {
		networks[i].LastSeenMs = now
	}

	sweepMs := uint32(400 * len(channels)) // dwell per channel + small overhead
	return iface, stations, channels, networks, sweepMs, nil
}
