package server

import (
	"encoding/json"
	"fmt"

	"github.com/fox27374/net-lama/internal/store"
	"github.com/fox27374/net-lama/internal/testtype"
	pb "github.com/fox27374/net-lama/proto"
)

// Thresholds represents warn/crit boundaries for a test.
type Thresholds struct {
	Warn float64 `json:"warn"`
	Crit float64 `json:"crit"`
}

// ValidateTestDef checks type, interval and the type-specific parameters
// and returns the definition with normalized params. Everything
// type-specific comes from the registry, so a new type needs no case here.
func ValidateTestDef(t *store.TestDef) error {
	if t.Name == "" {
		return fmt.Errorf("test name is required")
	}
	spec := testtype.Get(t.Type)
	if spec == nil {
		return fmt.Errorf("unknown test type %q", t.Type)
	}
	if t.IntervalSeconds < spec.MinIntervalSeconds {
		return fmt.Errorf("%s interval must be at least %d seconds", t.Type, spec.MinIntervalSeconds)
	}

	if len(t.Thresholds) > 0 {
		var th Thresholds
		if err := json.Unmarshal(t.Thresholds, &th); err != nil {
			return fmt.Errorf("invalid thresholds: %w", err)
		}
		if th.Warn > 0 && th.Crit > 0 {
			// A lower-is-worse type degrades by falling, so its bands run
			// the other way: orange below warn, red below crit.
			if spec.LowerIsWorse {
				if th.Warn <= th.Crit {
					return fmt.Errorf("warn threshold must be greater than crit threshold for %s", t.Type)
				}
			} else if th.Warn >= th.Crit {
				return fmt.Errorf("warn threshold must be less than crit threshold")
			}
		}
	}

	params, err := spec.DecodeParams(t.Params)
	if err != nil {
		return err
	}
	if err := params.Validate(); err != nil {
		return err
	}
	normalized, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encoding %s parameters: %w", t.Type, err)
	}
	t.Params = normalized
	return nil
}

// TestSpec converts a stored test definition to the protobuf spec
// pushed down the control stream. Params are re-validated on the way out:
// a definition stored before a validation rule tightened is skipped by the
// caller rather than pushed to an agent that cannot run it.
func TestSpec(t *store.TestDef) (*pb.TestSpec, error) {
	spec := testtype.Get(t.Type)
	if spec == nil {
		return nil, fmt.Errorf("unknown test type %q", t.Type)
	}
	params, err := spec.DecodeParams(t.Params)
	if err != nil {
		return nil, err
	}
	if err := params.Validate(); err != nil {
		return nil, err
	}
	out := &pb.TestSpec{
		Id:              t.ID,
		Name:            t.Name,
		IntervalSeconds: t.IntervalSeconds,
	}
	params.Apply(out)
	return out, nil
}
