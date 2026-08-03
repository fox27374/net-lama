package api

import (
	"net/http"

	"github.com/fox27374/net-lama/internal/store"
)

// This file is the single place tenant scoping is decided. Handlers must
// not compare TenantID themselves: fetch by ID through scoped(), and pick
// the tenant to list by through tenantScope/tenantFilter.

// tenantScope resolves the tenant a request operates on: admins may pick
// any tenant via ?tenantId= (or body field), users are fixed to their own.
// Used where exactly one tenant must be named — creating a row, or reading
// a tenant-wide view.
func tenantScope(user *store.User, requested string) (string, bool) {
	if user.IsAdmin {
		return requested, requested != ""
	}
	if requested != "" && requested != user.TenantID {
		return "", false
	}
	return user.TenantID, true
}

// tenantFilter is tenantScope's listing variant: an admin who names no
// tenant gets "" — every tenant — rather than an error. Non-admins are
// pinned to their own tenant exactly as in tenantScope.
func tenantFilter(user *store.User, requested string) (string, bool) {
	if user.IsAdmin {
		return requested, true
	}
	return tenantScope(user, requested)
}

// mayAccess reports whether user is allowed to touch rows of tenantID.
func mayAccess(user *store.User, tenantID string) bool {
	return user.IsAdmin || user.TenantID == tenantID
}

// scoped loads a row by ID and hides it from users of another tenant.
//
// Out-of-tenant is reported as 404, never 403: a 403 confirms the ID
// exists, which lets one tenant probe for another's agents and tests.
// Same rule as handleDeleteAPIKey. On failure it has already written the
// response, so callers just return.
func scoped[T store.Tenanted](w http.ResponseWriter, user *store.User, what, id string, get func(string) (T, error)) (T, bool) {
	row, err := get(id)
	if err != nil || !mayAccess(user, row.TenantOf()) {
		var zero T
		writeError(w, http.StatusNotFound, what+" not found")
		return zero, false
	}
	return row, true
}

// inTenant loads a row that must belong to an already-resolved tenant —
// a siteId filter, or a testId/targetId referenced from a request body.
// Unlike scoped(), the tenant here is not "the caller's" but the one the
// request already settled on (which for an admin may be any tenant), so a
// mismatch is bad input rather than a missing resource: 400, not 404.
func inTenant[T store.Tenanted](w http.ResponseWriter, tenantID, what, id string, get func(string) (T, error)) (T, bool) {
	row, err := get(id)
	if err != nil || row.TenantOf() != tenantID {
		var zero T
		writeError(w, http.StatusBadRequest, "unknown "+what+": "+id)
		return zero, false
	}
	return row, true
}
