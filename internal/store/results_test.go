package store

import (
	"testing"
	"time"
)

// TestResultPruneIsPerTest is the regression guard for the starvation bug the
// old per-agent cap had: a 1-minute test filled the whole quota and evicted an
// hourly test's history. A chatty test overflowing its own cap must not touch
// another test's rows.
func TestResultPruneIsPerTest(t *testing.T) {
	old := resultsPerTest
	resultsPerTest = 10
	t.Cleanup(func() { resultsPerTest = old })

	s := openTestStore(t)

	tenant, err := s.CreateTenant("t")
	if err != nil {
		t.Fatalf("creating tenant: %v", err)
	}
	site, err := s.CreateSite(tenant.ID, "s")
	if err != nil {
		t.Fatalf("creating site: %v", err)
	}
	agent, err := s.CreateAgent(tenant.ID, site.ID, "a")
	if err != nil {
		t.Fatalf("creating agent: %v", err)
	}

	add := func(testID string, n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			r := &Result{
				AgentID:  agent.ID,
				TestID:   testID,
				TestName: testID,
				TestType: "ping",
				Time:     time.Now(),
				OK:       true,
				Payload:  []byte(`{}`),
			}
			if err := s.AddResult(r); err != nil {
				t.Fatalf("adding result: %v", err)
			}
		}
	}

	count := func(testID string) int {
		t.Helper()
		rows, err := s.ListResults(ResultFilter{
			TenantID: tenant.ID, AgentID: agent.ID, TestID: testID, Limit: 1000,
		})
		if err != nil {
			t.Fatalf("listing results: %v", err)
		}
		return len(rows)
	}

	// The slow test runs a few times, then the chatty one runs far past its cap.
	add("slow", 3)
	add("chatty", resultsPerTest*3)

	if got := count("slow"); got != 3 {
		t.Errorf("slow test kept %d results, want 3 — a chatty test evicted them", got)
	}
	if got := count("chatty"); got != resultsPerTest {
		t.Errorf("chatty test kept %d results, want %d", got, resultsPerTest)
	}
}
