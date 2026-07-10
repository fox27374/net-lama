package probe

import (
	"os/exec"
)

// DetectCapabilities reports which test types this agent can run.
// Detection is based on availability of external tools (mtr) and demo
// environment flags. hasWireless is the agent's already-collected
// wireless interface inventory result (see WirelessInterfaces), so it
// is not re-enumerated here; that inventory is already empty when `iw`
// is missing and non-empty in WLAN demo mode.
func DetectCapabilities(hasWireless bool) []string {
	caps := []string{"ping", "dns", "http", "tcp", "speedtest"}

	// traceroute: needs mtr in PATH or demo mode enabled
	if _, err := exec.LookPath("mtr"); err == nil || tracerouteDemo() {
		caps = append(caps, "traceroute")
	}

	// wlan_scan: needs at least one detected wireless interface (which
	// implies `iw` is available), or demo mode enabled
	if wlanDemo() || hasWireless {
		caps = append(caps, "wlan_scan")
	}

	return caps
}
