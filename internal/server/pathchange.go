package server

import (
	"strings"
	"time"

	"github.com/fox27374/net-lama/internal/asn"
	"github.com/fox27374/net-lama/internal/store"
	pb "github.com/fox27374/net-lama/proto"
)

// Route-change detection compares a traceroute run with the previous run of
// the same test from the same agent. Two rules keep it from crying wolf,
// both learned from real traces rather than guessed:
//
//  1. An anonymous hop is a wildcard. Routers rate-limit their ICMP replies,
//     so a hop that answered in one run and was silent in the next is normal.
//     Treating that as a reroute would bury the real events.
//  2. The destination hop is not part of the signature. An anycast target
//     answers from a different address most runs (www.google.com resolved to
//     three different addresses in three consecutive runs on tpr06), which
//     says nothing about the path taken to get there.
//
// The signature is therefore the ordered list of *intermediate* hop
// addresses, with silence written as "*".
//
// Rule 1 has a consequence worth stating: comparing against only the
// previous run would let silence hide a change forever — hop 2 answers as A,
// goes quiet, then answers as B, and B is only ever compared against "*".
// The baseline is therefore the most recent *answer* per TTL across the last
// few runs (store.TracerouteBaselineFor), not the last run.
func pathSignature(hops []*pb.Hop, reached bool) []string {
	sig := make([]string, 0, len(hops))
	for _, h := range hops {
		host := h.GetHost()
		if host == "" {
			host = "*"
		}
		sig = append(sig, host)
	}
	// Drop the destination itself, which is the last hop of a run that
	// arrived. A stalled run has no destination hop to drop.
	if reached && len(sig) > 0 {
		sig = sig[:len(sig)-1]
	}
	return sig
}

// diffSignatures reports the first TTL at which two signatures name
// different hosts, treating "*" as matching anything. ok is false when they
// are equivalent.
func diffSignatures(from, to []string) (ttl uint32, fromHop, toHop string, ok bool) {
	n := len(from)
	if len(to) < n {
		n = len(to)
	}
	for i := 0; i < n; i++ {
		if from[i] == "*" || to[i] == "*" || from[i] == to[i] {
			continue
		}
		return uint32(i + 1), from[i], to[i], true
	}
	// Same prefix but a different length: the path grew or shrank, which is
	// a change even though no hop contradicts another.
	if len(from) != len(to) {
		if len(to) > len(from) {
			return uint32(len(from) + 1), "", to[len(from)], true
		}
		return uint32(len(to) + 1), from[len(to)], "", true
	}
	return 0, "", "", false
}

// seenRecently reports whether this address already appeared at this TTL in
// the baseline window.
func seenRecently(base store.TracerouteBaseline, ttl uint32, hop string) bool {
	i := int(ttl) - 1
	if hop == "" || i < 0 || i >= len(base.Seen) {
		return false
	}
	return base.Seen[i][hop]
}

// classifyChange labels a change by whether it moved traffic to a different
// network, using the embedded routing table from stage 2. An operator cares
// much more about leaving their transit provider than about a reroute
// inside it.
func classifyChange(fromHop, toHop string) (scope, fromNet, toNet string) {
	fromInfo, fromOK := asn.Lookup(fromHop)
	toInfo, toOK := asn.Lookup(toHop)

	if fromOK {
		fromNet = networkLabel(fromInfo)
	}
	if toOK {
		toNet = networkLabel(toInfo)
	}
	switch {
	case !fromOK || !toOK:
		// A private or unrouted hop on either side: real, but not
		// classifiable as staying inside a network or leaving one.
		return "unknown", fromNet, toNet
	case fromInfo.ASN == toInfo.ASN:
		return "intra-as", fromNet, toNet
	default:
		return "inter-as", fromNet, toNet
	}
}

func networkLabel(info asn.Info) string {
	if info.Owner == "" {
		return "AS" + itoa(info.ASN)
	}
	return info.Owner + " (AS" + itoa(info.ASN) + ")"
}

func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// detectPathChange compares this traceroute result against the previous run
// and records an event when the route differs. It sets result.PathChanged so
// the UI can mark the run and alert rules can watch it as an ordinary
// metric; agents never set that field.
//
// Returns silently for anything that is not a usable traceroute result, and
// never fails ingest: a missed change is better than a dropped measurement.
func (s *Server) detectPathChange(conn *connectedAgent, result *pb.TestResult, test *store.TestDef) {
	tr := result.GetTraceroute()
	if tr == nil || result.TestId == "" || len(tr.GetHops()) == 0 {
		return
	}

	base, err := s.Store.TracerouteBaselineFor(conn.agent.ID, result.TestId)
	if err != nil || len(base.Hosts) == 0 {
		// Nothing to compare against — the first trace of a test
		// establishes the baseline rather than reporting a change.
		return
	}

	current := pathSignature(tr.GetHops(), tr.GetReached())
	if len(current) == 0 {
		return
	}
	ttl, fromHop, toHop, changed := diffSignatures(base.Hosts, current)
	if !changed {
		return
	}
	// Under ECMP a hop alternates between routers rather than moving: on
	// rp01, hop 5 flipped between 89.105.160.18 and .19 every minute, which
	// produced an event every run. An address this test has already seen at
	// this TTL recently is that alternation. The first appearance of a
	// genuinely new address still reports, because the window has not seen
	// it yet.
	//
	// Pinning the flow (Paris-style) prevents this for tcp/udp probes, but
	// not for icmp: on an unprivileged ICMP datagram socket the kernel owns
	// the echo id and recomputes the checksum, so there is nothing left to
	// hold constant.
	if seenRecently(base, ttl, toHop) {
		return
	}
	// The same alternation seen as a length change: the path is 13 hops one
	// run and 14 the next, which has no address to match against. A length
	// the window has already observed is that flapping, not the route
	// growing somewhere new.
	if (toHop == "" || fromHop == "") && base.Lengths[len(current)] {
		return
	}

	tr.PathChanged = true
	scope, fromNet, toNet := classifyChange(fromHop, toHop)

	t := time.Now()
	if result.Time != nil {
		t = result.Time.AsTime()
	}
	testName := result.TestName
	if test != nil {
		testName = test.Name
	}
	err = s.Store.AddPathChange(&store.PathChange{
		AgentID: conn.agent.ID, TestID: result.TestId, TestName: testName,
		Time: t, FirstDiffTTL: ttl, FromHop: fromHop, ToHop: toHop,
		FromSig: strings.Join(base.Hosts, " "),
		ToSig:   strings.Join(current, " "),
		Scope:   scope, FromNetwork: fromNet, ToNetwork: toNet,
	})
	if err != nil {
		s.Logger.Warn("Storing path change failed", "error", err)
		return
	}
	s.Logger.Info("Path changed",
		"test", testName, "agent", conn.agent.Name, "ttl", ttl,
		"from", fromHop, "to", toHop, "scope", scope)
}
