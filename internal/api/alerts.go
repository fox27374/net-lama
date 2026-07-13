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
var validTargetTypes = map[string]bool{
	"webhook": true, "email": true, "script": true, "snmp": true,
}

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
	if req.ClearCount < 1 {
		req.ClearCount = 1
	}
	req.WebhookURL = strings.TrimSpace(req.WebhookURL)

	// Validate targetIds exist in the same tenant
	if len(req.TargetIds) > 0 {
		for _, tid := range req.TargetIds {
			target, err := a.Store.GetAlertTarget(tid)
			if err != nil || target.TenantID != tenantID {
				writeError(w, http.StatusBadRequest, "unknown or unauthorized alert target: "+tid)
				return
			}
		}
	}

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

func (a *API) handleUpdateAlertRule(w http.ResponseWriter, r *http.Request, user *store.User) {
	rule, err := a.Store.GetAlertRule(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	if !user.IsAdmin && user.TenantID != rule.TenantID {
		writeError(w, http.StatusForbidden, "no access to this rule")
		return
	}

	var req store.AlertRule
	if !decodeBody(w, r, &req) {
		return
	}

	// Preserve immutable fields
	rule.Name = strings.TrimSpace(req.Name)
	if rule.Name == "" {
		writeError(w, http.StatusBadRequest, "rule name is required")
		return
	}

	if !validMetrics[req.Metric] {
		writeError(w, http.StatusBadRequest, "invalid metric")
		return
	}
	rule.Metric = req.Metric

	if rule.Metric != "unhealthy" {
		if !validOperators[req.Operator] {
			writeError(w, http.StatusBadRequest, "invalid operator")
			return
		}
	}
	rule.Operator = req.Operator
	rule.Threshold = req.Threshold

	if req.ForCount < 1 {
		req.ForCount = 1
	}
	rule.ForCount = req.ForCount

	if req.ClearCount < 1 {
		req.ClearCount = 1
	}
	rule.ClearCount = req.ClearCount
	rule.ClearThreshold = req.ClearThreshold

	// Validate targetIds exist in the same tenant
	if len(req.TargetIds) > 0 {
		for _, tid := range req.TargetIds {
			target, err := a.Store.GetAlertTarget(tid)
			if err != nil || target.TenantID != rule.TenantID {
				writeError(w, http.StatusBadRequest, "unknown or unauthorized alert target: "+tid)
				return
			}
		}
	}
	rule.TargetIds = req.TargetIds

	rule.WebhookURL = strings.TrimSpace(req.WebhookURL)

	updated, err := a.Store.UpdateAlertRule(rule)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// --- Alert Targets ---

func (a *API) handleListAlertTargets(w http.ResponseWriter, r *http.Request, user *store.User) {
	tenantID, ok := tenantScope(user, r.URL.Query().Get("tenantId"))
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}
	targets, err := a.Store.ListAlertTargets(tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

func (a *API) handleCreateAlertTarget(w http.ResponseWriter, r *http.Request, user *store.User) {
	var req store.AlertTarget
	if !decodeBody(w, r, &req) {
		return
	}
	tenantID, ok := tenantScope(user, req.TenantID)
	if !ok {
		writeError(w, http.StatusBadRequest, "tenantId is required")
		return
	}
	req.TenantID = tenantID

	// Script targets are admin-only
	if req.Type == "script" && !user.IsAdmin {
		writeError(w, http.StatusForbidden, "script targets require admin access")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "target name is required")
		return
	}

	if !validTargetTypes[req.Type] {
		writeError(w, http.StatusBadRequest, "invalid target type")
		return
	}

	// Validate config has required fields
	if req.Config == nil {
		req.Config = make(map[string]any)
	}
	if !validateTargetConfig(req.Type, req.Config) {
		writeError(w, http.StatusBadRequest, "invalid config for target type "+req.Type)
		return
	}

	target, err := a.Store.CreateAlertTarget(&req)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, target)
}

func (a *API) handleUpdateAlertTarget(w http.ResponseWriter, r *http.Request, user *store.User) {
	target, err := a.Store.GetAlertTarget(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	if !user.IsAdmin && user.TenantID != target.TenantID {
		writeError(w, http.StatusForbidden, "no access to this target")
		return
	}

	// Script targets are admin-only
	if target.Type == "script" && !user.IsAdmin {
		writeError(w, http.StatusForbidden, "script targets require admin access")
		return
	}

	var req store.AlertTarget
	if !decodeBody(w, r, &req) {
		return
	}

	target.Name = strings.TrimSpace(req.Name)
	if target.Name == "" {
		writeError(w, http.StatusBadRequest, "target name is required")
		return
	}

	if !validTargetTypes[req.Type] {
		writeError(w, http.StatusBadRequest, "invalid target type")
		return
	}
	target.Type = req.Type

	if req.Config == nil {
		req.Config = make(map[string]any)
	}
	if !validateTargetConfig(target.Type, req.Config) {
		writeError(w, http.StatusBadRequest, "invalid config for target type "+target.Type)
		return
	}
	target.Config = req.Config

	updated, err := a.Store.UpdateAlertTarget(target)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) handleDeleteAlertTarget(w http.ResponseWriter, r *http.Request, user *store.User) {
	target, err := a.Store.GetAlertTarget(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	if !user.IsAdmin && user.TenantID != target.TenantID {
		writeError(w, http.StatusForbidden, "no access to this target")
		return
	}
	if err := a.Store.DeleteAlertTarget(target.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateTargetConfig checks that required fields are present in the config.
func validateTargetConfig(targetType string, config map[string]any) bool {
	switch targetType {
	case "webhook":
		url, ok := config["url"].(string)
		return ok && url != ""
	case "email":
		toList, ok := config["to"].([]interface{})
		if !ok || len(toList) == 0 {
			return false
		}
		for _, t := range toList {
			if _, ok := t.(string); !ok {
				return false
			}
		}
		return true
	case "script":
		path, ok := config["path"].(string)
		return ok && path != ""
	case "snmp":
		host, ok := config["host"].(string)
		return ok && host != ""
	}
	return false
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
