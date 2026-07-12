package server

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fox27374/net-lama/internal/store"
)

// HealthStatus is the health status of an agent.
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
	HealthUnknown   HealthStatus = "unknown"
)

// HealthEvaluation contains the evaluated health status and reasons.
type HealthEvaluation struct {
	Status        HealthStatus
	Reasons       []string
	UptimeSeconds uint64
}

// EvaluateAgentHealth computes the health status from agent self-metrics,
// connection stability, and agent-scoped error logs.
func (s *Server) EvaluateAgentHealth(agent *store.Agent) *HealthEvaluation {
	eval := &HealthEvaluation{
		Status:  HealthUnknown,
		Reasons: []string{},
	}

	// Get latest stats from the store
	stats := s.getLatestAgentStats(agent.ID)
	if stats == nil || stats.UptimeSeconds == 0 {
		// No stats ever received, or an older agent build that reports
		// host stats but no self-metrics (uptime is always > 0 once an
		// agent that knows about self-health has sent one sample) —
		// don't judge what wasn't measured.
		return eval
	}

	eval.UptimeSeconds = stats.UptimeSeconds

	// Collect health indicators
	var statusLevel HealthStatus = HealthHealthy

	// Check CPU usage: > 20% = degraded, no hard unhealthy threshold
	if stats.AgentCpuPercent > 20 {
		eval.Reasons = append(eval.Reasons, fmt.Sprintf("agent CPU share %.0f%%", stats.AgentCpuPercent))
		statusLevel = updateWorstStatus(statusLevel, HealthDegraded)
	}

	// Check process count: > 500 = degraded, > 1500 = unhealthy
	if stats.PidCount > 1500 {
		eval.Reasons = append(eval.Reasons, fmt.Sprintf("%d processes in container", stats.PidCount))
		statusLevel = updateWorstStatus(statusLevel, HealthUnhealthy)
	} else if stats.PidCount > 500 {
		eval.Reasons = append(eval.Reasons, fmt.Sprintf("%d processes in container", stats.PidCount))
		statusLevel = updateWorstStatus(statusLevel, HealthDegraded)
	}

	// Check stats staleness: > 2 min = degraded, > 5 min = unhealthy
	// Only check if agent is connected
	if s.AgentConnected(agent.ID) {
		statAge := time.Since(stats.Time)
		if statAge > 5*time.Minute {
			eval.Reasons = append(eval.Reasons, fmt.Sprintf("no stats for %.0f min", statAge.Minutes()))
			statusLevel = updateWorstStatus(statusLevel, HealthUnhealthy)
		} else if statAge > 2*time.Minute {
			eval.Reasons = append(eval.Reasons, fmt.Sprintf("no stats for %.0f min", statAge.Minutes()))
			statusLevel = updateWorstStatus(statusLevel, HealthDegraded)
		}
	}

	// Check reconnect flapping: >= 3 = degraded, >= 6 = unhealthy in 15m window
	s.reconnectMu.Lock()
	flaps := s.reconnectCount[agent.ID]
	s.reconnectMu.Unlock()
	if flaps >= 6 {
		eval.Reasons = append(eval.Reasons, fmt.Sprintf("%d reconnects in 15m", flaps))
		statusLevel = updateWorstStatus(statusLevel, HealthUnhealthy)
	} else if flaps >= 3 {
		eval.Reasons = append(eval.Reasons, fmt.Sprintf("%d reconnects in 15m", flaps))
		statusLevel = updateWorstStatus(statusLevel, HealthDegraded)
	}

	// Check agent-scoped error logs in the last 15 min
	cutoff := time.Now().Add(-15 * time.Minute)
	errorCount, err := s.Store.CountAgentErrors(agent.ID, cutoff)
	if err == nil {
		if errorCount >= 10 {
			eval.Reasons = append(eval.Reasons, fmt.Sprintf("%d agent errors in 15m", errorCount))
			statusLevel = updateWorstStatus(statusLevel, HealthUnhealthy)
		} else if errorCount >= 2 {
			eval.Reasons = append(eval.Reasons, fmt.Sprintf("%d agent errors in 15m", errorCount))
			statusLevel = updateWorstStatus(statusLevel, HealthDegraded)
		}
	}

	eval.Status = statusLevel
	return eval
}

// updateWorstStatus returns the worse of the two statuses.
func updateWorstStatus(current, new HealthStatus) HealthStatus {
	order := map[HealthStatus]int{
		HealthHealthy:   0,
		HealthDegraded:  1,
		HealthUnhealthy: 2,
	}
	if order[new] > order[current] {
		return new
	}
	return current
}

// AgentStats holds the latest stats for an agent, including a timestamp.
type AgentStats struct {
	Time              time.Time
	AgentCpuPercent   float64
	AgentMemBytes     uint64
	PidCount          uint32
	UptimeSeconds     uint64
	CpuPercent        float64
	MemUsedBytes      uint64
	MemTotalBytes     uint64
	DiskUsedBytes     uint64
	DiskTotalBytes    uint64
}

// getLatestAgentStats retrieves the latest stats for an agent from memory
// cache (populated from the store's JSON snapshot).
func (s *Server) getLatestAgentStats(agentID string) *AgentStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, ok := s.connected[agentID]
	if !ok {
		return nil
	}

	agent := conn.agent
	if agent.Stats == nil || len(agent.Stats) == 0 {
		return nil
	}

	// Parse the stats JSON snapshot stored in the agent record
	var statsMap struct {
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
	}

	if err := json.Unmarshal(agent.Stats, &statsMap); err != nil {
		return nil
	}

	// Parse the time string
	t, err := time.Parse(time.RFC3339, statsMap.Time)
	if err != nil {
		return nil
	}

	return &AgentStats{
		Time:            t,
		AgentCpuPercent: statsMap.AgentCpuPercent,
		AgentMemBytes:   statsMap.AgentMemBytes,
		PidCount:        statsMap.PidCount,
		UptimeSeconds:   statsMap.UptimeSeconds,
		CpuPercent:      statsMap.CpuPercent,
		MemUsedBytes:    statsMap.MemUsedBytes,
		MemTotalBytes:   statsMap.MemTotalBytes,
		DiskUsedBytes:   statsMap.DiskUsedBytes,
		DiskTotalBytes:  statsMap.DiskTotalBytes,
	}
}

// recordReconnect records a reconnection event and maintains a sliding 15-minute
// window of reconnects for flapping detection.
func (s *Server) recordReconnect(agentID string) {
	now := time.Now()
	cutoff := now.Add(-15 * time.Minute)

	s.reconnectMu.Lock()
	defer s.reconnectMu.Unlock()

	// Prune old reconnect timestamps outside the 15-minute window
	times := s.reconnectTimes[agentID]
	filtered := []time.Time{}
	for _, t := range times {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}

	// Add the new reconnect time and update the count
	filtered = append(filtered, now)
	s.reconnectTimes[agentID] = filtered
	s.reconnectCount[agentID] = len(filtered)
}
