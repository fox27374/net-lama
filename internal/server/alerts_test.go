package server

import (
	"testing"

	"github.com/fox27374/net-lama/internal/store"
)

// TestClearCondition verifies the hysteresis clear condition logic.
func TestClearCondition(t *testing.T) {
	tests := []struct {
		name           string
		operator       string
		threshold      float64
		clearThreshold *float64
		value          float64
		expectedClear  bool
	}{
		// No clear threshold: always clear on non-breach
		{
			name:           "no clear threshold, value below threshold",
			operator:       ">",
			threshold:      100,
			clearThreshold: nil,
			value:          50,
			expectedClear:  true,
		},

		// Operator >: clear when value < clearThreshold
		{
			name:           "operator > : value below clear threshold",
			operator:       ">",
			threshold:      100,
			clearThreshold: floatPtr(70),
			value:          60,
			expectedClear:  true,
		},
		{
			name:           "operator > : value in dead band",
			operator:       ">",
			threshold:      100,
			clearThreshold: floatPtr(70),
			value:          85,
			expectedClear:  false,
		},

		// Operator >=: clear when value < clearThreshold
		{
			name:           "operator >=: value below clear threshold",
			operator:       ">=",
			threshold:      100,
			clearThreshold: floatPtr(70),
			value:          60,
			expectedClear:  true,
		},
		{
			name:           "operator >=: value in dead band",
			operator:       ">=",
			threshold:      100,
			clearThreshold: floatPtr(70),
			value:          85,
			expectedClear:  false,
		},

		// Operator <: clear when value > clearThreshold
		{
			name:           "operator < : value above clear threshold",
			operator:       "<",
			threshold:      50,
			clearThreshold: floatPtr(70),
			value:          80,
			expectedClear:  true,
		},
		{
			name:           "operator < : value in dead band",
			operator:       "<",
			threshold:      50,
			clearThreshold: floatPtr(70),
			value:          60,
			expectedClear:  false,
		},

		// Operator <=: clear when value > clearThreshold
		{
			name:           "operator <=: value above clear threshold",
			operator:       "<=",
			threshold:      50,
			clearThreshold: floatPtr(70),
			value:          80,
			expectedClear:  true,
		},
		{
			name:           "operator <=: value in dead band",
			operator:       "<=",
			threshold:      50,
			clearThreshold: floatPtr(70),
			value:          60,
			expectedClear:  false,
		},

		// Operator ==: always clear on non-breach
		{
			name:           "operator ==: clear on non-breach",
			operator:       "==",
			threshold:      100,
			clearThreshold: floatPtr(100),
			value:          50,
			expectedClear:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := &store.AlertRule{
				Operator:       tt.operator,
				Threshold:      tt.threshold,
				ClearThreshold: tt.clearThreshold,
			}

			got := clearCondition(rule, tt.value)
			if got != tt.expectedClear {
				t.Errorf("clearCondition(%v, %v) = %v, want %v",
					rule.Operator, tt.value, got, tt.expectedClear)
			}
		})
	}
}

// TestHysteresisStateMachine drives the real decision function through a
// fire/clear sequence: ping latency_ms > 100, for_count 3, clear_threshold
// 70, clear_count 2 — fires on the third consecutive breach, resolves after
// two consecutive samples below 70, and a sample between 70 and 100 keeps
// the alert firing while resetting the clear progress.
func TestHysteresisStateMachine(t *testing.T) {
	rule := &store.AlertRule{
		ID:             "rule1",
		Metric:         "latency_ms",
		Operator:       ">",
		Threshold:      100,
		ForCount:       3,
		ClearCount:     2,
		ClearThreshold: floatPtr(70),
	}

	samples := []struct {
		value  float64
		want   alertAction
		reason string
	}{
		{150, alertNone, "breach 1 of 3"},
		{120, alertNone, "breach 2 of 3"},
		{110, alertFire, "breach 3 reaches ForCount"},
		{105, alertFire, "still breaching: fire again (fireAlert dedups)"},
		{80, alertNone, "dead band: no clear progress"},
		{60, alertNone, "good 1 of 2"},
		{80, alertNone, "dead band wipes the clear progress"},
		{60, alertNone, "good 1 of 2, starting over"},
		{50, alertResolve, "good 2 reaches ClearCount"},
		{50, alertNone, "already resolved, nothing to do"},
	}

	var st alertState
	for _, sample := range samples {
		breach := compare(sample.value, rule.Operator, rule.Threshold)
		next, action := decideAlert(st, rule, breach, sample.value)
		if action != sample.want {
			t.Errorf("sample %.0f (%s): got action %v, want %v",
				sample.value, sample.reason, action, sample.want)
		}
		st = next
	}
}

// TestDecideAlertResolveClearsState pins the bookkeeping the map relies on:
// a resolve must leave the zero state, which is what lets evaluateAlerts
// drop the key instead of growing one entry per rule|agent|subject forever.
func TestDecideAlertResolveClearsState(t *testing.T) {
	rule := &store.AlertRule{Operator: ">", Threshold: 100, ForCount: 1, ClearCount: 1}

	st, action := decideAlert(alertState{breaches: 5}, rule, false, 10)
	if action != alertResolve {
		t.Fatalf("got action %v, want alertResolve", action)
	}
	if st != (alertState{}) {
		t.Errorf("state after resolve = %+v, want zero", st)
	}
}

// TestDecideAlertWithoutClearThreshold covers the default rule shape: no
// clear threshold at all, so any non-breaching sample counts as good.
func TestDecideAlertWithoutClearThreshold(t *testing.T) {
	rule := &store.AlertRule{Operator: ">", Threshold: 100, ForCount: 2, ClearCount: 2}

	var st alertState
	for _, value := range []float64{150, 150} {
		st, _ = decideAlert(st, rule, true, value)
	}
	if st.breaches != 2 {
		t.Fatalf("breaches = %d, want 2", st.breaches)
	}

	st, action := decideAlert(st, rule, false, 99)
	if action != alertNone || st.breaches != 0 || st.goods != 1 {
		t.Fatalf("first good sample: action %v, state %+v", action, st)
	}
	if _, action := decideAlert(st, rule, false, 99); action != alertResolve {
		t.Errorf("second good sample: got %v, want alertResolve", action)
	}
}

func floatPtr(f float64) *float64 {
	return &f
}
