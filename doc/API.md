# Net-Lama API Reference

Everything the web UI does goes through this JSON API under `/api/v1` — every
`fetch` in `internal/web/static/app.js` targets one of the routes documented
here, so anything scriptable in the UI is scriptable against the API
directly. This document is written from the handler code
(`internal/api/*.go`) and the store types it returns
(`internal/store/*.go`), not from memory; every route registered in
`internal/api/api.go` appears below.

## Authentication

Two independent auth methods are accepted by every endpoint below (except
`POST /api/v1/login`, `POST /api/v1/logout`, which don't require auth):

1. **Session cookie** — what the web UI uses. `POST /api/v1/login` sets an
   `HttpOnly` cookie (`netlama_session`, `Secure` when TLS is on) valid for 7
   days; send it on subsequent requests (a browser does this automatically;
   with curl use a cookie jar).
2. **API key (Bearer token)** — for scripts/CI. Send
   `Authorization: Bearer nlk_<secret>`. A key carries exactly the owning
   user's privileges (global admin, or scoped to that user's tenant) — there
   is no separate permission model for keys.

If an `Authorization` header is present it takes priority over any cookie.
A missing/expired cookie and a missing/invalid/malformed bearer token both
return `401`. The presented secret is never logged by the server.

Keys are managed through the API itself (see [API Keys](#api-keys)) — there
is no separate provisioning step. The bootstrap flow for a new script is:

```sh
# 1. Log in, capturing the session cookie
curl -sk -c cookies.txt -X POST https://server:9090/api/v1/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"..."}'

# 2. Create an API key using that cookie (the response is the ONLY time
#    the full secret is returned — store it now, it cannot be recovered)
curl -sk -b cookies.txt -X POST https://server:9090/api/v1/apikeys \
  -H 'Content-Type: application/json' -d '{"name":"ci"}'
# => {"id":"...","name":"ci","prefix":"nlk_2a8fd623","createdAt":"...",
#     "secret":"nlk_2a8fd623342045c18f2725f0c7af01d157c590055342718ae9acdc4b094a0fc0"}

# 3. From then on, authenticate with the Bearer token — no more cookie needed
curl -sk -H 'Authorization: Bearer nlk_2a8fd623...' https://server:9090/api/v1/me
```

### Admin tenant scoping (`?tenantId=`)

Most resources belong to a tenant. Non-admin (tenant) users are always
implicitly scoped to their own tenant — they never need, and cannot
override, a `tenantId`. Admin users (`isAdmin: true`, `tenantId: ""`) operate
across all tenants and must say which tenant they mean, via a `?tenantId=`
query parameter (`sites`, `tests`, `results`, `overview`, `alert-rules`,
`alerts`) or a `tenantId` body field (`POST /sites`, `POST /tests`,
`POST /alert-rules`). Omitting it as an admin returns
`400 {"error":"tenantId is required"}`. `GET /api/v1/logs` is the one
exception: `tenantId` there is optional even for admins (empty = all
tenants), since server logs (which admins can also request) have no tenant.
`GET /api/v1/agents`, `GET /api/v1/tenants`, `GET /api/v1/users` behave the
same way, except `tenants`/`users` are admin-only outright.

### Errors

Every non-2xx response has the same shape:

```json
{"error": "human-readable message"}
```

Common status codes: `400` invalid input, `401` not authenticated,
`403` authenticated but not permitted (e.g. non-admin hitting an admin
route, or a tenant user asking for another tenant's data), `404` not found
(or, for `DELETE /api/v1/apikeys/{id}`, not *yours*), `409` conflict
(duplicate name), `500` internal error. Successful writes return `200`/`201`
with the created/updated resource, or `204` with an empty body for deletes.

---

## Auth

### `POST /api/v1/login`

No auth required. Body: `{"username": "...", "password": "..."}`. On
success sets the session cookie and returns the `User` (see
[Users](#users)). `401` on bad credentials.

### `POST /api/v1/logout`

Clears the session cookie (and deletes the server-side session, if any).
`204` no body.

### `GET /api/v1/me`

Returns the authenticated `User`. Works with either auth method — the
simplest way to check who a given API key belongs to.

---

## API Keys

Every logged-in user (admin or tenant) manages their own keys; there is no
endpoint to see or manage another user's keys. Deleting a user deletes their
keys too.

### `GET /api/v1/apikeys`

Lists the calling user's keys, newest first. Never includes the hash or
secret:

```json
[{
  "id": "09f6267b...",
  "userId": "e361dfaa...",
  "name": "ci",
  "prefix": "nlk_2a8fd623",
  "createdAt": "2026-07-09T20:26:45.61Z",
  "lastUsedAt": "2026-07-09T20:26:45.92Z"
}]
```

`lastUsedAt` is omitted (not present in the JSON) until the key has been
used at least once.

### `POST /api/v1/apikeys`

Body: `{"name": "ci"}` (required, non-empty). Creates a new key for the
calling user and returns it **with the full secret** — this is the only
response that ever contains it:

```json
{
  "id": "09f6267b...", "userId": "e361dfaa...", "name": "ci",
  "prefix": "nlk_2a8fd623",
  "createdAt": "2026-07-09T20:26:45.61Z",
  "secret": "nlk_2a8fd623342045c18f2725f0c7af01d157c590055342718ae9acdc4b094a0fc0"
}
```

The secret is `nlk_` followed by 64 hex characters (32 random bytes);
only its SHA-256 hash is ever stored server-side.

### `DELETE /api/v1/apikeys/{id}`

Revokes one of the calling user's own keys immediately — the next request
using it returns `401`. `404` if `{id}` doesn't exist or belongs to another
user (never `403`, so a key ID never confirms another user's key exists).

---

## Tenants

Admin only (`403` for tenant users).

### `GET /api/v1/tenants`

Lists all tenants: `[{"id": "...", "name": "..."}]`.

### `POST /api/v1/tenants`

Body: `{"name": "..."}` (required). `409` if the name is already taken.
Returns the created `Tenant`.

### `DELETE /api/v1/tenants/{id}`

Deletes a tenant and (via `ON DELETE CASCADE`) everything under it: users,
sites, tests, agents, results, alert rules/alerts, and those users' API
keys. `204`.

---

## Users

Admin only (`403` for tenant users).

### `GET /api/v1/users`

Lists all users across all tenants:

```json
[{"id": "...", "tenantId": "...", "username": "...", "isAdmin": false}]
```

`tenantId` is `""` for global admins.

### `POST /api/v1/users`

Body: `{"username": "...", "password": "...", "isAdmin": false, "tenantId": "..."}`.
`username`/`password` required, password must be ≥ 8 characters,
`tenantId` required unless `isAdmin` is `true`. `409` on duplicate
username. Returns the created `User`.

### `DELETE /api/v1/users/{id}`

Deletes a user (and, via cascade, their sessions and API keys). `400` if
you try to delete yourself. `204`.

---

## Sites

Sites group agents within a tenant and are what tests get assigned to.

### `GET /api/v1/sites`

Query: `tenantId` (required for admins, ignored/forced for tenant users).
Returns:

```json
[{"id": "...", "tenantId": "...", "name": "...", "testIds": ["..."], "agents": 2}]
```

### `POST /api/v1/sites`

Body: `{"name": "...", "tenantId": "..."}` (`tenantId` required for admins,
optional/ignored for tenant users — theirs is used). `409` on duplicate
name within the tenant. Returns the created `Site`.

### `DELETE /api/v1/sites/{id}`

`409` if the site still has agents (move or delete them first). `403` if
the site isn't yours (tenant users). `204`.

### `PUT /api/v1/sites/{id}/tests`

Replaces the full set of tests assigned to a site and immediately pushes
the new config to every connected agent of that site. Body:
`{"testIds": ["..."]}` — every ID must be an existing test belonging to the
site's tenant (`400` otherwise). Response:
`{"testIds": [...], "pushed": <n agents pushed>}`.

---

## Tests

Named, reusable test definitions within a tenant; a test only actually runs
once assigned to a site (`PUT /api/v1/sites/{id}/tests`).

### `GET /api/v1/tests`

Query: `tenantId` (same admin/tenant-user rules as Sites). Returns
`TestDef[]` (see shape below).

### `POST /api/v1/tests`

Body is a `TestDef`:

```json
{
  "tenantId": "...", "name": "...", "type": "ping",
  "intervalSeconds": 60, "params": { /* type-specific, see below */ }
}
```

`tenantId` required for admins. `name` required. `type` must be one of
`ping`, `dns`, `http`, `tcp`, `traceroute`, `wlan_scan`, `speedtest`;
validation and defaulting of `params` is type-specific (see
[Test parameter shapes](#test-parameter-shapes)); invalid params → `400`.
`409` on duplicate name within the tenant. Returns the created `TestDef`
(with `id` and normalized `params`).

### `PUT /api/v1/tests/{id}`

Body: same shape as create. `type` and `tenantId` are immutable (ignored if
sent); `name`, `intervalSeconds`, `params` can change, re-validated the same
way. Pushes the updated config to every agent whose site uses this test.
Response: `{"test": <TestDef>, "pushed": <n>}`.

### `DELETE /api/v1/tests/{id}`

Deletes the test (cascades its site assignments and alert rules) and pushes
the resulting config to previously-affected agents. `204`.

### Test parameter shapes

`params` per `type` (defaults applied server-side when omitted/zero, from
`internal/server/config.go`):

| Type | Params fields | Notes |
|------|----------------|-------|
| `ping` | `targets: string[]`, `count: uint32` | ≥1 target required; `count` default 5, max 20 |
| `dns` | `queries: string[]`, `servers: string[]` | ≥1 of each required |
| `http` | `url: string`, `timeoutSeconds: uint32`, `skipTlsVerify: bool` | `url` must start with `http://`/`https://`; `timeoutSeconds` default 10 |
| `tcp` | `targets: string[]` (each `host:port`), `timeoutSeconds: uint32` | ≥1 target required; `timeoutSeconds` default 5 |
| `traceroute` | `target: string`, `protocol: "tcp"\|"icmp"\|"udp"`, `port: uint32`, `maxHops: uint32`, `probesPerHop: uint32` | `target` required; `protocol` default `tcp`; `port` default 443 for tcp/udp; `maxHops` default 30 (max 64); `probesPerHop` default 5; interval ≥ 30s |
| `wlan_scan` | `{}` | interval ≥ 30s |
| `speedtest` | `provider: "ookla"\|"ndt7"\|"cloudflare"` | `provider` default (and empty string) is `ookla`; interval ≥ 60s |

All types require `intervalSeconds >= 5` (higher minimums noted above for
`wlan_scan`/`traceroute`/`speedtest`).

---

## Agents

An agent is a sensor process belonging to one site; `POST` returns a
one-time enrollment token used by `netlama-agent -token <token>`.

### `GET /api/v1/agents`

Query: `tenantId` (admins: empty = all tenants; tenant users: forced to
their own). Returns `Agent[]` with `token` always blanked out and
`connected` added from the live gRPC registry:

```json
[{
  "id": "...", "tenantId": "...", "siteId": "...", "siteName": "hq",
  "name": "sensor1", "token": "", "wlanInterface": "",
  "wirelessInterfaces": null, "createdAt": "...", "connected": false
}]
```

### `POST /api/v1/agents`

Body: `{"name": "...", "siteId": "..."}` (both required; the site must
belong to your tenant, or be any tenant's site if admin). `409` on
duplicate name within the tenant. Response is the only place `token` is
ever non-empty — save it, it authenticates the agent process:

```json
{"id": "...", "tenantId": "...", "siteId": "...", "siteName": "hq",
 "name": "sensor1", "token": "<enrollment token>", "wlanInterface": "",
 "wirelessInterfaces": null, "createdAt": "..."}
```

### `PUT /api/v1/agents/{id}`

Body: `{"name": "...", "siteId": "...", "wlanInterface": "..."}` (`name`/
`siteId` required; the new site must be in the same tenant as the agent).
Renames and/or moves the agent to another site of the same tenant, and
pushes the resulting config live if the agent is connected. Response:
`{"pushed": <bool>}`.

### `POST /api/v1/agents/{id}/run`

Triggers an immediate out-of-cycle run of one of the agent's tests. Body:
`{"testId": "..."}`; the test must currently be assigned to the agent's
site (`400` otherwise). `409` if the agent isn't currently connected.
Response: `{"triggered": true}`.

### `DELETE /api/v1/agents/{id}`

Deletes the agent (its token stops working immediately, and it will be
rejected on its next stream reconnect). `204`.

---

## Results

### `GET /api/v1/results`

Query: `tenantId` (required for admins), and optionally `siteId`,
`agentId`, `testId`, `type` (test type), `since` (RFC3339 timestamp),
`limit` (default 100, max 2000). Returns `Result[]`, newest first:

```json
[{
  "id": 123, "agentId": "...", "agentName": "sensor1",
  "siteId": "...", "siteName": "hq", "testId": "...", "testName": "ping-gw",
  "testType": "ping", "time": "2026-07-09T...", "error": "", "ok": true,
  "payload": { /* type-specific result payload */ }
}]
```

E.g. for `testType: "speedtest"` the payload is
`{"provider": "ookla"|"ndt7"|"cloudflare", "serverName": "...",
"latencyMs": 12.3, "downloadMbps": 280.1, "uploadMbps": 110.5, ...}`.

---

## Overview

### `GET /api/v1/overview`

Query: `tenantId` (required for admins). The tenant dashboard: counts plus
per-test health computed over a recent window (≈3 test cycles, clamped to
90s–1h):

```json
{
  "sites": 2, "agents": 3, "agentsConnected": 2, "tests": 4, "activeAlerts": 1,
  "testHealth": [{
    "testId": "...", "name": "ping-gw", "type": "ping",
    "checks": 15, "ok": 15, "agents": 1, "status": "healthy",
    "lastSeen": "2026-07-09T..."
  }]
}
```

`status` is one of `healthy` (all checks ok), `degraded` (some failed),
`failing` (all failed), `nodata` (no checks in the window).

---

## Logs

### `GET /api/v1/logs`

Server and agent `log/slog` output (Info level and above), newest first.
Query params: `agentId`, `level` (`INFO`/`WARN`/`ERROR`), `limit` (default/
max 500), and for admins only, `source` (`server`/`agent`, empty = both)
and `tenantId` (empty = all tenants, ignored when `source=server` since
server logs carry no tenant). Tenant users are always implicitly scoped to
their own tenant's agents and may not request `source=server` (`403`).

```json
[{
  "id": 1, "time": "2026-07-09T...", "tenantId": "...", "agentId": "...",
  "agentName": "sensor1", "source": "agent", "level": "WARN",
  "message": "..."
}]
```

`tenantId`/`agentId`/`agentName` are omitted (empty) for server-scope
entries.

---

## Alert Rules

Per-test rules that watch either overall health or a numeric metric.

### `GET /api/v1/alert-rules`

Query: `tenantId` (required for admins). Returns `AlertRule[]`:

```json
[{
  "id": "...", "tenantId": "...", "testId": "...", "testName": "ping-gw",
  "name": "High latency", "metric": "latency_ms", "operator": ">",
  "threshold": 100, "forCount": 2, "webhookUrl": "https://..."
}]
```

### `POST /api/v1/alert-rules`

Body: `{"name": "...", "testId": "...", "metric": "...", "operator": "...", "threshold": 0, "forCount": 1, "webhookUrl": "...", "tenantId": "..."}`.
`tenantId` required for admins. `name` required. `testId` must reference an
existing test in the tenant. `metric` must be one of `unhealthy`,
`latency_ms`, `loss_percent`, `download_mbps`, `upload_mbps`. For any
metric other than `unhealthy`, `operator` must be one of `>`, `>=`, `<`,
`<=`, `==`. `forCount` (consecutive breaches before firing) defaults to 1 if
< 1. `webhookUrl` is optional (POSTed a JSON payload when the rule
fires/resolves). Returns the created `AlertRule`.

### `DELETE /api/v1/alert-rules/{id}`

`403` if the rule isn't in your tenant (tenant users). `204`.

---

## Alerts

Read-only alert *state* produced by rule evaluation (see `internal/server/alerts.go`).

### `GET /api/v1/alerts`

Query: `tenantId` (required for admins), `limit` (default 100, max 500).
Firing alerts first, then recent resolved ones:

```json
[{
  "id": 1, "ruleId": "...", "ruleName": "High latency", "agentId": "...",
  "agentName": "sensor1", "subject": "8.8.8.8", "state": "firing",
  "value": 142.3, "message": "...", "startedAt": "2026-07-09T...",
  "resolvedAt": null
}]
```

`subject` disambiguates multi-target tests (e.g. which ping target);
`resolvedAt` is omitted while `state` is `firing`.
