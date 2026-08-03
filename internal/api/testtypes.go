package api

import (
	"net/http"

	"github.com/fox27374/net-lama/internal/store"
	"github.com/fox27374/net-lama/internal/testtype"
)

// testTypeView is one entry of the test-type registry as the UI sees it.
// The browser used to carry its own copies of these facts — a unit map, a
// capability function, a "lower is worse" check — which could drift from
// the server's without anything failing.
type testTypeView struct {
	Type         string   `json:"type"`
	Capability   string   `json:"capability"`
	Unit         string   `json:"unit"`
	LowerIsWorse bool     `json:"lowerIsWorse"`
	Metrics      []string `json:"metrics"`
}

// handleListTestTypes returns the registry. It is the same for every
// tenant — this is the shape of the software, not of anyone's data — so it
// needs authentication but no tenant scoping.
func (a *API) handleListTestTypes(w http.ResponseWriter, r *http.Request, _ *store.User) {
	specs := testtype.All()
	out := make([]testTypeView, 0, len(specs))
	for _, spec := range specs {
		out = append(out, testTypeView{
			Type:         spec.Type,
			Capability:   spec.Capability,
			Unit:         spec.Unit,
			LowerIsWorse: spec.LowerIsWorse,
			Metrics:      spec.MetricNames(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}
