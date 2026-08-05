package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// pathChangesPerTest bounds the history per agent *and test*, the same way
// results are bounded. A flapping route is exactly the case that would grow
// this table without a cap, and it is also the case where the oldest events
// matter least.
var pathChangesPerTest = 500

// PathChange is one recorded difference between a traceroute run and the
// previous run of the same test from the same agent.
type PathChange struct {
	ID           int64     `json:"id"`
	AgentID      string    `json:"agentId"`
	AgentName    string    `json:"agentName,omitempty"`
	TestID       string    `json:"testId"`
	TestName     string    `json:"testName"`
	Time         time.Time `json:"time"`
	FirstDiffTTL uint32    `json:"firstDiffTtl"`
	FromHop      string    `json:"fromHop"` // the hop at that TTL before
	ToHop        string    `json:"toHop"`   // and after
	FromSig      string    `json:"fromSig"`
	ToSig        string    `json:"toSig"`
	// Scope is "inter-as" when the change moved traffic to a different
	// network, "intra-as" when it stayed inside one operator, "unknown"
	// when either side is unannounced (a private hop, typically).
	Scope string `json:"scope"`
	// Networks the changed hop belonged to, for display.
	FromNetwork string `json:"fromNetwork,omitempty"`
	ToNetwork   string `json:"toNetwork,omitempty"`
}

func (s *Store) AddPathChange(c *PathChange) error {
	_, err := s.db.Exec(`
		INSERT INTO path_changes
			(agent_id, test_id, test_name, time, first_diff_ttl, from_hop, to_hop,
			 from_sig, to_sig, scope, from_network, to_network)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.AgentID, c.TestID, c.TestName, c.Time.UTC(), c.FirstDiffTTL, c.FromHop, c.ToHop,
		c.FromSig, c.ToSig, c.Scope, c.FromNetwork, c.ToNetwork)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`DELETE FROM path_changes WHERE agent_id = ? AND test_id = ? AND id NOT IN
		 (SELECT id FROM path_changes WHERE agent_id = ? AND test_id = ? ORDER BY id DESC LIMIT ?)`,
		c.AgentID, c.TestID, c.AgentID, c.TestID, pathChangesPerTest)
	return err
}

// PathChangeFilter scopes a listing. TenantID is always applied.
type PathChangeFilter struct {
	TenantID string
	AgentID  string
	TestID   string
	Since    time.Time
	Limit    int
}

// ListPathChanges returns recorded route changes, newest first.
func (s *Store) ListPathChanges(f PathChangeFilter) ([]*PathChange, error) {
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 200
	}
	query := `
		SELECT p.id, p.agent_id, a.name, p.test_id, p.test_name, p.time,
		       p.first_diff_ttl, p.from_hop, p.to_hop, p.from_sig, p.to_sig,
		       p.scope, p.from_network, p.to_network
		FROM path_changes p
		JOIN agents a ON a.id = p.agent_id
		WHERE a.tenant_id = ?`
	args := []any{f.TenantID}

	if f.AgentID != "" {
		query += ` AND p.agent_id = ?`
		args = append(args, f.AgentID)
	}
	if f.TestID != "" {
		query += ` AND p.test_id = ?`
		args = append(args, f.TestID)
	}
	if !f.Since.IsZero() {
		query += ` AND p.time >= ?`
		args = append(args, f.Since.UTC())
	}
	query += ` ORDER BY p.id DESC LIMIT ?`
	args = append(args, f.Limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*PathChange{}
	for rows.Next() {
		c := &PathChange{}
		var fromNet, toNet sql.NullString
		if err := rows.Scan(&c.ID, &c.AgentID, &c.AgentName, &c.TestID, &c.TestName,
			&c.Time, &c.FirstDiffTTL, &c.FromHop, &c.ToHop, &c.FromSig, &c.ToSig,
			&c.Scope, &fromNet, &toNet); err != nil {
			return nil, err
		}
		c.FromNetwork, c.ToNetwork = fromNet.String, toNet.String
		out = append(out, c)
	}
	return out, rows.Err()
}

// TracerouteBaseline is what route-change detection compares against: for
// each TTL, the most recent hop address that actually answered.
//
// It is built from several runs rather than just the previous one, because a
// single run's silence is not evidence. If hop 2 answered as A, then went
// quiet (rate-limited ICMP), then answered as B, comparing only consecutive
// runs would match B against "*" and miss the reroute permanently.
type TracerouteBaseline struct {
	// Hosts is indexed by TTL-1; "*" means no run in the window saw it.
	Hosts []string
	// Seen is every address the window observed at each TTL. Under ECMP a
	// hop legitimately alternates between routers run after run — measured
	// on rp01, hop 5 flipped between .18 and .19 every minute — and an
	// address the window has already seen at that TTL is that alternation,
	// not the route moving somewhere new.
	Seen []map[string]bool
	// Lengths is every path length the window observed. The same
	// alternation shows up as the path being 13 hops one run and 14 the
	// next, which reports as a change at the TTL that appeared or vanished
	// and has no address to match against.
	Lengths map[int]bool
}

// baselineRuns is how many past runs are consulted. Measured on rp01: a
// 5-run window was too short for streaky ECMP alternation — a path that sat
// on one branch for six runs made the other branch look new again, so a hop
// that only ever flips between two routers still produced an event every few
// hours. 20 runs covers those streaks and is still one cheap indexed query
// per traceroute ingest (they arrive at test interval, ≥30s).
const baselineRuns = 20

// TracerouteBaselineFor returns the per-TTL baseline for this agent and
// test, newest answer per hop wins. Called on ingest before the new result
// is stored, so it describes the past rather than the present run.
func (s *Store) TracerouteBaselineFor(agentID, testID string) (TracerouteBaseline, error) {
	rows, err := s.db.Query(
		`SELECT payload FROM results
		 WHERE agent_id = ? AND test_id = ? AND test_type = 'traceroute'
		 ORDER BY id DESC LIMIT ?`, agentID, testID, baselineRuns)
	if err != nil {
		return TracerouteBaseline{}, err
	}
	defer rows.Close()

	base := TracerouteBaseline{Lengths: map[int]bool{}}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return TracerouteBaseline{}, err
		}
		hosts, ok := tracerouteHosts(payload)
		if !ok {
			continue
		}
		base.Lengths[len(hosts)] = true
		for i, h := range hosts {
			for len(base.Hosts) <= i {
				base.Hosts = append(base.Hosts, "*")
				base.Seen = append(base.Seen, map[string]bool{})
			}
			// Newest run is seen first, so only fill gaps afterwards.
			if base.Hosts[i] == "*" && h != "*" {
				base.Hosts[i] = h
			}
			if h != "*" {
				base.Seen[i][h] = true
			}
		}
	}
	return base, rows.Err()
}

// tracerouteHosts extracts a run's hop addresses, silence as "*", with the
// destination hop dropped: an anycast target answers from a different
// address most runs, which is DNS and load balancing rather than the path
// changing.
//
// Decoded structurally rather than through the proto: this package must not
// depend on the wire types (see internal/testtype's package doc on keeping
// that dependency one-way).
func tracerouteHosts(payload string) ([]string, bool) {
	var decoded struct {
		Traceroute struct {
			Reached bool `json:"reached"`
			Hops    []struct {
				Host string `json:"host"`
			} `json:"hops"`
		} `json:"traceroute"`
	}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return nil, false
	}
	if len(decoded.Traceroute.Hops) == 0 {
		return nil, false
	}

	hosts := make([]string, 0, len(decoded.Traceroute.Hops))
	for _, h := range decoded.Traceroute.Hops {
		if h.Host == "" {
			hosts = append(hosts, "*")
			continue
		}
		hosts = append(hosts, h.Host)
	}
	if decoded.Traceroute.Reached {
		hosts = hosts[:len(hosts)-1]
	}
	return hosts, true
}
