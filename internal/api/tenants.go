package api

import (
	"net/http"
	"strings"

	"github.com/fox27374/net-lama/internal/store"
)

func (a *API) handleListTenants(w http.ResponseWriter, r *http.Request, user *store.User) {
	tenants, err := a.Store.ListTenants()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tenants)
}

func (a *API) handleCreateTenant(w http.ResponseWriter, r *http.Request, user *store.User) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "tenant name is required")
		return
	}

	tenant, err := a.Store.CreateTenant(req.Name)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, tenant)
}

func (a *API) handleDeleteTenant(w http.ResponseWriter, r *http.Request, user *store.User) {
	if err := a.Store.DeleteTenant(r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListUsers(w http.ResponseWriter, r *http.Request, user *store.User) {
	users, err := a.Store.ListUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (a *API) handleCreateUser(w http.ResponseWriter, r *http.Request, user *store.User) {
	var req struct {
		TenantID string `json:"tenantId"`
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"isAdmin"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if !req.IsAdmin && req.TenantID == "" {
		writeError(w, http.StatusBadRequest, "non-admin users need a tenantId")
		return
	}

	created, err := a.Store.CreateUser(req.TenantID, req.Username, req.Password, req.IsAdmin)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) handleDeleteUser(w http.ResponseWriter, r *http.Request, user *store.User) {
	id := r.PathValue("id")
	if id == user.ID {
		writeError(w, http.StatusBadRequest, "cannot delete your own user")
		return
	}
	if err := a.Store.DeleteUser(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
