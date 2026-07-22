package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// UnclaimedAgent is a device that registered with a tenant's enrollment
// token instead of a real per-agent one. It has no site or name yet; an
// admin claims it (turning it into a real Agent) via the Agents page.
type UnclaimedAgent struct {
	ID                string          `json:"id"`
	TenantID          string          `json:"tenantId"`
	ClientID          string          `json:"clientId"`
	Version           string          `json:"version,omitempty"`
	Capabilities      json.RawMessage `json:"capabilities,omitempty"`
	NetworkInterfaces json.RawMessage `json:"networkInterfaces,omitempty"`
	FirstSeen         time.Time       `json:"firstSeen"`
	LastSeen          time.Time       `json:"lastSeen"`
}

// UpsertUnclaimedAgent records (or refreshes) one pending enrollment,
// keyed by (tenantID, clientID) so a device retrying its reconnect/backoff
// loop updates the same row instead of piling up duplicates.
func (s *Store) UpsertUnclaimedAgent(tenantID, clientID, version string, capabilities, networkInterfaces json.RawMessage) error {
	_, err := s.db.Exec(`
		INSERT INTO unclaimed_agents (id, tenant_id, client_id, version, capabilities, network_interfaces)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, client_id) DO UPDATE SET
			version = excluded.version,
			capabilities = excluded.capabilities,
			network_interfaces = excluded.network_interfaces,
			last_seen = CURRENT_TIMESTAMP
	`, newID(), tenantID, clientID, version, string(capabilities), string(networkInterfaces))
	return err
}

func scanUnclaimedAgent(row interface{ Scan(...any) error }) (*UnclaimedAgent, error) {
	u := &UnclaimedAgent{}
	var capabilities, networkInterfaces string
	err := row.Scan(&u.ID, &u.TenantID, &u.ClientID, &u.Version, &capabilities, &networkInterfaces, &u.FirstSeen, &u.LastSeen)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if capabilities != "" {
		u.Capabilities = json.RawMessage(capabilities)
	}
	if networkInterfaces != "" {
		u.NetworkInterfaces = json.RawMessage(networkInterfaces)
	}
	return u, nil
}

const unclaimedAgentCols = `id, tenant_id, client_id, version, COALESCE(capabilities, ''), COALESCE(network_interfaces, ''), first_seen, last_seen`

// ListUnclaimedAgents returns pending enrollments, or only those of a
// tenant if tenantID is set, newest-seen first.
func (s *Store) ListUnclaimedAgents(tenantID string) ([]*UnclaimedAgent, error) {
	query := `SELECT ` + unclaimedAgentCols + ` FROM unclaimed_agents ORDER BY last_seen DESC`
	args := []any{}
	if tenantID != "" {
		query = `SELECT ` + unclaimedAgentCols + ` FROM unclaimed_agents WHERE tenant_id = ? ORDER BY last_seen DESC`
		args = append(args, tenantID)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := []*UnclaimedAgent{}
	for rows.Next() {
		u, err := scanUnclaimedAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, u)
	}
	return agents, rows.Err()
}

func (s *Store) GetUnclaimedAgent(id string) (*UnclaimedAgent, error) {
	return scanUnclaimedAgent(s.db.QueryRow(`SELECT `+unclaimedAgentCols+` FROM unclaimed_agents WHERE id = ?`, id))
}

func (s *Store) DeleteUnclaimedAgent(id string) error {
	_, err := s.db.Exec(`DELETE FROM unclaimed_agents WHERE id = ?`, id)
	return err
}

// SetTenantEnrollToken generates (or regenerates) a tenant's enrollment
// token. Regenerating invalidates the old one for any device that hasn't
// been claimed yet; already-claimed agents are unaffected.
func (s *Store) SetTenantEnrollToken(tenantID string) (string, error) {
	token := NewEnrollToken()
	if _, err := s.db.Exec(`UPDATE tenants SET enroll_token = ? WHERE id = ?`, token, tenantID); err != nil {
		return "", err
	}
	return token, nil
}

// GetTenantEnrollToken returns the tenant's current enrollment token, or ""
// if one has never been generated (or was revoked).
func (s *Store) GetTenantEnrollToken(tenantID string) (string, error) {
	var token string
	err := s.db.QueryRow(`SELECT enroll_token FROM tenants WHERE id = ?`, tenantID).Scan(&token)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return token, err
}

// RevokeTenantEnrollToken clears a tenant's enrollment token, disabling
// further self-enrollment until a new one is generated. Devices already
// recorded in unclaimed_agents are left as-is — dismiss them explicitly.
func (s *Store) RevokeTenantEnrollToken(tenantID string) error {
	_, err := s.db.Exec(`UPDATE tenants SET enroll_token = '' WHERE id = ?`, tenantID)
	return err
}

// GetTenantByEnrollToken resolves a tenant from a presented enrollment
// token. An empty stored token never matches (never-generated or revoked).
func (s *Store) GetTenantByEnrollToken(token string) (*Tenant, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	t := &Tenant{}
	err := s.db.QueryRow(`SELECT id, name FROM tenants WHERE enroll_token = ? AND enroll_token != ''`, token).Scan(&t.ID, &t.Name)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}
