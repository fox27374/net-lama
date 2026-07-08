package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/fox27374/net-lama/internal/store"
)

var validMetrics = map[string]bool{
	"unhealthy": true, "latency_ms": true, "loss_percent": true,
	"download_mbps": true, "upload_mbps": true,
}
var validOperators = map[string]bool{">": true, ">=": true, "<": true, "<=": true, "==": true}

func (a *API) handleListAlertRules(w http.ResponseWriter, r *http.Request, user *store.User) {
	tenantID, ok := tenantScope(user, r.URL.Query().Get("tenantId"))
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}
	rules, err := a.Store.ListAlertRules(tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (a *API) handleCreateAlertRule(w http.ResponseWriter, r *http.Request, user *store.User) {
	var req store.AlertRule
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
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "rule name is required")
		return
	}

	// The test must exist and belong to the tenant.
	test, err := a.Store.GetTest(req.TestID)
	if err != nil || test.TenantID != tenantID {
		writeError(w, http.StatusBadRequest, "unknown test")
		return
	}
	if !validMetrics[req.Metric] {
		writeError(w, http.StatusBadRequest, "invalid metric")
		return
	}
	if req.Metric != "unhealthy" {
		if !validOperators[req.Operator] {
			writeError(w, http.StatusBadRequest, "invalid operator")
			return
		}
	}
	if req.ForCount < 1 {
		req.ForCount = 1
	}
	req.WebhookURL = strings.TrimSpace(req.WebhookURL)

	rule, err := a.Store.CreateAlertRule(&req)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (a *API) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request, user *store.User) {
	rule, err := a.Store.GetAlertRule(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	if !user.IsAdmin && user.TenantID != rule.TenantID {
		writeError(w, http.StatusForbidden, "no access to this rule")
		return
	}
	if err := a.Store.DeleteAlertRule(rule.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListAlerts(w http.ResponseWriter, r *http.Request, user *store.User) {
	tenantID, ok := tenantScope(user, r.URL.Query().Get("tenantId"))
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	alerts, err := a.Store.ListAlerts(tenantID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}
