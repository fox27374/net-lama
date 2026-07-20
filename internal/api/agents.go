package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fox27374/net-lama/internal/oui"
	"github.com/fox27374/net-lama/internal/store"
)

// Health represents the agent health status.
type Health struct {
	Status        string   `json:"status"` // "healthy" | "degraded" | "unhealthy" | "unknown"
	Reasons       []string `json:"reasons,omitempty"`
	UptimeSeconds uint64   `json:"uptimeSeconds,omitempty"`
}

// agentView is the API representation of an agent. The enrollment token
// is only included directly after creation.
type agentView struct {
	*store.Agent
	Connected bool    `json:"connected"`
	Health    *Health `json:"health,omitempty"`
}

func (a *API) handleListAgents(w http.ResponseWriter, r *http.Request, user *store.User) {
	tenantID := user.TenantID
	if user.IsAdmin {
		tenantID = r.URL.Query().Get("tenantId") // empty = all tenants
	}

	agents, err := a.Store.ListAgents(tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	views := make([]*agentView, 0, len(agents))
	for _, agent := range agents {
		agent.Token = "" // never expose tokens in listings
		connected := a.Server.AgentConnected(agent.ID)

		// Evaluate health
		healthEval := a.Server.EvaluateAgentHealth(agent)
		health := &Health{
			Status: string(healthEval.Status),
		}
		if healthEval.Status != "unknown" {
			health.Reasons = healthEval.Reasons
			health.UptimeSeconds = healthEval.UptimeSeconds
		}

		views = append(views, &agentView{
			Agent:     agent,
			Connected: connected,
			Health:    health,
		})
	}
	writeJSON(w, http.StatusOK, views)
}

func (a *API) handleCreateAgent(w http.ResponseWriter, r *http.Request, user *store.User) {
	var req struct {
		Name   string `json:"name"`
		SiteID string `json:"siteId"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || req.SiteID == "" {
		writeError(w, http.StatusBadRequest, "agent name and siteId are required")
		return
	}

	site, err := a.Store.GetSite(req.SiteID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown site")
		return
	}
	if !user.IsAdmin && user.TenantID != site.TenantID {
		writeError(w, http.StatusForbidden, "no access to this site")
		return
	}

	agent, err := a.Store.CreateAgent(site.TenantID, site.ID, req.Name)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	agent.SiteName = site.Name

	// The one time the token is returned: right after creation
	writeJSON(w, http.StatusCreated, &agentView{Agent: agent})
}

// getScopedAgent loads an agent and enforces tenant access.
func (a *API) getScopedAgent(w http.ResponseWriter, r *http.Request, user *store.User) *store.Agent {
	agent, err := a.Store.GetAgent(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return nil
	}
	if !user.IsAdmin && user.TenantID != agent.TenantID {
		writeError(w, http.StatusForbidden, "no access to this agent")
		return nil
	}
	return agent
}

// handleUpdateAgent renames an agent and/or moves it to another site of
// the same tenant; the resulting config is pushed live.
func (a *API) handleUpdateAgent(w http.ResponseWriter, r *http.Request, user *store.User) {
	agent := a.getScopedAgent(w, r, user)
	if agent == nil {
		return
	}

	var req struct {
		Name   string `json:"name"`
		SiteID string `json:"siteId"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || req.SiteID == "" {
		writeError(w, http.StatusBadRequest, "agent name and siteId are required")
		return
	}

	site, err := a.Store.GetSite(req.SiteID)
	if err != nil || site.TenantID != agent.TenantID {
		writeError(w, http.StatusBadRequest, "unknown site")
		return
	}

	if err := a.Store.UpdateAgent(agent.ID, req.Name, site.ID); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	// The connected-agent registry holds the agent's old site; refresh it
	// by pushing the new site's config.
	pushed := a.Server.RefreshAgent(agent.ID)
	writeJSON(w, http.StatusOK, map[string]any{"pushed": pushed})
}

// handleRunTest triggers an immediate run of one of the agent's tests.
func (a *API) handleRunTest(w http.ResponseWriter, r *http.Request, user *store.User) {
	agent := a.getScopedAgent(w, r, user)
	if agent == nil {
		return
	}
	var req struct {
		TestID string `json:"testId"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.TestID == "" {
		writeError(w, http.StatusBadRequest, "testId is required")
		return
	}

	// The test must be assigned to the agent's site (that's what the agent
	// actually has scheduled and can run on demand).
	testIDs, err := a.Store.SiteTestIDs(agent.SiteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	assigned := false
	for _, id := range testIDs {
		if id == req.TestID {
			assigned = true
			break
		}
	}
	if !assigned {
		writeError(w, http.StatusBadRequest, "test is not assigned to this agent's site")
		return
	}

	if !a.Server.RunTest(agent.ID, req.TestID) {
		writeError(w, http.StatusConflict, "agent is offline")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"triggered": true})
}

func (a *API) handleDeleteAgent(w http.ResponseWriter, r *http.Request, user *store.User) {
	agent := a.getScopedAgent(w, r, user)
	if agent == nil {
		return
	}
	if err := a.Store.DeleteAgent(agent.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListResults(w http.ResponseWriter, r *http.Request, user *store.User) {
	q := r.URL.Query()
	tenantID, ok := tenantScope(user, q.Get("tenantId"))
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}

	limit, _ := strconv.Atoi(q.Get("limit"))
	var since time.Time
	if s := q.Get("since"); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be RFC3339")
			return
		}
		since = parsed
	}
	results, err := a.Store.ListResults(store.ResultFilter{
		TenantID: tenantID,
		SiteID:   q.Get("siteId"),
		AgentID:  q.Get("agentId"),
		TestID:   q.Get("testId"),
		TestType: q.Get("type"),
		Since:    since,
		Limit:    limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// handleOUILookup resolves MAC addresses to manufacturer names from the
// embedded IEEE registry. GET /api/v1/oui?macs=aa:bb:cc:dd:ee:ff,...
// Returns {"aa:bb:cc:dd:ee:ff": "Vendor", ...} with unknown MACs omitted.
func (a *API) handleOUILookup(w http.ResponseWriter, r *http.Request, _ *store.User) {
	out := map[string]string{}
	for _, mac := range strings.Split(r.URL.Query().Get("macs"), ",") {
		mac = strings.TrimSpace(mac)
		if mac == "" {
			continue
		}
		if v := oui.Lookup(mac); v != "" {
			out[mac] = v
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleWlanRoaming returns the aggregated roaming picture (summary tiles +
// classified event log) over a time window. GET /api/v1/wlan-roaming
// ?tenantId=&siteId=&agentId=&since= (RFC3339, default: last 7 days).
func (a *API) handleWlanRoaming(w http.ResponseWriter, r *http.Request, user *store.User) {
	q := r.URL.Query()
	tenantID, ok := tenantScope(user, q.Get("tenantId"))
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}

	since := time.Now().Add(-7 * 24 * time.Hour)
	if s := q.Get("since"); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be RFC3339")
			return
		}
		since = parsed
	}
	summary, err := a.Store.WlanRoaming(store.ResultFilter{
		TenantID: tenantID,
		SiteID:   q.Get("siteId"),
		AgentID:  q.Get("agentId"),
		Since:    since,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
