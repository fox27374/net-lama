package netlamapb

// WlanDiscoveryTestID is the sentinel TestId used for the automatic
// full-spectrum WLAN discovery sweep that runs once on a monitor sensor's
// first connect. It is delivered via a normal RUN_TEST command (no separate
// command type is needed) and its result is stored like an ordinary
// wlan_sense result, so the UI can render the full channel + SSID map and
// offer to narrow the recurring test to the interesting channels.
const WlanDiscoveryTestID = "__wlan_discovery__"

// WlanDiscoveryTestName is the human-readable name shown for discovery results.
const WlanDiscoveryTestName = "WLAN discovery (all channels)"

// WlanDiscoveryDwellMs is the per-channel dwell for the discovery sweep. It
// is deliberately short — discovery only needs to catch a beacon or two on
// each channel (beacon interval is ~100 ms), and it sweeps every channel the
// phy supports, so a long dwell would make the one-off scan needlessly slow.
const WlanDiscoveryDwellMs = 300
