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

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 3600,
	})
	writeJSON(w, http.StatusOK, user)
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

func (a *API) handleMe(w http.ResponseWriter, r *http.Request, user *store.User) {
	writeJSON(w, http.StatusOK, struct {
		*store.User
		ServerVersion string `json:"serverVersion"`
	}{user, version.Version})
}
