// Package probe: unified network-interface inventory (wired + wireless).
//
// Reported at agent Register so the operator can pick per-role interfaces
// (management display, WLAN sensor, perfmon reflector) from a dropdown of
// what the agent actually has, instead of typing an interface name or IP
// blind. Wireless-specific detail (monitor-mode support) comes from the
// existing WirelessInterfaces detection; this just adds wired interfaces,
// link speed, and current IP on top.
package probe

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
)

type NetworkInterface struct {
	Name            string
	Wireless        bool
	SupportsMonitor bool   // wireless only
	SpeedMbps       uint32 // wired link speed; 0 = unknown, wireless, or link down
	IPAddress       string // current IPv4; "" if none
}

// NetworkInterfaces enumerates the host's non-loopback network interfaces,
// merging in wireless-specific detail (monitor-mode support) from
// WirelessInterfaces for any interface that's wireless.
func NetworkInterfaces(ctx context.Context) []NetworkInterface {
	wirelessByName := make(map[string]WirelessInterface)
	for _, w := range WirelessInterfaces(ctx) {
		wirelessByName[w.Name] = w
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []NetworkInterface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		ni := NetworkInterface{Name: iface.Name, IPAddress: firstIPv4(addrs)}
		if w, ok := wirelessByName[iface.Name]; ok {
			ni.Wireless = true
			ni.SupportsMonitor = w.SupportsMonitor
		} else {
			ni.SpeedMbps = interfaceSpeedMbps(iface.Name)
		}
		out = append(out, ni)
	}
	return out
}

// firstIPv4 returns the first IPv4 address among addrs, or "" if none.
func firstIPv4(addrs []net.Addr) string {
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip4 := ip.To4(); ip4 != nil {
			return ip4.String()
		}
	}
	return ""
}

// interfaceSpeedMbps reads the kernel-reported link speed for a wired
// interface from sysfs (Linux only — the path simply doesn't exist
// elsewhere, or when the link is down/speed unknown, so this just
// returns 0 rather than treating any of that as an error).
func interfaceSpeedMbps(name string) uint32 {
	data, err := os.ReadFile("/sys/class/net/" + name + "/speed")
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || n < 0 {
		return 0
	}
	return uint32(n)
}
