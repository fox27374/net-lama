package api

import (
	"net/http"

	"github.com/fox27374/net-lama/internal/store"
	"github.com/fox27374/net-lama/internal/version"
)

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	user, err := a.Store.Authenticate(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	token, err := a.Store.CreateSession(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "creating session failed")
		return
	}

	a.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, user)
}

func (a *API) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 3600,
	})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		a.Store.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleSetPassword serves both self-service change (own ID, current password
// required) and admin reset (any other ID, admin only).
func (a *API) handleSetPassword(w http.ResponseWriter, r *http.Request, user *store.User) {
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		Password        string `json:"password"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	id := r.PathValue("id")
	if id == user.ID {
		if _, err := a.Store.Authenticate(user.Username, req.CurrentPassword); err != nil {
			writeError(w, http.StatusUnauthorized, "current password is incorrect")
			return
		}
	} else if !user.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	if err := a.Store.SetPassword(id, req.Password); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Setting the password killed every session of that user, including the
	// caller's own when they changed their own password.
	if id == user.ID {
		token, err := a.Store.CreateSession(user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "creating session failed")
			return
		}
		a.setSessionCookie(w, token)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request, user *store.User) {
	writeJSON(w, http.StatusOK, struct {
		*store.User
		ServerVersion string `json:"serverVersion"`
	}{user, version.Version})
}
