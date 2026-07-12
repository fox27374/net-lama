# Progress

What has been done so far, in chronological order. Planned work lives in
[ROADMAP.md](ROADMAP.md); this file records what actually shipped.

## Origins (pre-2026)

- Original Python prototype: MQTT-based client/server, Splunk HEC forwarder,
  Prometheus metrics, OpenAPI service API. Preserved unchanged in
  [legacy/](legacy/).

## 2026-07-07 — Go rebuild

- **Migrated the whole project to Go** (`4ab859a`, `8ffa65f`): single-binary
  server (`cmd/server`) and agent (`cmd/agent`), gRPC bidi control stream
  (token auth, config push down, results up), multi-tenant web UI + JSON API,
  SQLite persistence, Prometheus exporter. Python code moved to `legacy/`.
- **Containers & distribution** (`71c1c99`): multi-target `Containerfile`
  (server / agent / agent-sensor), `compose.yaml`, GHCR image publishing via CI.
- **Docs** (`4f3efaa`): README rewritten for the container workflow, ROADMAP
  backlog added.
- **WLAN Phase 1** (`1b09380`): agents report their wireless interface
  inventory, per-agent interface selection, periodic managed-mode AP/SSID
  scanning, Wireless page in the UI.

## 2026-07-08 — Tests, alerting, TLS

- **Traceroute / path analysis Phase 1** (`93081db`, `b85bc6c`): mtr-based
  path test (TCP/ICMP/UDP), per-hop RTT and loss, failure localization,
  hop-chain Path visualization in the UI. Follow-up fixed "reached" detection
  and added `compose.sensor.yaml` (host-network sensor agent) for real path
  tracing.
- **Fix** (`449b67c`): empty demo env vars are treated as disabled.
- **Alerting & on-demand runs** (`c2c812b`): per-test alert rules (unhealthy
  state or latency/loss/throughput thresholds) with consecutive-breach counts,
  per-target alert state, webhook notifications, Alerts UI with nav badge;
  `RUN_TEST` ("Run now") from the Path and Results pages.
- **TLS** (`37d3a1a`): one cert covers the gRPC control stream and the HTTPS
  UI; self-signed auto-generation or bring-your-own cert/key; agent verifies
  via CA file, system roots, or `NETLAMA_TLS_INSECURE=1`; secure cookies.

## 2026-07-09 — mTLS

- **Per-agent mTLS** (`022e978`): `NETLAMA_MTLS=1` (or `NETLAMA_MTLS_CA`)
  makes the gRPC listener require client certificates on top of the token
  (HTTPS UI stays server-auth only). A built-in agent CA
  (`netlama-agent-ca.pem/.key` next to the DB) is auto-generated;
  `netlama-server -issue-agent-cert <name>` mints per-agent certs; the cert
  CN must match the agent name the token resolves to.

## 2026-07-10 — Capability detection and reporting

- **Agent capabilities** — agents detect and report which test types they can run:
  `ping`, `dns`, `http`, `tcp`, `speedtest` are always claimed; `traceroute` is
  claimed if `mtr` is in PATH or `NETLAMA_TRACEROUTE_DEMO=1`; `wlan_scan` is
  claimed if `iw` is in PATH and at least one wireless interface exists, or
  `NETLAMA_WLAN_DEMO=1`. Capabilities are stored on the agent record and exposed
  in the JSON API.
- **Capability-aware test dispatch** — the server filters tests sent to agents,
  excluding any whose type is not in the agent's capability list. Backward
  compatible: agents with empty/unreported capabilities are assumed to support
  all tests, and the fixed capability list hardcoded by pre-detection agent
  binaries is recognized and treated as "unreported" so upgrading the server
  before the agents cannot drop tests. The server logs filtered tests once per
  agent connection.
- **Web UI** — agents page shows capability badges; sites page shows inline
  warnings when an assigned test won't run on some agents (client-side check).

## 2026-07-09 — Logs

- **Web UI logs, Phase 1**: server and agent `log/slog` output (Info level and
  above) is now captured centrally and shown on a new Logs page. The server
  tees its own logger into SQLite through a non-blocking buffered-channel
  handler (`internal/logtee`, `internal/server/logsink.go`); agents buffer
  the same way into a small ring buffer (capacity 200, drop-oldest while
  disconnected) and ship entries over their existing control stream
  (`AgentMessage.log`, already defined in the proto but previously unused).
  History is bounded per scope (server, or each agent) via
  `NETLAMA_LOG_HISTORY` (default 1000), pruned the same way results are.

## 2026-07-12 — Agent self-health

- **Agent self-health**: explainable health status (healthy/degraded/unhealthy/
  unknown) computed server-side from agent self-metrics (CPU share, process count,
  uptime), connection stability (reconnect flapping in a 15-minute sliding window),
  and agent-scoped error logs. Health shown as a badge in the Agents UI page,
  included in `/api/v1/agents` responses with reasons and uptime, and exported
  as the Prometheus gauge `netlama_agent_health` (0=healthy, 1=degraded,
  2=unhealthy, -1=unknown). Agents that never send stats show "unknown" status
  (backward-compatible, same as capabilities). Thresholds: CPU > 20% (degraded),
  processes > 500 (degraded) / > 1500 (unhealthy), stats stale > 2min (degraded) /
  > 5min (unhealthy), reconnects ≥3 in 15m (degraded) / ≥6 (unhealthy), errors
  ≥2 in 15m (degraded) / ≥10 (unhealthy).
  `GET /api/v1/logs` scopes tenant users to their own agents (never server
  logs) and lets admins filter by tenant/source/agent/level.

## 2026-07-09 — API keys + full API documentation

- **API-key authentication**: audited every UI flow against `internal/api`
  and confirmed GUI/API parity already existed (every `fetch` in `app.js`
  hits a route registered in `internal/api/api.go`); what was missing was a
  non-cookie auth path, self-service key management, and documentation.
  Added `api_keys` storage (`internal/store/apikeys.go`, SHA-256-hashed
  secrets, `nlk_...` bearer tokens, cascade-deleted with their owning user),
  extended `auth()` in `internal/api/api.go` to accept
  `Authorization: Bearer nlk_...` ahead of the session cookie with zero
  per-handler changes (a key carries exactly the owning user's privileges),
  and `GET/POST /api/v1/apikeys` + `DELETE /api/v1/apikeys/{id}`
  (`internal/api/apikeys.go`) so a script can bootstrap with
  `POST /api/v1/login` → `POST /api/v1/apikeys` → Bearer from then on. New
  API Keys page in the UI (list, create-with-name, revoke, one-time secret
  display). Unit tests cover create → lookup → revoke → lookup-fails and the
  user-delete cascade.
- **`doc/API.md`**: full API reference written from the handler/store code —
  every route in `internal/api/api.go`, request/response shapes, the
  `?tenantId=` admin-scoping convention, the error format, and an
  authentication section with curl examples for both the cookie and
  API-key flows. README and ROADMAP updated to point at it.

## 2026-07-10 — Speedtest provider selection (ndt7, Cloudflare)

- **Alternative speedtest providers**: the existing `speedtest` test type
  gained a `provider` param (`ookla`/`ndt7`/`cloudflare`) instead of new
  test types, so the wire shape (`SpeedtestResult`), Prometheus gauges and
  alert rules kept working unchanged — providers are told apart by the
  `test` label exactly like two speedtest tests already were.
  `internal/probe/ndt7.go` uses the official
  `github.com/m-lab/ndt7-client-go` client (download then upload against
  the nearest M-Lab server via the public Locate API); its dependency tree
  resolved to 6 new modules, all ndt7-relevant (`m-lab/go`, `m-lab/locate`,
  `m-lab/ndt-server`, `m-lab/tcp-info`, `gorilla/websocket`,
  `araddon/dateparse`) — no advisor consultation needed, and both native
  and Pi cross-compiles (`make pi`) succeeded on the first try.
  `internal/probe/cloudflare.go` is stdlib-only against
  speed.cloudflare.com: median of 5 small GETs for latency, 4 parallel
  connections for download/upload over a 10s window. One real-world
  surprise caught only by e2e testing: `/__down?bytes=N` rejects `N` over
  100,000,000 with a 403 (not documented), so download loops in
  90MB-chunks per connection instead of one oversized request; the colo
  code also came back on a plain `colo` response header, not the
  `cf-meta-colo` header name implied by the CORS-exposed-headers list.
  `internal/server/config.go` validates/threads the provider through
  (empty stays `ookla` for every pre-existing test row). Web UI: a
  Provider dropdown on the Tests page (shown only for `speedtest`), and
  the provider is now shown in the Results row detail.
- Verified with a real three-provider e2e run against the live internet
  (self-signed TLS, scratch ports): `ookla`, `ndt7` and `cloudflare` tests
  all produced plausible nonzero download **and** upload Mbps with the
  correct `provider` field via `GET /api/v1/results`, and a test created
  with an empty `provider` (pre-existing-row shape) ran as `ookla`,
  confirming backward compatibility.
- **Robustness fix**: treat uninterpolated compose placeholders as unset.
  Older podman-compose versions (e.g., Debian 12's) pass `${VAR:-default}`
  syntax literally to the container. Updated `envOr`, `envEnabled`, and
  `envIntOr` helpers in both cmd mains and `internal/probe/env.go` to detect
  and ignore such placeholders, so they behave like empty/unset values.
  Added unit tests and a README note about the old podman-compose behavior.

## 2026-07-10 — Agent resource statistics

- **Agent stats** (CPU, memory, disk): agents collect and report resource usage
  every 30s via a new `AgentStats` protobuf message. Stats are gathered by reading
  host-level `/proc/stat` (CPU percentage calculated from two samples spaced by
  reporting interval), `/proc/meminfo` (used = MemTotal - MemAvailable), and
  `syscall.Statfs` on the root filesystem (disk usage). On non-Linux systems
  stats collection fails gracefully and returns false/zero; no error loops.
  Fixture-based unit tests for `/proc` parsing with provided test data; e2e
  verification of stat collection and Prometheus export.
- **Storage & API**: latest stats are stored per agent on the agents table
  (JSON column), backward-compatible migration (NULL for old agents). `GET
  /api/v1/agents` includes a `stats` object (omitted when agent never reported).
- **Web UI**: Agents page shows three columns — CPU %, memory (used/total in GiB),
  and disk (used/total in GiB) — each marked stale if > 2 minutes old, with "—"
  when unavailable (non-Linux platforms, or never reported).
- **Metrics**: five new Prometheus gauges labeled by tenant/site/client:
  `netlama_agent_cpu_percent`, `netlama_agent_memory_used_bytes`,
  `netlama_agent_memory_total_bytes`, `netlama_agent_disk_used_bytes`,
  `netlama_agent_disk_total_bytes`.
- **Docs**: README updated with agent stats section (host-level semantics, 30s cadence),
  agent stats listed on the Agents page description, metrics section updated
  with the new gauges, ROADMAP checkbox completed with note about per-container
  scoping as a later refinement.

## 2026-07-11 — UI design tokens; zombie-reaping fix

- **UI design-system pass**: strict token system (4px spacing scale, radius/
  type/elevation scales, semantic ok/warn/bad colors, 8-hue categorical chip
  ramp with per-theme WCAG-checked variants, focus-visible rings, tabular
  numerals, reduced-motion support). No raw hex outside the theme blocks.
- **Fix: agent containers must run with an init** (`init: true` in both
  compose files, `--init` in the UI enrollment snippet). The agent is PID 1
  and never reaped orphaned children of exec'd tools (mtr), so one zombie
  per traceroute run accumulated until the container could not fork —
  found on the rp02 Pi after ~25h uptime (759 PIDs, every traceroute
  failing with "parsing mtr json: unexpected end of JSON input").

## Live deployment

- Running on `ataltpr06.lnxnet.org`: rootless podman + podman-compose,
  `compose.sensor.yaml` (server + sensor agent with host networking), images
  built locally from a shipped source tarball under `~/netlama/`.
  Tenant `lab`, site `tpr06`, agent `tpr06-sensor`. Self-signed TLS with
  `NETLAMA_TLS_INSECURE=1` on the agent; mTLS code is deployed but not yet
  enabled there.

## Conventions established

- New server/agent options are env-driven (`NETLAMA_*`) with a
  zero-external-dependency default (self-signed cert, built-in CA); each one
  is wired as flag + env in both cmd mains and documented in the README, the
  ROADMAP checkbox, and both compose files.
- End-to-end verification: build the binaries, start a server with
  self-signed TLS, create tenant → site → agent via the JSON API, run agents
  against it.

## 2026-07-12 — Dashboard restructure with sparklines

- **Left sidebar navigation** — replaced top-tab header with a fixed left
  sidebar (~220px; collapses on <900px viewports). Navigation order:
  Dashboard, Results, Path, Wireless, Logs, Alerts, Tests, Sites, Agents;
  Manage group (Tenants, Users, API Keys); brand at top, theme toggle + logout
  at bottom. Active item shows accent left border. All pages now stack
  vertically full-width with .card styling.
- **Dashboard (renamed Overview)** — landing page now shows a site filter
  dropdown at the top. Restructured into 5 full-width blocks: (1) stat tiles
  (sites, agents, tests, active alerts — count changed from test health); (2)
  Sites table with agent count and health rollup; (3) Alerts table (active +
  recent, reused from Alerts page); (4) Tests table with inline SVG sparklines
  (no external library) + current value; (5) Wireless table (latest scan APs
  per agent). Site filter re-renders all blocks.
- **Sparklines & series data** — extended `TestHealth` struct with `Series`
  (last ~30 values, oldest first; null values omitted), `Unit` (ms/Mbps/hops/APs),
  and `Current` (last value). `GET /api/v1/overview` now accepts optional
  `?siteId=` parameter (validated, tenant-scoped); `TenantOverview` now takes
  `siteID` and filters agent/test/alert queries accordingly. Series extraction
  pulls the primary metric per test type: ping→avg latency ms, dns/http/tcp→
  duration ms, speedtest→download Mbps, traceroute→hop count, wlan_scan→AP
  count. Client-side SVG sparklines (~160x36px) render with stroke, no axes/grid,
  a muted dot on the last point, and right-aligned current value (tabular
  numerals). Sparkline color uses --cat-1 design token.
- **ROADMAP additions** — added unchecked items under "Server & UI": configurable
  dashboard, separate configure/view menus, Path redesign, alert-rule config UI,
  logo, version tags.
- **API.md updated** — overview endpoint now documents optional `siteId` param
  and new TestHealth fields (series, unit, current).

## 2026-07-12 — Dashboard deep-links

- **Dashboard deep-links**: every dashboard block is now clickable. Stat tiles
  (Sites, Agents, Tests, Active alerts) navigate directly to their corresponding
  pages. Block headers have "View all →" links for Sites, Alerts, Tests, and
  Wireless blocks that navigate to those pages. Table rows on the dashboard are
  clickable: Sites and Alerts rows navigate to their pages; Tests rows navigate
  to Results with the test preselected; Wireless rows navigate to the Wireless
  page. Accessibility: all interactive elements support keyboard navigation
  (tabindex="0" on rows, Enter key triggers navigation). UI enhancements include
  hover effects (surface shift on tiles, muted→accent color transition on
  "View all" links) and focus-visible outlines.

## Known issues

- The agent logs "Registered with server" right after *sending* the register
  message, before the server accepts it — a rejected agent briefly logs
  success. Pre-existing, not yet fixed.
