package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"
)

type Agent struct {
	ID       string `json:"id"`
	TenantID string `json:"tenantId"`
	SiteID   string `json:"siteId"`
	SiteName string `json:"siteName,omitempty"`
	Name     string `json:"name"`
	Token    string `json:"token,omitempty"`
	// NetworkInterfaces is every non-loopback interface (wired or
	// wireless) the agent last reported at Register — name, wireless,
	// monitor-mode support, wired link speed, current IP. Lets the
	// operator pick per-role interfaces below from a dropdown instead of
	// typing a name or IP blind.
	NetworkInterfaces json.RawMessage `json:"networkInterfaces"`
	Capabilities      json.RawMessage `json:"capabilities"`
	Stats             json.RawMessage `json:"stats,omitempty"`
	Version           string          `json:"version,omitempty"`
	// WlanSensorInterface pins wlan_passive/wlan_active to a specific
	// interface (by name, from NetworkInterfaces); empty = auto-pick the
	// first monitor-capable one. Pushed live via Config.wlan_sensor_interface.
	WlanSensorInterface string `json:"wlanSensorInterface,omitempty"`
	// Perfmon reflector settings: operator-configured on the Agents page
	// and pushed live to the agent (see internal/server's
	// perfmonReflectorConfigFor) — the agent no longer self-reports any of
	// this, since the server has no other way to learn it (agents dial out
	// only, never dialed into). PerfmonReflectorInterface names an entry in
	// NetworkInterfaces; its current IP (see ResolvedPerfmonAddr) is what
	// gets offered as this agent's reachable address, so the operator never
	// types an IP by hand.
	PerfmonReflectorEnabled   bool            `json:"perfmonReflectorEnabled"`
	PerfmonReflectorPort      uint32          `json:"perfmonReflectorPort,omitempty"`
	PerfmonReflectorInterface string          `json:"perfmonReflectorInterface,omitempty"`
	PerfmonAllowedCIDRs       json.RawMessage `json:"perfmonAllowedCidrs,omitempty"`
	CreatedAt                 time.Time       `json:"createdAt"`
}

// InterfaceIP returns the current IP address of the named interface from
// this agent's last-reported NetworkInterfaces, or "" if unknown (name
// empty, interface not found, or it has no address right now).
func (a *Agent) InterfaceIP(name string) string {
	if name == "" || len(a.NetworkInterfaces) == 0 {
		return ""
	}
	var ifaces []struct {
		Name      string `json:"name"`
		IPAddress string `json:"ipAddress"`
	}
	if err := json.Unmarshal(a.NetworkInterfaces, &ifaces); err != nil {
		return ""
	}
	for _, ni := range ifaces {
		if ni.Name == name {
			return ni.IPAddress
		}
	}
	return ""
}

// ResolvedPerfmonAddr is the reachable host:port for this agent's perfmon
// reflector, derived from the current IP of PerfmonReflectorInterface —
// empty unless the reflector is enabled, an interface is picked, that
// interface currently has an IP, and a port is set. Display/API only;
// nothing here is persisted, it's always recomputed from the latest
// reported interface state.
func (a *Agent) ResolvedPerfmonAddr() string {
	if !a.PerfmonReflectorEnabled || a.PerfmonReflectorPort == 0 {
		return ""
	}
	ip := a.InterfaceIP(a.PerfmonReflectorInterface)
	if ip == "" {
		return ""
	}
	return net.JoinHostPort(ip, strconv.Itoa(int(a.PerfmonReflectorPort)))
}

// ResolvedManagementAddr is this agent's informational primary IP: the
// first wired interface with a current address, falling back to the first
// wireless one. Fully automatic (no operator pick) — display only, nothing
// server-side or agent-side behaves differently based on it.
func (a *Agent) ResolvedManagementAddr() string {
	if len(a.NetworkInterfaces) == 0 {
		return ""
	}
	var ifaces []struct {
		Wireless  bool   `json:"wireless"`
		IPAddress string `json:"ipAddress"`
	}
	if err := json.Unmarshal(a.NetworkInterfaces, &ifaces); err != nil {
		return ""
	}
	var wirelessFallback string
	for _, ni := range ifaces {
		if ni.IPAddress == "" {
			continue
		}
		if !ni.Wireless {
			return ni.IPAddress
		}
		if wirelessFallback == "" {
			wirelessFallback = ni.IPAddress
		}
	}
	return wirelessFallback
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
	var netIfaces, capabilities, stats, allowedCIDRs string
	var enabled int
	err := row.Scan(&a.ID, &a.TenantID, &a.SiteID, &a.SiteName, &a.Name, &a.Token,
		&netIfaces, &capabilities, &stats, &a.Version,
		&a.WlanSensorInterface,
		&enabled, &a.PerfmonReflectorPort, &a.PerfmonReflectorInterface, &allowedCIDRs,
		&a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if netIfaces == "" {
		netIfaces = "[]"
	}
	a.NetworkInterfaces = json.RawMessage(netIfaces)
	if capabilities == "" {
		capabilities = "[]"
	}
	a.Capabilities = json.RawMessage(capabilities)
	if stats != "" {
		a.Stats = json.RawMessage(stats)
	}
	if allowedCIDRs == "" {
		allowedCIDRs = "[]"
	}
	a.PerfmonAllowedCIDRs = json.RawMessage(allowedCIDRs)
	a.PerfmonReflectorEnabled = enabled != 0
	return a, nil
}

// agentCols reuses two pre-existing columns under new meanings, added
// before either was ever wired to real behavior, so no destructive
// migration was needed: wireless_interfaces now holds the full wired+
// wireless NetworkInterfaces inventory (was wireless-only), and
// wlan_interface (added, never read/written by any code) now holds the
// operator-picked WLAN sensor interface. perfmon_advertise_host,
// perfmon_addr and management_interface from earlier designs are left in
// the schema unused — the management address is now auto-derived (see
// Agent.ResolvedManagementAddr), not operator-picked.
const agentCols = `a.id, a.tenant_id, a.site_id, s.name, a.name, a.token,
	COALESCE(a.wireless_interfaces, ''), COALESCE(a.capabilities, ''), COALESCE(a.stats, ''), a.version,
	a.wlan_interface,
	a.perfmon_reflector_enabled, a.perfmon_reflector_port, a.perfmon_reflector_interface,
	COALESCE(a.perfmon_allowed_cidrs, ''), a.created_at`
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
func (s *Store) UpdateAgent(id, name, siteID string) error {
	res, err := s.db.Exec(
		`UPDATE agents SET name = ?, site_id = ? WHERE id = ?`,
		name, siteID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetAgentVersion records the build version an agent reported on register.
func (s *Store) SetAgentVersion(id, version string) error {
	_, err := s.db.Exec(`UPDATE agents SET version = ? WHERE id = ?`, version, id)
	return err
}

// SetAgentPerfmonReflector records an agent's perfmon reflector settings
// (operator-configured on the Agents page) — enabled, port, which reported
// interface to advertise (resolved to an IP at read time, see
// Agent.ResolvedPerfmonAddr), and the source-CIDR allowlist the agent
// itself enforces.
func (s *Store) SetAgentPerfmonReflector(id string, enabled bool, port uint32, iface string, allowedCIDRs json.RawMessage) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err := s.db.Exec(
		`UPDATE agents SET perfmon_reflector_enabled = ?, perfmon_reflector_port = ?,
		 perfmon_reflector_interface = ?, perfmon_allowed_cidrs = ? WHERE id = ?`,
		enabledInt, port, iface, string(allowedCIDRs), id,
	)
	return err
}

// SetAgentWlanSensorInterface records which interface pins wlan_passive/
// wlan_active — pushed to the agent live via Config.wlan_sensor_interface,
// so this takes effect without a restart.
func (s *Store) SetAgentWlanSensorInterface(id, iface string) error {
	_, err := s.db.Exec(`UPDATE agents SET wlan_interface = ? WHERE id = ?`, iface, id)
	return err
}

// SetAgentNetworkInterfaces records the full wired+wireless interface
// inventory an agent reported at Register.
func (s *Store) SetAgentNetworkInterfaces(id string, interfaces json.RawMessage) error {
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
