package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type AlertRule struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenantId"`
	TestID         string    `json:"testId"`
	TestName       string    `json:"testName,omitempty"`
	Name           string    `json:"name"`
	Metric         string    `json:"metric"`
	Operator       string    `json:"operator"`
	Threshold      float64   `json:"threshold"`
	ForCount       int       `json:"forCount"`
	ClearThreshold *float64  `json:"clearThreshold"`
	ClearCount     int       `json:"clearCount"`
	TargetIds      []string  `json:"targetIds"`
	WebhookURL     string    `json:"webhookUrl"`
}

type AlertTarget struct {
	ID       string            `json:"id"`
	TenantID string            `json:"tenantId"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Config   map[string]any    `json:"config"`
}

type Alert struct {
	ID         int64      `json:"id"`
	RuleID     string     `json:"ruleId"`
	RuleName   string     `json:"ruleName,omitempty"`
	AgentID    string     `json:"agentId"`
	AgentName  string     `json:"agentName,omitempty"`
	Subject    string     `json:"subject,omitempty"`
	State      string     `json:"state"`
	Value      float64    `json:"value"`
	Message    string     `json:"message"`
	StartedAt  time.Time  `json:"startedAt"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
}

// --- Rules ---

func (s *Store) CreateAlertRule(r *AlertRule) (*AlertRule, error) {
	r.ID = newID()
	if r.TargetIds == nil {
		r.TargetIds = []string{}
	}
	if r.ClearCount == 0 {
		r.ClearCount = 1
	}
	targetIDsJSON, _ := json.Marshal(r.TargetIds)
	_, err := s.db.Exec(
		`INSERT INTO alert_rules (id, tenant_id, test_id, name, metric, operator, threshold, for_count, clear_threshold, clear_count, target_ids, webhook_url)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.TenantID, r.TestID, r.Name, r.Metric, r.Operator, r.Threshold, r.ForCount, r.ClearThreshold, r.ClearCount, string(targetIDsJSON), r.WebhookURL)
	if err != nil {
		return nil, fmt.Errorf("creating alert rule: %w", err)
	}
	return r, nil
}

func (s *Store) DeleteAlertRule(id string) error {
	_, err := s.db.Exec(`DELETE FROM alert_rules WHERE id = ?`, id)
	return err
}

func (s *Store) GetAlertRule(id string) (*AlertRule, error) {
	r := &AlertRule{}
	var targetIDsJSON string
	err := s.db.QueryRow(
		`SELECT id, tenant_id, test_id, name, metric, operator, threshold, for_count, clear_threshold, clear_count, target_ids, webhook_url
		 FROM alert_rules WHERE id = ?`, id).
		Scan(&r.ID, &r.TenantID, &r.TestID, &r.Name, &r.Metric, &r.Operator, &r.Threshold, &r.ForCount, &r.ClearThreshold, &r.ClearCount, &targetIDsJSON, &r.WebhookURL)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if targetIDsJSON != "" {
		json.Unmarshal([]byte(targetIDsJSON), &r.TargetIds)
	}
	return r, nil
}

func (s *Store) ListAlertRules(tenantID string) ([]*AlertRule, error) {
	rows, err := s.db.Query(`
		SELECT ar.id, ar.tenant_id, ar.test_id, t.name, ar.name, ar.metric,
		       ar.operator, ar.threshold, ar.for_count, ar.clear_threshold, ar.clear_count, ar.target_ids, ar.webhook_url
		FROM alert_rules ar JOIN tests t ON t.id = ar.test_id
		WHERE ar.tenant_id = ? ORDER BY ar.name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := []*AlertRule{}
	for rows.Next() {
		r := &AlertRule{}
		var targetIDsJSON string
		if err := rows.Scan(&r.ID, &r.TenantID, &r.TestID, &r.TestName, &r.Name, &r.Metric,
			&r.Operator, &r.Threshold, &r.ForCount, &r.ClearThreshold, &r.ClearCount, &targetIDsJSON, &r.WebhookURL); err != nil {
			return nil, err
		}
		if targetIDsJSON != "" {
			json.Unmarshal([]byte(targetIDsJSON), &r.TargetIds)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// RulesForTest returns the rules watching a given test (for evaluation).
func (s *Store) RulesForTest(testID string) ([]*AlertRule, error) {
	rows, err := s.db.Query(
		`SELECT ar.id, ar.tenant_id, ar.test_id, t.name, ar.name, ar.metric,
		        ar.operator, ar.threshold, ar.for_count, ar.clear_threshold, ar.clear_count, ar.target_ids, ar.webhook_url
		 FROM alert_rules ar JOIN tests t ON t.id = ar.test_id
		 WHERE ar.test_id = ?`, testID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := []*AlertRule{}
	for rows.Next() {
		r := &AlertRule{}
		var targetIDsJSON string
		if err := rows.Scan(&r.ID, &r.TenantID, &r.TestID, &r.TestName, &r.Name, &r.Metric,
			&r.Operator, &r.Threshold, &r.ForCount, &r.ClearThreshold, &r.ClearCount, &targetIDsJSON, &r.WebhookURL); err != nil {
			return nil, err
		}
		if targetIDsJSON != "" {
			json.Unmarshal([]byte(targetIDsJSON), &r.TargetIds)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// --- Alert state ---

// ActiveAlert returns the firing alert for a rule+agent+subject, or nil.
func (s *Store) ActiveAlert(ruleID, agentID, subject string) (*Alert, error) {
	a := &Alert{}
	err := s.db.QueryRow(
		`SELECT id, rule_id, agent_id, subject, state, value, message, started_at
		 FROM alerts WHERE rule_id = ? AND agent_id = ? AND subject = ? AND state = 'firing'
		 ORDER BY id DESC LIMIT 1`, ruleID, agentID, subject).
		Scan(&a.ID, &a.RuleID, &a.AgentID, &a.Subject, &a.State, &a.Value, &a.Message, &a.StartedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) FireAlert(ruleID, agentID, subject string, value float64, message string) (*Alert, error) {
	res, err := s.db.Exec(
		`INSERT INTO alerts (rule_id, agent_id, subject, state, value, message, started_at)
		 VALUES (?, ?, ?, 'firing', ?, ?, ?)`,
		ruleID, agentID, subject, value, message, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Alert{ID: id, RuleID: ruleID, AgentID: agentID, Subject: subject, State: "firing", Value: value, Message: message, StartedAt: time.Now()}, nil
}

func (s *Store) ResolveAlert(id int64) error {
	_, err := s.db.Exec(`UPDATE alerts SET state = 'resolved', resolved_at = ? WHERE id = ?`,
		time.Now().UTC(), id)
	return err
}

// ListAlerts returns active alerts first, then recent resolved ones.
func (s *Store) ListAlerts(tenantID string, limit int) ([]*Alert, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT al.id, al.rule_id, ar.name, al.agent_id, a.name, al.subject, al.state,
		       al.value, al.message, al.started_at, al.resolved_at
		FROM alerts al
		JOIN alert_rules ar ON ar.id = al.rule_id
		JOIN agents a ON a.id = al.agent_id
		WHERE ar.tenant_id = ?
		ORDER BY (al.state = 'firing') DESC, al.id DESC
		LIMIT ?`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	alerts := []*Alert{}
	for rows.Next() {
		a := &Alert{}
		var resolved sql.NullTime
		if err := rows.Scan(&a.ID, &a.RuleID, &a.RuleName, &a.AgentID, &a.AgentName, &a.Subject,
			&a.State, &a.Value, &a.Message, &a.StartedAt, &resolved); err != nil {
			return nil, err
		}
		if resolved.Valid {
			a.ResolvedAt = &resolved.Time
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// CountActiveAlerts returns the number of firing alerts for a tenant.
func (s *Store) CountActiveAlerts(tenantID string) (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM alerts al JOIN alert_rules ar ON ar.id = al.rule_id
		WHERE ar.tenant_id = ? AND al.state = 'firing'`, tenantID).Scan(&n)
	return n, err
}

// UpdateAlertRule updates an existing alert rule.
func (s *Store) UpdateAlertRule(r *AlertRule) (*AlertRule, error) {
	if r.TargetIds == nil {
		r.TargetIds = []string{}
	}
	if r.ClearCount == 0 {
		r.ClearCount = 1
	}
	targetIDsJSON, _ := json.Marshal(r.TargetIds)
	_, err := s.db.Exec(
		`UPDATE alert_rules SET name = ?, metric = ?, operator = ?, threshold = ?, for_count = ?, clear_threshold = ?, clear_count = ?, target_ids = ?, webhook_url = ?
		 WHERE id = ?`,
		r.Name, r.Metric, r.Operator, r.Threshold, r.ForCount, r.ClearThreshold, r.ClearCount, string(targetIDsJSON), r.WebhookURL, r.ID)
	if err != nil {
		return nil, fmt.Errorf("updating alert rule: %w", err)
	}
	return r, nil
}

// --- Alert targets ---

// CreateAlertTarget creates a new alert target.
func (s *Store) CreateAlertTarget(t *AlertTarget) (*AlertTarget, error) {
	t.ID = newID()
	configJSON, _ := json.Marshal(t.Config)
	_, err := s.db.Exec(
		`INSERT INTO alert_targets (id, tenant_id, name, type, config) VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.TenantID, t.Name, t.Type, string(configJSON))
	if err != nil {
		return nil, fmt.Errorf("creating alert target: %w", err)
	}
	return t, nil
}

// GetAlertTarget retrieves an alert target by ID.
func (s *Store) GetAlertTarget(id string) (*AlertTarget, error) {
	t := &AlertTarget{}
	var configJSON string
	err := s.db.QueryRow(
		`SELECT id, tenant_id, name, type, config FROM alert_targets WHERE id = ?`, id).
		Scan(&t.ID, &t.TenantID, &t.Name, &t.Type, &configJSON)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Config = map[string]any{}
	if configJSON != "" {
		json.Unmarshal([]byte(configJSON), &t.Config)
	}
	return t, nil
}

// ListAlertTargets returns all alert targets for a tenant.
func (s *Store) ListAlertTargets(tenantID string) ([]*AlertTarget, error) {
	rows, err := s.db.Query(
		`SELECT id, tenant_id, name, type, config FROM alert_targets WHERE tenant_id = ? ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := []*AlertTarget{}
	for rows.Next() {
		t := &AlertTarget{}
		var configJSON string
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Type, &configJSON); err != nil {
			return nil, err
		}
		t.Config = map[string]any{}
		if configJSON != "" {
			json.Unmarshal([]byte(configJSON), &t.Config)
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// UpdateAlertTarget updates an existing alert target.
func (s *Store) UpdateAlertTarget(t *AlertTarget) (*AlertTarget, error) {
	configJSON, _ := json.Marshal(t.Config)
	_, err := s.db.Exec(
		`UPDATE alert_targets SET name = ?, type = ?, config = ? WHERE id = ?`,
		t.Name, t.Type, string(configJSON), t.ID)
	if err != nil {
		return nil, fmt.Errorf("updating alert target: %w", err)
	}
	return t, nil
}

// DeleteAlertTarget deletes an alert target.
func (s *Store) DeleteAlertTarget(id string) error {
	_, err := s.db.Exec(`DELETE FROM alert_targets WHERE id = ?`, id)
	return err
}
