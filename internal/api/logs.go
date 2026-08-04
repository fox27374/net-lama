package api

import (
	"net/http"
	"strconv"

	"github.com/fox27374/net-lama/internal/store"
)

// handleListLogs returns recent server/agent log lines, newest first.
// Scoping goes through tenantFilter like every other listing: tenant users
// see their own tenant, admins see everything unless they name a tenant.
// Server logs are the one wrinkle — they carry no tenant, so a tenant
// filter would match nothing and is dropped, and tenant users may not ask
// for them at all.
func (a *API) handleListLogs(w http.ResponseWriter, r *http.Request, user *store.User) {
	q := r.URL.Query()
	source := q.Get("source")

	if source == "server" && !user.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	limit, _ := strconv.Atoi(q.Get("limit"))
	filter := store.LogFilter{
		AgentID: q.Get("agentId"),
		Source:  source,
		Level:   q.Get("level"),
		Limit:   limit,
	}
	tenantID, ok := tenantFilter(user, q.Get("tenantId"))
	if !ok {
		writeError(w, http.StatusForbidden, "not your tenant")
		return
	}
	if source != "server" {
		// Server logs carry no tenant, so any filter would match nothing.
		filter.TenantID = tenantID
	}

	logs, err := a.Store.ListLogs(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, logs)
}
