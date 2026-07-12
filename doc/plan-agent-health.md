# Plan: agent self-health (status + reasons)

Adds an explainable health status per agent: healthy / degraded /
unhealthy / unknown, computed server-side from agent self-metrics,
connection stability, and agent-scoped (non-test) error logs. Old agents
that don't report self-metrics show "unknown" (same backward-compat bar
as capabilities — never penalize them).

## 1. Proto (`proto/netlama.proto`, additive only, then `make proto`)

- `AgentStats`: add `double agent_cpu_percent = 7;` (agent process share
  of host CPU), `uint64 agent_mem_bytes = 8;` (RSS),
  `uint32 pid_count = 9;` (processes in the agent's container),
  `uint64 uptime_seconds = 10;` (process uptime).
- `LogEntry`: add `string scope = 4;` — "test" when the record carries a
  test attribute, else "agent".

## 2. Agent

- `internal/probe/stats.go`: extend the collector — agent CPU share =
  delta(utime+stime from /proc/self/stat) / delta(total from /proc/stat)
  * 100; RSS from /proc/self/status VmRSS; pid count from
  /sys/fs/cgroup/pids.current when readable, else count numeric entries
  in /proc; uptime from a start timestamp captured at collector init.
  Non-Linux: zero values, ok stays as today. Fixture-based unit tests
  for the new parsers.
- `internal/logtee`: `Entry` gets `Scope string`; the handler sets
  "test" when the record (or bound attrs) contains a `test` attr, else
  "agent". Update both existing sinks; agent ships scope in LogEntry.

## 3. Server

- Store: `logs` table gains `scope TEXT NOT NULL DEFAULT ''` (additive
  migration); persist it for agent logs (server logs keep "").
- `internal/server/health.go` (new): evaluator returning
  `{status, reasons []string}` from:
  - latest stats: agentCpuPercent > 20 → degraded ("agent CPU share
    N%"); pidCount > 500 → degraded, > 1500 → unhealthy ("N processes
    in container"); stats older than 2 min while connected → degraded,
    5 min → unhealthy ("no stats for Nm").
  - reconnect flapping: count "connected" transitions per agent over a
    15-min sliding window (in-memory on the Server struct): ≥3 →
    degraded, ≥6 → unhealthy ("N reconnects in 15m").
  - agent-scoped logs: WARN+ERROR rows with scope != "test" for the
    agent in the last 15 min: ≥2 → degraded, ≥10 → unhealthy ("N agent
    errors in 15m").
  - Worst component wins; reasons lists every firing component. No
    self-stats ever received → status "unknown", empty reasons.
- `GET /api/v1/agents`: each agent gains
  `health: {status, reasons, uptimeSeconds}` (uptime from latest stats;
  omit block for unknown-with-no-stats agents if simpler — document
  whichever). Computed on request, not stored.
- Metrics: gauge `netlama_agent_health` (labels like
  netlama_agent_connected; value 0 healthy / 1 degraded / 2 unhealthy /
  -1 unknown), set wherever stats/connection events update.

## 4. Web UI

Agents page: health badge column (reuse ok/warn/bad chip styling;
muted chip for unknown) with the reasons joined in the title attribute
(hover) and uptime shown humanized (e.g. "3d 4h") next to it.
Dashboard stat tiles: "agents healthy" tile shows healthy/total.

## 5. Docs

README (agent health paragraph: components + thresholds), doc/API.md
(health field), ROADMAP new checked item under "Tests & monitoring"
(style-matched: agent self-health), PROGRESS.md entry (2026-07-12).
No new env vars → no compose changes.

## Verification

1. `make proto`, `make build`, `go vet ./...`, `go test ./...`. Unit
   tests: evaluator thresholds (each component + worst-of + unknown),
   /proc/self parsers (fixtures), scope tagging in logtee (record with
   and without test attr).
2. E2E (self-signed TLS, scratch ports): agent runs ≥90s → agents API
   shows health.status "healthy", empty reasons, uptimeSeconds > 0 and
   growing between two polls; /metrics contains netlama_agent_health 0.
   Kill and restart the agent 3x quickly → status becomes degraded with
   a reconnect reason.
3. Backward compat: store-level test that an agent row with stats/logs
   absent evaluates to "unknown".

## Constraints

No new dependencies. Single-writer stream discipline unchanged (new
fields ride the existing 30s stats send). Do not commit.
