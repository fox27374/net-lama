package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type Agent struct {
	ID                 string          `json:"id"`
	TenantID           string          `json:"tenantId"`
	SiteID             string          `json:"siteId"`
	SiteName           string          `json:"siteName,omitempty"`
	Name               string          `json:"name"`
	Token              string          `json:"token,omitempty"`
	WlanInterface      string          `json:"wlanInterface"`
	WirelessInterfaces json.RawMessage `json:"wirelessInterfaces"`
	Capabilities       json.RawMessage `json:"capabilities"`
	Stats              json.RawMessage `json:"stats,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
}

type Result struct {
	ID        int64           `json:"id"`
	AgentID   string          `json:"agentId"`
	AgentName string          `json:"agentName,omitempty"`
	SiteID    string          `json:"siteId,omitempty"`
	SiteName  string          `json:"siteName,omitempty"`
	TestID    string          `json:"testId,omitempty"`
	TestName  string          `json:"testName,omitempty"`
	TestType  string          `json:"testType"`
	Time      time.Time       `json:"time"`
	Error     string          `json:"error,omitempty"`
	OK        bool            `json:"ok"`
	Payload   json.RawMessage `json:"payload"`
}

// ResultFilter narrows down result queries; empty fields are ignored.
// TenantID is mandatory for scoping.
type ResultFilter struct {
	TenantID string
	SiteID   string
	AgentID  string
	TestID   string
	TestType string
	Since    time.Time
	Limit    int
}

// resultsPerAgent caps stored history per agent; older rows are pruned.
const resultsPerAgent = 5000

func (s *Store) CreateAgent(tenantID, siteID, name string) (*Agent, error) {
	a := &Agent{
		ID:        newID(),
		TenantID:  tenantID,
		SiteID:    siteID,
		Name:      name,
		Token:     NewToken(),
		CreatedAt: time.Now(),
	}
	_, err := s.db.Exec(
		`INSERT INTO agents (id, tenant_id, site_id, name, token) VALUES (?, ?, ?, ?, ?)`,
		a.ID, a.TenantID, a.SiteID, a.Name, a.Token,
	)
	if err != nil {
		return nil, fmt.Errorf("creating agent: %w", err)
	}
	return a, nil
}

func (s *Store) scanAgent(row interface{ Scan(...any) error }) (*Agent, error) {
	a := &Agent{}
	var wlanIface, wireless, capabilities, stats string
	err := row.Scan(&a.ID, &a.TenantID, &a.SiteID, &a.SiteName, &a.Name, &a.Token,
		&wlanIface, &wireless, &capabilities, &stats, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.WlanInterface = wlanIface
	if wireless == "" {
		wireless = "[]"
	}
	a.WirelessInterfaces = json.RawMessage(wireless)
	if capabilities == "" {
		capabilities = "[]"
	}
	a.Capabilities = json.RawMessage(capabilities)
	if stats != "" {
		a.Stats = json.RawMessage(stats)
	}
	return a, nil
}

const agentCols = `a.id, a.tenant_id, a.site_id, s.name, a.name, a.token,
	COALESCE(a.wlan_interface, ''), COALESCE(a.wireless_interfaces, ''), COALESCE(a.capabilities, ''), COALESCE(a.stats, ''), a.created_at`
const agentFrom = ` FROM agents a JOIN sites s ON s.id = a.site_id `

func (s *Store) GetAgent(id string) (*Agent, error) {
	return s.scanAgent(s.db.QueryRow(`SELECT `+agentCols+agentFrom+`WHERE a.id = ?`, id))
}

func (s *Store) GetAgentByToken(token string) (*Agent, error) {
	return s.scanAgent(s.db.QueryRow(`SELECT `+agentCols+agentFrom+`WHERE a.token = ?`, token))
}

// ListAgents returns all agents, or only those of a tenant if tenantID is set.
func (s *Store) ListAgents(tenantID string) ([]*Agent, error) {
	query := `SELECT ` + agentCols + agentFrom + `ORDER BY a.name`
	args := []any{}
	if tenantID != "" {
		query = `SELECT ` + agentCols + agentFrom + `WHERE a.tenant_id = ? ORDER BY a.name`
		args = append(args, tenantID)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := []*Agent{}
	for rows.Next() {
		a, err := s.scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *Store) DeleteAgent(id string) error {
	_, err := s.db.Exec(`DELETE FROM agents WHERE id = ?`, id)
	return err
}

// UpdateAgent renames an agent, moves it to another site and sets its
// WLAN sensor interface.
func (s *Store) UpdateAgent(id, name, siteID, wlanInterface string) error {
	res, err := s.db.Exec(
		`UPDATE agents SET name = ?, site_id = ?, wlan_interface = ? WHERE id = ?`,
		name, siteID, wlanInterface, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetAgentInterfaces records the wireless interfaces an agent reported.
func (s *Store) SetAgentInterfaces(id string, interfaces json.RawMessage) error {
	_, err := s.db.Exec(`UPDATE agents SET wireless_interfaces = ? WHERE id = ?`, string(interfaces), id)
	return err
}

// SetAgentCapabilities records the capabilities an agent reported.
func (s *Store) SetAgentCapabilities(id string, capabilities json.RawMessage) error {
	_, err := s.db.Exec(`UPDATE agents SET capabilities = ? WHERE id = ?`, string(capabilities), id)
	return err
}

// SetAgentStats records the latest statistics reported by an agent.
func (s *Store) SetAgentStats(id string, stats interface{}) error {
	statsJSON, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("marshalling stats: %w", err)
	}
	_, err = s.db.Exec(`UPDATE agents SET stats = ? WHERE id = ?`, string(statsJSON), id)
	return err
}

func (s *Store) AddResult(r *Result) error {
	okInt := 0
	if r.OK {
		okInt = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO results (agent_id, test_id, test_name, test_type, time, error, ok, payload)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.AgentID, r.TestID, r.TestName, r.TestType, r.Time.UTC(), r.Error, okInt, string(r.Payload),
	)
	if err != nil {
		return err
	}
	// Prune old rows to keep the database bounded
	_, err = s.db.Exec(
		`DELETE FROM results WHERE agent_id = ? AND id NOT IN
		 (SELECT id FROM results WHERE agent_id = ? ORDER BY id DESC LIMIT ?)`,
		r.AgentID, r.AgentID, resultsPerAgent,
	)
	return err
}

// ListResults returns the most recent results matching the filter,
// newest first, including agent and site names for display.
func (s *Store) ListResults(f ResultFilter) ([]*Result, error) {
	if f.Limit <= 0 || f.Limit > 2000 {
		f.Limit = 100
	}

	query := `
		SELECT r.id, r.agent_id, a.name, a.site_id, s.name,
		       r.test_id, r.test_name, r.test_type, r.time, r.error, r.ok, r.payload
		FROM results r
		JOIN agents a ON a.id = r.agent_id
		JOIN sites s ON s.id = a.site_id
		WHERE a.tenant_id = ?`
	args := []any{f.TenantID}

	if f.SiteID != "" {
		query += ` AND a.site_id = ?`
		args = append(args, f.SiteID)
	}
	if f.AgentID != "" {
		query += ` AND r.agent_id = ?`
		args = append(args, f.AgentID)
	}
	if f.TestID != "" {
		query += ` AND r.test_id = ?`
		args = append(args, f.TestID)
	}
	if f.TestType != "" {
		query += ` AND r.test_type = ?`
		args = append(args, f.TestType)
	}
	if !f.Since.IsZero() {
		query += ` AND r.time >= ?`
		args = append(args, f.Since.UTC())
	}
	query += ` ORDER BY r.id DESC LIMIT ?`
	args = append(args, f.Limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []*Result{}
	for rows.Next() {
		r := &Result{}
		var payload string
		var okInt int
		if err := rows.Scan(&r.ID, &r.AgentID, &r.AgentName, &r.SiteID, &r.SiteName,
			&r.TestID, &r.TestName, &r.TestType, &r.Time, &r.Error, &okInt, &payload); err != nil {
			return nil, err
		}
		r.OK = okInt == 1
		r.Payload = json.RawMessage(payload)
		results = append(results, r)
	}
	return results, rows.Err()
}
