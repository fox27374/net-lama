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

## 2026-07-13 — Browser back/forward navigation between sections

- Sections are now recorded in browser history via the URL hash
  (`#dashboard`, `#agents`, …): the mouse/browser back and forward buttons
  move between previously visited sections instead of leaving the app, the
  hash acts as a shareable deep-link to a section, and a reload stays on
  the current page. The first section replaces the history entry so "back"
  from it still exits cleanly.

## 2026-07-13 — Path view rework (vertical subway line, MTR-style latency bars, ECharts heatmap)

- **Path view redesign** (UI-only, no Go changes): replaced the horizontal
  hop-chain box-and-arrow visualization with a vertical "subway line" —
  left rail with station dots colored by loss class, showing hop number,
  host (mono), and inline avg/loss. Failed hops (stalled path) show a
  dashed rail break below; unreached target shows as muted/dashed station.
  Pure CSS with no SVG or library.
- **MTR-style latency range bars**: added a new "Latency" column to the
  Hops table. Each cell holds an inline range bar (track + best→worst span +
  avg marker) positioned as percent of max worst RTT across all hops. Bar
  color follows the loss class (ok/warn/bad). No SVG math, just percent-
  positioned divs.
- **Path history heatmap** (NEW): third card fetches last 48 results
  (in reverse for display), renders an ECharts 5.6.0 heatmap with x-axis =
  result time (HH:MM), y-axis = hop TTL (inverted so hop 1 at top), cell
  value = avgRttMs. No-reply hops produce no data points. Tooltip shows
  host, avg/best/worst ms, loss %. visualMap: continuous, min 0, max =
  nice-rounded max of avg RTTs; green→amber→red ramp from --ok/--bad CSS
  tokens read at render time. Fewer than 2 results shows empty state.
  Chart instance is lazily initialized (section visible), cached and
  re-rendered on theme toggle (results cached in module scope), and
  resized on window resize (only when path section visible).
- **ECharts wiring**: added `<script src="vendor/echarts.min.js"></script>`
  before app.js in index.html. Vendored ECharts 5.6.0 is already present at
  internal/web/static/vendor/echarts.min.js (no re-download, no Go changes).
- **Documentation**: ROADMAP checked off "Modify the path view to look more
  professional"; CLAUDE.md amended to note vendored third-party libs
  (currently ECharts for the path history heatmap); PROGRESS.md entry added.

## 2026-07-13 — Path latency waterfall + history click-to-inspect

- **Latency contribution waterfall chart**: new card between Hops table and
  Path history. ECharts stacked-bar waterfall showing cumulative RTT by hop,
  with the contribution (delta) of each hop highlighted. Colors: green
  (--accent) for positive deltas, red (--bad) for the largest positive delta
  (the hop that hurts most), and muted (--border) for negative deltas (jitter/
  asymmetric return path). Tooltip shows host, +delta ms, cumulative avg RTT ms.
  Fewer than 2 responding hops shows empty state. Chart height 260px; axis/text
  colors read from CSS tokens at render time; theme toggle re-renders.
- **Click-to-inspect heatmap cells**: clicking a cell in the Path history heatmap
  loads that exact run into the view (status banner, subway, hops table, waterfall).
  Refactored renderPath() → renderPathResult(result, agent) extraction to render
  one result; the heatmap click handler finds and calls renderPathResult with the
  clicked timestamp. Heatmap x-axis now uses raw r.time as the category key (exact
  match, no fragile formatted-time lookup); display formatting is applied via
  axisLabel formatter + tooltip, eliminating the previous find-by-time bug.
- **"Back to latest" affordance**: when a historical run is displayed, a chip
  prepends the status banner ("Viewing run from [time] — Back to latest button")
  re-rendering the latest cached result without a refetch. Refresh / Run now /
  agent/test change reset to latest (they already re-run renderPath).
- **Cache & re-render**: module variables `paDisplayedResult` and `paLatestResult`
  track the current display and latest result; theme toggle re-renders the
  waterfall (via paDisplayedResult); both charts (waterfall + heatmap) use the
  same lazy-init / setOption(true) / dispose-on-empty / resize / theme-re-render
  pattern.
- **Styling**: CSS for .viewing-indicator badge and #pa-back-latest button added
  to style.css under the path-* section.
- **Verification**: /app.js contains renderPathResult and renderPathWaterfall
  functions, heatmap click handler with paHeatmapInstance.on("click"), and
  paWaterfallInstance lifecycle. index.html has the new waterfall card container.
  Line implementing largest-delta highlight: in renderPathWaterfall, the itemStyle
  color logic at the data mapping step, checking `i === largestDeltaIndex &&
  waterfallData[i].delta > 0 ? badColor : ...`.

## 2026-07-13 — Path horizontal waterfall (APM-style) + latency/loss metric toggle

- **Horizontal APM-trace waterfall**: reworked renderPathWaterfall to display as
  horizontal bars (one row per hop) instead of vertical columns. yAxis is now a
  category axis with hop labels (TTL + host, truncated to ~24 chars, monoish small
  font in --muted-solid color); xAxis is value (ms) at the top with grid lines on.
  Floating-bar stacking transposed: invisible "base" series positions each bar to
  start at its previous hop's cumulative RTT; visible "delta" series shows the
  hop's latency contribution. Bar height ~16px (barWidth), rows scale chart height
  to `Math.max(180, rows*28 + 70)` px with dynamic resize. Same color scheme and
  tooltips as before (largest positive delta → --bad, positive → --accent, negative
  → --border).
- **Latency/Loss segmented control**: new pill-button toggle in the Path section
  header with two states ("Latency" / "Loss"). Active button styled with --accent
  background. Module variable `paMetric` tracks the selected metric. Card h3 titles
  given IDs (pa-waterfall-title, pa-history-title) and updated dynamically when
  metric changes.
- **Loss mode — waterfall**: plain (non-cumulative) horizontal bars showing loss %
  from 0 to 100 on the xAxis. Bar color by loss thresholds: ≥60% → --bad, ≥20% →
  --warn, else → --ok. Tooltip shows host, loss %, and avg RTT for context. Title
  becomes "Packet loss by hop".
- **Loss mode — heatmap**: heatmap cells now display lossPercent instead of avgRttMs.
  visualMap fixed to 0–100 % with --ok → --warn → --bad ramp. Title becomes
  "Path history — loss". Tooltip updated to show loss % as primary value. All
  heatmap interactions (click-to-inspect, zoom) and theme toggle work in both modes.
- **No API changes**: all data is re-rendered from cached paDisplayedResult and
  paHistoryResults; no refetch on metric toggle. Theme toggle respects paMetric
  (re-renders via existing renderPathWaterfall/Heatmap calls). Backward compatible:
  paMetric defaults to "latency".
- **Styling**: new .seg-control, .seg-btn, .seg-btn.active CSS classes added to
  style.css after the button styles. Segmented control uses existing design tokens
  for consistent light/dark theme support.
- **Verification**: make build, go vet, go test all pass. Serve check confirms
  /app.js contains `paMetric = "latency"`, segmented-control event handlers for
  metric toggle, yAxis category/inverse and xAxis position top configuration; /
  contains two segment buttons with data-metric and the two h3 id attributes;
  both cards have functioning loss-mode bars and cells. Evidence from modified
  files: axis-swap configuration at lines yAxis: { type: "category", data: labels,
  inverse: true } and xAxis: { type: "value", position: "top", ... }; dynamic
  height at `const chartHeight = Math.max(180, respondingHops.length * 28 + 70)`;
  loss-mode visualMap at `{ min: 0, max: 100, ..., inRange: { color: [okColor,
  warnColor, badColor] } }`.

## 2026-07-13 — Per-hop jitter and honest no-reply hops

- **Jitter parsing end-to-end**: mtr's StDev field (per-hop jitter) is now
  parsed from `mtr --json` output through the full pipeline: probe result
  (`Hop.JitterMs`), protobuf (`Hop.jitter_ms`, field 8), agent-side
  conversion, and stored as `jitterMs` in JSON results.
- **Jitter demo mode**: synthetic traceroute data emits realistic jitter values
  (0.2–3 ms per hop).
- **UI metric toggle**: Path view now has three metric segments (Latency /
  Jitter / Loss). Waterfall and heatmap charts render jitter values with
  appropriate scaling and color ramps. Tooltip shows jitterMs when in jitter
  mode.
- **Hops table**: added "Jitter (ms)" column (after Worst RTT) showing jitter
  for responding hops, "–" for anonymous/no-reply hops or missing data. Old
  results without jitterMs are handled gracefully (treated as 0).
- **No-reply hops fix**: anonymous hops (no host) now render "no reply" in the
  Loss cell instead of "100%", which reads as an outage. Subway diagram and
  charts already handled this correctly.
- **Path view polish**: removed the redundant subway (vertical hop diagram) that
  duplicated the hops table; moved the status banner into the waterfall card
  above the chart; reordered cards (waterfall → hops table → history); fixed
  waterfall axis clipping with proper grid sizing (`top: 44, bottom: 24, left:
  140, right: 30`) and visible top-axis labels + units in all three metric modes
  (ms for latency/jitter, % for loss); rendered negative-delta hops (jitter
  artifacts where a hop's avg RTT is lower than the previous hop) as thin tick
  marks (scatter series, symbol "rect", 3×16px) instead of misleading gray bars;
  rebuilt the hops table with columns `# | Host | Latency | Loss | Jitter`
  (dropped Avg/Best/Worst), each metric cell containing right-aligned value text
  + inline bar (latency shows best–worst range with avg marker, loss is a
  0–100% bar, jitter is a 0–max bar). All no-reply hops show "* * *" and "–"
  for metric values. Updated chart height formula to `rows*28 + 100` for
  proper spacing.

## 2026-07-13 — Path reverse-DNS (PTR) resolution

- **Hop name resolution**: traceroute probes now perform best-effort parallel
  reverse-DNS (PTR) lookups on hop IPs. `internal/probe/traceroute.go` adds
  `HostName string` field to `Hop`, and `resolveHopNames()` function that
  spawns goroutines for each IP with a 1500ms context timeout per lookup,
  strips the trailing dot from results, and never fails the test (errors/
  timeouts leave `HostName` empty). Called after `parseMTR()` completes.
- **Proto & agent**: `proto/netlama.proto` adds `string host_name = 9;` to
  message `Hop`; `make proto` regenerates `*.pb.go`; `internal/agent/scheduler.go`
  copies `HostName` in the probe→proto hop conversion.
- **Demo mode**: `internal/probe/traceroute_demo.go` assigns synthetic hostnames
  to two hops ("gw.demo.lan" for the first hop, "core1.demo-isp.net" for a
  mid-path hop) while the rest stay empty, exercising the UI fallback path.
- **UI display rule** (hostname || IP): Hops table shows `hostName` as the main
  display with IP as a muted second line (monospace) when a name exists; bare
  IP (mono) when no name. Waterfall y-axis labels and tooltips follow the same
  rule (name + IP in parentheses when both exist). Heatmap tooltip shows the
  same. No-reply hops ("* * *") unchanged.
- **Server & storage**: protojson passes `hostName` through without change; it
  is omitted from JSON when empty, so older agents continue to work.

## 2026-07-13 — Alert-rule configuration UI as its own menu item

- **UI restructuring**: moved alert rules and added alert targets configuration
  to a new dedicated "Alerts & Alert Rules" page under a new "Configuration" sidebar
  group (above the "Manage" group). The existing Alerts page now shows only active
  and recent alert instances (firing/resolved state history).
- **Alert targets management**: new Alert Targets block with a table and create/edit
  dialog supporting all four target types: webhook (URL), email (to/subject), SNMPv2c
  trap (host/port/community), and script (path/args, admin-only). A static built-in
  "Dashboard" row reminds users that alerts are always stored and visible regardless
  of targets. Type-switching UI hides/shows relevant config fields; edit button allows
  updating existing targets; delete removes targets (validating they're not in use).
  Target type "script" is hidden from non-admin users (403 errors on API for non-admins
  creating or editing script targets).
- **Alert rules extended**: rule dialog now includes clear threshold (optional number),
  clear count (for hysteresis exit), and a checkbox multi-select list of alert targets
  (populated from the API). Rules table shows a "Clear Condition" column summarizing
  the inverse condition and clear count (e.g., "latency (ms) < 70 ×10"). Rules support
  edit mode (PUT /api/v1/alert-rules/{id}) in addition to create.
- **Alerts page simplified**: the Alerts section now displays only the active & recent
  alerts view (removed the rules table from this page). Firing alerts appear first,
  then recent resolved ones, all routable to their rules via the Configuration page.
- **Navigation**: new URL hash section "alertcfg" automatically works with the existing
  browser history and deep-link system (showSection, reloadSection, sections array).
- **ROADMAP** checkbox completed with this entry. README alerting paragraph covers
  targets, clear hysteresis, and SMTP env vars — no changes needed there (already
  documented).

## Known issues

- The agent logs "Registered with server" right after *sending* the register
  message, before the server accepts it — a rejected agent briefly logs
  success. Pre-existing, not yet fixed.
