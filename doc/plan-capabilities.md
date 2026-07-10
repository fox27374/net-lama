# Plan: agent capability reporting and capability-aware test dispatch

## Problem

The agent image variants (slim `agent` vs `agent-sensor`) can run different
test types, but the server doesn't know which is which: it pushes e.g. a
traceroute test to a slim agent, which then fails every interval with
`exec: "mtr": executable file not found` (observed on a real deployment).

## Design (fixed)

Capability **detection**, not variant labels: the agent reports what it can
actually do on its host; the server filters what it pushes and surfaces the
information. No per-variant test catalogs.

The proto groundwork exists: `Register.capabilities` (repeated string) is
already in `proto/netlama.proto`, declared but never populated. No proto
change should be needed — verify before touching the proto; only run
`make proto` if you genuinely must add something.

Capability strings = test type names: `ping`, `dns`, `http`, `tcp`,
`speedtest`, `traceroute`, `wlan_scan`.

### 1. Agent: detect + report (`internal/agent/`, maybe `internal/probe/`)

At startup (before Register), detect:

- `ping`, `dns`, `http`, `tcp`, `speedtest`: always claimed (stdlib/Go
  implementations, no external tools).
- `traceroute`: claimed if `exec.LookPath("mtr")` succeeds OR the
  traceroute demo env is enabled (reuse the existing probe env logic —
  including the `${...}` placeholder rule from commit d28606c).
- `wlan_scan`: claimed if `iw` is in PATH and at least one wireless
  interface was detected (the inventory the agent already collects for
  Register) OR the WLAN demo env is enabled.

Send the list in the existing `capabilities` field of Register. Log one
Info line listing the detected capabilities.

### 2. Server: store + expose

- Persist capabilities on the agent row, following the exact pattern used
  for the agent's wireless interfaces today (look at how those are stored
  on registration and exposed via the API — mirror it).
- `GET /api/v1/agents` includes `capabilities` (array of strings).
- An agent that never re-registered since this feature (old agent version,
  empty list) is treated as "unknown": assume it can run everything —
  do NOT filter for empty capability lists (backward compatibility; old
  agents keep behaving exactly as today).

### 3. Server: capability-aware dispatch (`internal/server/server.go`)

In `ConfigForAgent`, exclude tests whose type is not in the agent's
(non-empty) capability list. Log one Info line per connection when tests
were filtered (`"Skipping unsupported tests" agent=... tests=...`), not one
per push. This alone stops the recurring mtr failures.

### 4. Web UI (`internal/web/static/`)

- Agents page: capability badges per agent (reuse the existing chip
  styling; render nothing special for unknown/empty = old agent).
- Sites page (test assignment): after loading agents + tests, if an
  assigned test's type is missing from some agent's non-empty capability
  list, show an inline warning under the assignment control:
  "<test> won't run on <agent> (no <type> capability)". Client-side check
  only — no new endpoint needed.

### 5. Docs

- README: short paragraph in the agent/containers area — agents report
  capabilities; unsupported tests are not pushed; slim vs sensor image
  capability difference as the concrete example.
- doc/API.md: `capabilities` field on the agents resource.
- ROADMAP.md: add a new checked item under "Tests & monitoring" for
  capability reporting/dispatch (match existing style).
- PROGRESS.md: dated entry (2026-07-10).
- No new env vars → no compose changes.

## Verification (required)

1. `make build`, `go vet ./...`, `go test ./...`. Unit-test the detection
   decision logic if it's factored testably (e.g. table test mapping
   {mtr present, demo flags, interfaces} → capability list).
2. E2E per CLAUDE.md (self-signed TLS, scratch ports): register an agent
   in an environment WITHOUT mtr in PATH (or PATH-restricted) and one test
   each of type ping + traceroute assigned to its site; confirm via the
   agent log/config that the traceroute test was NOT pushed, the ping test
   runs, `GET /api/v1/agents` shows the capability list without
   `traceroute`, and no mtr error results appear. Then enable
   NETLAMA_TRACEROUTE_DEMO=1, restart the agent, and confirm traceroute IS
   pushed and produces (demo) results.
3. Backward compat: simulate an empty capability list (comment nothing
   out — you can craft this by registering with the current agent BEFORE
   your agent-side change is compiled in, or by a store-level unit test)
   and confirm no filtering happens.

## Constraints

- No new dependencies. Avoid proto changes if the existing field suffices.
- Do not touch the uncommitted working-tree state beyond your own changes.
- Do not commit; leave changes in the working tree for review.
