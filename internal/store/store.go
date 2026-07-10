package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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
	return s.addColumnIfMissing("alerts", "subject", "TEXT NOT NULL DEFAULT ''")
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
