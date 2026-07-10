package api

import (
	"net/http"
	"strings"

	"github.com/fox27374/net-lama/internal/store"
)

// handleListAPIKeys returns the calling user's own API keys. Never
// includes the hash or the secret.
func (a *API) handleListAPIKeys(w http.ResponseWriter, r *http.Request, user *store.User) {
	keys, err := a.Store.ListAPIKeys(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// handleCreateAPIKey creates a new key for the calling user. The response
// is the only time the full secret is returned.
func (a *API) handleCreateAPIKey(w http.ResponseWriter, r *http.Request, user *store.User) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	key, err := a.Store.CreateAPIKey(user.ID, req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

// handleDeleteAPIKey revokes one of the calling user's own keys; keys
// belonging to another user 404 rather than 403, to avoid confirming
// their existence.
func (a *API) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request, user *store.User) {
	if err := a.Store.DeleteAPIKey(r.PathValue("id"), user.ID); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "api key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
