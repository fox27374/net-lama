package store

import (
	"encoding/json"
	"time"

	"github.com/fox27374/net-lama/internal/testtype"
)

// Thresholds represents the warn/crit boundaries for test state.
type Thresholds struct {
	Warn float64 `json:"warn"`
	Crit float64 `json:"crit"`
}

// computeResultState computes the state of a result (green/orange/red)
// against the test's thresholds. Which direction counts as worse is the
// test type's own property (throughput degrades by falling, latency by
// rising), so it comes from the registry rather than a name check here.
func computeResultState(testType string, value float64, thresholds *Thresholds) string {
	if thresholds == nil {
		return "green"
	}

	spec := testtype.Get(testType)
	if spec != nil && spec.LowerIsWorse {
		if thresholds.Crit > 0 && value < thresholds.Crit {
			return "red"
		}
		if thresholds.Warn > 0 && value < thresholds.Warn {
			return "orange"
		}
		return "green"
	}

	if thresholds.Crit > 0 && value > thresholds.Crit {
		return "red"
	}
	if thresholds.Warn > 0 && value > thresholds.Warn {
		return "orange"
	}
	return "green"
}

type TestHealth struct {
	TestID     string     `json:"testId"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	Checks     int        `json:"checks"`               // recent checks considered
	OK         int        `json:"ok"`                   // of those, how many were healthy
	Agents     int        `json:"agents"`               // distinct agents reporting
	AgentNames []string   `json:"agentNames,omitempty"` // names of those agents
	Status     string     `json:"status"`               // healthy | degraded | failing | nodata
	LastSeen   *time.Time `json:"lastSeen,omitempty"`
	Series     []float64  `json:"series,omitempty"`  // last ~30 values, oldest first; null values omitted
	Unit       string     `json:"unit,omitempty"`    // ms, Mbps, hops, APs
	Current    *float64   `json:"current,omitempty"` // last value
}

// SiteHealth is the per-site rollup: how many of the site's assigned tests
// are in each status, judged only against results from that site's agents.
type SiteHealth struct {
	SiteID   string `json:"siteId"`
	Healthy  int    `json:"healthy"`
	Degraded int    `json:"degraded"`
	Failing  int    `json:"failing"`
	NoData   int    `json:"nodata"`
}

type Overview struct {
	Sites           int           `json:"sites"`
	Agents          int           `json:"agents"`
	AgentsConnected int           `json:"agentsConnected"`
	Tests           int           `json:"tests"`
	ActiveAlerts    int           `json:"activeAlerts"`
	TestHealth      []*TestHealth `json:"testHealth"`
	SiteHealth      []*SiteHealth `json:"siteHealth"`
}

// TenantOverview returns the counts and per-test health for a tenant.
// Health is aggregated over recent checks per test, using a window sized
// to the test's own interval so multi-target tests (which emit several
// results per cycle) are judged as a whole and stale tests fall to "no data".
// If siteID is non-empty, only that site's data is included.
func (s *Store) TenantOverview(tenantID, siteID string) (*Overview, error) {
	ov := &Overview{TestHealth: []*TestHealth{}}

	// Query agents - possibly filtered by site
	agentQuery := `SELECT id FROM agents WHERE tenant_id = ?`
	agentArgs := []interface{}{tenantID}
	if siteID != "" {
		agentQuery += ` AND site_id = ?`
		agentArgs = append(agentArgs, siteID)
	}
	var agentIDs []string
	rows, err := s.db.Query(agentQuery, agentArgs...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		agentIDs = append(agentIDs, id)
	}
	rows.Close()
	ov.Agents = len(agentIDs)

	// Site count
	if siteID != "" {
		ov.Sites = 1
	} else {
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM sites WHERE tenant_id = ?`, tenantID).Scan(&ov.Sites); err != nil {
			return nil, err
		}
	}

	tests, err := s.ListTests(tenantID)
	if err != nil {
		return nil, err
	}
	ov.Tests = len(tests)

	// Count active alerts (site-filtered if needed)
	if siteID != "" {
		// Count alerts for tests assigned to this site
		if err := s.db.QueryRow(`
			SELECT COALESCE(COUNT(*), 0)
			FROM alerts a
			JOIN alert_rules ar ON ar.id = a.rule_id
			JOIN tests t ON t.id = ar.test_id
			WHERE t.tenant_id = ? AND a.state = 'firing'
			AND t.id IN (SELECT test_id FROM site_tests WHERE site_id = ?)
		`, tenantID, siteID).Scan(&ov.ActiveAlerts); err != nil {
			ov.ActiveAlerts = 0
		}
	} else {
		if n, err := s.CountActiveAlerts(tenantID); err == nil {
			ov.ActiveAlerts = n
		}
	}

	for _, t := range tests {
		h := &TestHealth{TestID: t.ID, Name: t.Name, Type: t.Type, Status: "nodata"}

		status, checks, okSum, agents, agentNames, err := s.testStatus(tenantID, siteID, t)
		if err != nil {
			return nil, err
		}

		if checks > 0 {
			h.Status = status
			h.Checks = checks
			h.OK = okSum
			h.Agents = agents
			h.AgentNames = agentNames

			// Fetch last-seen as a plain column scan (aggregate MAX loses
			// the time affinity in the sqlite driver).
			lastQuery := `
				SELECT r.time FROM results r
				JOIN agents a ON a.id = r.agent_id
				WHERE a.tenant_id = ? AND r.test_id = ?`
			lastArgs := []interface{}{tenantID, t.ID}
			if siteID != "" {
				lastQuery += ` AND a.site_id = ?`
				lastArgs = append(lastArgs, siteID)
			}
			lastQuery += ` ORDER BY r.id DESC LIMIT 1`
			var last time.Time
			if err := s.db.QueryRow(lastQuery, lastArgs...).Scan(&last); err == nil {
				h.LastSeen = &last
			}

			// Extract series: last ~30 result values for this test (site-filtered)
			// Unit and current value are determined by test type
			h.Unit, h.Series, h.Current = s.extractSeries(t.Type, t.ID, tenantID, siteID)
		}
		ov.TestHealth = append(ov.TestHealth, h)
	}

	// Per-site rollup: judge each site's assigned tests only against results
	// from that site's own agents, so shared tests can't mask a broken site.
	testByID := make(map[string]*TestDef, len(tests))
	for _, t := range tests {
		testByID[t.ID] = t
	}
	pairQuery := `
		SELECT st.site_id, st.test_id
		FROM site_tests st
		JOIN sites s ON s.id = st.site_id
		WHERE s.tenant_id = ?`
	pairArgs := []interface{}{tenantID}
	if siteID != "" {
		pairQuery += ` AND st.site_id = ?`
		pairArgs = append(pairArgs, siteID)
	}
	pairQuery += ` ORDER BY st.site_id`
	pairs, err := s.db.Query(pairQuery, pairArgs...)
	if err != nil {
		return nil, err
	}
	type pair struct{ siteID, testID string }
	var assigned []pair
	for pairs.Next() {
		var p pair
		if err := pairs.Scan(&p.siteID, &p.testID); err == nil {
			assigned = append(assigned, p)
		}
	}
	pairs.Close()

	rollups := map[string]*SiteHealth{}
	for _, p := range assigned {
		t, ok := testByID[p.testID]
		if !ok {
			continue
		}
		status, _, _, _, _, err := s.testStatus(tenantID, p.siteID, t)
		if err != nil {
			return nil, err
		}
		r := rollups[p.siteID]
		if r == nil {
			r = &SiteHealth{SiteID: p.siteID}
			rollups[p.siteID] = r
			ov.SiteHealth = append(ov.SiteHealth, r)
		}
		switch status {
		case "healthy":
			r.Healthy++
		case "degraded":
			r.Degraded++
		case "failing":
			r.Failing++
		default:
			r.NoData++
		}
	}
	return ov, nil
}

// testStatus judges one test from its recent results — the last ~3 cycles,
// clamped to [90s, 1h] — optionally restricted to a single site's agents.
// A test with no results in the window is "nodata".
// Health rollup now incorporates state thresholds: red > orange > green.
func (s *Store) testStatus(tenantID, siteID string, t *TestDef) (status string, checks, okSum, agents int, agentNames []string, err error) {
	windowSec := t.IntervalSeconds * 3
	if windowSec < 90 {
		windowSec = 90
	}
	if windowSec > 3600 {
		windowSec = 3600
	}
	since := time.Now().Add(-time.Duration(windowSec) * time.Second).UTC()

	// Parse thresholds if present
	var thresholds *Thresholds
	if len(t.Thresholds) > 0 {
		th := &Thresholds{}
		if err := json.Unmarshal(t.Thresholds, th); err == nil {
			thresholds = th
		}
	}

	query := `
		SELECT ok, payload
		FROM results r
		JOIN agents a ON a.id = r.agent_id
		WHERE a.tenant_id = ? AND r.test_id = ? AND r.time >= ?`
	args := []interface{}{tenantID, t.ID, since}
	if siteID != "" {
		query += ` AND a.site_id = ?`
		args = append(args, siteID)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return "", 0, 0, 0, nil, err
	}
	defer rows.Close()

	checks = 0
	okSum = 0
	redCount := 0
	orangeCount := 0

	for rows.Next() {
		checks++
		var ok int
		var payload string
		if err := rows.Scan(&ok, &payload); err != nil {
			continue
		}
		if ok == 1 {
			okSum++
		}

		// Extract agent_id from the subquery to count distinct agents
		// (simplified: just count all results; per-agent tracking would need more query changes)

		// If thresholds are set and result is ok, compute state
		if ok == 1 && thresholds != nil {
			val, hasVal := primaryMetric(t.Type, payload)
			if hasVal {
				state := computeResultState(t.Type, val, thresholds)
				if state == "red" {
					redCount++
				} else if state == "orange" {
					orangeCount++
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", 0, 0, 0, nil, err
	}

	// Distinct agents that reported in the window (name, not just count)
	agentQuery := `
		SELECT DISTINCT a.name
		FROM results r
		JOIN agents a ON a.id = r.agent_id
		WHERE a.tenant_id = ? AND r.test_id = ? AND r.time >= ?`
	agentArgs := []interface{}{tenantID, t.ID, since}
	if siteID != "" {
		agentQuery += ` AND a.site_id = ?`
		agentArgs = append(agentArgs, siteID)
	}
	agentQuery += ` ORDER BY a.name`
	if nameRows, err := s.db.Query(agentQuery, agentArgs...); err == nil {
		for nameRows.Next() {
			var name string
			if nameRows.Scan(&name) == nil {
				agentNames = append(agentNames, name)
			}
		}
		nameRows.Close()
	}
	agents = len(agentNames)

	// Determine status: red > orange > mixed > all green
	switch {
	case checks == 0:
		status = "nodata"
	case redCount > 0:
		status = "failing"
	case orangeCount > 0:
		status = "degraded"
	case okSum == checks:
		status = "healthy"
	case okSum == 0:
		status = "failing"
	default:
		status = "degraded"
	}
	return status, checks, okSum, agents, agentNames, nil
}

// primaryMetric reads a test type's primary metric out of a stored result
// payload. The payload is protojson written by the server, so it is decoded
// back into the typed message rather than parsed as a map and groped for
// field names — a proto field rename is now a compile error here instead of
// a blank sparkline.
func primaryMetric(testType, payload string) (float64, bool) {
	spec := testtype.Get(testType)
	if spec == nil {
		return 0, false
	}
	result, err := testtype.DecodeResult([]byte(payload))
	if err != nil {
		return 0, false
	}
	return spec.Primary(result)
}

// extractSeries returns the unit and the last ~30 primary-metric values for
// a test (oldest first), plus the most recent one. Runs with no usable
// value are skipped rather than plotted as zero.
func (s *Store) extractSeries(testType, testID, tenantID, siteID string) (string, []float64, *float64) {
	spec := testtype.Get(testType)
	if spec == nil {
		return "", nil, nil
	}

	query := `
		SELECT r.payload
		FROM results r
		JOIN agents a ON a.id = r.agent_id
		WHERE a.tenant_id = ? AND r.test_id = ?`
	args := []interface{}{tenantID, testID}
	if siteID != "" {
		query += ` AND a.site_id = ?`
		args = append(args, siteID)
	}
	query += ` ORDER BY r.id DESC LIMIT 30`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return spec.Unit, nil, nil
	}
	defer rows.Close()

	// Newest first out of the query; reverse into chronological order below.
	var series []float64
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			continue
		}
		if val, ok := primaryMetric(testType, payload); ok {
			series = append(series, val)
		}
	}
	for i, j := 0, len(series)-1; i < j; i, j = i+1, j-1 {
		series[i], series[j] = series[j], series[i]
	}

	if len(series) == 0 {
		return spec.Unit, nil, nil
	}
	last := series[len(series)-1]
	return spec.Unit, series, &last
}
