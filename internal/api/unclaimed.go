package api

import (
	"net/http"
	"strings"

	"github.com/fox27374/net-lama/internal/store"
)

// handleListUnclaimedAgents lists pending enrollments, same tenant-scoping
// shape as handleListAgents: admins may pass ?tenantId= or omit it for all
// tenants, other users are pinned to their own.
func (a *API) handleListUnclaimedAgents(w http.ResponseWriter, r *http.Request, user *store.User) {
	tenantID := user.TenantID
	if user.IsAdmin {
		tenantID = r.URL.Query().Get("tenantId")
	}

	agents, err := a.Store.ListUnclaimedAgents(tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

// getScopedUnclaimedAgent loads a pending enrollment and enforces tenant access.
func (a *API) getScopedUnclaimedAgent(w http.ResponseWriter, r *http.Request, user *store.User) *store.UnclaimedAgent {
	agent, err := a.Store.GetUnclaimedAgent(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "unclaimed agent not found")
		return nil
	}
	if !user.IsAdmin && user.TenantID != agent.TenantID {
		writeError(w, http.StatusForbidden, "no access to this agent")
		return nil
	}
	return agent
}

// handleClaimAgent turns a pending enrollment into a real agent: creates
// it exactly like handleCreateAgent (fresh random token), carries over the
// capabilities/interfaces/version already reported while unclaimed, and
// removes the pending row. The response has the same shape as
// handleCreateAgent's, so the frontend's "copy token, start the agent"
// dialog is reused unchanged for both flows.
func (a *API) handleClaimAgent(w http.ResponseWriter, r *http.Request, user *store.User) {
	unclaimed := a.getScopedUnclaimedAgent(w, r, user)
	if unclaimed == nil {
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
	if err != nil || site.TenantID != unclaimed.TenantID {
		writeError(w, http.StatusBadRequest, "unknown site")
		return
	}

	agent, err := a.Store.CreateAgent(unclaimed.TenantID, site.ID, req.Name)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	agent.SiteName = site.Name

	// Carry over what was already reported while pending, so the agent
	// doesn't show blank capabilities/interfaces until its next reconnect.
	if len(unclaimed.Capabilities) > 0 {
		if err := a.Store.SetAgentCapabilities(agent.ID, unclaimed.Capabilities); err == nil {
			agent.Capabilities = unclaimed.Capabilities
		}
	}
	if len(unclaimed.NetworkInterfaces) > 0 {
		if err := a.Store.SetAgentNetworkInterfaces(agent.ID, unclaimed.NetworkInterfaces); err == nil {
			agent.NetworkInterfaces = unclaimed.NetworkInterfaces
		}
	}
	if unclaimed.Version != "" {
		if err := a.Store.SetAgentVersion(agent.ID, unclaimed.Version); err == nil {
			agent.Version = unclaimed.Version
		}
	}

	if err := a.Store.DeleteUnclaimedAgent(unclaimed.ID); err != nil {
		a.Logger.Warn("Deleting claimed unclaimed-agent row failed", "error", err)
	}

	writeJSON(w, http.StatusCreated, newAgentView(agent, false, nil))
}

// handleDismissUnclaimedAgent removes a pending enrollment without
// claiming it. If the device is still retrying, it simply reappears on its
// next reconnect attempt — this isn't a hard block, just "not now".
func (a *API) handleDismissUnclaimedAgent(w http.ResponseWriter, r *http.Request, user *store.User) {
	unclaimed := a.getScopedUnclaimedAgent(w, r, user)
	if unclaimed == nil {
		return
	}
	if err := a.Store.DeleteUnclaimedAgent(unclaimed.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// handleGetEnrollToken returns the calling tenant's current enrollment
// token ("" if never generated or revoked).
func (a *API) handleGetEnrollToken(w http.ResponseWriter, r *http.Request, user *store.User) {
	tenantID, ok := tenantScope(user, r.URL.Query().Get("tenantId"))
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}
	token, err := a.Store.GetTenantEnrollToken(tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// handleGenerateEnrollToken generates (or regenerates) the calling
// tenant's enrollment token. Regenerating invalidates the old one for any
// device that hasn't been claimed yet.
func (a *API) handleGenerateEnrollToken(w http.ResponseWriter, r *http.Request, user *store.User) {
	var req struct {
		TenantID string `json:"tenantId"`
	}
	if r.ContentLength != 0 {
		if !decodeBody(w, r, &req) {
			return
		}
	}
	tenantID, ok := tenantScope(user, req.TenantID)
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}
	token, err := a.Store.SetTenantEnrollToken(tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// handleRevokeEnrollToken clears the calling tenant's enrollment token,
// disabling further self-enrollment until a new one is generated.
func (a *API) handleRevokeEnrollToken(w http.ResponseWriter, r *http.Request, user *store.User) {
	tenantID, ok := tenantScope(user, r.URL.Query().Get("tenantId"))
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}
	if err := a.Store.RevokeTenantEnrollToken(tenantID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}
