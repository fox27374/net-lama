package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/fox27374/net-lama/internal/store"
	pb "github.com/fox27374/net-lama/proto"
	"github.com/prometheus/client_golang/prometheus"
)

func hopsFrom(hosts ...string) []*pb.Hop {
	out := make([]*pb.Hop, 0, len(hosts))
	for i, h := range hosts {
		out = append(out, &pb.Hop{Ttl: uint32(i + 1), Host: h})
	}
	return out
}

// TestPathSignatureDropsDestination covers the anycast rule: a target like
// www.google.com answers from a different address most runs (three distinct
// addresses in three consecutive runs on tpr06), which is DNS and load
// balancing, not the path changing. A stalled run has no destination hop to
// drop, so its last hop stays in the signature.
func TestPathSignatureDropsDestination(t *testing.T) {
	hops := hopsFrom("10.0.0.1", "62.115.4.9", "142.251.202.162")

	reached := pathSignature(hops, true)
	if len(reached) != 2 || reached[1] != "62.115.4.9" {
		t.Errorf("reached signature = %v, want the two hops before the destination", reached)
	}

	stalled := pathSignature(hops, false)
	if len(stalled) != 3 {
		t.Errorf("stalled signature = %v, want every hop kept", stalled)
	}
}

func TestPathSignatureWritesSilenceAsWildcard(t *testing.T) {
	sig := pathSignature(hopsFrom("10.0.0.1", "", "1.1.1.1"), false)
	if len(sig) != 3 || sig[1] != "*" {
		t.Errorf("signature = %v, want the silent hop as *", sig)
	}
}

// TestDiffSignatures is the heart of the feature: what counts as a change.
func TestDiffSignatures(t *testing.T) {
	cases := []struct {
		name       string
		from, to   []string
		wantChange bool
		wantTTL    uint32
	}{
		{
			name: "identical paths",
			from: []string{"10.0.0.1", "62.115.4.9"},
			to:   []string{"10.0.0.1", "62.115.4.9"},
		},
		{
			// A router that rate-limits its ICMP replies goes quiet and
			// comes back constantly. Calling that a reroute would bury the
			// real events.
			name: "a hop went silent",
			from: []string{"10.0.0.1", "62.115.4.9", "1.1.1.1"},
			to:   []string{"10.0.0.1", "*", "1.1.1.1"},
		},
		{
			name: "a silent hop started answering",
			from: []string{"10.0.0.1", "*", "1.1.1.1"},
			to:   []string{"10.0.0.1", "62.115.4.9", "1.1.1.1"},
		},
		{
			name:       "a hop was replaced",
			from:       []string{"10.0.0.1", "62.115.4.9", "1.1.1.1"},
			to:         []string{"10.0.0.1", "80.91.1.7", "1.1.1.1"},
			wantChange: true, wantTTL: 2,
		},
		{
			name:       "the path grew",
			from:       []string{"10.0.0.1", "62.115.4.9"},
			to:         []string{"10.0.0.1", "62.115.4.9", "80.91.1.7"},
			wantChange: true, wantTTL: 3,
		},
		{
			name:       "the path shrank",
			from:       []string{"10.0.0.1", "62.115.4.9", "80.91.1.7"},
			to:         []string{"10.0.0.1", "62.115.4.9"},
			wantChange: true, wantTTL: 3,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ttl, _, _, changed := diffSignatures(c.from, c.to)
			if changed != c.wantChange {
				t.Fatalf("changed = %v, want %v", changed, c.wantChange)
			}
			if changed && ttl != c.wantTTL {
				t.Errorf("first differing TTL = %d, want %d", ttl, c.wantTTL)
			}
		})
	}
}

// TestClassifyChange uses real addresses from the deploy host's own paths,
// so the classification is checked against the embedded routing table rather
// than a stub.
func TestClassifyChange(t *testing.T) {
	// 1.1.1.1 and 1.0.0.1 are both Cloudflare (AS13335).
	if scope, _, _ := classifyChange("1.1.1.1", "1.0.0.1"); scope != "intra-as" {
		t.Errorf("Cloudflare -> Cloudflare classified as %q, want intra-as", scope)
	}
	// Cloudflare -> Google is a different network.
	scope, fromNet, toNet := classifyChange("1.1.1.1", "8.8.8.8")
	if scope != "inter-as" {
		t.Errorf("Cloudflare -> Google classified as %q, want inter-as", scope)
	}
	if fromNet == "" || toNet == "" {
		t.Errorf("network labels missing: from=%q to=%q", fromNet, toNet)
	}
	// A private hop is not announced, so the change is real but unclassifiable.
	if scope, _, _ := classifyChange("10.0.0.1", "8.8.8.8"); scope != "unknown" {
		t.Errorf("private -> Google classified as %q, want unknown", scope)
	}
}

// TestDetectPathChangeThroughIngest drives the real ingest path: three
// results for one test, where only the middle one takes a different route.
// Exactly one event must be recorded, on the run that moved — and the run
// that merely lost a hop to ICMP rate limiting must not produce one.
func TestDetectPathChangeThroughIngest(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "netlama-pathchange-*.db")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	st, err := store.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	srv := &Server{
		Store:   st,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Metrics: NewMetrics(prometheus.NewRegistry()),
	}
	tenant, _ := st.CreateTenant("t")
	site, _ := st.CreateSite(tenant.ID, "s")
	agent, err := st.CreateAgent(tenant.ID, site.ID, "a")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	test, err := st.CreateTest(&store.TestDef{
		TenantID: tenant.ID, Name: "Path", Type: "traceroute", IntervalSeconds: 60,
		Params: json.RawMessage(`{"target":"1.1.1.1","protocol":"icmp"}`),
	})
	if err != nil {
		t.Fatalf("create test: %v", err)
	}

	conn := &connectedAgent{agent: agent, tenant: tenant.Name}
	run := func(hosts ...string) *pb.TestResult {
		res := &pb.TestResult{
			TestId: test.ID, TestName: "Path",
			Result: &pb.TestResult_Traceroute{Traceroute: &pb.TracerouteResult{
				Target: "1.1.1.1", Reached: true, Status: "reached",
				Hops: hopsFrom(hosts...),
			}},
		}
		srv.handleResult(srv.Logger, conn, res)
		return res
	}

	// Baseline, then the same route with a rate-limited hop gone silent,
	// then a genuine reroute at TTL 2.
	first := run("10.0.0.1", "1.0.0.1", "1.1.1.1")
	quiet := run("10.0.0.1", "*", "1.1.1.1")
	moved := run("10.0.0.1", "8.8.8.8", "1.1.1.1")

	if first.GetTraceroute().GetPathChanged() {
		t.Error("the first run reported a change with nothing to compare against")
	}
	if quiet.GetTraceroute().GetPathChanged() {
		t.Error("a hop going silent was reported as a route change")
	}
	if !moved.GetTraceroute().GetPathChanged() {
		t.Error("a replaced hop was not reported as a route change")
	}

	changes, err := st.ListPathChanges(store.PathChangeFilter{TenantID: tenant.ID})
	if err != nil {
		t.Fatalf("list path changes: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("recorded %d changes, want exactly 1", len(changes))
	}
	c := changes[0]
	if c.FirstDiffTTL != 2 || c.FromHop != "1.0.0.1" || c.ToHop != "8.8.8.8" {
		t.Errorf("event = ttl %d, %s -> %s; want ttl 2, 1.0.0.1 -> 8.8.8.8",
			c.FirstDiffTTL, c.FromHop, c.ToHop)
	}
	// Cloudflare -> Google, so the change left the network it was in.
	if c.Scope != "inter-as" {
		t.Errorf("scope = %q, want inter-as", c.Scope)
	}
	if c.FromNetwork == "" || c.ToNetwork == "" {
		t.Errorf("networks not labelled: %q -> %q", c.FromNetwork, c.ToNetwork)
	}
}
