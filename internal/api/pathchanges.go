package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/fox27374/net-lama/internal/store"
)

// handleListPathChanges returns recorded route changes, newest first.
// GET /api/v1/path-changes?tenantId=&agentId=&testId=&since=&limit=
//
// Tenant-scoped like results: a change event names an agent and the hops it
// traverses, which is that tenant's data.
func (a *API) handleListPathChanges(w http.ResponseWriter, r *http.Request, user *store.User) {
	q := r.URL.Query()
	tenantID, ok := tenantScope(user, q.Get("tenantId"))
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}

	f := store.PathChangeFilter{
		TenantID: tenantID,
		AgentID:  q.Get("agentId"),
		TestID:   q.Get("testId"),
	}
	if s := q.Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be RFC3339")
			return
		}
		f.Since = t
	}
	if l := q.Get("limit"); l != "" {
		f.Limit, _ = strconv.Atoi(l)
	}

	changes, err := a.Store.ListPathChanges(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, changes)
}
