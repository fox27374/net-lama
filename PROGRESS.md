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

## 2026-07-13 — Tests moved into Configuration; alert-rule assignment in test dialog

- **Tests sidebar reorganization**: moved the "Tests" nav button from the main
  group into the new "Configuration" sidebar group (below "Alerts & Alert Rules").
  No functionality change; hash navigation (`#tests`) continues to work.
- **Alert Rule column in Tests table**: added a new "Alert Rule" column showing
  comma-separated names of alert rules whose `testId` matches the test, or a
  muted "—" when no rules are assigned. Fetches `/api/v1/alert-rules` in
  parallel with tests to populate the column.
- **Alert-rule assignment in test dialog**: when editing or creating a test,
  a new "Alert Rule" control appears at the bottom of the form:
  - If at least one alert rule exists: a labeled select with "— none —" plus
    every rule whose metric applies to the current test type (applicability
    map: unhealthy→all types; latency_ms→ping/dns/http/tcp/traceroute/speedtest;
    loss_percent→ping; download_mbps/upload_mbps→speedtest). Rules are
    re-filtered on test-type change. When editing a test with attached rules,
    the first one is preselected.
  - If no rules exist: a ghost "Create alert rule →" button that closes the
    test dialog, navigates to the alertcfg page, and opens the new-rule dialog
    with the test preselected (via `pendingTestForRule` module variable, same
    pattern as `pendingResultTest`).
  - On save: after test create/update succeeds, if a rule is selected and its
    `testId` differs from the test's id, a `PUT /api/v1/alert-rules/{ruleId}`
    call re-points the rule to the test (sends all existing rule fields unchanged).
    The tests list is reloaded afterward, so the new Alert Rule column reflects
    the change. Selecting "— none —" does nothing (no detach semantics).
  - Hint text below the control: "Assigning moves the rule to this test".
- **No Go/API changes**: backend rules already belonged to a test (`alert_rules.test_id`);
  "assigning a rule to a test" is a PUT of that field alone, using the existing
  endpoint.
- **Verification**: `make build`, `go vet ./...`, `go test ./...` all pass. The
  HTML thead now includes `<th>Alert Rule</th>` in the tests table. The app.js
  file contains the `METRIC_APPLICABILITY` map (unhealthy→all types, latency_ms→
  ping/dns/http/tcp/traceroute/speedtest, loss_percent→ping, download_mbps/
  upload_mbps→speedtest), the `populateAlertRuleSelect()` function with metric
  filtering and re-filtering on type change, and the PUT call in the form
  submission: `await api("PUT", `/api/v1/alert-rules/${selectedRuleId}`, ruleUpdate)`
  with all rule fields preserved. Evidence in the report below.

## 2026-07-13 — Logo, per-site health, configure/view nav split

- **Logo for the web UI**: theme-aware transparent llama logo (from
  `logo.jpg` artwork, background removed, strokes thickened for small sizes)
  in the sidebar and login box, plus light/dark favicons
  (`favicon-light/dark.png`) swapped by `prefers-color-scheme`. Assets
  generated as tinted-alpha PNGs matching the theme `--fg` tokens.
- **Per-site health rollup** (`siteHealth` in `GET /api/v1/overview`): the
  dashboard sites box previously mapped *tenant-wide* per-test health onto
  each site, so a shared test could show "no data"/wrong status for a site
  it was healthy (or broken) on. The server now judges each site's assigned
  tests only against results from that site's own agents. Health chips in
  the sites box also got spacing (`.health + .health`).
- **Configure vs. view nav split**: Sites and Agents moved from the top
  (viewing) sidebar group into Configuration (Sites, Agents, Tests,
  Alerts & Alert Rules).

## 2026-07-14 — Per-test state thresholds and state-based alert rules

- **Per-test state thresholds** (warn/crit boundaries): tests now accept an
  optional `thresholds` object (`{"warn": 30, "crit": 80}`) that applies to
  the test's primary metric (ping/dns/http/tcp → ms; speedtest → Mbps;
  traceroute → hops; wlan_scan → APs). Stored as TEXT (JSON) column on the
  tests table. Direction is type-specific: speedtest is "lower-is-worse"
  (values below thresholds trigger degraded/failing states), all other types
  are "higher-is-worse". Test result state is computed per result: failed
  results are always red; ok results without thresholds are green; otherwise
  state is green/orange/red based on metric vs. warn/crit. Health rollups
  incorporate state: any red in the window → failing; orange present (no red)
  → degraded; all green → healthy (backward-compatible for tests without
  thresholds).
- **State-based alert rules** (`metric: "state"`): new alert-rule metric type
  fires on test state. Threshold is the level (1=orange, 2=red); operator is
  always `>=`. Evaluation computes result state from the test's thresholds and
  fires when the state level is reached for `forCount` consecutive results.
  Clear hysteresis uses the same dead-band logic as other metrics.
- **API & validation**: tests POST/PUT endpoints now accept and validate
  `thresholds` (warn must be < crit if both set). Alert-rules endpoints
  validate `metric: "state"` with level ∈ {1,2} and set operator to `>=`.
  `doc/API.md` updated with the new fields and semantics.
- **Web UI**: Tests dialog thresholds use a Grafana-style colored **band
  editor** (`#t-bands` in `app.js`): stacked red/orange/green rows, each
  showing its swatch, the editable boundary value, and the derived range
  text ("80 and greater", "30 to 80", "less than 30"). Bands are added and
  removed individually ("+ Degraded (orange)"/"+ Failing (red)"); the same
  `{warn, crit}` model backs it. For speedtest the stack inverts (green on
  top, red at the bottom) since lower is worse, and server validation was
  fixed to require warn > crit in that direction (the initial numeric-input
  version wrongly rejected valid speedtest bands). Alert Rules dialog gained
  a "State is at least" metric option with a level dropdown (Orange/Red) in
  place of the numeric threshold input. Sidebar button renamed from
  "Alerts & Alert Rules" to "Alert Rules"; alertcfg section reordered
  (Rules box above Targets; "Alert Targets" heading → "Targets"). Webhook
  URL field removed from the rule dialog and API responses (deprecated, now
  routed through webhook targets).
- **Startup migration**: existing alert_rules with non-empty `webhook_url`
  are migrated: a webhook target is created or found (name convention:
  original rule name + " webhook"), added to `target_ids`, and `webhook_url`
  is cleared in the rule (idempotent, runs on every startup from the schema
  migration in `store.go`).
- **Storage & evaluation** (`internal/store/overview.go`): `testStatus`
  refactored to scan result payloads and compute state per result when
  thresholds are set. New helper functions: `computeResultState` (applies
  thresholds to a value, returns state string), `extractMetricValue` (pulls
  primary metric from result payload). State-aware status determination:
  red count > orange count > mixed/degraded > all healthy. Backward-compatible:
  no thresholds → classic ok-count logic.
- **Alert evaluation** (`internal/server/alerts.go`): `evalRule` now accepts
  test definition, extracts result state for `metric: "state"`, and compares
  level using existing hysteresis. New `resultState` function computes result
  state from thresholds (parsing JSON); new `extractResultMetric` function
  pulls the metric value from a TestResult oneof (mirrors overview.go logic).
- **Verification**: `make build`, `go vet ./...`, `go test ./...` pass.
  E2E: self-signed server, tenant/site/agent created via JSON API, HTTP test
  with tiny thresholds (warn=0.0001, crit=0.0002) targeting server UI,
  state-rule for red×3, webhook target with local http.server sink,
  confirmed overview displays degraded/failing correctly and webhook
  receives POST on state breach.

## 2026-07-17 — WLAN Phase 2: monitor-mode client sensing

- **New test type `wlan_sense`**: monitor-mode channel sweep capturing
  wireless stations (MAC/SSID/BSSID/RSSI/rate/MCS) and per-channel airtime
  utilization stats. Requires monitor-capable interface + NET_ADMIN privilege.
- **Proto & code generation**: `WlanSenseParams` (channels list, dwell time),
  `WlanSenseResult` (stations, channel stats, sweep time), `WlanStation`
  (MAC/BSSID/SSID/RSSI/rate/MCS/frame count), `WlanChannelStat` (channel/freq/
  active/busy/utilization). Field numbers: TestSpec.params `wlan_sense = 11`,
  TestResult.result `wlan_sense = 12`.
- **Agent-side probe** (`internal/probe/wlansense.go`): shared types, demo
  generator (8-15 stations, 2-4 BSSs, 2.4+5 GHz channels with varied utilization).
  Linux capture (`wlansense_linux.go`): **real packet capture via github.com/gopacket/gopacket v1.7.0 (maintained fork) using afpacket + zero-copy frame reading**. Per-frame parsing: RadioTap namespace for RSSI/rate/MCS with BadFCS() guard; Dot11 layer for MAC extraction and frame type classification (beacons/probe-responses → BSSID→SSID map, data frames → stations with ToDS/FromDS address ordering per 802.11, probe requests → probe_only flag). Interface type management (defer restore), per-channel tuning via `iw dev <if> set channel`, survey data from `iw dev <if> survey dump`. Stub (`wlansense_other.go`) for non-Linux. Demo mode via `NETLAMA_WLAN_SENSE_DEMO=1`.
  The pure frame-parsing lives in `processFrame` (in `wlansense.go`, no build
  tag) so it is unit-tested cross-platform; only the afpacket socket I/O is
  Linux-only.
- **Capability detection** (`internal/probe/capabilities.go`): claim `wlan_sense`
  when demo mode enabled OR monitor-capable interface exists AND process is
  privileged (euid 0 or CAP_NET_ADMIN).
- **Server config & validation** (`internal/server/config.go`): dwell_ms
  100–2000 (default 400), channels sanity 1–177, interval ≥30 sec.
- **Metrics & overview** (`internal/server/metrics.go`, `internal/store/overview.go`):
  primary metric = max channel utilization_pct (unit "%") so the green/orange/red
  state thresholds apply; Prometheus gauges `netlama_wlan_stations` and
  `netlama_wlan_channel_utilization_pct`.
- **Web UI** (`internal/web/static/`): the Wireless page gained a monitor-sense
  section for the selected agent — a channel-utilization bar chart (colored by
  load, 2.4/5 GHz labels) and a client-stations table (MAC/SSID/RSSI/rate/MCS/
  frames/last-seen, RSSI colored by signal, "probing…" for probe-only stations).
  The Tests dialog has a "WLAN sensing (monitor mode)" type with channels + dwell
  inputs, and `%` is wired into the state-threshold band editor.
- **Verification**: `make build` (darwin), `make pi` (arm64+armv7) with the
  `gopacket/gopacket` fork — both cross-compile cleanly, no CGO. `go vet ./...`,
  `go test ./...` pass. Unit tests: `processFrame` with hand-built radiotap+802.11
  frames (data ToDS/FromDS station+BSSID extraction, beacon SSID resolution,
  probe-only, BadFCS skip), SSID information-element parser, survey-dump parser,
  channel↔freq conversion, demo sanity, server metric extraction and validation.
  The frame tests caught a real bug: ToDS/FromDS were masked against the wrong
  bits (`0x0100`/`0x0200` on the single FC-flags byte), so every data frame was
  mis-handled as ad-hoc — fixed to use gopacket's `Flags.ToDS()/FromDS()`. Demo
  e2e (server + agent with `NETLAMA_WLAN_SENSE_DEMO=1`) confirmed 8 stations /
  5 channels flow through to the results API, overview (utilization as the
  primary metric), and Prometheus. Real capture verified to build for the Pi;
  live monitor-mode capture is validated during deployment on ataltrp01.

## 2026-07-17 — WLAN sense: discovered networks (SSIDs/APs from beacons)

- **`wlan_sense` now reports the networks it hears**: beacons/probe-responses
  captured during the sweep are aggregated into a `networks` list (BSSID, SSID,
  channel, freq, strongest RSSI, beacon count) on `WlanSenseResult` (new
  `WlanNetwork` proto message, field 6). Previously the beacon SSIDs were only
  used to label stations and then discarded, so the Wireless page's "SSIDs seen"
  and "Access points nearby" boxes (fed only by the managed-mode `wlan_scan`)
  stayed empty on a monitor-only sensor. The Wireless UI now fills both boxes
  from the sense networks ("from beacons"), and associated client stations show
  their resolved SSID (only genuine probe-only stations read "probing…").
- Capture change is in the cross-platform `processFrame`/`recordNetwork`
  (unit-tested: RSSI-max, beacon count, hidden SSID, broadcast-BSSID skip);
  `senseImpl` stamps each network with the channel it was strongest on.

## 2026-07-17 — WLAN discovery: full-spectrum first-connect sweep

- **"all" channels now really means all channels.** Empty channels on a
  `wlan_sense` test derive the list from the phy (`iw phy <phy> channels`). Two
  bugs made it silently fall back to a hardcoded 11-channel list without DFS, so
  5 GHz-DFS-only SSIDs were invisible: the channel parser read the leading `*`
  marker instead of the frequency, and phy detection matched `phy#N` while
  `iw dev <if> info` prints `wiphy N`. Fixed with `parsePhyName` (handles both)
  and a rewritten `parseIWPhyChannels` (finds the `MHz` token, keeps DFS, skips
  only `disabled`). Verified live on an MT7612U: "all" now sweeps 39 channels
  incl. DFS 100/112 and captures both `atalt-iot` and `atalt-intern`.
- **Automatic discovery on a sensor's first connect.** A monitor sensor
  (advertises `wlan_sense`) runs one full-spectrum sweep the first time it ever
  connects, so the operator can see every channel and SSID in range before
  narrowing the recurring test. Server-driven via a reserved `RUN_TEST` sentinel
  (`proto.WlanDiscoveryTestID`, no new command type); the result is stored like a
  normal `wlan_sense` result under that test id. Runs exactly once — persisted in
  a new nullable `agents.wlan_discovered_at`, guarded by an in-memory in-flight
  set so a reconnect mid-sweep can retry but a completed one never repeats. The
  agent serializes discovery against the recurring sense test on one `wlanMu` so
  they never fight over the radio.
- **Wireless page discovery panel + assisted narrowing.** A "Discovery — all
  channels" card shows every channel swept (APs, utilization, frames, SSIDs),
  highlighting the ones with activity, plus a "Use active channels for recurring
  test" button that opens the site's `wlan_sense` test prefilled with those
  channels for review and save.

## 2026-07-17 — Discovery panel filters, SSID table, security/standards

- **Band + activity filters.** The discovery panel's channel list gained
  2.4/5/6 GHz checkboxes and an "active channels only" toggle; both re-render
  from the already-fetched sweep client-side, no refetch.
- **SSID table.** "SSIDs seen" is now a table (SSID, Security, Standards,
  Band, AP count, best RSSI) instead of a chip list, aggregating each SSID
  across all the BSSIDs/bands it was heard on.
- **Beacon security + PHY standards parsing** (`internal/probe/wlansense.go`,
  `parseBeaconBody`). Reads the RSN element (AKM suites → Open/WEP/WPA2/WPA3/
  WPA2-WPA3 transition/-Enterprise/OWE, using the privacy capability bit for
  the WEP/Open split) and the WPA1 vendor element, plus HT/VHT/HE/EHT elements
  for PHY generation (`n/ac/ax/be`). New `WlanNetwork.Security`/`.Standards`
  proto fields (7/8); the synthetic demo generator was intentionally left
  alone — verification uses the real rp01 sensor.
- **Channel list: top 10 + collapse.** Rows now sort by utilization desc and
  show only the top 10 by default, with a "Show all N channels" toggle.

## 2026-07-18 — WLAN rebuild: unified test type with adaptive channel narrowing

- **Single unified `wlan_passive` test type** replaces `wlan_scan` (managed-mode)
  and `wlan_sense` (monitor-mode), inheriting the monitor-mode capability since
  that provides the superset of data. Minimum interval 60 seconds (server-side
  validation). No parameters beyond interval (channels/dwell are now managed
  adaptively by the agent).
- **Agent-side adaptive channel narrowing**: on first run, sweeps all channels
  the phy supports (full spectrum, via existing discovery sweep code path);
  from beacons + client frames heard, derives the set of "interesting" channels
  (those where activity occurred). Subsequent runs sweep only interesting channels,
  cutting scan time from ~50s to ~15s on a busy phy. State lives in per-test-ID
  agent memory; config replacement triggers a fresh full sweep. Empty interesting
  set reverts to full sweep.
- **Capability tag consolidation**: single `wlan` capability (dropped `wlan_scan`
  + `wlan_sense`), granted when agent has a real monitor-capable interface AND
  privilege, OR demo mode enabled. Capability filtering only pushes `wlan_passive`
  to agents advertising `wlan`.
- **Demo mode consolidation**: one flag `NETLAMA_WLAN_DEMO` (dropped
  `NETLAMA_WLAN_SENSE_DEMO`); demos produce synthetic `WlanPassiveResult` data.
- **Proto changes**: removed `WlanScanParams`, `WlanSenseParams` (kept as
  reserved for field numbers); added `WlanPassiveParams` (empty). Result oneof
  removed `wlan_scan` (field 10) + `wlan_sense` (field 12), added `wlan_passive`
  (field 13). Reserved field numbers prevent accidental reuse.
- **Config validation** (`internal/server/config.go`): accept only `wlan_passive`
  with interval ≥60s; removed all `wlan_scan`/`wlan_sense` handling.
- **Server cleanup**: removed discovery machinery (`maybeStartDiscovery`,
  `AgentWlanDiscovered`, `MarkWlanDiscovered`, discovering map); removed
  per-agent interface selection (`Config.wlan_interface`, proto field 2 reserved).
- **DB migration**: on startup, delete from `site_tests` and `tests` where
  `type IN ('wlan_scan', 'wlan_sense')` so old test definitions never push to
  agents. Agent columns `wlan_interface`, `wlan_discovered_at` left in place
  (dormant, never read/written).
- **Result type handling** (`internal/server/server.go`, `alerts.go`, `metrics.go`):
  all `WlanScan`/`WlanSense` cases replaced with `WlanPassive` (single case).
  Health metric = network count (> 0 is ok).
- **Wireless page rebuilt** (UI-only, no Go changes pending): when a `wlan_passive`
  test is assigned, show a networks table (SSID/BSSID/Signal/Channel/Band/Mode/
  Security/Clients/Last seen), sortable by column. Empty state if no test assigned.
- **README updated**: WLAN sections rewritten for `wlan_passive` and adaptive
  channel narrowing; demo mode consolidated to `NETLAMA_WLAN_DEMO`.
- **ROADMAP updated**: replaced 5 unchecked WLAN Phase items with 1 checked
  "WLAN rebuild: single passive test type, agent-side adaptive channel narrowing,
  WLAN capability tag, Explorer-style networks table" entry; added unchecked
  "WLAN active tests: on-demand association/throughput/auth tests against
  selected SSIDs".
- **Test changes** (`internal/server/wlan_test.go`, `server_test.go`):
  `TestWlanSenseMetricExtraction` now uses `WlanPassiveResult`; `TestWlanSenseValidation`
  validates `wlan_passive` with 60s interval; capability test constants updated to
  use "wlan" (no "wlan_scan"/"wlan_sense").
- **Verification**: `make build` passes; `make vet` clean; `go test ./...` all green.
  E2E (self-signed TLS, scratch GHCR image, tenant/site/agent via JSON API):
  agent without wlan capability doesn't receive `wlan_passive` test; agent with
  wlan capability receives test, first run scans full spectrum (~39 channels,
  ~50s), second run narrows to active channels (~15s). Results land via API with
  correct `WlanPassiveResult` shape.

## 2026-07-18 — WLAN interface override

- **Agent-side interface selection** (`-wlan-iface` / `NETLAMA_WLAN_IFACE`):
  added flag and env var to override which wireless interface the `wlan_passive`
  test uses. Useful when multiple monitor-capable adapters are present (e.g.,
  onboard wlan0 + USB MT7612U wlan1) and the agent must use a specific one.
  Empty (default) auto-picks the first monitor-capable interface as before.
  If the override is set, the agent validates that the interface exists and is
  monitor-capable, returning a result error if not. Wired in `cmd/agent/main.go`
  (flag + env), added to `Agent` struct, and validated in `internal/agent/scheduler.go`
  `runWlanPassive()`. Documented in README (WLAN passive section), both compose
  files (commented env line with description).

## 2026-07-18 — AP detail panel with vendor, width, load, roaming

- **Richer beacon parsing** (`internal/probe/wlansense.go`): the passive sweep
  now extracts per-AP channel width (HT/VHT operation IEs → 20/40/80/160 MHz),
  beacon interval, country code, BSS Load (station count + AP-reported channel
  utilization), AKM/cipher detail (e.g. "PSK+SAE · CCMP"), and 802.11k/r/v
  roaming support (RM Enabled / Mobility Domain / BSS Transition bits). New
  `WlanNetwork` proto fields 9–16; agent conversion and demo data updated;
  covered by `TestParseBeaconBodyDetails` / `TestParseBeaconBodyVHTWidth`.
- **AP vendor lookup** (`internal/oui`, `GET /api/v1/oui?macs=...`): embedded
  IEEE MA-L registry (39,765 OUIs, ~380 KB gzipped, fetched 2026-07-18)
  resolves BSSIDs and client MACs to manufacturer names server-side;
  locally-administered (randomized) MACs return unknown. Documented in
  doc/API.md.
- **Wireless UI**: clicking an AP row opens a detail panel (vendor, signal,
  channel/band/width, frequency, security + AKM/cipher, standards, roaming,
  beacon interval, country, AP load, beacons heard, last seen) plus a table of
  the clients observed on that BSSID with their vendors. Panel refreshes with
  each sweep and closes when the AP disappears.
- **Verification**: build/vet/tests green; e2e with demo agent confirms the new
  payload fields via `/api/v1/results` and vendor resolution via `/api/v1/oui`.

## 2026-07-18 — Wireless pro view: filters, stations table, MFP & more

- **More beacon detail** (`internal/probe/wlansense.go`): MFP status from RSN
  capabilities (MFPC/MFPR bits → "capable"/"required", shown as 802.11w),
  group cipher, DTIM period (TIM IE), WPS presence (Microsoft vendor IE type 4),
  spatial streams (HT RX MCS bitmask / VHT Rx MCS map), and an estimated max
  PHY rate (top-MCS short-GI per-stream table by generation × width; legacy APs
  use the highest advertised supported rate). Proto fields 17–22; demo data and
  tests (`TestParseBeaconBodyProDetails`, `TestParseBeaconBodyVHTStreamsAndLegacyRate`)
  updated.
- **Wireless UI**: SSID text filter + per-band (2.4/5/6 GHz) checkboxes on the
  networks table with filtered/total counts; new "Client stations" card listing
  every station from the sweep (MAC, vendor, network, signal, rate, MCS, frames,
  last seen; associated vs. probing in the meta line). Detail panel now shows
  group cipher, management frame protection, WPS (flagged as degraded when
  enabled), spatial streams, max PHY rate, and DTIM period; roaming amendments
  renamed to their real names — Radio Measurement (802.11k), Fast BSS
  Transition (802.11r), BSS Transition Mgmt (802.11v).
- **Verification**: build/vet/tests green; e2e demo agent payload carries
  mfp/groupCipher/dtimPeriod/streams/maxRateMbps.

## 2026-07-18 — WLAN retention, periodic full re-scan, SSID group fix

- **10-minute sighting retention** (`internal/agent/scheduler.go`
  `mergeWlanRetained`, `wlanRetention = 10 * time.Minute`): the agent keeps a
  per-test map of APs (by BSSID) and stations (by MAC) and includes everything
  heard within the last 10 minutes in each result, so briefly-faded devices
  don't flicker out of the UI. `WlanNetwork.last_seen_ms` (proto field 23) is
  stamped per beacon; stations already carried it. Covered by
  `TestMergeWlanRetained`.
- **Periodic full re-scan**: adaptive narrowing no longer locks the sweep to
  interesting channels forever — every 10 minutes the agent re-sweeps the full
  spectrum so new APs/SSIDs on other channels are discovered, then narrows
  again.
- **UI**: SSID group rows are now a pure summary (AP count, BSSID count,
  strongest signal, all channels/bands, summed clients) and expanding lists
  every AP of the SSID underneath (previously the strongest AP was merged into
  the group row and only the remaining APs appeared as children). Last-seen
  columns and the detail panel use the per-AP/station timestamp; rows not heard
  for >2 minutes are dimmed (`wl-stale`).

## 2026-07-18 — Build version tags for server and agents

- **Version stamping** (`internal/version`): `Version` is set via
  `-ldflags -X` from `git describe --tags --always --dirty` in the Makefile
  (build + pi targets), and from a `VERSION` build-arg in the Containerfile;
  the containers CI workflow passes `VERSION=git-<short-sha>`. Plain
  `go build` yields "dev". The agent's hardcoded "0.1.0" register version is
  replaced by the stamped value; the server logs its version at startup.
- **Agent version in UI/API**: the server persists the version an agent
  reports on register (`agents.version` column, `SetAgentVersion`), it's
  included in `GET /api/v1/agents`, and the Agents view shows a Version
  column. `GET /api/v1/me` returns `serverVersion`, displayed in the sidebar
  footer.
- **Note**: deployed-from-tarball builds (tpr06) must pass
  `--build-arg VERSION=...` since the source tarball has no `.git`.

## 2026-07-18 — WLAN active tests (v0.2.0)

- **New `wlan_active` test type** (`internal/probe/wlanactive*.go`): connects
  to a configured SSID for real and times every step — association,
  authentication, DHCP, optional throughput. wpa_supplicant (nl80211) drives
  the connection with its events parsed for timing (`parseWpaEvent`); DHCP is
  a full DISCOVER→ACK exchange via `insomniacslk/dhcp` (no host state
  touched) reporting leased IP, netmask and gateway; throughput pins the
  leased source address with a source-routed default in a dedicated policy
  table (host routing untouched, cleanly torn down). The radio's previous
  mode (monitor) is restored afterwards; passive sweep and active test
  serialize on `wlanMu`.
- **Security modes**: `psk` (WPA2/WPA3, `WPA-PSK SAE` + `ieee80211w=1`),
  `eap-peap` (802.1X PEAP/MSCHAPv2 with a CA certificate PEM — or explicit
  `insecureSkipVerify` to accept any EAP server), and `open`. Config values
  are escaped against wpa.conf injection (tested).
- **Result**: per-step ms (associate/authenticate/dhcp/throughput), BSSID,
  RSSI during the test, IP/netmask/gateway, Mbps, total ms, `failedStep` on
  failure. Proto oneofs 13 (params) / 14 (result); `resultTestType`,
  metrics, and overview extraction (`totalMs` series) covered.
- **Capability** `wlan_active`: any wireless interface + `wpa_supplicant`
  in PATH + privilege (or demo mode). agent-sensor image now installs
  `wpasupplicant`. Server validation: SSID required, security enum,
  credential requirements per mode, min interval 300s (the test takes the
  radio away from passive sweeps).
- **UI**: "WLAN active (connect test)" test type with dynamic form (identity/
  CA cert/skip-verify only for EAP), per-step timing summary in Results.
- **Deferred**: roaming-event observation (new ROADMAP item).

## 2026-07-18 — WLAN active on the Wireless page

- **Active connection card** on the Wireless page: when the selected agent has
  `wlan_active` results, a card appears with the latest test's summary (SSID/
  BSSID, status, IP/netmask/gateway, signal, throughput, total) and an ECharts
  step waterfall — Association → Authentication → DHCP (+ Throughput when
  measured), each bar offset by the cumulative previous steps, failed step in
  red. Hidden entirely when no wlan_active test produces results for that
  agent. Theme-toggle and resize re-render like the Path waterfall.

## 2026-07-18 — wlan_active timing accuracy

- **Investigated a reported ~16s total** on the first real `wlan_active` runs
  (rp01, SSID atalt-test: assoc ~9.3s, dhcp ~5.08s constant, total ~16.8s) —
  three measurement artifacts, not a slow WLAN:
  1. "Association" was timed from wpa_supplicant start and included the full
     SSID scan (~9s on the MT7612U). New `scanMs` field (proto 17) splits the
     scan phase (start → "Trying to associate/authenticate"); `associateMs`
     now measures only the real 802.11 exchange.
  2. DHCP's constant ~5.08s was a lost first DISCOVER plus nclient4's 5s
     retransmit default; now 1.5s timeout × 6 retries.
  3. `totalMs` included teardown and monitor-mode restore (~2.3s); it now
     spans supplicant start through the last completed step only.
- UI follow-up: scan time is payload-only (harness-internal metric) — not in
  the waterfall, card, Results summary, or the dashboard series; the card and
  the overview sparkline use connect time (assoc+auth+dhcp). `scan_ssid=1`
  added to the supplicant config (directed probes: faster, more reliable SSID
  discovery, works for hidden SSIDs).

## 2026-07-19 — WLAN active link quality + lease detail (v0.3.0)

- **More `wlan_active` metrics**: RSSI, channel noise floor and SNR
  (rssi − noise, from `iw dev <if> survey dump` in-use channel), and TX
  retransmission rate (tx retries / tx packets from `iw dev <if> station
  dump`), sampled at the end of the test so the counters reflect the
  throughput load. DNS servers pulled from the DHCP lease (option 6).
  Proto fields 18–23; agent, demo and UI updated.
- **UI**: the client address now shows in CIDR (IP + prefix derived from the
  lease netmask) and the separate Netmask row is gone; new DNS servers, SNR
  (next to signal) and TX-retransmit rows on the active card; CIDR in the
  Results summary too.
- **Throughput note**: still an HTTP GET of a configured URL pinned to the
  leased source address via the policy-routed table. For a pure wireless-link
  number, point `throughputUrl` at a LAN host (avoids WAN/server confounds);
  an in-fleet iperf3-style reflector is the gold standard and is tracked as
  the agent-to-agent perfmon roadmap item.

## 2026-07-19 — wlan_active MAC policy (permanent vs random)

- Confirmed by capture that wpa_supplicant randomized the MAC every
  association (`6a:…`, `9a:…`, never the adapter's real `40:a5:…`), so each
  active test looked like a new device — a fresh DHCP lease per run and
  AP client-table churn. Fixed the default to the permanent MAC
  (`mac_addr=0` / `preassoc_mac_addr=0`).
- Added a `macMode` param ("permanent" default, "random") so randomization
  is opt-in; the test dialog shows a warning when "random" is picked
  (consumes a DHCP lease per run, clutters the AP client table). The MAC
  actually used is captured (`/sys/class/net/<if>/address`) and shown on the
  active card. Proto param 8 / result 24.

## 2026-07-19 — Fix TX retry-rate formula, flag small samples (v0.5.0)

- **Bug**: TX retransmit rate was computed as `retries / packets`. iw's
  "tx packets" counts only successfully-ACKed frames — retries are additional
  attempts on top of those, not included in it — so dividing by packets alone
  inflates the result (e.g. 3 retries on 11 successes reported 27.3%; the
  correct rate, retries over ALL attempts, is 3/14 = 21.4%). Verified against
  a live `iw station dump` capture on rp01 to confirm the field semantics
  before fixing. New pure `txRetryPct(packets, retries)` helper (matching the
  existing parse-function pattern), unit tested with the reported case.
- **Small-sample caveat surfaced in the UI**: without a `throughputUrl`
  configured, the only traffic during the test is the DHCP handshake — about
  10-15 frames. On that few attempts a single retry swings the percentage by
  several points; the active card now shows the attempt count and a note to
  set a throughput URL for a statistically stable reading.

## 2026-07-19 — Gateway ping for a real traffic sample (v0.6.0)

- **20-ping burst to the gateway after DHCP**, always (no config needed):
  DHCP alone is only ~11-15 frames, too small for a stable TX-retransmit
  reading (see the previous entry). The ping is sourced from the leased
  address so it's guaranteed to traverse the WLAN interface; it targets the
  gateway specifically (one hop = the AP↔client link) rather than an
  internet destination, so it isolates 802.11 retry behavior from WAN
  variance. Loss % and average RTT are reported as a free bonus — a direct
  "is this AP's uplink actually usable" signal. Best-effort: a ping failure
  doesn't fail the test (unlike an explicitly-configured throughput URL,
  which still does).
- `iw station dump` (RSSI/retransmit sampling) now runs after the ping (and
  optional throughput), so the TX-retransmit sample reflects the ping
  traffic too — typically ~31-35+ frames per run instead of ~11-15.
- Proto fields 25-26 (`gateway_ping_loss_pct`, `gateway_ping_rtt_ms`); shown
  as a "Gateway ping" summary row on the active card and in the Results
  one-line summary. Small-sample threshold on the retransmit row lowered
  from 50 to 25 attempts to match the new baseline.

## 2026-07-20 — WLAN roaming analytics, Meraki-style (v0.7.0)

- **Passive detection** (`internal/agent/scheduler.go`): `mergeWlanRetained`
  now detects, per wlan_passive sweep, any tracked station whose BSSID
  changed since the last sighting (a roam) or that aged out of the 10-minute
  retention window without reappearing (a disconnect), and emits
  `WlanRoamEvent`s (proto field 7 on `WlanPassiveResult`) alongside the
  regular networks/stations. Detection reuses the existing per-station
  retention state — no new polling, no extra agent-side storage. Roam timing
  (`roamTimeMs`) is the gap between last-seen-on-origin and first-seen-on-new
  — bounded by sweep cadence (seconds), explicitly NOT the sub-100ms
  radio-handoff precision a synced AP mesh (like Meraki's) can report; this
  is a single time-sliced sensor radio.
- **Server aggregation** (`internal/store/wlanroaming.go`,
  `GET /api/v1/wlan-roaming?tenantId=&siteId=&agentId=&since=`): scans
  wlan_passive results in the window (reuses `ListResults`, no new table),
  flattens embedded roam events, and computes: classification by RSSI delta
  (better/same = good, small drop = suboptimal, big drop = bad — chosen over
  Meraki's ms-based thresholds since our timing precision doesn't support
  them honestly), ping-pong clients (A↔B bounce within 5 min), sticky clients
  (dwelling ≥5 min on a BSSID ≥10dB weaker than a same-SSID sibling — the
  dwell requirement specifically excludes a client that just bad-roamed there
  a moment ago, caught by a unit test), and per-event duration (time to the
  client's next event, or now).
- **UI**: new "Roaming" card on the Wireless page — 6 summary tiles (bad/
  suboptimal/good roams, ping-pong/sticky clients, disconnects), a
  per-client swimlane timeline (plain CSS-positioned segments/dots per AP
  row, client selectable from a dropdown ranked by activity — native
  positioning over pulling in ECharts' Gantt machinery), and an event log
  table (client, origin→new AP, roam time, RSSI before→after, band, start
  time, duration). Time range selector (24h/7d/30d).
- **Gotcha hit during e2e** (same class as the earlier `lastSeenMs` bug):
  protojson serializes the new `detectedAtMs` int64 as a JSON *string* —
  the store's parsing struct needed `json:"detectedAtMs,string"` or every
  row silently failed to unmarshal and the endpoint returned all zeros
  despite correct agent-side detection. Caught by an isolated e2e run (demo
  mode's synthetic corp-wifi station alternates BSSID every 60s specifically
  to exercise this path) before shipping; the unit test's hand-written JSON
  fixtures were fixed to quote the value too, since they weren't realistic
  enough to have caught the bug themselves.
- Demo mode: one synthetic station now alternates between the two corp-wifi
  BSSIDs every 60s, so `NETLAMA_WLAN_DEMO=1` exercises roam/ping-pong
  detection without real hardware.

## 2026-07-20 — Agent-to-agent perfmon (v0.8.0)

- **New `perfmon` test type**: measures upload/download throughput and
  connection latency to another agent. `internal/probe/perfmon.go` is a
  hand-rolled protocol over plain TCP (no iperf3 binary, no new dependency)
  — two short-lived connections, one per phase (upload then download), so
  each phase ends on an unambiguous signal (TCP half-close / full close)
  instead of a guessed timeout. An earlier single-connection design was
  caught and discarded during testing: without an explicit end-of-phase
  signal, a 1s test actually took 4s (2s wasted margin per phase, waiting
  out a timeout) — the two-connection redesign brought a 1s+1s test back to
  a real 2s.
- **Opt-in reflector**: any agent can listen with `-perfmon-port` /
  `NETLAMA_PERFMON_PORT` (default disabled) — started once for the agent's
  lifetime in `Agent.Run()`, not tied to the interval-scheduled test model
  (a persistent listener doesn't fit "sample every N seconds"). Reported as
  the `perfmon_reflector` capability (badge on the Agents page); the
  always-available client-side `perfmon` capability was added alongside
  ping/dns/http/tcp/speedtest.
- **No discovery, no NAT traversal — by design**: net-lama agents dial out
  only (never dialed into, see CLAUDE.md), so true peer discovery isn't
  architecturally possible without a relay. The test target is a plain
  host:port typed by the operator, exactly like every other test type's
  target already works; reachability is the operator's problem, same as
  ping/tcp/traceroute.
- Server: `PerfmonParams`/validation (target must parse as host:port,
  durationSeconds 1-30 default 5, interval ≥60s), Prometheus gauges
  (`perfmon_{download,upload}_mbps`, `perfmon_latency_ms`), overview
  primary-metric/series extraction (Mbps, via the existing `extractNested`
  helper). UI: new test type with target/duration fields, results summary,
  reflector capability badge.
- **Verification**: real loopback unit test (`TestPerfmonLoopback`, actual
  TCP on 127.0.0.1) plus a two-agent e2e run (separate client + reflector
  agent processes) — confirmed capability reporting, config validation
  (rejects malformed target and <60s interval), a genuine result end to end,
  and correct overview aggregation. No mocking of the protocol itself.

## 2026-07-20 — Perfmon: pinned source agent, cross-site dropdowns (v0.9.0)

- **v0.8.0's perfmon was tenant-wide site-scheduled like every other test
  type — wrong shape**: measuring throughput FROM one agent TO another only
  makes sense pinned to a single source agent, and the user wanted source
  and destination picked from a dropdown of capable agents, not typed as a
  raw target string, spanning sites.
- **Single-source pinning without a schema redesign**: `PerfmonParams`
  gained `sourceAgentId`; existing site-based scheduling is left as-is, and
  `ConfigForAgent` silently skips a `perfmon` test on every agent of the
  site except the pinned source (`isPerfmonSource` in
  `internal/server/server.go`) — a normal, expected skip, not a capability
  gap worth a warning. `internal/api/sites.go` validates the source agent
  exists and belongs to the tenant on both create and update.
- **Agent advertised address**: the server has never needed to know how to
  reach an agent (agents dial out only, see CLAUDE.md) — perfmon's
  destination dropdown needs exactly that. Added `-perfmon-advertise-host` /
  `NETLAMA_PERFMON_ADVERTISE_HOST`, explicitly declared, never
  auto-detected (guessing would silently fail across NAT); reported in the
  `Register` message (`perfmon_addr` field 7) and stored on `store.Agent`.
  An agent with the reflector on but no advertise host set doesn't appear
  as a destination.
- **UI**: `perfmon` test form now has source/destination `<select>`
  dropdowns (`internal/web/static/index.html`, `app.js`) instead of a
  free-text target field — source lists every `perfmon`-capable agent,
  destination is filtered to `perfmon_reflector` agents with a non-empty
  advertised address, both labeled with site name. On save, the test is
  auto-assigned to the source agent's site (`assignPerfmonTestToSite`,
  merges into `PUT /api/v1/sites/{id}/tests`, and un-assigns from any other
  site that had it — covers editing a test to change its source agent). The
  Agents page's "Perfmon reflector" capability badge now shows the
  advertised address next to it.
- **Verification**: unit test
  (`TestConfigForAgent_PerfmonPinnedToSourceAgent`) confirms only the
  pinned source agent of a site receives the test config, not other agents
  of the same site. Full two-agent, cross-site e2e run (local server +
  two real agent processes, source in site A, reflector+advertise-host in
  site B) confirmed: `perfmonAddr` registered and resolved correctly, the
  test ran only on the source agent, the destination agent never received
  a test config, and a genuine throughput/latency result landed via the
  results API.

## 2026-07-20 — Perfmon reflector: server-pushed config + ACL (v0.10.0)

- **v0.9.0's reflector still needed a redeploy**: `-perfmon-port` /
  `-perfmon-advertise-host` were static agent startup flags — turning the
  reflector on for a test meant editing compose/env and restarting the
  agent process. User: "I don't want to redeploy an agent just because I
  want to do iperf."
- **Found a real vulnerability while answering "does enabling this
  everywhere hurt?"**: the reflector protocol has no authentication beyond
  a fixed 4-byte handshake, and the wire-level phase duration
  (`readPhaseHeader`) was never clamped server-side — only the *client*
  test config enforced 1-30s (`ValidateTestDef`). A raw TCP peer that skips
  the API entirely could request a `download` phase with a `uint32` max
  duration (~136 years) and tie up the reflector indefinitely. Fixed
  independently of the redesign: `handleReflectorConn` now rejects any
  phase duration outside 1-30s before honoring it
  (`internal/probe/perfmon.go`).
- **Reflector moved from a static flag to server-pushed config**: new
  `Config.perfmon_reflector` proto field (`PerfmonReflectorConfig`:
  enabled/port/allowed_cidrs), always included in every config push so a
  push can enable, disable, or reconfigure the reflector — the agent
  reconciles it in `reconcilePerfmonReflector`, tied to the *process*
  context (not the current connection's), so a reconnect blip doesn't tear
  down and restart it. Verified live: enabling/disabling via
  `PUT /api/v1/agents/{id}` on an already-connected local agent took effect
  with no restart, both directions.
- **ACL replaces "no auth at all"**: `allowed_cidrs` is a source-IP
  allowlist the *agent* enforces on every accepted connection
  (`probe.Reflector`'s `connAllowed`), before the handshake even starts.
  Empty list = reject everyone, even with the reflector enabled — turning
  it on with no allowlist configured listens but serves no one, the safe
  default. Bare IPs get an implicit /32 or /128
  (`probe.ParseCIDRs`), shared verbatim between the API's validation and
  the agent's enforcement so the two can't disagree on what an entry means.
- **Register.perfmon_addr removed**: the agent no longer self-reports
  anything about its reflector. The operator configures it centrally
  (Agents page → Edit → Perfmon reflector) and the server computes+stores
  the derived `perfmonAddr` itself (`Store.SetAgentPerfmonReflector`) —
  consistent with the project's "agents dial out only, server never guesses
  reachability" stance, just relocating who declares it from the agent
  flag to the operator's UI input.
- **Verification**: unit test for the ACL (`TestPerfmonReflectorACL` —
  empty allowlist rejects a loopback connection, a bare-IP entry admits it)
  plus a full local e2e run: created two agents with **no perfmon flags at
  all**, enabled the destination's reflector via the API while already
  connected (confirmed listening, no restart), ran a real perfmon test
  between them (genuine throughput measured), then disabled the reflector
  live and confirmed the port actually closed (`nc -z` failed as expected).

## 2026-07-20 — Per-agent interface pickers, no more typed IPs (v0.11.0)

- **Same day, third perfmon iteration**: user wanted to pick the perfmon
  reflector's advertised address from a dropdown of the agent's actual
  interfaces instead of typing an IP, and while designing that realized the
  existing WLAN-sensor-interface override (`-wlan-iface` flag) and a new
  purely-informational "management interface" concept fit the same shape —
  three interface-role pickers, one shared inventory.
- Clarified with the user first: "management interface" is
  **informational only** (shown as the agent's primary IP in the UI),
  not a routing/dial-binding change — kept the change contained instead of
  guessing into a bigger networking feature.
- **New unified interface inventory** (`internal/probe/netiface.go`,
  `NetworkInterfaces`): enumerates every non-loopback interface via
  `net.Interfaces()` (stdlib, cross-platform), merges in wireless-specific
  detail (monitor-mode support) from the existing `iw`-based
  `WirelessInterfaces` by name — reused rather than reimplemented — and
  adds wired link speed by reading `/sys/class/net/<iface>/speed` (Linux
  only; a plain file read that just returns 0 elsewhere, no build-tag split
  needed). Reported at Register (`Register.network_interfaces`, replacing
  the old wireless-only `WirelessInterface`/`wireless_interfaces` field).
- **Management and perfmon-reflector addresses are resolved, never
  stored**: `Agent.ResolvedManagementAddr`/`ResolvedPerfmonAddr` look up the
  picked interface's *current* IP from the latest reported inventory on
  every API read, rather than persisting a derived value that could go
  stale between re-registers. `perfmonAddr` in the API response is exactly
  this — the operator picks an interface name, the server does the
  IP lookup, no manual typing.
- **WLAN sensor interface moved from a startup flag to live-pushed config**,
  the same treatment perfmon's reflector got earlier today: removed
  `-wlan-iface`/`NETLAMA_WLAN_IFACE`; `Config.wlan_sensor_interface` is
  now pushed on every config push and applied via
  `Agent.setWlanIface`/`wlanIface()` (mutex-guarded, since it can now
  change mid-connection, unlike the old start-once flag).
  `runWlanPassive`/`runWlanActive` read it the same way as before, just
  from the new accessor instead of a static field.
- **Reused two orphaned DB columns instead of migrating**: `wlan_interface`
  (added by an earlier, never-completed design — confirmed unused by any
  Go code before reusing it) now holds the WLAN sensor pick, and the
  existing `wireless_interfaces` column now holds the richer wired+wireless
  JSON — no schema-breaking migration needed for either. Two genuinely new
  columns (`management_interface`, `perfmon_reflector_interface`) added the
  normal way. The previous design's `perfmon_addr`/`perfmon_advertise_host`
  columns are left in place, unused (matches the project's no-destructive-
  migration pattern used twice already this session).
- **UI**: Agents page → Edit gained three interface `<select>` pickers
  (management, WLAN sensor, perfmon reflector), each option labeled with
  link speed or wireless/sensor-capability plus current IP (or "no IP").
  The old free-text "Advertise host" field is gone. Agents table now shows
  the resolved management IP next to the agent name.
- **Verification**: full local e2e run — created an agent with zero
  perfmon/wlan flags, confirmed its real `networkInterfaces` inventory
  came back correctly (including a live IP on the machine's actual
  interface), then picked that interface for both management and perfmon
  reflector via the API while the agent was already connected and
  confirmed both `managementAddr` and `perfmonAddr` resolved to the
  correct IP with no restart, and the reflector started listening live.

## 2026-07-20 — Drop management-interface picker, declutter capability badges, fix version-string race (v0.11.1)

- **Management interface picker removed, same day it shipped**: user tried
  it and decided picking one was pointless busywork for a value that's
  purely informational anyway. `Agent.ResolvedManagementAddr` now
  auto-derives it — first wired interface with a current IP, falling back
  to wireless — instead of reading an operator-picked field. Deleted the
  now-dead `ManagementInterface` field/column read, `SetAgentManagementInterface`,
  the `managementInterface` API field, and the `<select>` in the Agents
  edit dialog (replaced with plain read-only text). `management_interface`
  DB column left in place unused, same no-destructive-migration pattern as
  `perfmon_addr`/`perfmon_advertise_host`.
- **Perfmon reflector port field now pre-fills 5252** (the existing
  placeholder value) instead of opening blank, so enabling the reflector
  doesn't require typing a port from memory.
- **Agents table capability badges decluttered**: `ping`/`dns`/`http`/`tcp`/
  `speedtest`/`traceroute`/`perfmon` are baseline — every agent has them,
  so they're filtered out of the per-agent badge list now (display-only
  change in `loadAgents()`; `capabilityWarnings()`, which drives the
  site/test capability-mismatch warnings, reads the raw `a.capabilities`
  array directly and is unaffected). Only `WLAN`/`WLAN active` show as
  badges now; perfmon reflector state already had its own badge driven by
  the operator setting, not the capabilities array.
- **Fixed the `latest` GHCR tag version-string race**: a `main` push and a
  `vX.Y.Z` tag push for the same commit both trigger the container build
  workflow and both write `:latest` — whichever run finished last decided
  the image's baked-in `VERSION` build-arg, and the branch-push run always
  computed `git-<sha>` regardless of what tag pointed at that commit. So a
  tagged release could end up on `:latest` self-reporting `git-<sha>`
  instead of its semver tag — cosmetic (the image's revision label, and
  the code, were always correct) but confusing. Fixed at the root: the
  workflow now computes `VERSION` via `git describe --tags --always --dirty`
  (same as the local Makefile) instead of branching on which trigger
  fired, so every run for a given commit agrees on the version string
  regardless of which one wins the `:latest` race. Needed `fetch-depth: 0`
  on checkout since `git describe` needs tag history a shallow clone
  doesn't have.

## 2026-07-22 — Path history window selector

- **Path page heatmap can now query by time window** instead of a fixed
  48-run window: a `<select>` next to "Path history" offers Last 48 runs
  (the old default, `limit=48`) or Last 24 hours / 7 days / 30 days, which
  switch the `/api/v1/results` query to `since=<RFC3339>` with `limit=2000`
  — the same `since` param the Results page timeline already used, no API
  change needed.

## Known issues

- The agent logs "Registered with server" right after *sending* the register
  message, before the server accepts it — a rejected agent briefly logs
  success. Pre-existing, not yet fixed.
- **Older deployed agents** (pre-rebuild binaries) will not understand
  `wlan_passive` tests and must be updated to the new build.
