// Package testtype is the registry of what a Net-Lama test type is.
//
// A test type used to be a bare string that a dozen switch statements
// agreed on by convention, spread across the server, the store and the
// browser. They drifted: the alert engine and the dashboard read different
// numbers as "the" metric of a wlan_passive run, and nothing failed.
//
// Everything a caller needs to know about a type given only its name or a
// result off the wire now lives in one Spec here. Adding a type means
// adding an entry (plus the proto message and the probe that fills it),
// not editing every switch that mentions the old ones.
//
// The registry deliberately knows nothing about store.TestDef: it owns the
// parameter payload (Params, in params.go) but not the row it is stored in,
// so internal/server/config.go keeps the store-shaped shell around it.
// Keeping the dependency one-way is what lets internal/store use the
// registry too.
package testtype

import (
	"encoding/json"
	"fmt"
	"sort"

	pb "github.com/fox27374/net-lama/proto"
	"google.golang.org/protobuf/encoding/protojson"
)

// Metric reads one number out of a result. ok is false when the result
// carries no usable value for it — a failed run, the wrong result type, or
// a zero where zero means "not measured".
type Metric func(*pb.TestResult) (value float64, ok bool)

// Params is one test type's stored parameter payload: the JSON in
// tests.params. It checks itself and turns itself into the proto oneof
// pushed to an agent, which is what keeps both jobs off a type switch.
type Params interface {
	// Validate reports whether the payload is usable and fills in
	// defaults in place. The normalized form is what gets stored.
	Validate() error

	// Apply sets this type's oneof on a spec bound for an agent.
	Apply(*pb.TestSpec)
}

// Spec is everything the rest of the system needs to know about one test
// type without switching on its name.
type Spec struct {
	// Type is the identifier used on the wire, in the database's
	// results.test_type column, and in the ?type= API filter.
	Type string

	// Capability an agent must report before the server pushes tests of
	// this type to it. Usually the type's own name.
	Capability string

	// Unit of Primary, for axis labels and threshold editors ("ms", "Mbps").
	Unit string

	// LowerIsWorse flips threshold comparisons: throughput degrades as it
	// falls, latency as it rises.
	LowerIsWorse bool

	// Primary is the single number that represents a run: what the
	// dashboard sparkline plots and what warn/crit thresholds compare
	// against. Every type must have one.
	Primary Metric

	// Metrics are the named values an alert rule may watch, beyond the
	// type-independent "unhealthy" and "state" rules.
	Metrics map[string]Metric

	// Subject identifies the target within a result, so a test that emits
	// one result per target gets one alert per target rather than a single
	// alert flapping between them. Empty for single-result types.
	Subject func(*pb.TestResult) string

	// NewParams returns an empty parameter payload for this type, to
	// unmarshal a stored test's params into. Every type has one, even
	// the parameterless ones.
	NewParams func() Params

	// MinIntervalSeconds is the shortest schedule this type may run on:
	// a floor for the expensive types (a speedtest saturates the link, a
	// wlan_active run takes the radio away from passive sweeps).
	// Defaults to the global 5s floor.
	MinIntervalSeconds uint32
}

// DecodeParams unmarshals a stored params payload for this type. An empty
// payload decodes to the zero value rather than an error, so a type whose
// params are all optional needs no stored JSON at all.
func (s *Spec) DecodeParams(raw []byte) (Params, error) {
	p := s.NewParams()
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, p); err != nil {
			return nil, fmt.Errorf("invalid %s parameters: %w", s.Type, err)
		}
	}
	return p, nil
}

// specs is the registry. Order is not significant; All() sorts by name.
var specs = map[string]*Spec{}

func register(s *Spec) {
	if s.Primary == nil {
		panic("testtype: " + s.Type + " has no Primary metric")
	}
	if s.NewParams == nil {
		panic("testtype: " + s.Type + " has no NewParams")
	}
	if s.Capability == "" {
		s.Capability = s.Type
	}
	if s.MinIntervalSeconds == 0 {
		s.MinIntervalSeconds = 5
	}
	specs[s.Type] = s
}

// Get returns the spec for a test type, or nil if the type is unknown.
// Callers handle nil rather than getting a zero Spec whose Primary would
// panic.
func Get(testType string) *Spec {
	return specs[testType]
}

// All returns every registered spec, ordered by type name so API output
// and generated docs are stable.
func All() []*Spec {
	out := make([]*Spec, 0, len(specs))
	for _, s := range specs {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// OfResult identifies a result by its payload oneof rather than by a type
// string, for the paths that receive a result before knowing which test
// produced it. Returns nil for an unset or unrecognised payload.
func OfResult(r *pb.TestResult) *Spec {
	if r == nil {
		return nil
	}
	return specs[resultTypeName(r)]
}

// TypeOf is OfResult's name-only form, for storing the type alongside a
// result. Returns "unknown" so an unrecognised payload is visible in the
// database rather than silently blank.
func TypeOf(r *pb.TestResult) string {
	if s := OfResult(r); s != nil {
		return s.Type
	}
	return "unknown"
}

// MetricNames returns this type's alert metric names, sorted.
func (s *Spec) MetricNames() []string {
	out := make([]string, 0, len(s.Metrics))
	for name := range s.Metrics {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// AlertMetrics returns every metric name any type exposes to alert rules,
// sorted. Used to validate a rule's metric field without a second list to
// keep in sync.
func AlertMetrics() []string {
	seen := map[string]bool{}
	for _, s := range specs {
		for name := range s.Metrics {
			seen[name] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// MetricValue reads a named alert metric off a result, identifying the
// type from the result itself. ok is false when the metric does not apply
// to this result's type, which is how a rule ends up skipped rather than
// evaluated against a meaningless zero.
func MetricValue(metric string, r *pb.TestResult) (float64, bool) {
	s := OfResult(r)
	if s == nil {
		return 0, false
	}
	fn, ok := s.Metrics[metric]
	if !ok {
		return 0, false
	}
	return fn(r)
}

// SubjectOf returns the per-target alert subject for a result.
func SubjectOf(r *pb.TestResult) string {
	s := OfResult(r)
	if s == nil || s.Subject == nil {
		return ""
	}
	return s.Subject(r)
}

// --- stored payload codec ---
//
// A result is written to the database as protojson and has to come back as
// the same typed message. Both directions live here so they cannot drift:
// the store used to re-parse the payload as map[string]interface{} and
// grope for field names by string, which a proto field rename would have
// broken silently.

// EncodeResult marshals a result for the results.payload column.
// EmitDefaultValues keeps zero-valued fields (reached=false, loss=0) in the
// JSON, which the browser relies on.
func EncodeResult(r *pb.TestResult) ([]byte, error) {
	return protojson.MarshalOptions{EmitDefaultValues: true}.Marshal(r)
}

// DecodeResult parses a stored payload back into a result. Unknown fields
// are tolerated so a payload written by a newer server still loads.
func DecodeResult(payload []byte) (*pb.TestResult, error) {
	var r pb.TestResult
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(payload, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// --- helpers ---

// positive reports a value only when it is above zero. Across every type a
// zero primary metric means "not measured" rather than a real reading of
// zero, so it is reported as absent — a gap in a sparkline, not a dip.
func positive(v float64) (float64, bool) {
	if v > 0 {
		return v, true
	}
	return 0, false
}
