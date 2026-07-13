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
			srv := &Server{}
			rule := &store.AlertRule{
				Operator:       tt.operator,
				Threshold:      tt.threshold,
				ClearThreshold: tt.clearThreshold,
			}

			got := srv.checkClearCondition(rule, tt.value)
			if got != tt.expectedClear {
				t.Errorf("checkClearCondition(%v, %v) = %v, want %v",
					rule.Operator, tt.value, got, tt.expectedClear)
			}
		})
	}
}

// TestHysteresisStateMachine verifies the hysteresis fire/clear sequence.
// Example from the plan: ping latency_ms > 100, for_count 10,
// clear_threshold 70, clear_count 10 → fires after 10 consecutive >100 samples,
// resolves after 10 consecutive <70 samples, samples between 70 and 100 keep
// the alert firing and reset the clear progress.
func TestHysteresisStateMachine(t *testing.T) {
	t.Run("fire after ForCount breaches, resolve after ClearCount good samples", func(t *testing.T) {
		// Setup: rule with for_count=3, clear_count=2, clear_threshold=70
		rule := &store.AlertRule{
			ID:             "rule1",
			Metric:         "latency_ms",
			Operator:       ">",
			Threshold:      100,
			ForCount:       3,
			ClearCount:     2,
			ClearThreshold: floatPtr(70),
		}

		srv := &Server{
			breachCount: make(map[string]int),
			goodCount:   make(map[string]int),
		}

		key := "rule1|agent1|subject1"

		// Sequence of samples and expected states
		samples := []struct {
			value          float64
			breaching      bool
			expectedFires  int  // incremented when breach count reaches ForCount
			expectedResets int  // incremented when good count reaches ClearCount
		}{
			{value: 150, breaching: true, expectedFires: 0, expectedResets: 0},  // breach 1
			{value: 120, breaching: true, expectedFires: 0, expectedResets: 0},  // breach 2
			{value: 110, breaching: true, expectedFires: 1, expectedResets: 0},  // breach 3 -> fires
			{value: 80, breaching: false, expectedFires: 1, expectedResets: 0},  // in dead band, clear progress resets
			{value: 60, breaching: false, expectedFires: 1, expectedResets: 0},  // good 1
			{value: 50, breaching: false, expectedFires: 1, expectedResets: 1},  // good 2 -> resolves
		}

		firesObserved := 0
		resolvesObserved := 0

		for _, sample := range samples {
			breach := sample.value > rule.Threshold
			clearOk := srv.checkClearCondition(rule, sample.value)

			if breach {
				srv.breachCount[key]++
				delete(srv.goodCount, key)

				if srv.breachCount[key] >= rule.ForCount {
					firesObserved++
				}
			} else {
				if clearOk {
					srv.goodCount[key]++
					goodN := srv.goodCount[key]

					if goodN >= rule.ClearCount {
						resolvesObserved++
						delete(srv.breachCount, key)
						delete(srv.goodCount, key)
					}
				} else {
					// Dead band: reset clear progress
					delete(srv.goodCount, key)
				}
			}

			if firesObserved != sample.expectedFires {
				t.Errorf("After sample %.0f: expected %d fires, got %d",
					sample.value, sample.expectedFires, firesObserved)
			}
			if resolvesObserved != sample.expectedResets {
				t.Errorf("After sample %.0f: expected %d resolves, got %d",
					sample.value, sample.expectedResets, resolvesObserved)
			}
		}
	})
}

func floatPtr(f float64) *float64 {
	return &f
}
