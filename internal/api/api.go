package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fox27374/net-lama/internal/server"
	"github.com/fox27374/net-lama/internal/store"
)

const sessionCookie = "netlama_session"

type API struct {
	Store         *store.Store
	Server        *server.Server
	Logger        *slog.Logger
	SecureCookies bool
}

func New(st *store.Store, srv *server.Server, logger *slog.Logger, secureCookies bool) *API {
	return &API{Store: st, Server: srv, Logger: logger, SecureCookies: secureCookies}
}

// Register mounts all API routes on the mux.
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/login", a.handleLogin)
	mux.HandleFunc("POST /api/v1/logout", a.handleLogout)
	mux.HandleFunc("GET /api/v1/me", a.auth(a.handleMe))

	mux.HandleFunc("GET /api/v1/tenants", a.auth(a.admin(a.handleListTenants)))
	mux.HandleFunc("POST /api/v1/tenants", a.auth(a.admin(a.handleCreateTenant)))
	mux.HandleFunc("DELETE /api/v1/tenants/{id}", a.auth(a.admin(a.handleDeleteTenant)))

	mux.HandleFunc("GET /api/v1/users", a.auth(a.admin(a.handleListUsers)))
	mux.HandleFunc("POST /api/v1/users", a.auth(a.admin(a.handleCreateUser)))
	mux.HandleFunc("DELETE /api/v1/users/{id}", a.auth(a.admin(a.handleDeleteUser)))

	mux.HandleFunc("GET /api/v1/sites", a.auth(a.handleListSites))
	mux.HandleFunc("POST /api/v1/sites", a.auth(a.handleCreateSite))
	mux.HandleFunc("DELETE /api/v1/sites/{id}", a.auth(a.handleDeleteSite))
	mux.HandleFunc("PUT /api/v1/sites/{id}/tests", a.auth(a.handleSetSiteTests))

	mux.HandleFunc("GET /api/v1/tests", a.auth(a.handleListTests))
	mux.HandleFunc("POST /api/v1/tests", a.auth(a.handleCreateTest))
	mux.HandleFunc("PUT /api/v1/tests/{id}", a.auth(a.handleUpdateTest))
	mux.HandleFunc("DELETE /api/v1/tests/{id}", a.auth(a.handleDeleteTest))

	mux.HandleFunc("GET /api/v1/agents", a.auth(a.handleListAgents))
	mux.HandleFunc("POST /api/v1/agents", a.auth(a.handleCreateAgent))
	mux.HandleFunc("PUT /api/v1/agents/{id}", a.auth(a.handleUpdateAgent))
	mux.HandleFunc("POST /api/v1/agents/{id}/run", a.auth(a.handleRunTest))
	mux.HandleFunc("DELETE /api/v1/agents/{id}", a.auth(a.handleDeleteAgent))

	mux.HandleFunc("GET /api/v1/results", a.auth(a.handleListResults))
	mux.HandleFunc("GET /api/v1/oui", a.auth(a.handleOUILookup))
	mux.HandleFunc("GET /api/v1/wlan-roaming", a.auth(a.handleWlanRoaming))
	mux.HandleFunc("GET /api/v1/overview", a.auth(a.handleOverview))
	mux.HandleFunc("GET /api/v1/logs", a.auth(a.handleListLogs))

	mux.HandleFunc("GET /api/v1/alert-rules", a.auth(a.handleListAlertRules))
	mux.HandleFunc("POST /api/v1/alert-rules", a.auth(a.handleCreateAlertRule))
	mux.HandleFunc("PUT /api/v1/alert-rules/{id}", a.auth(a.handleUpdateAlertRule))
	mux.HandleFunc("DELETE /api/v1/alert-rules/{id}", a.auth(a.handleDeleteAlertRule))

	mux.HandleFunc("GET /api/v1/alert-targets", a.auth(a.handleListAlertTargets))
	mux.HandleFunc("POST /api/v1/alert-targets", a.auth(a.handleCreateAlertTarget))
	mux.HandleFunc("PUT /api/v1/alert-targets/{id}", a.auth(a.handleUpdateAlertTarget))
	mux.HandleFunc("DELETE /api/v1/alert-targets/{id}", a.auth(a.handleDeleteAlertTarget))

	mux.HandleFunc("GET /api/v1/alerts", a.auth(a.handleListAlerts))

	mux.HandleFunc("GET /api/v1/apikeys", a.auth(a.handleListAPIKeys))
	mux.HandleFunc("POST /api/v1/apikeys", a.auth(a.handleCreateAPIKey))
	mux.HandleFunc("DELETE /api/v1/apikeys/{id}", a.auth(a.handleDeleteAPIKey))
}

// --- middleware ---

type authedHandler func(w http.ResponseWriter, r *http.Request, user *store.User)

// auth accepts either an "Authorization: Bearer nlk_..." API key (for
// scripted/programmatic use) or the session cookie set by /api/v1/login.
// A presented bearer token is never logged, even on failure.
func (a *API) auth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if hdr := r.Header.Get("Authorization"); hdr != "" {
			secret, ok := strings.CutPrefix(hdr, "Bearer ")
			if !ok || secret == "" {
				writeError(w, http.StatusUnauthorized, "malformed Authorization header")
				return
			}
			user, err := a.Store.APIKeyUser(secret)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid API key")
				return
			}
			next(w, r, user)
			return
		}

		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		user, err := a.Store.SessionUser(cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "session expired")
			return
		}
		next(w, r, user)
	}
}

func (a *API) admin(next authedHandler) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, user *store.User) {
		if !user.IsAdmin {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next(w, r, user)
	}
}

// canAccessAgent checks tenant scoping for a specific agent.
func canAccessAgent(user *store.User, agent *store.Agent) bool {
	return user.IsAdmin || user.TenantID == agent.TenantID
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}
