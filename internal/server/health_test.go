package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fox27374/net-lama/internal/store"
)

func TestHealthEvaluation(t *testing.T) {
	// Create a temporary database for testing
	tmpfile := t.TempDir()
	st, err := store.Open(tmpfile + "/test.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	// Create a mock server with minimal setup
	srv := &Server{
		connected:      make(map[string]*connectedAgent),
		reconnectCount: make(map[string]int),
		reconnectTimes: make(map[string][]time.Time),
		Store:          st,
	}

	agent1 := &store.Agent{
		ID:    "agent1",
		Name:  "test-agent",
		Stats: nil,
	}
	agent2 := &store.Agent{
		ID:    "agent2",
		Name:  "test-agent2",
		Stats: marshalStats(AgentStats{
			Time:            time.Now(),
			AgentCpuPercent: 5.0,
			PidCount:        10,
			UptimeSeconds:   100,
		}),
	}
	agent3 := &store.Agent{
		ID:    "agent3",
		Name:  "test-agent3",
		Stats: marshalStats(AgentStats{
			Time:            time.Now(),
			AgentCpuPercent: 25.0,
			PidCount:        10,
			UptimeSeconds:   100,
		}),
	}
	agent4 := &store.Agent{
		ID:    "agent4",
		Name:  "test-agent4",
		Stats: marshalStats(AgentStats{
			Time:            time.Now(),
			AgentCpuPercent: 5.0,
			PidCount:        600,
			UptimeSeconds:   100,
		}),
	}
	agent5 := &store.Agent{
		ID:    "agent5",
		Name:  "test-agent5",
		Stats: marshalStats(AgentStats{
			Time:            time.Now(),
			AgentCpuPercent: 5.0,
			PidCount:        2000,
			UptimeSeconds:   100,
		}),
	}
	agent6 := &store.Agent{
		ID:    "agent6",
		Name:  "test-agent6",
		Stats: marshalStats(AgentStats{
			Time:            time.Now(),
			AgentCpuPercent: 5.0,
			PidCount:        10,
			UptimeSeconds:   100,
		}),
	}
	agent7 := &store.Agent{
		ID:    "agent7",
		Name:  "test-agent7",
		Stats: marshalStats(AgentStats{
			Time:            time.Now(),
			AgentCpuPercent: 5.0,
			PidCount:        10,
			UptimeSeconds:   100,
		}),
	}

	tests := []struct {
		name          string
		agent         *store.Agent
		setup         func()
		expectStatus  HealthStatus
		expectReasons int
	}{
		{
			name:          "unknown with no stats",
			agent:         agent1,
			setup:         func() {},
			expectStatus:  HealthUnknown,
			expectReasons: 0,
		},
		{
			name:  "healthy agent",
			agent: agent2,
			setup: func() {
				srv.mu.Lock()
				srv.connected["agent2"] = &connectedAgent{agent: agent2}
				srv.mu.Unlock()
			},
			expectStatus:  HealthHealthy,
			expectReasons: 0,
		},
		{
			name:  "degraded: high CPU",
			agent: agent3,
			setup: func() {
				srv.mu.Lock()
				srv.connected["agent3"] = &connectedAgent{agent: agent3}
				srv.mu.Unlock()
			},
			expectStatus:  HealthDegraded,
			expectReasons: 1,
		},
		{
			name:  "degraded: many processes",
			agent: agent4,
			setup: func() {
				srv.mu.Lock()
				srv.connected["agent4"] = &connectedAgent{agent: agent4}
				srv.mu.Unlock()
			},
			expectStatus:  HealthDegraded,
			expectReasons: 1,
		},
		{
			name:  "unhealthy: very many processes",
			agent: agent5,
			setup: func() {
				srv.mu.Lock()
				srv.connected["agent5"] = &connectedAgent{agent: agent5}
				srv.mu.Unlock()
			},
			expectStatus:  HealthUnhealthy,
			expectReasons: 1,
		},
		{
			name:  "degraded: flapping (3 reconnects)",
			agent: agent6,
			setup: func() {
				srv.mu.Lock()
				srv.connected["agent6"] = &connectedAgent{agent: agent6}
				srv.mu.Unlock()
				srv.reconnectMu.Lock()
				srv.reconnectCount["agent6"] = 3
				srv.reconnectMu.Unlock()
			},
			expectStatus:  HealthDegraded,
			expectReasons: 1,
		},
		{
			name:  "unhealthy: flapping (6 reconnects)",
			agent: agent7,
			setup: func() {
				srv.mu.Lock()
				srv.connected["agent7"] = &connectedAgent{agent: agent7}
				srv.mu.Unlock()
				srv.reconnectMu.Lock()
				srv.reconnectCount["agent7"] = 6
				srv.reconnectMu.Unlock()
			},
			expectStatus:  HealthUnhealthy,
			expectReasons: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			eval := srv.EvaluateAgentHealth(tt.agent)

			if eval.Status != tt.expectStatus {
				t.Errorf("expected status %s, got %s", tt.expectStatus, eval.Status)
			}
			if len(eval.Reasons) != tt.expectReasons {
				t.Errorf("expected %d reasons, got %d: %v", tt.expectReasons, len(eval.Reasons), eval.Reasons)
			}
		})
	}
}

func marshalStats(stats AgentStats) json.RawMessage {
	snapshot := struct {
		Time              string  `json:"time"`
		AgentCpuPercent   float64 `json:"agentCpuPercent"`
		AgentMemBytes     uint64  `json:"agentMemBytes"`
		PidCount          uint32  `json:"pidCount"`
		UptimeSeconds     uint64  `json:"uptimeSeconds"`
		CpuPercent        float64 `json:"cpuPercent"`
		MemUsedBytes      uint64  `json:"memUsedBytes"`
		MemTotalBytes     uint64  `json:"memTotalBytes"`
		DiskUsedBytes     uint64  `json:"diskUsedBytes"`
		DiskTotalBytes    uint64  `json:"diskTotalBytes"`
	}{
		Time:            stats.Time.Format(time.RFC3339),
		AgentCpuPercent: stats.AgentCpuPercent,
		AgentMemBytes:   stats.AgentMemBytes,
		PidCount:        stats.PidCount,
		UptimeSeconds:   stats.UptimeSeconds,
	}
	data, _ := json.Marshal(snapshot)
	return data
}
