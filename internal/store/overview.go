package store

import (
	"encoding/json"
	"time"
)

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
		resultQuery := `
			SELECT COUNT(*), COALESCE(SUM(ok), 0), COUNT(DISTINCT r.agent_id)
			FROM results r
			JOIN agents a ON a.id = r.agent_id
			WHERE a.tenant_id = ? AND r.test_id = ? AND r.time >= ?`
		resultArgs := []interface{}{tenantID, t.ID, since}
		if siteID != "" {
			resultQuery += ` AND a.site_id = ?`
			resultArgs = append(resultArgs, siteID)
		}
		err := s.db.QueryRow(resultQuery, resultArgs...).Scan(&checks, &okSum, &agents)
		if err != nil {
			return nil, err
		}

		if checks > 0 {
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
	case "wlan_scan":
		unit = "APs"
		series, current = extractWLANMetric(payloads)
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

// extractWLANMetric uses the number of access points seen per scan.
func extractWLANMetric(payloads []string) ([]float64, *float64) {
	var series []float64
	for _, p := range payloads {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(p), &data); err != nil {
			continue
		}
		ws, ok := data["wlanScan"].(map[string]interface{})
		if !ok {
			continue
		}
		if aps, ok := ws["accessPoints"].([]interface{}); ok {
			series = append(series, float64(len(aps)))
		}
	}
	if len(series) == 0 {
		return series, nil
	}
	last := series[len(series)-1]
	return series, &last
}
