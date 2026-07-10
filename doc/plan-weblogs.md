# Plan: logs visible in the web UI (agent + server)

Implements the ROADMAP item "Enhanced logging; logs visible in the web UI
(agent + server)" — Phase 1: capture server and agent logs centrally and show
them on a new Logs page.

## Existing groundwork

- `proto/netlama.proto` already defines `LogEntry {time, level, message}` and
  it is already part of the `AgentMessage` oneof (`log = 3`). **Nothing sends
  or handles it yet.** No proto change is required; do not run `make proto`
  unless you add fields.
- Both server and agent log via `log/slog` (loggers built in `cmd/*/main.go`).
- Bounded-history pattern to copy: per-agent result pruning in
  `internal/store/agents.go` (~line 169).

## Design

### 1. Storage (`internal/store/logs.go` + schema in `store.go`)

New table `logs`: autoincrement `id`, timestamp (same convention as the
`results` table), `tenant_id` (empty for server logs), `agent_id` (empty for
server logs), `source` (`server` | `agent`), `level`, `message`. Index to make
the list query cheap.

- `InsertLog(...)` prunes like results do: keep the newest N rows per scope
  (server logs as one scope, each agent as one scope). N from
  `NETLAMA_LOG_HISTORY`, default 1000, wired as flag+env in `cmd/server`.
- `ListLogs(tenantID, agentID, source, level, limit)` newest-first,
  default/max limit 500.

### 2. Server log capture

A `slog.Handler` wrapper that tees every record (Info and above) to the
existing handler AND to the store. Hard requirements:

- **Non-blocking**: hand records to a buffered channel consumed by one
  goroutine; if the buffer is full, drop the record (optionally count drops).
  The handler must never block the caller and never slow the hot path.
- **No recursion**: failures inside the sink (e.g. DB error) must NOT be
  logged through the teeing logger. Log them to stderr directly or drop.
- Wire it in `cmd/server/main.go` after the store is opened; logs before that
  point only go to stderr, which is fine.

### 3. Agent log shipping

Same tee-handler idea on the agent (`internal/agent`): records (Info and
above) go to a ring buffer (capacity ~200, drop oldest). Whenever the control
stream is connected, drain the buffer as `AgentMessage{log: LogEntry}`.
Details:

- Sending must reuse the existing stream-send path/mutex so it cannot
  interleave with result sends.
- While disconnected, keep buffering (drop-oldest). On reconnect, drain.
- Do not ship the very chatty per-connection debug lines; Info and above only.

Server side: handle the `log` payload in `ControlStream`
(`internal/server/server.go`), persisting with the connected agent's tenant
and agent IDs. Ignore log messages from agents that have not completed
registration.

### 4. API (`internal/api/logs.go`)

`GET /api/v1/logs?source=&agentId=&level=&limit=` following the existing
handler/routing style in `internal/api/api.go`:

- Tenant users: implicitly scoped to their tenant; they see only their
  agents' logs (never server logs).
- Admins: see everything; may filter with `?tenantId=`; `source=server`
  returns server logs (admin-only — a tenant asking for it gets an empty
  list or 403, match how existing handlers treat admin-only data).

### 5. Web UI (`internal/web/static/index.html` + `app.js`)

New **Logs** nav page, modeled on the Results page: newest-first table
(time, source, agent, level, message), filters for agent and level (plus a
source filter for admins), an auto-refresh toggle (reuse the Results page's
refresh pattern if there is one, otherwise a 5s interval while the page is
visible). Level column color-coded (warn/error stand out in both themes).

### 6. Docs & config (CLAUDE.md conventions)

- README: short "Logs" subsection under the Web UI section; document
  `NETLAMA_LOG_HISTORY`.
- ROADMAP.md: check the item off, rewording it to what was actually built
  (mirror how other checked items read); note remaining ideas (log download,
  debug level, retention by age) stay unchecked or as a Phase 2 line.
- PROGRESS.md: dated entry under today (2026-07-09).
- compose.yaml + compose.sensor.yaml: add `NETLAMA_LOG_HISTORY` (commented or
  defaulted) to the server environment blocks.

## Verification (required)

1. `make build`, `go vet ./...`, `go test ./...`.
2. E2E per CLAUDE.md: start the server with `NETLAMA_TLS_SELF_SIGNED=1`,
   create tenant/site/agent via the JSON API, run the agent
   (`NETLAMA_TLS=1 NETLAMA_TLS_INSECURE=1`), then confirm via
   `GET /api/v1/logs` that BOTH a server-sourced and an agent-sourced entry
   exist, and that a tenant-scoped user does not see server logs.
3. Add a unit test where natural, e.g. for the tee handler (non-blocking,
   level filtering) or the ring buffer.

## Constraints

- No new third-party dependencies.
- Do not commit; leave changes in the working tree for review.
- Keep the UI consistent with the existing vanilla-JS page patterns.
