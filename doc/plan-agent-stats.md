# Plan: agent resource stats (CPU, memory, disk)

Implements the ROADMAP item "Agent resource monitoring: CPU, memory,
storage — reported over the stream and visible in the UI/metrics".

## Design (fixed)

### 1. Collection (`internal/probe/stats.go`, new — stdlib only)

Linux implementation (the agents run in Linux containers; on other OSes
return zero-values and ok=false, never an error loop):

- CPU percent: two reads of `/proc/stat` (`cpu ` aggregate line) spaced by
  the sampling interval — busy delta / total delta * 100. Keep the previous
  sample in the collector struct; the first report after start may omit CPU
  (no delta yet).
- Memory: `/proc/meminfo` — used = MemTotal - MemAvailable; report used and
  total bytes.
- Disk: `syscall.Statfs("/")` — used = (Blocks-Bfree)*Bsize, total =
  Blocks*Bsize.
- These are HOST-level readings when running in a container (host /proc,
  host rootfs size may reflect the container layer — report what is
  observable; document the semantics in the README: "the sensor's view of
  its host"). No cgroup awareness in this phase.

### 2. Proto (`proto/netlama.proto` + `make proto`)

New message and a new AgentMessage oneof entry (next free field number):

```proto
message AgentStats {
  google.protobuf.Timestamp time = 1;
  double cpu_percent = 2;      // 0 when not yet measurable
  uint64 mem_used_bytes = 3;
  uint64 mem_total_bytes = 4;
  uint64 disk_used_bytes = 5;
  uint64 disk_total_bytes = 6;
}
```

### 3. Agent (`internal/agent/agent.go`)

Send one AgentStats every 30s while the stream is connected, through the
existing single-writer send loop (same pattern as the log ticker — do NOT
add a second stream writer). First send shortly after registration.

### 4. Server

- Store the LATEST stats per agent only — columns on the agents table or a
  small JSON column, following whichever pattern is cheapest next to
  capabilities/wireless (no history table; Prometheus owns time series).
- `GET /api/v1/agents` includes a `stats` object (cpuPercent, memUsedBytes,
  memTotalBytes, diskUsedBytes, diskTotalBytes, time) — omitted when the
  agent has never reported (old binaries: everything stays working, same
  backward-compat bar as capabilities).
- Prometheus gauges labeled like `netlama_agent_connected`:
  `netlama_agent_cpu_percent`, `netlama_agent_memory_used_bytes`,
  `netlama_agent_memory_total_bytes`, `netlama_agent_disk_used_bytes`,
  `netlama_agent_disk_total_bytes`.

### 5. Web UI

Agents page: compact stats in the agent row/details — CPU %, memory as
"used / total" (human-readable GiB), disk likewise, plus how old the
reading is if stale (> 2 minutes). Nothing rendered for agents that never
reported.

### 6. Docs

README (agent section: what is reported, host-level semantics, 30s cadence),
doc/API.md (agents resource `stats` field), ROADMAP checkbox for the
resource-monitoring item (CPU/memory/storage done; note "per-cgroup /
container-scoped readings" as a possible later refinement), PROGRESS.md
dated entry. No new env vars → no compose changes.

## Verification (required)

1. `make proto`, `make build`, `go vet ./...`, `go test ./...`. Unit tests
   for the /proc/stat delta math and /proc/meminfo parsing (feed fixture
   strings, don't read the real files in tests).
2. E2E per CLAUDE.md (self-signed TLS, scratch ports): run a real agent
   ≥ 40s, then confirm (a) `GET /api/v1/agents` shows a stats object with
   plausible nonzero memory and disk totals and a fresh timestamp,
   (b) `/metrics` contains the new gauges with the tenant/site/client
   labels, (c) the agent log shows no errors from the stats path.
3. Backward compat: `ConfigForAgent`/API behavior unchanged for an agent
   row with NULL stats (unit test at the store level is fine).

## Constraints

- No new dependencies (no gopsutil). No history storage.
- Single-writer stream discipline must be preserved.
- Do not commit; leave changes in the working tree for review.
