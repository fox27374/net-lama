package server

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/fox27374/net-lama/internal/store"
)

func TestConfigForAgent_FilteringByCapability(t *testing.T) {
	// Create a real temporary database for testing
	tmpfile, err := os.CreateTemp("", "netlama-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	s, err := store.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	server := &Server{Store: s, Logger: logger}

	// Create test data
	tenant, err := s.CreateTenant("test-tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	site, err := s.CreateSite(tenant.ID, "test-site")
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create tests
	pingTest := &store.TestDef{
		ID:              "test-1",
		TenantID:        tenant.ID,
		Name:            "test-ping",
		Type:            "ping",
		IntervalSeconds: 10,
		Params:          json.RawMessage(`{"targets":["8.8.8.8"]}`),
	}
	_, err = s.CreateTest(pingTest)
	if err != nil {
		t.Fatalf("Failed to create ping test: %v", err)
	}

	tracerouteTest := &store.TestDef{
		ID:              "test-2",
		TenantID:        tenant.ID,
		Name:            "test-traceroute",
		Type:            "traceroute",
		IntervalSeconds: 30,
		Params:          json.RawMessage(`{"target":"8.8.8.8"}`),
	}
	_, err = s.CreateTest(tracerouteTest)
	if err != nil {
		t.Fatalf("Failed to create traceroute test: %v", err)
	}

	// Assign tests to site
	if err := s.SetSiteTests(site.ID, []string{pingTest.ID, tracerouteTest.ID}); err != nil {
		t.Fatalf("Failed to assign tests to site: %v", err)
	}

	// Test case 1: Agent with only ping capability (should not get traceroute)
	agent1 := &store.Agent{
		ID:           "agent-1",
		Name:         "test-agent-1",
		SiteID:       site.ID,
		Capabilities: json.RawMessage(`["ping","dns"]`),
		CreatedAt:    time.Now(),
	}

	cfg, skipped, err := server.ConfigForAgent(agent1)
	if err != nil {
		t.Fatalf("ConfigForAgent failed: %v", err)
	}

	if len(cfg.Tests) != 1 {
		t.Errorf("Expected 1 test for agent without traceroute, got %d", len(cfg.Tests))
	}
	if len(cfg.Tests) > 0 && cfg.Tests[0].Name != "test-ping" {
		t.Errorf("Expected test-ping, got %s", cfg.Tests[0].Name)
	}
	if len(skipped) != 1 || skipped[0] != "test-traceroute" {
		t.Errorf("Expected skipped=[test-traceroute], got %v", skipped)
	}

	// Test case 2: Agent with empty capabilities (backward compat - should get all tests)
	agent2 := &store.Agent{
		ID:           "agent-2",
		Name:         "test-agent-2",
		SiteID:       site.ID,
		Capabilities: json.RawMessage(`[]`),
		CreatedAt:    time.Now(),
	}

	cfg, skipped, err = server.ConfigForAgent(agent2)
	if err != nil {
		t.Fatalf("ConfigForAgent failed: %v", err)
	}

	if len(cfg.Tests) != 2 {
		t.Errorf("Expected 2 tests for agent with empty capabilities (backward compat), got %d", len(cfg.Tests))
	}
	if len(skipped) != 0 {
		t.Errorf("Expected no skipped tests for empty capabilities, got %v", skipped)
	}

	// Test case 3: Agent with all capabilities (should get all tests)
	agent3 := &store.Agent{
		ID:           "agent-3",
		Name:         "test-agent-3",
		SiteID:       site.ID,
		Capabilities: json.RawMessage(`["ping","dns","traceroute","speedtest","http","tcp","wlan_scan"]`),
		CreatedAt:    time.Now(),
	}

	cfg, skipped, err = server.ConfigForAgent(agent3)
	if err != nil {
		t.Fatalf("ConfigForAgent failed: %v", err)
	}

	if len(cfg.Tests) != 2 {
		t.Errorf("Expected 2 tests for agent with all capabilities, got %d", len(cfg.Tests))
	}
	if len(skipped) != 0 {
		t.Errorf("Expected no skipped tests for full capabilities, got %v", skipped)
	}
}

func TestIsLegacyCapabilities(t *testing.T) {
	cases := []struct {
		name string
		caps []string
		want bool
	}{
		{
			name: "exact legacy list in legacy order",
			caps: []string{"speedtest", "ping", "dns", "http", "tcp", "wlan"},
			want: true,
		},
		{
			name: "detection ordering is not legacy",
			caps: []string{"ping", "dns", "http", "tcp", "speedtest", "wlan"},
			want: false,
		},
		{
			name: "detected list without wlan",
			caps: []string{"ping", "dns", "http", "tcp", "speedtest"},
			want: false,
		},
		{
			name: "detected list with traceroute",
			caps: []string{"ping", "dns", "http", "tcp", "speedtest", "traceroute", "wlan"},
			want: false,
		},
		{
			name: "empty",
			caps: nil,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLegacyCapabilities(tc.caps); got != tc.want {
				t.Errorf("isLegacyCapabilities(%v) = %v, want %v", tc.caps, got, tc.want)
			}
		})
	}
}

func TestControlStream_LegacyCapabilitiesNotStored(t *testing.T) {
	// Simulates the server-first upgrade path: an agent row already has
	// (or has no) stored capabilities, and an old binary re-registers with
	// the hardcoded legacy list. The legacy list must NOT overwrite the
	// store: SetAgentCapabilities is skipped, so filtering keeps behaving
	// as before the reconnect.
	tmpfile, err := os.CreateTemp("", "netlama-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	s, err := store.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	tenant, err := s.CreateTenant("test-tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}
	site, err := s.CreateSite(tenant.ID, "test-site")
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}
	agent, err := s.CreateAgent(tenant.ID, site.ID, "legacy-agent")
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	// Mirror the ControlStream registration logic for capabilities.
	record := func(caps []string) {
		if len(caps) > 0 && !isLegacyCapabilities(caps) {
			data, _ := json.Marshal(caps)
			if err := s.SetAgentCapabilities(agent.ID, data); err != nil {
				t.Fatalf("SetAgentCapabilities failed: %v", err)
			}
		}
	}

	// 1. Old binary registers with the legacy hardcoded list: nothing stored.
	record([]string{"speedtest", "ping", "dns", "http", "tcp", "wlan"})
	got, err := s.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if string(got.Capabilities) != "[]" {
		t.Errorf("Legacy list must not be stored, got %s", got.Capabilities)
	}

	// 2. New binary registers with detected capabilities: stored.
	record([]string{"ping", "dns", "http", "tcp", "speedtest", "traceroute"})
	got, err = s.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	want := `["ping","dns","http","tcp","speedtest","traceroute"]`
	if string(got.Capabilities) != want {
		t.Errorf("Detected capabilities not stored: got %s, want %s", got.Capabilities, want)
	}

	// 3. Old binary reconnects with the legacy list: stored value is kept.
	record([]string{"speedtest", "ping", "dns", "http", "tcp", "wlan"})
	got, err = s.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if string(got.Capabilities) != want {
		t.Errorf("Legacy re-register must keep stored capabilities: got %s, want %s", got.Capabilities, want)
	}
}

// TestValidateTestDefThresholds covers the direction-dependent warn/crit
// ordering: higher-is-worse types need warn < crit, speedtest (lower-is-worse)
// needs warn > crit.
func TestValidateTestDefThresholds(t *testing.T) {
	cases := []struct {
		name    string
		typ     string
		params  string
		th      string
		wantErr bool
	}{
		{"ping warn<crit ok", "ping", `{"targets":["8.8.8.8"],"count":3}`, `{"warn":30,"crit":80}`, false},
		{"ping warn>=crit bad", "ping", `{"targets":["8.8.8.8"],"count":3}`, `{"warn":80,"crit":30}`, true},
		{"speedtest warn>crit ok", "speedtest", `{"provider":"ookla"}`, `{"warn":80,"crit":40}`, false},
		{"speedtest warn<=crit bad", "speedtest", `{"provider":"ookla"}`, `{"warn":40,"crit":80}`, true},
		{"single warn only ok", "ping", `{"targets":["8.8.8.8"],"count":3}`, `{"warn":30}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			interval := uint32(30)
			if c.typ == "speedtest" {
				interval = 300
			}
			td := &store.TestDef{
				Name: "x", Type: c.typ, IntervalSeconds: interval,
				Params: []byte(c.params), Thresholds: []byte(c.th),
			}
			err := ValidateTestDef(td)
			if c.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestConfigForAgent_WlanPassiveCapability(t *testing.T) {
	// Create a real temporary database for testing
	tmpfile, err := os.CreateTemp("", "netlama-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	s, err := store.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	server := &Server{Store: s, Logger: logger}

	// Create test data
	tenant, err := s.CreateTenant("test-tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	site, err := s.CreateSite(tenant.ID, "test-site")
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create a wlan_passive test
	wlanTest := &store.TestDef{
		ID:              "test-wlan",
		TenantID:        tenant.ID,
		Name:            "test-wlan-passive",
		Type:            "wlan_passive",
		IntervalSeconds: 60,
		Params:          json.RawMessage(`{}`),
	}
	_, err = s.CreateTest(wlanTest)
	if err != nil {
		t.Fatalf("Failed to create wlan test: %v", err)
	}

	// Assign test to site
	if err := s.SetSiteTests(site.ID, []string{wlanTest.ID}); err != nil {
		t.Fatalf("Failed to assign tests to site: %v", err)
	}

	// Test case 1: Agent WITH wlan capability should receive wlan_passive test
	agentWithWlan := &store.Agent{
		ID:           "agent-with-wlan",
		Name:         "test-agent-with-wlan",
		SiteID:       site.ID,
		Capabilities: json.RawMessage(`["ping","dns","http","tcp","speedtest","wlan"]`),
		CreatedAt:    time.Now(),
	}

	cfg, skipped, err := server.ConfigForAgent(agentWithWlan)
	if err != nil {
		t.Fatalf("ConfigForAgent failed: %v", err)
	}

	if len(cfg.Tests) != 1 {
		t.Errorf("Expected wlan-capable agent to receive 1 test, got %d", len(cfg.Tests))
	}
	if len(cfg.Tests) > 0 && cfg.Tests[0].Name != "test-wlan-passive" {
		t.Errorf("Expected test-wlan-passive, got %s", cfg.Tests[0].Name)
	}
	if len(skipped) != 0 {
		t.Errorf("Expected no skipped tests for wlan-capable agent, got %v", skipped)
	}

	// Test case 2: Agent WITHOUT wlan capability should NOT receive wlan_passive test
	agentWithoutWlan := &store.Agent{
		ID:           "agent-without-wlan",
		Name:         "test-agent-without-wlan",
		SiteID:       site.ID,
		Capabilities: json.RawMessage(`["ping","dns","http","tcp"]`),
		CreatedAt:    time.Now(),
	}

	cfg, skipped, err = server.ConfigForAgent(agentWithoutWlan)
	if err != nil {
		t.Fatalf("ConfigForAgent failed: %v", err)
	}

	if len(cfg.Tests) != 0 {
		t.Errorf("Expected non-wlan agent to receive 0 tests, got %d", len(cfg.Tests))
	}
	if len(skipped) != 1 || skipped[0] != "test-wlan-passive" {
		t.Errorf("Expected skipped=[test-wlan-passive], got %v", skipped)
	}
}

// TestConfigForAgent_PerfmonPinnedToSourceAgent verifies a perfmon test is
// pushed only to its pinned source agent, even though (unlike every other
// test type) it's assigned to a whole site that may have other agents.
func TestConfigForAgent_PerfmonPinnedToSourceAgent(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "netlama-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	s, err := store.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := &Server{Store: s, Logger: logger}

	tenant, err := s.CreateTenant("perfmon-tenant")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	site, err := s.CreateSite(tenant.ID, "perfmon-site")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	perfmonTest := &store.TestDef{
		ID:              "pm-test-1",
		TenantID:        tenant.ID,
		Name:            "pm-a-to-b",
		Type:            "perfmon",
		IntervalSeconds: 60,
		Params:          json.RawMessage(`{"sourceAgentId":"agent-source","target":"10.0.0.5:5252","durationSeconds":5}`),
	}
	if _, err := s.CreateTest(perfmonTest); err != nil {
		t.Fatalf("CreateTest: %v", err)
	}
	if err := s.SetSiteTests(site.ID, []string{perfmonTest.ID}); err != nil {
		t.Fatalf("SetSiteTests: %v", err)
	}

	source := &store.Agent{
		ID: "agent-source", Name: "source", SiteID: site.ID,
		Capabilities: json.RawMessage(`["ping","perfmon"]`), CreatedAt: time.Now(),
	}
	other := &store.Agent{
		ID: "agent-other", Name: "other", SiteID: site.ID,
		Capabilities: json.RawMessage(`["ping","perfmon"]`), CreatedAt: time.Now(),
	}

	cfg, _, err := srv.ConfigForAgent(source)
	if err != nil {
		t.Fatalf("ConfigForAgent(source): %v", err)
	}
	if len(cfg.Tests) != 1 {
		t.Fatalf("expected the pinned source agent to get 1 test, got %d", len(cfg.Tests))
	}

	cfg, _, err = srv.ConfigForAgent(other)
	if err != nil {
		t.Fatalf("ConfigForAgent(other): %v", err)
	}
	if len(cfg.Tests) != 0 {
		t.Fatalf("expected the non-source agent of the same site to get 0 tests, got %d", len(cfg.Tests))
	}
}
