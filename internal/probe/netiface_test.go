package probe

import (
	"context"
	"net"
	"testing"
)

func TestFirstIPv4(t *testing.T) {
	v6only := &net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)}
	v4 := &net.IPNet{IP: net.ParseIP("10.0.1.5"), Mask: net.CIDRMask(24, 32)}
	if got := firstIPv4([]net.Addr{v6only, v4}); got != "10.0.1.5" {
		t.Errorf("firstIPv4 = %q, want 10.0.1.5", got)
	}
	if got := firstIPv4([]net.Addr{v6only}); got != "" {
		t.Errorf("firstIPv4 with no v4 address = %q, want empty", got)
	}
	if got := firstIPv4(nil); got != "" {
		t.Errorf("firstIPv4(nil) = %q, want empty", got)
	}
}

func TestInterfaceSpeedMbps(t *testing.T) {
	if got := interfaceSpeedMbps("nonexistent-iface-xyz"); got != 0 {
		t.Errorf("interfaceSpeedMbps for a missing interface = %d, want 0 (no panic, no error surfaced)", got)
	}
}

func TestNetworkInterfacesExcludesLoopback(t *testing.T) {
	for _, ni := range NetworkInterfaces(context.Background()) {
		if ni.Name == "lo" || ni.Name == "lo0" {
			t.Errorf("NetworkInterfaces included loopback interface %q", ni.Name)
		}
	}
}
