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

	key := throttleKey(req.Username, r)
	if !a.authThrottle.allow(key) {
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}

	user, err := a.Store.Authenticate(req.Username, req.Password)
	if err != nil {
		a.logAuthFailure("Login failed", req.Username, r, key)
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	a.authThrottle.success(key)

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

// logAuthFailure records a rejected credential and notes the moment a caller
// runs out of attempts. The attempted password is never logged.
func (a *API) logAuthFailure(msg, username string, r *http.Request, key string) {
	a.Logger.Warn(msg, "username", username, "ip", clientIP(r))
	if a.authThrottle.fail(key) {
		a.Logger.Warn("Auth throttled", "username", username, "ip", clientIP(r),
			"note", "too many failed attempts within the window")
	}
}

// handleSetPassword serves two flows that differ in more than the credential
// they demand:
//
//   - self-service change (own ID): current password required, sessions
//     dropped but a fresh cookie issued, API keys left alone.
//   - admin reset (any other ID, admin only): no current password, a password
//     is generated unless one is supplied, and the target's API keys are
//     revoked as well — otherwise resetting a compromised account would leave
//     every key of that account working.
func (a *API) handleSetPassword(w http.ResponseWriter, r *http.Request, user *store.User) {
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		Password        string `json:"password"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	id := r.PathValue("id")
	self := id == user.ID
	if !self && !user.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	password := req.Password
	if password == "" && !self {
		password = store.NewPassword()
	}
	if len(password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	if self {
		key := throttleKey(user.Username, r)
		if !a.authThrottle.allow(key) {
			writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
			return
		}
		if _, err := a.Store.Authenticate(user.Username, req.CurrentPassword); err != nil {
			a.logAuthFailure("Password change rejected", user.Username, r, key)
			writeError(w, http.StatusUnauthorized, "current password is incorrect")
			return
		}
		a.authThrottle.success(key)
	}

	if err := a.Store.SetPassword(id, password); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Setting the password killed every session of that user, including the
	// caller's own when they changed their own password.
	if self {
		token, err := a.Store.CreateSession(user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "creating session failed")
			return
		}
		a.setSessionCookie(w, token)
		a.Logger.Info("Password changed", "actor", user.Username, "target", user.Username, "path", "self")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	revoked, err := a.Store.DeleteAPIKeysForUser(id)
	if err != nil {
		a.Logger.Warn("Revoking API keys after a password reset failed", "user", id, "error", err)
	}
	a.Logger.Info("Password changed", "actor", user.Username, "target", id, "path", "admin",
		"apiKeysRevoked", revoked)
	writeJSON(w, http.StatusOK, map[string]any{"password": password, "apiKeysRevoked": revoked})
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request, user *store.User) {
	writeJSON(w, http.StatusOK, struct {
		*store.User
		ServerVersion string `json:"serverVersion"`
	}{user, version.Version})
}
