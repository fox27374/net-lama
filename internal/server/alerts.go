package server

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/fox27374/net-lama/internal/store"
	"github.com/fox27374/net-lama/internal/testtype"
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

	// Get test definition for state computation
	var test *store.TestDef
	if testDef, err := s.Store.GetTest(result.TestId); err == nil {
		test = testDef
	}

	// Some tests emit several results per run (one per target/query); the
	// subject keeps a separate alert per target so a healthy target doesn't
	// resolve an alert raised by a failing one.
	subject := testtype.SubjectOf(result)

	for _, rule := range rules {
		breach, value, applicable := evalRule(rule, result, test)
		if !applicable {
			continue
		}
		key := rule.ID + "|" + conn.agent.ID + "|" + subject

		s.alertMu.Lock()
		next, action := decideAlert(s.alertStates[key], rule, breach, value)
		if next == (alertState{}) {
			delete(s.alertStates, key)
		} else {
			s.alertStates[key] = next
		}
		s.alertMu.Unlock()

		switch action {
		case alertFire:
			s.fireAlert(rule, conn, subject, value)
		case alertResolve:
			s.resolveAlert(rule, conn, subject)
		}
	}
}

// alertState is the hysteresis progress of one rule against one agent and
// subject: how many samples in a row have breached, and how many in a row
// have satisfied the clear condition since.
type alertState struct {
	breaches int
	goods    int
}

// alertAction is what a sample calls for. Firing and resolving are
// idempotent downstream (both check for an active alert first), so a
// sustained breach asks to fire on every sample past ForCount.
type alertAction int

const (
	alertNone alertAction = iota
	alertFire
	alertResolve
)

// decideAlert advances the hysteresis state machine by one sample and says
// what it calls for. It is pure — no locks, no store, no notifications —
// so the fire/clear sequence can be tested as the sequence it is.
func decideAlert(st alertState, rule *store.AlertRule, breach bool, value float64) (alertState, alertAction) {
	if breach {
		// A breach resets clear progress: "N good samples in a row".
		st = alertState{breaches: st.breaches + 1}
		if st.breaches >= rule.ForCount {
			return st, alertFire
		}
		return st, alertNone
	}

	// Any non-breaching sample breaks the consecutive-breach streak
	// toward firing ("N times in a row").
	st.breaches = 0

	if !clearCondition(rule, value) {
		// Sample in the dead band: too good to breach, not good enough to
		// clear. Reset clear progress, leave a firing alert firing.
		st.goods = 0
		return st, alertNone
	}

	st.goods++
	if st.goods >= rule.ClearCount {
		return alertState{}, alertResolve
	}
	return st, alertNone
}

// clearCondition determines if a non-breach sample satisfies the clear condition.
// If clear_threshold is set, the value must satisfy the inverse condition.
// Otherwise, non-breach is sufficient.
func clearCondition(rule *store.AlertRule, value float64) bool {
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

// evalRule computes whether a result breaches a rule. applicable is false
// when the metric does not apply to this result's type (rule is skipped).
func evalRule(rule *store.AlertRule, result *pb.TestResult, test *store.TestDef) (breach bool, value float64, applicable bool) {
	if rule.Metric == "unhealthy" {
		return !resultOK(result), 0, true
	}

	// State metric: compute result state and compare against level (1=orange, 2=red)
	if rule.Metric == "state" {
		stateLevel := resultState(result, test)
		return compare(stateLevel, rule.Operator, rule.Threshold), stateLevel, true
	}

	// Numeric metrics only apply to successful results of the right type;
	// use the "unhealthy" metric to catch outright failures.
	if result.Error != "" {
		return false, 0, false
	}

	v, ok := testtype.MetricValue(rule.Metric, result)
	if !ok {
		return false, 0, false
	}
	return compare(v, rule.Operator, rule.Threshold), v, true
}

// resultState computes the numeric state of a result for "state" metric alerts.
// Returns: 0=green, 1=orange, 2=red (failed result).
// Requires the test's thresholds from the database.
func resultState(result *pb.TestResult, test *store.TestDef) float64 {
	// Failed result is always red (level 2)
	if result.Error != "" {
		return 2
	}
	if !resultOK(result) {
		return 2
	}
	// Parse test thresholds if present
	if test == nil || len(test.Thresholds) == 0 {
		return 0 // green if no thresholds
	}

	var thresholds struct {
		Warn float64 `json:"warn"`
		Crit float64 `json:"crit"`
	}
	if err := json.Unmarshal(test.Thresholds, &thresholds); err != nil {
		return 0
	}

	spec := testtype.Get(test.Type)
	if spec == nil {
		return 0
	}
	val, ok := spec.Primary(result)
	if !ok {
		return 0
	}

	if spec.LowerIsWorse {
		if thresholds.Crit > 0 && val < thresholds.Crit {
			return 2
		}
		if thresholds.Warn > 0 && val < thresholds.Warn {
			return 1
		}
	} else {
		if thresholds.Crit > 0 && val > thresholds.Crit {
			return 2
		}
		if thresholds.Warn > 0 && val > thresholds.Warn {
			return 1
		}
	}
	return 0
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
