package store

import (
	"encoding/json"
	"testing"
)

func TestExtractSeriesHappyPath(t *testing.T) {
	s := openTestStore(t)

	// Create tenant, site, agent, test
	tenant, err := s.CreateTenant("test-tenant")
	if err != nil {
		t.Fatalf("creating tenant: %v", err)
	}
	site, err := s.CreateSite(tenant.ID, "test-site")
	if err != nil {
		t.Fatalf("creating site: %v", err)
	}
	agent, err := s.CreateAgent(tenant.ID, site.ID, "test-agent")
	if err != nil {
		t.Fatalf("creating agent: %v", err)
	}
	testDef, err := s.CreateTest(&TestDef{
		TenantID:        tenant.ID,
		Name:            "test-ping",
		Type:            "ping",
		IntervalSeconds: 60,
		Params: json.RawMessage(`{"targets": ["8.8.8.8"], "count": 5}`),
	})
	if err != nil {
		t.Fatalf("creating test: %v", err)
	}

	// Create results with series data (matching actual payload structure)
	for i := 1; i <= 5; i++ {
		payload := `{"ping": {"avgRttMs": 12.5, "minRttMs": 11.0, "maxRttMs": 14.0, "packetsSent": 5, "packetsReceived": 5, "lossPercent": 0}}`
		_, err := s.db.Exec(`
			INSERT INTO results (agent_id, test_id, test_name, test_type, time, payload)
			VALUES (?, ?, ?, ?, datetime('now', '+' || ? || ' seconds'), ?)
		`, agent.ID, testDef.ID, testDef.Name, testDef.Type, i, payload)
		if err != nil {
			t.Fatalf("inserting result %d: %v", i, err)
		}
	}

	// Extract series
	unit, series, current := s.extractSeries("ping", testDef.ID, tenant.ID, "")
	if unit != "ms" {
		t.Fatalf("expected unit 'ms', got %q", unit)
	}
	if len(series) == 0 {
		t.Fatal("expected non-empty series")
	}
	if current == nil {
		t.Fatal("expected non-nil current value")
	}
	if *current != 12.5 {
		t.Fatalf("expected current value 12.5, got %v", *current)
	}
	t.Logf("Series extraction happy path: unit=%s, len(series)=%d, current=%v", unit, len(series), *current)
}

func TestExtractSeriesEmpty(t *testing.T) {
	s := openTestStore(t)

	// Create tenant and test
	tenant, err := s.CreateTenant("test-tenant")
	if err != nil {
		t.Fatalf("creating tenant: %v", err)
	}
	testDef, err := s.CreateTest(&TestDef{
		TenantID:        tenant.ID,
		Name:            "test-empty",
		Type:            "ping",
		IntervalSeconds: 60,
		Params: json.RawMessage(`{"targets": ["8.8.8.8"], "count": 5}`),
	})
	if err != nil {
		t.Fatalf("creating test: %v", err)
	}

	// Extract series (should be empty)
	unit, series, current := s.extractSeries("ping", testDef.ID, tenant.ID, "")
	if unit != "ms" {
		t.Fatalf("expected unit 'ms', got %q", unit)
	}
	if len(series) != 0 {
		t.Fatalf("expected empty series, got %d values", len(series))
	}
	if current != nil {
		t.Fatalf("expected nil current value, got %v", *current)
	}
	t.Logf("Series extraction empty: unit=%s, len(series)=%d, current=%v", unit, len(series), current)
}

// TestOverviewWithSiteFilter verifies that TenantOverview respects the siteId parameter.
func TestOverviewWithSiteFilter(t *testing.T) {
	s := openTestStore(t)

	// Create tenant, two sites, agents, and a test
	tenant, err := s.CreateTenant("test-tenant")
	if err != nil {
		t.Fatalf("creating tenant: %v", err)
	}
	site1, err := s.CreateSite(tenant.ID, "site1")
	if err != nil {
		t.Fatalf("creating site1: %v", err)
	}
	site2, err := s.CreateSite(tenant.ID, "site2")
	if err != nil {
		t.Fatalf("creating site2: %v", err)
	}
	agent1, err := s.CreateAgent(tenant.ID, site1.ID, "agent1")
	if err != nil {
		t.Fatalf("creating agent1: %v", err)
	}
	agent2, err := s.CreateAgent(tenant.ID, site2.ID, "agent2")
	if err != nil {
		t.Fatalf("creating agent2: %v", err)
	}
	testDef, err := s.CreateTest(&TestDef{
		TenantID:        tenant.ID,
		Name:            "test-ping",
		Type:            "ping",
		IntervalSeconds: 60,
		Params: json.RawMessage(`{"targets": ["8.8.8.8"], "count": 5}`),
	})
	if err != nil {
		t.Fatalf("creating test: %v", err)
	}

	// Create results for both agents
	for _, agent := range []*Agent{agent1, agent2} {
		for i := 1; i <= 2; i++ {
			payload := `{"ping": {"avgRttMs": 12.5, "minRttMs": 11.0, "maxRttMs": 14.0, "packetsSent": 5, "packetsReceived": 5, "lossPercent": 0}}`
			_, err := s.db.Exec(`
				INSERT INTO results (agent_id, test_id, test_name, test_type, time, payload)
				VALUES (?, ?, ?, ?, datetime('now', '+' || ? || ' seconds'), ?)
			`, agent.ID, testDef.ID, testDef.Name, testDef.Type, i, payload)
			if err != nil {
				t.Fatalf("inserting result: %v", err)
			}
		}
	}

	// Get overview without filter (should include both agents)
	ov, err := s.TenantOverview(tenant.ID, "")
	if err != nil {
		t.Fatalf("getting overview: %v", err)
	}
	if ov.Agents != 2 {
		t.Fatalf("expected 2 agents, got %d", ov.Agents)
	}
	t.Logf("Overview without filter: agents=%d", ov.Agents)

	// Get overview with site filter (should include only one agent)
	ov, err = s.TenantOverview(tenant.ID, site1.ID)
	if err != nil {
		t.Fatalf("getting overview with site filter: %v", err)
	}
	if ov.Agents != 1 {
		t.Fatalf("expected 1 agent, got %d", ov.Agents)
	}
	t.Logf("Overview with site1 filter: agents=%d", ov.Agents)
}

// TestListTestsNullThresholds reproduces a migrated database where the
// thresholds column was added by ALTER TABLE and existing rows hold NULL.
func TestListTestsNullThresholds(t *testing.T) {
	s := openTestStore(t)
	tn, err := s.CreateTenant("t1")
	if err != nil {
		t.Fatal(err)
	}
	td, err := s.CreateTest(&TestDef{TenantID: tn.ID, Name: "ping", Type: "ping",
		IntervalSeconds: 60, Params: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE tests SET thresholds = NULL WHERE id = ?`, td.ID); err != nil {
		t.Fatal(err)
	}
	tests, err := s.ListTests(tn.ID)
	if err != nil {
		t.Fatalf("ListTests with NULL thresholds: %v", err)
	}
	if len(tests) != 1 || tests[0].Thresholds != nil {
		t.Fatalf("unexpected result: %+v", tests)
	}
}
