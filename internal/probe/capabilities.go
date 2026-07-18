package probe

import (
	"os"
	"os/exec"
)

// DetectCapabilities reports which test types this agent can run.
// Detection is based on availability of external tools (mtr) and demo
// environment flags. hasWireless is the agent's already-collected
// wireless interface inventory result (see WirelessInterfaces), so it
// is not re-enumerated here; that inventory is already empty when `iw`
// is missing and non-empty in WLAN demo mode.
// wifaces contains the full wireless interface details including monitor support info.
func DetectCapabilities(hasWireless bool, wifaces []WirelessInterface) []string {
	caps := []string{"ping", "dns", "http", "tcp", "speedtest"}

	// traceroute: needs mtr in PATH or demo mode enabled
	if _, err := exec.LookPath("mtr"); err == nil || tracerouteDemo() {
		caps = append(caps, "traceroute")
	}

	// wlan: needs monitor-capable interface and privilege, or demo mode enabled
	if wlanDemo() || hasMonitorCapableIface(wifaces) && isPrivileged() {
		caps = append(caps, "wlan")
	}

	return caps
}

// hasMonitorCapableIface checks if any wireless interface supports monitor mode.
func hasMonitorCapableIface(wifaces []WirelessInterface) bool {
	for _, iface := range wifaces {
		if iface.SupportsMonitor {
			return true
		}
	}
	return false
}

// isPrivileged checks if the process has NET_ADMIN capability or is root.
func isPrivileged() bool {
	// Simple check: running as root (euid 0)
	if os.Geteuid() == 0 {
		return true
	}
	// TODO: Check for CAP_NET_ADMIN capability when available
	return false
}
