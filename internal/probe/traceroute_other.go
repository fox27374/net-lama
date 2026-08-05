//go:build !linux

package probe

import (
	"context"
	"net"
)

// The unprivileged probing this engine relies on (IP_RECVERR plus reading
// ICMP off the socket error queue) is a Linux interface. Agents run on
// Linux; this stub keeps the package building for local development on
// darwin, where traceroute is reported as unavailable unless demo mode is on.

func sendProbe(context.Context, net.IP, string, uint32, uint32, uint16, uint16) probeReply {
	return probeReply{}
}

func tracerouteSupported() bool { return false }
