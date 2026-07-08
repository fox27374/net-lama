package store

import "time"

type TestHealth struct {
	TestID   string     `json:"testId"`
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Checks   int        `json:"checks"` // recent checks considered
	OK       int        `json:"ok"`     // of those, how many were healthy
	Agents   int        `json:"agents"` // distinct agents reporting
	Status   string     `json:"status"` // healthy | degraded | failing | nodata
	LastSeen *time.Time `json:"lastSeen,omitempty"`
}

type Overview struct {
	Sites           int           `json:"sites"`
	Agents          int           `json:"agents"`
	AgentsConnected int           `json:"agentsConnected"`
	Tests           int           `json:"tests"`
	ActiveAlerts    int           `json:"activeAlerts"`
	TestHealth      []*TestHealth `json:"testHealth"`
}

// TenantOverview returns the counts and per-test health for a tenant.
// Health is aggregated over recent checks per test, using a window sized
// to the test's own interval so multi-target tests (which emit several
// results per cycle) are judged as a whole and stale tests fall to "no data".
func (s *Store) TenantOverview(tenantID string) (*Overview, error) {
	ov := &Overview{TestHealth: []*TestHealth{}}

	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sites WHERE tenant_id = ?`, tenantID).Scan(&ov.Sites); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE tenant_id = ?`, tenantID).Scan(&ov.Agents); err != nil {
		return nil, err
	}

	tests, err := s.ListTests(tenantID)
	if err != nil {
		return nil, err
	}
	ov.Tests = len(tests)

	if n, err := s.CountActiveAlerts(tenantID); err == nil {
		ov.ActiveAlerts = n
	}

	for _, t := range tests {
		h := &TestHealth{TestID: t.ID, Name: t.Name, Type: t.Type, Status: "nodata"}

		// Consider the last ~3 cycles, clamped to [90s, 1h].
		windowSec := t.IntervalSeconds * 3
		if windowSec < 90 {
			windowSec = 90
		}
		if windowSec > 3600 {
			windowSec = 3600
		}
		since := time.Now().Add(-time.Duration(windowSec) * time.Second).UTC()

		var checks, okSum, agents int
		err := s.db.QueryRow(`
			SELECT COUNT(*), COALESCE(SUM(ok), 0), COUNT(DISTINCT r.agent_id)
			FROM results r
			JOIN agents a ON a.id = r.agent_id
			WHERE a.tenant_id = ? AND r.test_id = ? AND r.time >= ?`,
			tenantID, t.ID, since,
		).Scan(&checks, &okSum, &agents)
		if err != nil {
			return nil, err
		}

		if checks > 0 {
			h.Checks = checks
			h.OK = okSum
			h.Agents = agents

			// Fetch last-seen as a plain column scan (aggregate MAX loses
			// the time affinity in the sqlite driver).
			var last time.Time
			if err := s.db.QueryRow(`
				SELECT r.time FROM results r
				JOIN agents a ON a.id = r.agent_id
				WHERE a.tenant_id = ? AND r.test_id = ?
				ORDER BY r.id DESC LIMIT 1`,
				tenantID, t.ID,
			).Scan(&last); err == nil {
				h.LastSeen = &last
			}

			switch {
			case okSum == checks:
				h.Status = "healthy"
			case okSum == 0:
				h.Status = "failing"
			default:
				h.Status = "degraded"
			}
		}
		ov.TestHealth = append(ov.TestHealth, h)
	}
	return ov, nil
}
