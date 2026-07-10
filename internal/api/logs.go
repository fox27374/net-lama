package api

import (
	"net/http"
	"strconv"

	"github.com/fox27374/net-lama/internal/store"
)

// handleListLogs returns recent server/agent log lines, newest first.
// Tenant users are implicitly scoped to their own tenant (never seeing
// server logs, which carry no tenant) and may not request source=server.
// Admins see everything by default and may filter with ?tenantId=; a
// tenantId filter is ignored for source=server since server logs have no
// tenant to match.
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
	if user.IsAdmin {
		if source != "server" {
			filter.TenantID = q.Get("tenantId") // empty = all tenants
		}
	} else {
		filter.TenantID = user.TenantID
	}

	logs, err := a.Store.ListLogs(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, logs)
}
