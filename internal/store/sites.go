package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type Site struct {
	ID       string   `json:"id"`
	TenantID string   `json:"tenantId"`
	Name     string   `json:"name"`
	TestIDs  []string `json:"testIds"`
	Agents   int      `json:"agents"`
}

type TestDef struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenantId"`
	Name            string          `json:"name"`
	Type            string          `json:"type"`
	IntervalSeconds uint32          `json:"intervalSeconds"`
	Params          json.RawMessage `json:"params"`
	Thresholds      json.RawMessage `json:"thresholds,omitempty"`
}

// --- Sites ---

func (s *Store) CreateSite(tenantID, name string) (*Site, error) {
	site := &Site{ID: newID(), TenantID: tenantID, Name: name, TestIDs: []string{}}
	_, err := s.db.Exec(`INSERT INTO sites (id, tenant_id, name) VALUES (?, ?, ?)`,
		site.ID, site.TenantID, site.Name)
	if err != nil {
		return nil, fmt.Errorf("creating site: %w", err)
	}
	return site, nil
}

func (s *Store) GetSite(id string) (*Site, error) {
	site := &Site{}
	err := s.db.QueryRow(`SELECT id, tenant_id, name FROM sites WHERE id = ?`, id).
		Scan(&site.ID, &site.TenantID, &site.Name)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	site.TestIDs, err = s.SiteTestIDs(id)
	return site, err
}

func (s *Store) ListSites(tenantID string) ([]*Site, error) {
	rows, err := s.db.Query(`
		SELECT s.id, s.tenant_id, s.name, COUNT(a.id)
		FROM sites s LEFT JOIN agents a ON a.site_id = s.id
		WHERE s.tenant_id = ?
		GROUP BY s.id ORDER BY s.name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sites := []*Site{}
	for rows.Next() {
		site := &Site{}
		if err := rows.Scan(&site.ID, &site.TenantID, &site.Name, &site.Agents); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, site := range sites {
		if site.TestIDs, err = s.SiteTestIDs(site.ID); err != nil {
			return nil, err
		}
	}
	return sites, nil
}

func (s *Store) DeleteSite(id string) error {
	_, err := s.db.Exec(`DELETE FROM sites WHERE id = ?`, id)
	return err
}

// SiteTestIDs returns the IDs of the tests assigned to a site.
func (s *Store) SiteTestIDs(siteID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT test_id FROM site_tests WHERE site_id = ?`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetSiteTests replaces the test assignment of a site.
func (s *Store) SetSiteTests(siteID string, testIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM site_tests WHERE site_id = ?`, siteID); err != nil {
		return err
	}
	for _, testID := range testIDs {
		if _, err := tx.Exec(`INSERT INTO site_tests (site_id, test_id) VALUES (?, ?)`, siteID, testID); err != nil {
			return fmt.Errorf("assigning test %s: %w", testID, err)
		}
	}
	return tx.Commit()
}

// --- Test definitions ---

func (s *Store) CreateTest(t *TestDef) (*TestDef, error) {
	t.ID = newID()
	_, err := s.db.Exec(
		`INSERT INTO tests (id, tenant_id, name, type, interval_seconds, params, thresholds) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.TenantID, t.Name, t.Type, t.IntervalSeconds, string(t.Params), string(t.Thresholds))
	if err != nil {
		return nil, fmt.Errorf("creating test: %w", err)
	}
	return t, nil
}

func (s *Store) UpdateTest(t *TestDef) error {
	res, err := s.db.Exec(
		`UPDATE tests SET name = ?, interval_seconds = ?, params = ?, thresholds = ? WHERE id = ?`,
		t.Name, t.IntervalSeconds, string(t.Params), string(t.Thresholds), t.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) scanTest(row interface{ Scan(...any) error }) (*TestDef, error) {
	t := &TestDef{}
	var params string
	var thresholds sql.NullString // NULL on rows predating the column
	err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Type, &t.IntervalSeconds, &params, &thresholds)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Params = json.RawMessage(params)
	if thresholds.String != "" {
		t.Thresholds = json.RawMessage(thresholds.String)
	}
	return t, nil
}

const testCols = `id, tenant_id, name, type, interval_seconds, params, thresholds`

func (s *Store) GetTest(id string) (*TestDef, error) {
	return s.scanTest(s.db.QueryRow(`SELECT `+testCols+` FROM tests WHERE id = ?`, id))
}

func (s *Store) ListTests(tenantID string) ([]*TestDef, error) {
	rows, err := s.db.Query(`SELECT `+testCols+` FROM tests WHERE tenant_id = ? ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tests := []*TestDef{}
	for rows.Next() {
		t, err := s.scanTest(rows)
		if err != nil {
			return nil, err
		}
		tests = append(tests, t)
	}
	return tests, rows.Err()
}

func (s *Store) DeleteTest(id string) error {
	_, err := s.db.Exec(`DELETE FROM tests WHERE id = ?`, id)
	return err
}

// TestsForSite returns the test definitions assigned to a site.
func (s *Store) TestsForSite(siteID string) ([]*TestDef, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.tenant_id, t.name, t.type, t.interval_seconds, t.params, t.thresholds
		FROM tests t JOIN site_tests st ON st.test_id = t.id
		WHERE st.site_id = ? ORDER BY t.name`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tests := []*TestDef{}
	for rows.Next() {
		t, err := s.scanTest(rows)
		if err != nil {
			return nil, err
		}
		tests = append(tests, t)
	}
	return tests, rows.Err()
}

// AgentIDsForTest returns the IDs of all agents whose site has this test
// assigned (used to push config updates).
func (s *Store) AgentIDsForTest(testID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT a.id FROM agents a
		JOIN site_tests st ON st.site_id = a.site_id
		WHERE st.test_id = ?`, testID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AgentIDsForSite returns the IDs of all agents of a site.
func (s *Store) AgentIDsForSite(siteID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM agents WHERE site_id = ?`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
