package store

import (
	"encoding/json"
	"time"
)

// Thresholds represents the warn/crit boundaries for test state.
type Thresholds struct {
	Warn float64 `json:"warn"`
	Crit float64 `json:"crit"`
}

// computeResultState computes the state of a result (green/orange/red)
// based on the test type's thresholds. Failed results are always red.
// For speedtest (lower-is-worse): value < warn => orange, < crit => red
// For others (higher-is-worse): value > warn => orange, > crit => red
func computeResultState(testType string, value float64, thresholds *Thresholds) string {
	if thresholds == nil {
		return "green"
	}

	isSpeedtest := testType == "speedtest"

	if isSpeedtest {
		// Lower is worse: below thresholds is bad
		if thresholds.Crit > 0 && value < thresholds.Crit {
			return "red"
		}
		if thresholds.Warn > 0 && value < thresholds.Warn {
			return "orange"
		}
	} else {
		// Higher is worse: above thresholds is bad
		if thresholds.Crit > 0 && value > thresholds.Crit {
			return "red"
		}
		if thresholds.Warn > 0 && value > thresholds.Warn {
			return "orange"
		}
	}

	return "green"
}

type TestHealth struct {
	TestID   string     `json:"testId"`
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Checks   int        `json:"checks"` // recent checks considered
	OK       int        `json:"ok"`     // of those, how many were healthy
	Agents   int        `json:"agents"` // distinct agents reporting
	Status   string     `json:"status"` // healthy | degraded | failing | nodata
	LastSeen *time.Time `json:"lastSeen,omitempty"`
	Series   []float64  `json:"series,omitempty"`   // last ~30 values, oldest first; null values omitted
	Unit     string     `json:"unit,omitempty"`     // ms, Mbps, hops, APs
	Current  *float64   `json:"current,omitempty"` // last value
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

		status, checks, okSum, agents, err := s.testStatus(tenantID, siteID, t)
		if err != nil {
			return nil, err
		}

		if checks > 0 {
			h.Status = status
			h.Checks = checks
			h.OK = okSum
			h.Agents = agents

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
		status, _, _, _, err := s.testStatus(tenantID, p.siteID, t)
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
func (s *Store) testStatus(tenantID, siteID string, t *TestDef) (status string, checks, okSum, agents int, err error) {
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
		return "", 0, 0, 0, err
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
			val := extractMetricValue(t.Type, payload)
			if val != nil {
				state := computeResultState(t.Type, *val, thresholds)
				if state == "red" {
					redCount++
				} else if state == "orange" {
					orangeCount++
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", 0, 0, 0, err
	}

	// Count distinct agents
	agentQuery := `
		SELECT COUNT(DISTINCT r.agent_id)
		FROM results r
		JOIN agents a ON a.id = r.agent_id
		WHERE a.tenant_id = ? AND r.test_id = ? AND r.time >= ?`
	agentArgs := []interface{}{tenantID, t.ID, since}
	if siteID != "" {
		agentQuery += ` AND a.site_id = ?`
		agentArgs = append(agentArgs, siteID)
	}
	if err := s.db.QueryRow(agentQuery, agentArgs...).Scan(&agents); err != nil {
		agents = 0
	}

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
	return status, checks, okSum, agents, nil
}

// extractMetricValue extracts the primary metric from a result payload.
func extractMetricValue(testType, payload string) *float64 {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return nil
	}

	var val float64
	switch testType {
	case "ping":
		if ping, ok := data["ping"].(map[string]interface{}); ok {
			if avgRtt, ok := ping["avgRttMs"].(float64); ok && avgRtt > 0 {
				val = avgRtt
			}
		}
	case "dns":
		if dns, ok := data["dns"].(map[string]interface{}); ok {
			if resolveTime, ok := dns["resolveTimeMs"].(float64); ok && resolveTime > 0 {
				val = resolveTime
			}
		}
	case "http":
		if http, ok := data["http"].(map[string]interface{}); ok {
			if total, ok := http["totalMs"].(float64); ok && total > 0 {
				val = total
			}
		}
	case "tcp":
		if tcp, ok := data["tcp"].(map[string]interface{}); ok {
			if connectTime, ok := tcp["connectMs"].(float64); ok && connectTime > 0 {
				val = connectTime
			}
		}
	case "speedtest":
		if speedtest, ok := data["speedtest"].(map[string]interface{}); ok {
			if download, ok := speedtest["downloadMbps"].(float64); ok && download > 0 {
				val = download
			}
		}
	case "traceroute":
		if tr, ok := data["traceroute"].(map[string]interface{}); ok {
			if hops, ok := tr["hops"].([]interface{}); ok {
				val = float64(len(hops))
			}
		}
	case "wlan_passive":
		if wp, ok := data["wlanPassive"].(map[string]interface{}); ok {
			// Extract max utilization across all channels
			if channels, ok := wp["channels"].([]interface{}); ok {
				for _, chData := range channels {
					if ch, ok := chData.(map[string]interface{}); ok {
						if util, ok := ch["utilizationPct"].(float64); ok && util > val {
							val = util
						}
					}
				}
			}
		}
	case "wlan_active":
		if wa, ok := data["wlanActive"].(map[string]interface{}); ok {
			assoc, _ := wa["associateMs"].(float64)
			auth, _ := wa["authenticateMs"].(float64)
			dhcp, _ := wa["dhcpMs"].(float64)
			val = assoc + auth + dhcp
		}
	}

	if val > 0 {
		return &val
	}
	return nil
}

// extractSeries extracts the last ~30 results for a test and returns the unit, series data, and current value.
// Series data is the primary metric per test type (oldest first), with null values omitted.
func (s *Store) extractSeries(testType, testID, tenantID, siteID string) (string, []float64, *float64) {
	unit := "ms" // default
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
		return unit, nil, nil
	}
	defer rows.Close()

	var payloads []string
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err == nil {
			payloads = append(payloads, payload)
		}
	}

	// Reverse to get oldest first
	for i := len(payloads)/2 - 1; i >= 0; i-- {
		opp := len(payloads) - 1 - i
		payloads[i], payloads[opp] = payloads[opp], payloads[i]
	}

	// Extract series based on test type
	var series []float64
	var current *float64

	// This is a simplified extraction; in production you'd parse JSON payloads
	// For now, this is a placeholder that will be filled in per test type
	switch testType {
	case "ping":
		unit = "ms"
		// Extract avg latency from ping results
		series, current = extractPingMetric(payloads)
	case "dns":
		unit = "ms"
		series, current = extractDNSMetric(payloads)
	case "http":
		unit = "ms"
		series, current = extractHTTPMetric(payloads)
	case "tcp":
		unit = "ms"
		series, current = extractTCPMetric(payloads)
	case "speedtest":
		unit = "Mbps"
		series, current = extractSpeedtestMetric(payloads)
	case "traceroute":
		unit = "hops"
		series, current = extractTracerouteMetric(payloads)
	case "wlan_passive":
		unit = "%"
		series, current = extractWlanPassiveMetric(payloads)
	case "wlan_active":
		unit = "ms"
		series, current = extractWlanActiveMetric(payloads)
	}

	return unit, series, current
}

// Helper functions to extract metrics from JSON payloads
func extractPingMetric(payloads []string) ([]float64, *float64) {
	var series []float64
	for _, p := range payloads {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(p), &data); err != nil {
			continue
		}
		// Extract AvgRttMs from nested ping result (payload.ping.avgRttMs)
		var val float64
		if ping, ok := data["ping"].(map[string]interface{}); ok {
			if avgRtt, ok := ping["avgRttMs"].(float64); ok && avgRtt > 0 {
				val = avgRtt
			}
		}
		if val > 0 {
			series = append(series, val)
		}
	}
	if len(series) == 0 {
		return series, nil
	}
	last := series[len(series)-1]
	return series, &last
}

// extractNested pulls payload[section][key] as the metric for each result.
func extractNested(payloads []string, section, key string) ([]float64, *float64) {
	var series []float64
	for _, p := range payloads {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(p), &data); err != nil {
			continue
		}
		sec, ok := data[section].(map[string]interface{})
		if !ok {
			continue
		}
		if val, ok := sec[key].(float64); ok && val > 0 {
			series = append(series, val)
		}
	}
	if len(series) == 0 {
		return series, nil
	}
	last := series[len(series)-1]
	return series, &last
}

func extractDNSMetric(payloads []string) ([]float64, *float64) {
	return extractNested(payloads, "dns", "resolveTimeMs")
}

func extractHTTPMetric(payloads []string) ([]float64, *float64) {
	return extractNested(payloads, "http", "totalMs")
}

func extractTCPMetric(payloads []string) ([]float64, *float64) {
	return extractNested(payloads, "tcp", "connectMs")
}

func extractSpeedtestMetric(payloads []string) ([]float64, *float64) {
	return extractNested(payloads, "speedtest", "downloadMbps")
}

// extractTracerouteMetric uses the hop count of each run.
func extractTracerouteMetric(payloads []string) ([]float64, *float64) {
	var series []float64
	for _, p := range payloads {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(p), &data); err != nil {
			continue
		}
		tr, ok := data["traceroute"].(map[string]interface{})
		if !ok {
			continue
		}
		if hops, ok := tr["hops"].([]interface{}); ok && len(hops) > 0 {
			series = append(series, float64(len(hops)))
		}
	}
	if len(series) == 0 {
		return series, nil
	}
	last := series[len(series)-1]
	return series, &last
}

// extractWlanActiveMetric extracts the connect time (associate + authenticate
// + DHCP) from WLAN active results. Scan time is excluded — SSID discovery is
// harness-internal and its variance would drown the signal.
func extractWlanActiveMetric(payloads []string) ([]float64, *float64) {
	var series []float64
	for _, p := range payloads {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(p), &data); err != nil {
			continue
		}
		wa, ok := data["wlanActive"].(map[string]interface{})
		if !ok {
			continue
		}
		assoc, _ := wa["associateMs"].(float64)
		auth, _ := wa["authenticateMs"].(float64)
		dhcp, _ := wa["dhcpMs"].(float64)
		if connect := assoc + auth + dhcp; connect > 0 {
			series = append(series, connect)
		}
	}
	if len(series) == 0 {
		return series, nil
	}
	last := series[len(series)-1]
	return series, &last
}

// extractWlanPassiveMetric extracts the max channel utilization from WLAN passive results.
func extractWlanPassiveMetric(payloads []string) ([]float64, *float64) {
	var series []float64
	for _, p := range payloads {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(p), &data); err != nil {
			continue
		}
		wp, ok := data["wlanPassive"].(map[string]interface{})
		if !ok {
			continue
		}
		// Extract max utilization across all channels
		var maxUtil float64
		if channels, ok := wp["channels"].([]interface{}); ok {
			for _, chData := range channels {
				if ch, ok := chData.(map[string]interface{}); ok {
					if util, ok := ch["utilizationPct"].(float64); ok && util > maxUtil {
						maxUtil = util
					}
				}
			}
		}
		if maxUtil > 0 {
			series = append(series, maxUtil)
		}
	}
	if len(series) == 0 {
		return series, nil
	}
	last := series[len(series)-1]
	return series, &last
}
