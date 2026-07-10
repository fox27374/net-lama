# Plan: API keys + full API documentation

Implements the ROADMAP item "Everything controllable via API (audit the
UI-only flows; document the API)" plus API-key authentication.

## Audit result (already done — for context)

Every UI flow already goes through `/api/v1` (all `fetch` targets in
`internal/web/static/app.js` are routes registered in `internal/api/api.go`),
so GUI/API parity exists. What is missing: (a) a non-cookie auth mechanism for
programmatic use, (b) key management, (c) documentation. That is this task.

## Design

### 1. Storage (`internal/store/apikeys.go` + schema in `store.go`)

New table `api_keys`: `id`, `user_id` (FK → users), `name` (user-chosen
label), `key_hash` (SHA-256 hex of the secret — the secret itself is NEVER
stored), `prefix` (first 12 chars of the secret, for display), `created_at`,
`last_used_at` (nullable). Timestamps follow the existing table conventions.

- `CreateAPIKey(userID, name)` — generate the secret as `nlk_` + 32 bytes
  from `crypto/rand` (hex or base32, no ambiguous chars), store hash+prefix,
  return the full secret **once**.
- `ListAPIKeys(userID)` — never returns hash or secret; returns prefix, name,
  createdAt, lastUsedAt.
- `DeleteAPIKey(id, userID)` — scoped to the owning user (a user can only
  revoke their own keys).
- `APIKeyUser(secret)` — hash the presented secret, look up by `key_hash`,
  return the owning `*store.User`; update `last_used_at` on hit.
- Deleting a user must delete their keys (extend the existing user-delete
  path the same way other dependent rows are handled).

No expiry in this phase — note "key expiry / scopes" as a possible later
extension in ROADMAP, do not build it.

### 2. Auth middleware (`internal/api/api.go`)

Extend `auth()` to accept, in order:

1. `Authorization: Bearer nlk_...` header → `APIKeyUser` lookup;
2. otherwise the existing session cookie, unchanged.

Same `authedHandler` signature, so every existing endpoint (tenants, users,
sites, tests, agents, run-now, results, overview, logs, alert-rules, alerts,
me) works with API keys with zero per-handler changes — this is what makes
"everything the GUI can do, the API can do" true for scripts. A key carries
exactly the owning user's privileges (admin or tenant-scoped). 401 on unknown
or malformed bearer tokens; never log the presented secret.

### 3. Key-management endpoints (`internal/api/apikeys.go`)

- `GET /api/v1/apikeys` — list the calling user's keys.
- `POST /api/v1/apikeys` `{"name": "ci"}` — create; the response is the only
  time the full key is returned. Reject empty name.
- `DELETE /api/v1/apikeys/{id}` — revoke own key (404 for someone else's).

All three work with either auth method, so the scripted flow is:
`POST /api/v1/login` (username/password → cookie) → `POST /api/v1/apikeys`
(with cookie) → use the returned key as Bearer from then on.

### 4. Web UI (`internal/web/static/`)

New "API Keys" nav page (visible to every logged-in user, modeled on the
existing table pages): list (name, prefix…, created, last used), a
create form (name field), and after creation show the full key once in a
highlighted, selectable block with a "copy now — it won't be shown again"
note, plus a revoke button per key.

### 5. Documentation

- **`doc/API.md`** (new) — the complete API reference. Write it FROM THE
  HANDLER CODE, not from memory; every route registered in
  `internal/api/api.go` must appear. Structure: an Authentication section
  (session-cookie flow, API-key flow, curl examples for both, including the
  login→create-key bootstrap), then per resource: method, path, query
  params, request body fields, response shape (actual JSON field names from
  the handlers/store structs), admin-vs-tenant-user scoping rules, and the
  error format. Include the `?tenantId=` admin-scoping convention once,
  prominently.
- README: extend the API paragraph — mention API keys and link doc/API.md.
- ROADMAP: check off the "Everything controllable via API" item (mirror the
  wording style of other checked items; mention the audit conclusion and
  API keys); add "API key expiry/scopes" as a new unchecked line.
- PROGRESS.md: dated entry under 2026-07-09.
- No new env vars expected, so compose files should not need changes.

## Verification (required)

1. `make build`, `go vet ./...`, `go test ./...`. Add a unit test for the
   store (create → lookup by secret → revoke → lookup fails).
2. E2E per CLAUDE.md (self-signed TLS server on scratch ports):
   - login as admin (cookie) → create an API key;
   - using ONLY `Authorization: Bearer <key>` (no cookie): walk the full
     surface — create tenant, tenant user, site, test, agent (token comes
     back), assign test to site, GET results/overview/logs/alerts/me;
   - create a key as a tenant user and confirm it is tenant-scoped (no
     tenants list, no server logs);
   - revoke a key → the next Bearer request returns 401; a garbage key
     returns 401;
   - GET /api/v1/apikeys never contains the full secret.
3. Doc completeness check: extract all registered routes from
   `internal/api/api.go` (grep) and assert every one appears in doc/API.md.

## Constraints

- No new third-party dependencies (crypto/rand + crypto/sha256 are stdlib).
- Never store, log, or list the full key after the create response.
- Do not commit; leave changes in the working tree for review.
