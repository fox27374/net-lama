package server

import (
	"fmt"
	"log/slog"

	"github.com/fox27374/net-lama/internal/store"
	pb "github.com/fox27374/net-lama/proto"
)

// evaluateAlerts checks a received result against the rules watching its
// test, tracking consecutive breaches so a rule only fires after its
// configured count, and resolves a firing alert when a good result arrives.
// Implements hysteresis: clear_threshold + clear_count with dead-band semantics.
func (s *Server) evaluateAlerts(conn *connectedAgent, result *pb.TestResult) {
	if result.TestId == "" {
		return
	}
	rules, err := s.Store.RulesForTest(result.TestId)
	if err != nil || len(rules) == 0 {
		return
	}

	// Some tests emit several results per run (one per target/query); the
	// subject keeps a separate alert per target so a healthy target doesn't
	// resolve an alert raised by a failing one.
	subject := resultSubject(result)

	for _, rule := range rules {
		breach, value, applicable := evalRule(rule, result)
		if !applicable {
			continue
		}
		key := rule.ID + "|" + conn.agent.ID + "|" + subject

		if breach {
			// Increment breach counter and reset good counter
			s.breachMu.Lock()
			s.breachCount[key]++
			n := s.breachCount[key]
			delete(s.goodCount, key)
			s.breachMu.Unlock()

			if n >= rule.ForCount {
				s.fireAlert(rule, conn, subject, value)
			}
		} else {
			// Any non-breaching sample breaks the consecutive-breach streak
			// toward firing ("N times in a row").
			s.breachMu.Lock()
			delete(s.breachCount, key)
			s.breachMu.Unlock()

			clearOk := s.checkClearCondition(rule, value)
			if clearOk {
				// Good sample; increment clear counter
				s.breachMu.Lock()
				s.goodCount[key]++
				goodN := s.goodCount[key]
				s.breachMu.Unlock()

				if goodN >= rule.ClearCount {
					s.resolveAlert(rule, conn, subject)
					s.breachMu.Lock()
					delete(s.goodCount, key)
					s.breachMu.Unlock()
				}
			} else {
				// Sample in dead band; reset clear progress but keep firing
				s.breachMu.Lock()
				delete(s.goodCount, key)
				s.breachMu.Unlock()
			}
		}
	}
}

// checkClearCondition determines if a non-breach sample satisfies the clear condition.
// If clear_threshold is set, the value must satisfy the inverse condition.
// Otherwise, non-breach is sufficient.
func (s *Server) checkClearCondition(rule *store.AlertRule, value float64) bool {
	if rule.ClearThreshold == nil {
		return true // Non-breach is sufficient for clear
	}

	// Inverse condition: if threshold op is >, clear is <; if <=, clear is >
	switch rule.Operator {
	case ">":
		return value < *rule.ClearThreshold
	case ">=":
		return value < *rule.ClearThreshold
	case "<":
		return value > *rule.ClearThreshold
	case "<=":
		return value > *rule.ClearThreshold
	case "==":
		return true // For equality, non-breach is the clear condition
	}
	return true
}

func (s *Server) fireAlert(rule *store.AlertRule, conn *connectedAgent, subject string, value float64) {
	active, err := s.Store.ActiveAlert(rule.ID, conn.agent.ID, subject)
	if err != nil || active != nil {
		return // already firing (or lookup failed)
	}
	msg := alertMessage(rule, conn, subject, value)
	alert, err := s.Store.FireAlert(rule.ID, conn.agent.ID, subject, value, msg)
	if err != nil {
		s.Logger.Error("Firing alert failed", slog.Any("error", err))
		return
	}
	s.Logger.Warn("Alert firing",
		slog.String("rule", rule.Name), slog.String("agent", conn.agent.Name), slog.String("message", msg))
	s.notifyTargets(rule, conn, alert, "firing", value)
}

func (s *Server) resolveAlert(rule *store.AlertRule, conn *connectedAgent, subject string) {
	active, err := s.Store.ActiveAlert(rule.ID, conn.agent.ID, subject)
	if err != nil || active == nil {
		return
	}
	if err := s.Store.ResolveAlert(active.ID); err != nil {
		s.Logger.Error("Resolving alert failed", slog.Any("error", err))
		return
	}
	s.Logger.Info("Alert resolved",
		slog.String("rule", rule.Name), slog.String("agent", conn.agent.Name))
	active.State = "resolved"
	s.notifyTargets(rule, conn, active, "resolved", active.Value)
}

// resultSubject identifies the specific target within a result, so
// multi-target tests get one alert per target.
func resultSubject(result *pb.TestResult) string {
	switch r := result.Result.(type) {
	case *pb.TestResult_Ping:
		return r.Ping.Target
	case *pb.TestResult_Dns:
		return r.Dns.Query + "@" + r.Dns.Server
	case *pb.TestResult_Http:
		return r.Http.Url
	case *pb.TestResult_Tcp:
		return r.Tcp.Target
	case *pb.TestResult_Traceroute:
		return r.Traceroute.Target
	}
	return ""
}

// evalRule computes whether a result breaches a rule. applicable is false
// when the metric does not apply to this result's type (rule is skipped).
func evalRule(rule *store.AlertRule, result *pb.TestResult) (breach bool, value float64, applicable bool) {
	if rule.Metric == "unhealthy" {
		return !resultOK(result), 0, true
	}
	// Numeric metrics only apply to successful results of the right type;
	// use the "unhealthy" metric to catch outright failures.
	if result.Error != "" {
		return false, 0, false
	}

	v, ok := metricValue(rule.Metric, result)
	if !ok {
		return false, 0, false
	}
	return compare(v, rule.Operator, rule.Threshold), v, true
}

func metricValue(metric string, result *pb.TestResult) (float64, bool) {
	switch r := result.Result.(type) {
	case *pb.TestResult_Ping:
		switch metric {
		case "latency_ms":
			return r.Ping.AvgRttMs, true
		case "loss_percent":
			return r.Ping.LossPercent, true
		}
	case *pb.TestResult_Dns:
		if metric == "latency_ms" {
			return r.Dns.ResolveTimeMs, true
		}
	case *pb.TestResult_Http:
		if metric == "latency_ms" {
			return r.Http.TotalMs, true
		}
	case *pb.TestResult_Tcp:
		if metric == "latency_ms" {
			return r.Tcp.ConnectMs, true
		}
	case *pb.TestResult_Traceroute:
		if metric == "latency_ms" {
			return r.Traceroute.RttMs, true
		}
	case *pb.TestResult_Speedtest:
		switch metric {
		case "latency_ms":
			return r.Speedtest.LatencyMs, true
		case "download_mbps":
			return r.Speedtest.DownloadMbps, true
		case "upload_mbps":
			return r.Speedtest.UploadMbps, true
		}
	}
	return 0, false
}

func compare(v float64, op string, threshold float64) bool {
	switch op {
	case ">":
		return v > threshold
	case ">=":
		return v >= threshold
	case "<":
		return v < threshold
	case "<=":
		return v <= threshold
	case "==":
		return v == threshold
	}
	return false
}

func alertMessage(rule *store.AlertRule, conn *connectedAgent, subject string, value float64) string {
	target := ""
	if subject != "" {
		target = " [" + subject + "]"
	}
	if rule.Metric == "unhealthy" {
		return fmt.Sprintf("%s: %s%s is unhealthy on %s", rule.Name, rule.TestName, target, conn.agent.Name)
	}
	return fmt.Sprintf("%s: %s%s %s %.2f (value %.2f) on %s",
		rule.Name, rule.Metric, target, rule.Operator, rule.Threshold, value, conn.agent.Name)
}
