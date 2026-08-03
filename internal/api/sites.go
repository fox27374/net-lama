package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/fox27374/net-lama/internal/server"
	"github.com/fox27374/net-lama/internal/store"
)

// validatePerfmonSource checks a perfmon test's sourceAgentId refers to a
// real agent belonging to this tenant. ValidateTestDef can't do this
// itself — it has no store access — so it's a separate check here, run
// after ValidateTestDef has normalized params.
func validatePerfmonSource(st *store.Store, tenantID, testType string, params json.RawMessage) error {
	if testType != "perfmon" {
		return nil
	}
	var p struct {
		SourceAgentID string `json:"sourceAgentId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return err
	}
	agent, err := st.GetAgent(p.SourceAgentID)
	if err != nil || agent.TenantID != tenantID {
		return fmt.Errorf("perfmon source agent not found in this tenant")
	}
	return nil
}

// handleOverview returns the tenant dashboard: counts and per-test health.
// Optional siteId parameter filters to a specific site.
func (a *API) handleOverview(w http.ResponseWriter, r *http.Request, user *store.User) {
	tenantID, ok := tenantScope(user, r.URL.Query().Get("tenantId"))
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}

	siteID := r.URL.Query().Get("siteId")
	if siteID != "" {
		if _, ok := inTenant(w, tenantID, "site", siteID, a.Store.GetSite); !ok {
			return
		}
	}

	ov, err := a.Store.TenantOverview(tenantID, siteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Connected count comes from the in-memory stream registry.
	agents, err := a.Store.ListAgents(tenantID)
	if err == nil {
		for _, agent := range agents {
			if siteID != "" && agent.SiteID != siteID {
				continue
			}
			if a.Server.AgentConnected(agent.ID) {
				ov.AgentsConnected++
			}
		}
	}

	writeJSON(w, http.StatusOK, ov)
}

// --- Sites ---

func (a *API) handleListSites(w http.ResponseWriter, r *http.Request, user *store.User) {
	tenantID, ok := tenantScope(user, r.URL.Query().Get("tenantId"))
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}
	sites, err := a.Store.ListSites(tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sites)
}

func (a *API) handleCreateSite(w http.ResponseWriter, r *http.Request, user *store.User) {
	var req struct {
		Name     string `json:"name"`
		TenantID string `json:"tenantId"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	tenantID, ok := tenantScope(user, req.TenantID)
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "site name is required")
		return
	}

	site, err := a.Store.CreateSite(tenantID, req.Name)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, site)
}

func (a *API) handleDeleteSite(w http.ResponseWriter, r *http.Request, user *store.User) {
	site, ok := scoped(w, user, "site", r.PathValue("id"), a.Store.GetSite)
	if !ok {
		return
	}
	if site.Agents > 0 {
		writeError(w, http.StatusConflict, "site still has agents; delete or move them first")
		return
	}
	if err := a.Store.DeleteSite(site.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSetSiteTests replaces the tests assigned to a site and pushes
// the resulting config to all connected agents of the site.
func (a *API) handleSetSiteTests(w http.ResponseWriter, r *http.Request, user *store.User) {
	site, ok := scoped(w, user, "site", r.PathValue("id"), a.Store.GetSite)
	if !ok {
		return
	}

	var req struct {
		TestIDs []string `json:"testIds"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	// All tests must exist and belong to the site's tenant
	for _, testID := range req.TestIDs {
		if _, ok := inTenant(w, site.TenantID, "test", testID, a.Store.GetTest); !ok {
			return
		}
	}

	if err := a.Store.SetSiteTests(site.ID, req.TestIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pushed := a.pushSite(site.ID)
	writeJSON(w, http.StatusOK, map[string]any{"testIds": req.TestIDs, "pushed": pushed})
}

// pushSite pushes fresh configs to all connected agents of a site.
func (a *API) pushSite(siteID string) int {
	agentIDs, err := a.Store.AgentIDsForSite(siteID)
	if err != nil {
		return 0
	}
	return a.Server.PushConfigs(agentIDs)
}

// --- Test definitions ---

func (a *API) handleListTests(w http.ResponseWriter, r *http.Request, user *store.User) {
	tenantID, ok := tenantScope(user, r.URL.Query().Get("tenantId"))
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}
	tests, err := a.Store.ListTests(tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tests)
}

func (a *API) handleCreateTest(w http.ResponseWriter, r *http.Request, user *store.User) {
	var req store.TestDef
	if !decodeBody(w, r, &req) {
		return
	}
	tenantID, ok := tenantScope(user, req.TenantID)
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}
	req.TenantID = tenantID
	req.Name = strings.TrimSpace(req.Name)
	if err := server.ValidateTestDef(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePerfmonSource(a.Store, tenantID, req.Type, req.Params); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	test, err := a.Store.CreateTest(&req)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, test)
}

func (a *API) handleUpdateTest(w http.ResponseWriter, r *http.Request, user *store.User) {
	test, ok := scoped(w, user, "test", r.PathValue("id"), a.Store.GetTest)
	if !ok {
		return
	}

	var req store.TestDef
	if !decodeBody(w, r, &req) {
		return
	}
	// Type and tenant are immutable, name/interval/params/thresholds can change
	test.Name = strings.TrimSpace(req.Name)
	test.IntervalSeconds = req.IntervalSeconds
	test.Params = req.Params
	test.Thresholds = req.Thresholds
	if err := server.ValidateTestDef(test); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePerfmonSource(a.Store, test.TenantID, test.Type, test.Params); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.Store.UpdateTest(test); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Push updated config to all agents whose site uses this test
	pushed := 0
	if agentIDs, err := a.Store.AgentIDsForTest(test.ID); err == nil {
		pushed = a.Server.PushConfigs(agentIDs)
	}
	writeJSON(w, http.StatusOK, map[string]any{"test": test, "pushed": pushed})
}

func (a *API) handleDeleteTest(w http.ResponseWriter, r *http.Request, user *store.User) {
	test, ok := scoped(w, user, "test", r.PathValue("id"), a.Store.GetTest)
	if !ok {
		return
	}

	// Collect affected agents before the assignment rows are cascaded away
	agentIDs, _ := a.Store.AgentIDsForTest(test.ID)

	if err := a.Store.DeleteTest(test.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.Server.PushConfigs(agentIDs)
	w.WriteHeader(http.StatusNoContent)
}
