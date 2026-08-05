package probe

import (
	"context"
	"fmt"
	"math"
	"net"
	"strings"
	"sync"
	"time"
)

type Hop struct {
	TTL         uint32
	Host        string // IP; empty = anonymous (no reply)
	HostName    string // reverse-DNS name; empty when not resolved or failed
	LossPercent float64
	AvgRttMs    float64
	BestRttMs   float64
	WorstRttMs  float64
	JitterMs    float64
	Sent        uint32
}

type TracerouteResult struct {
	Target     string
	TargetIP   string
	Reached    bool
	Status     string // "reached" | "stalled" | "error"
	FailureHop uint32
	RttMs      float64
	Demo       bool
	Hops       []Hop
	// DestinationState is what the target said: open | closed | filtered |
	// unreachable | echoed. Empty while the path never got there.
	DestinationState string
	Engine           string // "native"
}

// Destination states. A path can reach the destination host and still be
// refused by it, which is a different operational problem from a path that
// breaks in the middle — the reason these are not folded into Status.
const (
	DestOpen        = "open"        // TCP SYN-ACK: host up, port serving
	DestClosed      = "closed"      // TCP RST: host up, port shut
	DestFiltered    = "filtered"    // silence, or ICMP administratively prohibited
	DestUnreachable = "unreachable" // ICMP destination unreachable
	DestEchoed      = "echoed"      // ICMP echo reply, or UDP port unreachable
)

// replyKind is what came back for one probe.
type replyKind int

const (
	replyNone         replyKind = iota // nothing arrived before the deadline
	replyTimeExceeded                  // an intermediate router
	replyDestination                   // the target answered; see destState
)

// probeReply is one probe's outcome.
type probeReply struct {
	kind      replyKind
	from      net.IP
	rtt       time.Duration
	destState string // set when kind == replyDestination
}

func tracerouteDemo() bool {
	return envEnabled("NETLAMA_TRACEROUTE_DEMO")
}

// TracerouteDemoMode reports whether traceroute results are synthetic.
func TracerouteDemoMode() bool { return tracerouteDemo() }

// Traceroute traces the path to target and classifies where a failing path
// breaks and what the destination itself said. protocol is icmp|tcp|udp.
//
// flowID pins the ECMP branch: the probe keeps the flow tuple identical
// across every TTL and every run (Paris-style), so consecutive hops really
// are on one path and a changed hop means the route changed rather than the
// load balancer hashing us elsewhere. Callers derive it from the test id.
func Traceroute(ctx context.Context, target, protocol string, port, maxHops, probes uint32, flowID uint16) (*TracerouteResult, error) {
	if tracerouteDemo() {
		return demoTraceroute(target, protocol, port), nil
	}
	if maxHops == 0 {
		maxHops = 30
	}
	if probes == 0 {
		probes = 5
	}
	if protocol == "" {
		protocol = "tcp"
	}

	addr, err := net.ResolveIPAddr("ip4", target)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", target, err)
	}
	dst := addr.IP.To4()
	if dst == nil {
		return nil, fmt.Errorf("%s is not an IPv4 address", target)
	}

	res := &TracerouteResult{Target: target, TargetIP: dst.String(), Engine: "native"}

	for ttl := uint32(1); ttl <= maxHops; ttl++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		hop, arrived, state := probeTTL(ctx, dst, protocol, port, ttl, probes, flowID)
		res.Hops = append(res.Hops, hop)
		if arrived {
			res.Reached = true
			res.Status = "reached"
			res.DestinationState = state
			res.RttMs = hop.AvgRttMs
			if hop.Host != "" {
				res.TargetIP = hop.Host
			}
			break
		}
	}

	if !res.Reached {
		res.Status = "stalled"
		for i := len(res.Hops) - 1; i >= 0; i-- {
			if res.Hops[i].Host != "" && res.Hops[i].LossPercent < 100 {
				res.FailureHop = res.Hops[i].TTL
				res.RttMs = res.Hops[i].AvgRttMs
				break
			}
		}
		// The path ran out without the target answering. Silence at the
		// destination is a filtering verdict, not an unknown.
		res.DestinationState = DestFiltered
	}

	resolveHopNames(ctx, res.Hops)
	return res, nil
}

// probeTTL sends probes packets at one TTL and aggregates them into a Hop.
// arrived reports whether the destination itself answered, in which case
// destState carries its verdict.
func probeTTL(ctx context.Context, dst net.IP, protocol string, port, ttl, probes uint32, flowID uint16) (hop Hop, arrived bool, destState string) {
	hop = Hop{TTL: ttl, Sent: probes}

	var rtts []float64
	var responder net.IP
	for i := uint32(0); i < probes; i++ {
		if ctx.Err() != nil {
			break
		}
		reply := sendProbe(ctx, dst, protocol, port, ttl, flowID, uint16(i))
		if reply.kind == replyNone {
			continue
		}
		rtts = append(rtts, float64(reply.rtt.Microseconds())/1000)
		if responder == nil {
			responder = reply.from
		}
		if reply.kind == replyDestination {
			arrived = true
			destState = reply.destState
			if responder == nil || responder.IsUnspecified() {
				responder = dst
			}
		}
	}

	if responder != nil && !responder.IsUnspecified() {
		hop.Host = responder.String()
	} else if arrived {
		// TCP handshakes complete without an ICMP message to carry a
		// source address; the responder is the destination by definition.
		hop.Host = dst.String()
	}
	hop.LossPercent = float64(probes-uint32(len(rtts))) / float64(probes) * 100
	hop.AvgRttMs, hop.BestRttMs, hop.WorstRttMs, hop.JitterMs = rttStats(rtts)
	return hop, arrived, destState
}

// rttStats returns avg, best, worst and jitter for a hop's samples. Jitter
// is the standard deviation, matching what mtr reported as StDev so history
// spanning the engine change stays comparable.
func rttStats(rtts []float64) (avg, best, worst, jitter float64) {
	if len(rtts) == 0 {
		return 0, 0, 0, 0
	}
	best, worst = rtts[0], rtts[0]
	var sum float64
	for _, v := range rtts {
		sum += v
		if v < best {
			best = v
		}
		if v > worst {
			worst = v
		}
	}
	avg = sum / float64(len(rtts))

	var sq float64
	for _, v := range rtts {
		sq += (v - avg) * (v - avg)
	}
	jitter = math.Sqrt(sq / float64(len(rtts)))
	return avg, best, worst, jitter
}

// resolveHopNames performs best-effort parallel reverse-DNS resolution on hop IPs.
// Each lookup has a 1500ms timeout and never fails the test. Names are stored in-place
// with trailing dots stripped; failures or timeouts leave HostName empty.
func resolveHopNames(ctx context.Context, hops []Hop) {
	var wg sync.WaitGroup
	for i := range hops {
		if hops[i].Host == "" {
			continue // anonymous hop; nothing to resolve
		}
		// Check if Host is already a name (not an IP)
		if net.ParseIP(hops[i].Host) == nil {
			continue // already a name, skip resolution
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			lookupCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer cancel()
			names, err := net.DefaultResolver.LookupAddr(lookupCtx, hops[i].Host)
			if err == nil && len(names) > 0 {
				// Take the first name and strip trailing dot
				hops[i].HostName = strings.TrimSuffix(names[0], ".")
			}
		}(i)
	}
	wg.Wait()
}

// ICMP types and codes the engine reacts to.
const (
	icmpEchoReply   = 0
	icmpDestUnreach = 3
	icmpTimeExceed  = 11
	icmpPortUnreach = 3  // code within destination-unreachable
	icmpAdminProhib = 13 // code within destination-unreachable
)

// classifyICMP turns an ICMP type/code into either an intermediate hop or a
// verdict about the destination. This is the distinction the native engine
// exists to make: mtr could only say "something answered".
func classifyICMP(icmpType, icmpCode uint8) (replyKind, string) {
	switch icmpType {
	case icmpTimeExceed:
		return replyTimeExceeded, ""
	case icmpDestUnreach:
		switch icmpCode {
		case icmpPortUnreach:
			// Classic UDP traceroute: the target itself answered.
			return replyDestination, DestEchoed
		case icmpAdminProhib:
			return replyDestination, DestFiltered
		default:
			return replyDestination, DestUnreachable
		}
	case icmpEchoReply:
		return replyDestination, DestEchoed
	}
	return replyTimeExceeded, ""
}

// flowPort maps a flow id onto a stable source port. Holding the source
// port fixed is what keeps every probe of a run on the same ECMP branch,
// and deriving it from the test id keeps it fixed across runs too.
func flowPort(flowID uint16) int {
	return 33000 + int(flowID%2000)
}
