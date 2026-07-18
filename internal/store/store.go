package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB

	// logHistory caps how many log rows are kept per scope (the server,
	// and each agent). Overridable via SetLogHistory.
	logHistory int
}

func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	// modernc.org/sqlite serializes writes; a single connection avoids
	// SQLITE_BUSY errors under concurrent API and result writes.
	db.SetMaxOpenConns(1)

	s := &Store{db: db, logHistory: defaultLogHistory}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
	CREATE TABLE IF NOT EXISTS tenants (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL UNIQUE,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS users (
		id            TEXT PRIMARY KEY,
		tenant_id     TEXT REFERENCES tenants(id) ON DELETE CASCADE,
		username      TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		is_admin      INTEGER NOT NULL DEFAULT 0,
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS sessions (
		token      TEXT PRIMARY KEY,
		user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at TIMESTAMP NOT NULL
	);
	CREATE TABLE IF NOT EXISTS sites (
		id         TEXT PRIMARY KEY,
		tenant_id  TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		name       TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (tenant_id, name)
	);
	CREATE TABLE IF NOT EXISTS tests (
		id               TEXT PRIMARY KEY,
		tenant_id        TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		name             TEXT NOT NULL,
		type             TEXT NOT NULL,
		interval_seconds INTEGER NOT NULL,
		params           TEXT NOT NULL,
		UNIQUE (tenant_id, name)
	);
	CREATE TABLE IF NOT EXISTS site_tests (
		site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		test_id TEXT NOT NULL REFERENCES tests(id) ON DELETE CASCADE,
		PRIMARY KEY (site_id, test_id)
	);
	CREATE TABLE IF NOT EXISTS agents (
		id         TEXT PRIMARY KEY,
		tenant_id  TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		site_id    TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
		name       TEXT NOT NULL,
		token      TEXT NOT NULL UNIQUE,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (tenant_id, name)
	);
	CREATE TABLE IF NOT EXISTS results (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id  TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
		test_id   TEXT NOT NULL DEFAULT '',
		test_name TEXT NOT NULL DEFAULT '',
		test_type TEXT NOT NULL,
		time      TIMESTAMP NOT NULL,
		error     TEXT NOT NULL DEFAULT '',
		payload   TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_results_agent_time ON results (agent_id, time DESC);
	CREATE INDEX IF NOT EXISTS idx_results_test ON results (test_id, time DESC);
	CREATE TABLE IF NOT EXISTS alert_rules (
		id          TEXT PRIMARY KEY,
		tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		test_id     TEXT NOT NULL REFERENCES tests(id) ON DELETE CASCADE,
		name        TEXT NOT NULL,
		metric      TEXT NOT NULL,   -- unhealthy | latency_ms | loss_percent | download_mbps | upload_mbps
		operator    TEXT NOT NULL DEFAULT '>',
		threshold   REAL NOT NULL DEFAULT 0,
		for_count   INTEGER NOT NULL DEFAULT 1,
		webhook_url TEXT NOT NULL DEFAULT '',
		created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS alerts (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		rule_id     TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
		agent_id    TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
		state       TEXT NOT NULL,   -- firing | resolved
		value       REAL NOT NULL,
		message     TEXT NOT NULL,
		started_at  TIMESTAMP NOT NULL,
		resolved_at TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_alerts_active ON alerts (rule_id, agent_id, state);
	CREATE TABLE IF NOT EXISTS logs (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		time      TIMESTAMP NOT NULL,
		tenant_id TEXT NOT NULL DEFAULT '',
		agent_id  TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL,   -- server | agent
		level     TEXT NOT NULL,
		message   TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_logs_scope_time ON logs (source, agent_id, time DESC);
	CREATE INDEX IF NOT EXISTS idx_logs_tenant_time ON logs (tenant_id, time DESC);
	CREATE TABLE IF NOT EXISTS api_keys (
		id           TEXT PRIMARY KEY,
		user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name         TEXT NOT NULL,
		key_hash     TEXT NOT NULL UNIQUE,
		prefix       TEXT NOT NULL,
		created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_used_at TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys (user_id);
	`)
	if err != nil {
		return err
	}

	// Additive migrations for existing databases.
	if err := s.addColumnIfMissing("results", "ok", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("agents", "wlan_interface", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("agents", "wireless_interfaces", "TEXT"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("agents", "capabilities", "TEXT"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("agents", "stats", "TEXT"); err != nil {
		return err
	}
	// Set once a monitor sensor has completed its first full-spectrum WLAN
	// discovery sweep, so it is never re-triggered on later reconnects.
	if err := s.addColumnIfMissing("agents", "version", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("agents", "wlan_discovered_at", "TIMESTAMP"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("logs", "scope", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Alert targets table
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS alert_targets (
			id         TEXT PRIMARY KEY,
			tenant_id  TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			type       TEXT NOT NULL,
			config     TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (tenant_id, name)
		)
	`); err != nil {
		return err
	}

	// New alert_rules columns for hysteresis and targets
	if err := s.addColumnIfMissing("alert_rules", "clear_threshold", "REAL"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("alert_rules", "clear_count", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("alert_rules", "target_ids", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}

	// Add thresholds column to tests table
	if err := s.addColumnIfMissing("tests", "thresholds", "TEXT"); err != nil {
		return err
	}

	// Add subject column to alerts table (for multi-target tests)
	if err := s.addColumnIfMissing("alerts", "subject", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Migration: move webhook_url entries to alert_targets
	if err := s.migrateWebhookTargets(); err != nil {
		return err
	}

	// Migration: delete stale wlan_scan and wlan_sense test definitions
	// (replaced by wlan_passive with adaptive channel narrowing)
	_, err = s.db.Exec(`DELETE FROM site_tests WHERE test_id IN (
		SELECT id FROM tests WHERE type IN ('wlan_scan', 'wlan_sense')
	)`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM tests WHERE type IN ('wlan_scan', 'wlan_sense')`)
	return err
}

// addColumnIfMissing adds a column to a table if it is not already present.
func (s *Store) addColumnIfMissing(table, column, definition string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if _, err := s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition); err != nil {
		return err
	}
	// Backfill: rows that recorded an error are not healthy.
	_, err = s.db.Exec("UPDATE results SET ok = 0 WHERE error != ''")
	return err
}

// migrateWebhookTargets converts existing webhook_url entries into alert_targets.
func (s *Store) migrateWebhookTargets() error {
	// Find all rules with non-empty webhook_url
	rows, err := s.db.Query(`SELECT id, tenant_id, name, webhook_url FROM alert_rules WHERE webhook_url != ''`)
	if err != nil {
		return err
	}
	defer rows.Close()

	rules := []struct {
		id, tenantID, name, webhookURL string
	}{}
	for rows.Next() {
		var r struct {
			id, tenantID, name, webhookURL string
		}
		if err := rows.Scan(&r.id, &r.tenantID, &r.name, &r.webhookURL); err != nil {
			return err
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// For each rule, create a webhook target and update the rule
	for _, r := range rules {
		// Check if target already exists (idempotent)
		var exists int
		err := s.db.QueryRow(
			`SELECT COUNT(*) FROM alert_targets WHERE tenant_id = ? AND type = 'webhook' AND name = ?`,
			r.tenantID, r.name+" webhook").Scan(&exists)
		if err != nil {
			return err
		}
		if exists > 0 {
			continue
		}

		// Create webhook target
		targetID := newID()
		config := map[string]string{"url": r.webhookURL}
		configJSON, _ := json.Marshal(config)

		if _, err := s.db.Exec(
			`INSERT INTO alert_targets (id, tenant_id, name, type, config) VALUES (?, ?, ?, 'webhook', ?)`,
			targetID, r.tenantID, r.name+" webhook", string(configJSON)); err != nil {
			return err
		}

		// Update rule to include the target (as JSON array)
		targetIDs := []string{targetID}
		targetIDsJSON, _ := json.Marshal(targetIDs)
		if _, err := s.db.Exec(
			`UPDATE alert_rules SET target_ids = ? WHERE id = ?`,
			string(targetIDsJSON), r.id); err != nil {
			return err
		}
	}

	return nil
}

// newID returns a random 16-byte hex ID.
func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// NewToken returns a random 32-byte hex token.
func NewToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
