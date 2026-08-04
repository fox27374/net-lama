package api

import (
	"net/http"

	"github.com/fox27374/net-lama/internal/saas"
	"github.com/fox27374/net-lama/internal/store"
)

// handleListSaasServices returns the service catalog a saas test can pick
// from, including each service's endpoints so an operator can see what a
// test will actually check before creating it. Like the test-type
// registry, this is the shape of the software rather than anyone's data,
// so it needs authentication but no tenant scoping.
func (a *API) handleListSaasServices(w http.ResponseWriter, r *http.Request, _ *store.User) {
	writeJSON(w, http.StatusOK, saas.All())
}
