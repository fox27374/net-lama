package store

import "time"

// LogEntry is a persisted log line from the server or an agent.
type LogEntry struct {
	ID        int64     `json:"id"`
	Time      time.Time `json:"time"`
	TenantID  string    `json:"tenantId,omitempty"`
	AgentID   string    `json:"agentId,omitempty"`
	AgentName string    `json:"agentName,omitempty"`
	Source    string    `json:"source"` // "server" | "agent"
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

// defaultLogHistory is the number of log rows kept per scope (the server
// is one scope, each agent is its own scope) when NETLAMA_LOG_HISTORY is
// not set.
const defaultLogHistory = 1000

// SetLogHistory overrides the number of log rows kept per scope. n <= 0
// is ignored and the current value (the default, unless already set) is
// kept.
func (s *Store) SetLogHistory(n int) {
	if n > 0 {
		s.logHistory = n
	}
}

// InsertLog stores a log line and prunes older rows in the same scope
// down to the configured history size — server logs are one scope, each
// agent's logs are their own scope, mirroring the per-agent pruning in
// AddResult.
func (s *Store) InsertLog(e *LogEntry) error {
	_, err := s.db.Exec(
		`INSERT INTO logs (time, tenant_id, agent_id, source, level, message) VALUES (?, ?, ?, ?, ?, ?)`,
		e.Time.UTC(), e.TenantID, e.AgentID, e.Source, e.Level, e.Message,
	)
	if err != nil {
		return err
	}

	if e.Source == "agent" {
		_, err = s.db.Exec(
			`DELETE FROM logs WHERE source = 'agent' AND agent_id = ? AND id NOT IN
			 (SELECT id FROM logs WHERE source = 'agent' AND agent_id = ? ORDER BY id DESC LIMIT ?)`,
			e.AgentID, e.AgentID, s.logHistory,
		)
	} else {
		_, err = s.db.Exec(
			`DELETE FROM logs WHERE source = 'server' AND id NOT IN
			 (SELECT id FROM logs WHERE source = 'server' ORDER BY id DESC LIMIT ?)`,
			s.logHistory,
		)
	}
	return err
}

// LogFilter narrows down log queries; empty fields are ignored. An empty
// TenantID means "all tenants" and is only meaningful for admin queries —
// tenant-scoped callers must set it.
type LogFilter struct {
	TenantID string
	AgentID  string
	Source   string // "server" | "agent" | "" = both
	Level    string
	Limit    int
}

const maxLogLimit = 500

// ListLogs returns the most recent log lines matching the filter, newest
// first, including the agent name for display (empty for server logs).
func (s *Store) ListLogs(f LogFilter) ([]*LogEntry, error) {
	if f.Limit <= 0 || f.Limit > maxLogLimit {
		f.Limit = maxLogLimit
	}

	query := `
		SELECT l.id, l.time, l.tenant_id, l.agent_id, COALESCE(a.name, ''), l.source, l.level, l.message
		FROM logs l
		LEFT JOIN agents a ON a.id = l.agent_id
		WHERE 1 = 1`
	args := []any{}

	if f.TenantID != "" {
		query += ` AND l.tenant_id = ?`
		args = append(args, f.TenantID)
	}
	if f.AgentID != "" {
		query += ` AND l.agent_id = ?`
		args = append(args, f.AgentID)
	}
	if f.Source != "" {
		query += ` AND l.source = ?`
		args = append(args, f.Source)
	}
	if f.Level != "" {
		query += ` AND l.level = ?`
		args = append(args, f.Level)
	}
	query += ` ORDER BY l.id DESC LIMIT ?`
	args = append(args, f.Limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := []*LogEntry{}
	for rows.Next() {
		e := &LogEntry{}
		if err := rows.Scan(&e.ID, &e.Time, &e.TenantID, &e.AgentID, &e.AgentName, &e.Source, &e.Level, &e.Message); err != nil {
			return nil, err
		}
		logs = append(logs, e)
	}
	return logs, rows.Err()
}
