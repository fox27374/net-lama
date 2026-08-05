//go:build linux

package probe

import (
	"context"
	"encoding/binary"
	"net"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// The probes below need no privileges at all. Instead of building packets on
// a raw socket (which is what mtr does, and why path testing used to require
// NET_RAW and the sensor image), they send ordinary UDP/TCP/ICMP-datagram
// traffic with IP_TTL set and read the resulting ICMP time-exceeded messages
// off the socket error queue via IP_RECVERR.
//
// Verified before this engine was written, on both x86_64 and aarch64, as an
// unprivileged user and inside the distroless image with --cap-drop=ALL.
//
// ponytail: one known limitation, deliberately not worked around. Under
// rootless *bridge* networking (slirp4netns/pasta) the user-mode stack
// terminates TCP locally, so IP_TTL is ignored and every TTL looks like a
// completed handshake; UDP and ICMP still work there. Agents run with host
// networking, where all three are correct. Detect and warn if someone ever
// runs a TCP path test on a bridged agent.

// sock_extended_err is 16 bytes, immediately followed by the offender's
// sockaddr_in (SO_EE_OFFENDER): family(2) port(2) addr(4) pad(8).
const (
	extErrTypeOff = 5  // ee_type within sock_extended_err
	extErrCodeOff = 6  // ee_code
	offenderIPOff = 20 // 16 + family(2) + port(2)
	probeTimeout  = 2 * time.Second
)

// waitFd blocks until the socket has one of events pending or the deadline
// passes. Sleeping in a poll loop instead would add its own granularity to
// every RTT — measured against mtr, a 2ms sleep inflated LAN hops from
// 0.7ms to 2.3ms, which is the difference between a useful latency
// threshold and a meaningless one.
func waitFd(fd int, events int16, deadline time.Time) {
	ms := int(time.Until(deadline).Milliseconds())
	if ms <= 0 {
		return
	}
	fds := []unix.PollFd{{Fd: int32(fd), Events: events}}
	_, _ = unix.Poll(fds, ms)
}

// readErrQueue drains the socket error queue until it finds an ICMP message
// or the deadline passes. This is the whole trick: the ICMP time-exceeded
// that a raw socket would receive directly is delivered here as an error on
// the ordinary socket that sent the probe.
func readErrQueue(fd int, deadline time.Time) (from net.IP, icmpType, icmpCode uint8, ok bool) {
	buf := make([]byte, 512)
	oob := make([]byte, 512)

	// Always attempt one read before consulting the deadline: callers that
	// have just been woken by poll pass a deadline of "now", meaning "check
	// what is already queued, don't wait".
	for {
		_, oobn, _, _, err := syscall.Recvmsg(fd, buf, oob, syscall.MSG_ERRQUEUE|syscall.MSG_DONTWAIT)
		if err != nil {
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				if !time.Now().Before(deadline) {
					return nil, 0, 0, false
				}
				waitFd(fd, unix.POLLERR|unix.POLLIN, deadline)
				continue
			}
			return nil, 0, 0, false
		}
		msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			return nil, 0, 0, false
		}
		for _, m := range msgs {
			if m.Header.Level != syscall.IPPROTO_IP || m.Header.Type != syscall.IP_RECVERR {
				continue
			}
			if len(m.Data) < offenderIPOff+4 {
				continue
			}
			ip := net.IPv4(m.Data[offenderIPOff], m.Data[offenderIPOff+1],
				m.Data[offenderIPOff+2], m.Data[offenderIPOff+3])
			return ip, m.Data[extErrTypeOff], m.Data[extErrCodeOff], true
		}
	}
}

func setupSocket(fd, ttl int, flowID uint16, bindPort bool) error {
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_RECVERR, 1); err != nil {
		return err
	}
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_TTL, ttl); err != nil {
		return err
	}
	if bindPort {
		// Best effort: a taken port just means this run picks an ephemeral
		// one, which costs ECMP stability rather than the measurement.
		_ = syscall.Bind(fd, &syscall.SockaddrInet4{Port: flowPort(flowID)})
	}
	return nil
}

func sendProbe(ctx context.Context, dst net.IP, protocol string, port, ttl uint32, flowID, seq uint16) probeReply {
	var addr [4]byte
	copy(addr[:], dst.To4())

	switch protocol {
	case "udp":
		return udpProbe(addr, port, ttl, flowID)
	case "icmp":
		return icmpProbe(addr, ttl, flowID, seq)
	default:
		return tcpProbe(addr, port, ttl, flowID)
	}
}

func udpProbe(addr [4]byte, port, ttl uint32, flowID uint16) probeReply {
	if port == 0 {
		port = 33434
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return probeReply{}
	}
	defer syscall.Close(fd)
	if err := setupSocket(fd, int(ttl), flowID, true); err != nil {
		return probeReply{}
	}

	start := time.Now()
	// Fixed payload: the UDP checksum is part of the ECMP hash, and it is
	// derived from the payload, so a constant payload keeps the flow stable.
	if err := syscall.Sendto(fd, []byte("net-lama"), 0, &syscall.SockaddrInet4{Port: int(port), Addr: addr}); err != nil {
		return probeReply{}
	}
	from, t, c, ok := readErrQueue(fd, start.Add(probeTimeout))
	if !ok {
		return probeReply{}
	}
	kind, state := classifyICMP(t, c)
	return probeReply{kind: kind, from: from, rtt: time.Since(start), destState: state}
}

func tcpProbe(addr [4]byte, port, ttl uint32, flowID uint16) probeReply {
	if port == 0 {
		port = 443
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return probeReply{}
	}
	defer syscall.Close(fd)
	if err := setupSocket(fd, int(ttl), flowID, false); err != nil {
		return probeReply{}
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		return probeReply{}
	}
	// One SYN, no retransmits. The kernel owns retransmission for TCP, and
	// its ~1s retry turns a router's rate-limited (dropped) ICMP reply into
	// a hop that appears to take a second instead of a probe that was lost:
	// measured against ICMP mode, one LAN hop read 815ms where it truly
	// answers in 0.3ms. This probe counts its own losses.
	_ = syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, unix.TCP_SYNCNT, 1)

	start := time.Now()
	err = syscall.Connect(fd, &syscall.SockaddrInet4{Port: int(port), Addr: addr})
	if err != nil && err != syscall.EINPROGRESS {
		return probeReply{}
	}

	dstIP := net.IPv4(addr[0], addr[1], addr[2], addr[3])
	deadline := start.Add(probeTimeout)
	for time.Now().Before(deadline) {
		// Either the handshake completes (POLLOUT) or an ICMP error lands
		// (POLLERR); both wake the poll immediately, so the RTT is the
		// network's and not the loop's.
		waitFd(fd, unix.POLLOUT|unix.POLLERR, deadline)
		if from, t, c, ok := readErrQueue(fd, time.Now()); ok {
			kind, state := classifyICMP(t, c)
			return probeReply{kind: kind, from: from, rtt: time.Since(start), destState: state}
		}
		// Re-connecting the same socket reports the handshake's outcome
		// portably: EISCONN once it completed, EALREADY while it is still
		// pending. (select(2) would need FdSet bit arithmetic that differs
		// between 32- and 64-bit builds, and the agent ships for armv7.)
		switch err := syscall.Connect(fd, &syscall.SockaddrInet4{Port: int(port), Addr: addr}); err {
		case syscall.EISCONN:
			// SYN-ACK: the destination accepted the connection.
			return probeReply{kind: replyDestination, from: dstIP,
				rtt: time.Since(start), destState: DestOpen}
		case syscall.ECONNREFUSED:
			// RST: the host is up and reachable, the port is shut.
			return probeReply{kind: replyDestination, from: dstIP,
				rtt: time.Since(start), destState: DestClosed}
		case syscall.EINPROGRESS, syscall.EALREADY:
			// Still waiting.
		case nil:
			return probeReply{kind: replyDestination, from: dstIP,
				rtt: time.Since(start), destState: DestOpen}
		default:
			return probeReply{}
		}
	}
	return probeReply{}
}

// icmpProbe uses an unprivileged ICMP datagram socket (SOCK_DGRAM), which
// Linux allows when net.ipv4.ping_group_range covers the process's gid — the
// default on both deploy hosts. The kernel owns the echo id on these
// sockets, so probes are matched by sequence number.
//
// ponytail: ICMP has no ports, so flow constancy here is weaker than for
// UDP/TCP — a load balancer hashing on the echo id/sequence can still move
// us. Compensating (varying the payload to hold the checksum constant) is
// the Paris trick for ICMP; add it if ICMP paths prove unstable in practice.
func icmpProbe(addr [4]byte, ttl uint32, flowID, seq uint16) probeReply {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_ICMP)
	if err != nil {
		// No unprivileged ICMP on this host; a raw socket would need
		// NET_RAW, which is exactly what this engine avoids requiring.
		return probeReply{}
	}
	defer syscall.Close(fd)
	if err := setupSocket(fd, int(ttl), flowID, false); err != nil {
		return probeReply{}
	}

	pkt := icmpEcho(flowID, seq)
	start := time.Now()
	if err := syscall.Sendto(fd, pkt, 0, &syscall.SockaddrInet4{Addr: addr}); err != nil {
		return probeReply{}
	}

	deadline := start.Add(probeTimeout)
	for time.Now().Before(deadline) {
		waitFd(fd, unix.POLLIN|unix.POLLERR, deadline)
		if from, t, c, ok := readErrQueue(fd, time.Now()); ok {
			kind, state := classifyICMP(t, c)
			return probeReply{kind: kind, from: from, rtt: time.Since(start), destState: state}
		}
		// The echo reply itself arrives on the socket, not the error queue.
		buf := make([]byte, 128)
		n, _, err := syscall.Recvfrom(fd, buf, syscall.MSG_DONTWAIT)
		if err == nil && n >= 8 && buf[0] == icmpEchoReply {
			return probeReply{kind: replyDestination, from: net.IPv4(addr[0], addr[1], addr[2], addr[3]),
				rtt: time.Since(start), destState: DestEchoed}
		}
	}
	return probeReply{}
}

// icmpEcho builds an echo request. The kernel rewrites the id on datagram
// sockets and fills in the checksum, but it is computed here anyway so the
// same builder works if this ever runs on a raw socket.
func icmpEcho(id, seq uint16) []byte {
	pkt := make([]byte, 16)
	pkt[0] = 8 // echo request
	binary.BigEndian.PutUint16(pkt[4:], id)
	binary.BigEndian.PutUint16(pkt[6:], seq)
	copy(pkt[8:], "net-lama")

	var sum uint32
	for i := 0; i < len(pkt); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(pkt[i:]))
	}
	for sum>>16 > 0 {
		sum = sum&0xffff + sum>>16
	}
	binary.BigEndian.PutUint16(pkt[2:], ^uint16(sum))
	return pkt
}

// tracerouteSupported: the unprivileged path always works on Linux.
func tracerouteSupported() bool { return true }
