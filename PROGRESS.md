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

## 2026-07-22 — Unclaimed agent state

- **A new device can now self-enroll without a pre-created agent record.**
  Previously an admin had to create the agent's row (name + site chosen up
  front) before the physical device could connect at all. Now the Agents
  page has an "Enrollment code" button generating a per-tenant token
  (`nle_...`, same random-token machinery as API keys' `nlk_...`); any
  number of devices can start with that one code (`-token <code>`) instead
  of a per-agent token and show up under a new "Pending enrollment" table
  until an admin claims one (name + site), at which point it becomes a
  completely normal agent with its own fresh token — the manual
  create-agent-first flow keeps working unchanged alongside this.
- **New `unclaimed_agents` table**, not a nullable `agents.site_id`:
  `site_id` is `NOT NULL` with `ON DELETE CASCADE` and every agent read
  goes through an `INNER JOIN` on it, so nullifying it would have meant a
  full SQLite table rebuild. A separate table keyed by
  `(tenant_id, client_id)` — agents already send a stable `client_id`,
  defaulting to hostname — needed zero changes to the `agents` schema.
  `ControlStream` (`internal/server/server.go`) gained one branch: a
  token that doesn't match any agent is now also checked against
  `tenants.enroll_token`; a match upserts the pending row and rejects the
  stream with `FailedPrecondition` — the agent's own reconnect/backoff
  loop (1s→30s) becomes the enrollment heartbeat for free, no agent-side
  changes needed anywhere, and no proto changes either (the existing
  `Register` message already carries everything).
- Claiming (`POST /api/v1/agents/unclaimed/{id}/claim`) runs the exact
  same `CreateAgent` path as the manual flow — a fresh unique token, shown
  once via the same token dialog — and carries over the
  capabilities/interfaces/version already reported while pending, so a
  freshly claimed agent doesn't show blank fields until its next
  reconnect.
- **Known limitation, by design**: self-enrollment only works with mTLS
  off. With `NETLAMA_MTLS=1` the gRPC listener requires a CA-signed
  client cert at the TLS handshake layer, before `ControlStream` ever
  runs — an unclaimed device has no cert yet. Solving that would need a
  second, provisional-cert PKI flow; out of scope here, documented in the
  README instead.

## 2026-07-27 — DNS server discovery

- The agent's `-server`/`NETLAMA_SERVER` now defaults to `auto`: it looks
  the server up in DNS the way a Cisco access point finds its controller —
  SRV `_netlama._tcp` first (host *and* port, resolver-sorted by
  priority/weight so extra records are free failover), then A `net-lama`
  on port 50051. Both names are looked up unqualified so `/etc/resolv.conf`
  search domains apply; the A lookup also picks up an `/etc/hosts` entry.
- Resolution happens per connection attempt inside the reconnect loop, not
  once at startup, so an agent that boots before its DNS entry exists finds
  the server on a later retry, and moving the server is a DNS change only.
  With nothing in DNS the agent falls back to `localhost:50051`, the old
  default, so no existing deployment changes behaviour.
- **Security note**: DNS now decides where the token is sent, so the agent
  warns at startup when discovery runs with TLS off or `-tls-insecure` set —
  a spoofed DNS answer could otherwise steer it into a token-harvesting
  server. It warns rather than refuses, since self-signed setups are the
  common case and a hard refusal would just get discovery turned off.
- Half of the "zero-touch enrollment" roadmap item; the WireGuard tunnel
  part is still open.

## 2026-07-27 — Per-test result retention

- The stored-result cap was 5000 rows **per agent**, shared across every test
  that agent runs, so a frequent test starved the slower ones. Measured on the
  live deployment before the fix: all three agents sat exactly at the cap, and
  on rp02-sensor the 1-minute DNS and ping tests held 4181 of the 5000 slots,
  leaving the hourly speedtest **35 samples** and every agent under ~10 hours
  of total history — far short of what the 7-day path-history selector and
  multi-day alert baselines query.
- The prune in `AddResult` is now scoped to `agent_id` **and** `test_id`, so
  each test keeps its own 5000 rows and no test can evict another's. Added
  `idx_results_agent_test` on `(agent_id, test_id, id DESC)` to serve it —
  this DELETE runs on every single result insert, and it now scans one test's
  rows instead of all of an agent's.
- Regression test `TestResultPruneIsPerTest` (verified to fail against the old
  per-agent query). Existing rows are not backfilled: history already evicted
  is gone, the fix stops further loss.
- Follow-up left on the roadmap: time-based retention for a database-size
  guarantee, since a row cap now scales with the number of tests per agent.

## 2026-07-29 — Native agent packages

- **`.deb` and `.rpm` for the agent**, for amd64, arm64 and armv7. The sensor
  path already needs host networking and raw sockets, so the container adds a
  runtime to babysit and no isolation; a package installs the binary to
  `/usr/bin/netlama-agent`, the config to `/etc/netlama/agent.env` (0600,
  conffile — upgrades keep it) and a systemd unit, and pulls in `mtr`, `iw`,
  `iproute2` and `wpa_supplicant` as weak dependencies.
- Built by `make pkg` via [nfpm](https://github.com/goreleaser/nfpm) run
  through `go run`, so no local install and no root. Config in
  `packaging/nfpm.yaml`; the binary path cannot be an environment variable
  there (nfpm resolves content globs before expanding them), so the Makefile
  stages each architecture at `dist/build/netlama-agent`.
- `postinstall` enables the unit and starts it only once a token is present,
  restarting instead on an upgrade; `preremove` stops/disables it only on a
  real removal, not on an upgrade.
- New workflow `.github/workflows/packages.yml` builds them on every push and
  attaches them to the GitHub release on a `v*` tag.
- Verified in containers: `dpkg -i`/`dpkg -r` on Debian 12 and `rpm -i`/`rpm -e`
  on Fedora 41 (correct layout, agent binary runs), plus `systemd-analyze
  verify` clean on the installed unit.

## 2026-08-03 — Tenant scoping as a seam, not a habit

- **One place now decides tenant access** (`internal/api/scope.go`). Handlers
  used to fetch a row by ID and then compare `TenantID` themselves — three
  different idioms across ~20 call sites, each an independent chance to forget.
  Two generic helpers replace all of them: `scoped()` for the resource a route
  addresses (a path `{id}`), `inTenant()` for IDs referenced from a request
  body (`siteId`, `testId`, `targetIds`). `store.Tenanted` (`TenantOf()`) is
  what makes them generic over agents, sites, tests, rules, targets and
  unclaimed agents.
- **Cross-tenant IDs now return `404`, not `403`.** A `403` confirms the ID
  exists, letting one tenant probe for another's agents and tests. This is the
  rule `DELETE /api/v1/apikeys/{id}` already followed; the tenant-scoped routes
  now match it. `doc/API.md` updated.
- **First tests in `internal/api`** (the package had none):
  `TestCrossTenantIDsAreNotFound` drives every `{id}` route with another
  tenant's row through the real mux, with `TestOwnTenantIDsAreReachable` as the
  positive control so it can't pass vacuously, plus body-referenced IDs, list
  scoping, and unit tests for `tenantScope`/`tenantFilter`.
- **`TestEveryIDRouteIsScoped` keeps the seam from eroding**: it reads the route
  table in `api.go` and fails when a route takes an `{id}` without a
  cross-tenant case in the table.
- `handleListAgents` and `handleListUnclaimedAgents` had a third, hand-rolled
  scoping idiom; both now use the named `tenantFilter` (admin + no `tenantId` =
  all tenants), leaving `tenantScope` for routes that need exactly one tenant.
- No behaviour change for the UI, which only ever checks `res.ok`. Dead
  `canAccessAgent` removed.

## 2026-08-03 — A test type is a module, not a string

- **New `internal/testtype` registry.** "Test type" was the most load-bearing
  concept in the system and had no home: eleven `switch` statements across five
  Go files plus six spots in `app.js` each re-stated what a type means. One
  `Spec` per type now holds its capability, unit, threshold direction, primary
  metric, alert metrics and per-target alert subject. Adding a type is an entry
  in `specs.go` plus its proto message and probe.
- **Fixed: the two copies had drifted.** The alert engine scored a
  `wlan_passive` run by *how many networks it heard*
  (`len(WlanPassive.Networks)`) while the dashboard plotted *max channel
  utilization* — so one `%` threshold meant two different things and nothing
  failed. Utilization is now the single answer.
- **Fixed: `perfmon` and `wlan_active` were missing** from the alert engine's
  metric extraction, so `state` rules on those tests were permanently green.
  Both are registered.
- **Fixed: the UI's `METRIC_APPLICABILITY` map** was a fourth copy and out of
  date — it hid `perfmon` and `wlan_active` alert rules from the rule picker,
  and `wlan_passive`'s utilization metric wasn't selectable at all. The
  condition dropdown is now filled from the registry.
- **The store no longer parses results as `map[string]interface{}`.** Results
  were written as protojson and read back by groping for field names by string
  (~150 lines of nine near-identical `extract*Metric` helpers, and an in-repo
  admission that it was a placeholder). `testtype.EncodeResult`/`DecodeResult`
  own both directions, so a proto field rename is a compile error instead of a
  silently blank sparkline.
- **New `GET /api/v1/test-types`** serves the registry; the browser drives its
  unit labels, capability warnings, threshold-band direction and alert-metric
  dropdown from it instead of its own hardcoded maps. The API's `validMetrics`
  is derived from the registry too.
- **Tests**: `TestEveryResultVariantIsRegistered` walks the `TestResult` oneof
  by proto reflection, so adding a result variant without registering its type
  fails there rather than storing results as `"unknown"`; plus spec
  completeness, the pinned primary metric per type, the wlan_passive drift
  regression, metric applicability, and an encode/decode round trip.
- Deliberately left alone: `resultOK` and the Prometheus `Metrics.Record`
  switch (one switch each, no duplication to concentrate), and
  `ValidateTestDef`/`TestSpec` in `internal/server/config.go` (they work on
  `store.TestDef`, and the registry stays store-free so `internal/store` can
  import it).
- **Known wrinkle:** `perfmon` measures throughput like `speedtest` and
  arguably wants `lowerIsWorse` too, but it has always been evaluated
  higher-is-worse. Flipping it would silently invert every existing perfmon
  threshold, so it stays as-is with a note in `specs.go` until someone decides
  to migrate them.
- Verified end to end against a local server: registry endpoint, results stored
  with the right type, dashboard series/unit/current, and a `state` rule firing
  off the registry's primary metric.

## 2026-08-04 — Test parameters belong to the test type

- **The two switches left in `internal/server/config.go` are gone.**
  `ValidateTestDef` and `TestSpec` each had one case per type — ~220 lines
  restating which parameters a type takes, what it defaults to, how rare it
  must run, and which proto oneof it becomes. Both are now generic: look the
  type up in the registry, decode its params, validate, apply. `config.go` went
  from 388 lines to 89.
- **New `testtype.Params`** (`internal/testtype/params.go`): each type's stored
  parameter payload validates itself (`Validate()`) and turns itself into the
  spec pushed to an agent (`Apply(*pb.TestSpec)`). `Spec` gained `NewParams`
  and `MinIntervalSeconds`; `register()` panics on a type that forgets the
  former and defaults the latter to the global 5s floor. The per-type interval
  minimums (speedtest/perfmon/wlan_passive 60s, traceroute 30s, wlan_active
  300s) are data next to the type now, not `if`s in the validator.
- The registry still knows nothing about `store.TestDef` — it owns the payload,
  not the row it is stored in — so `internal/store` can keep importing it.
- **Fixed: three hardcoded copies of "speedtest is the lower-is-worse type"**
  (`config.go`'s threshold check, `readThresholdBands()` and `buildSeries()` in
  `app.js`) that all bypassed the registry both sides already read this fact
  from. Adding a second lower-is-worse type would have had threshold
  validation, the band editor and the chart disagreeing with
  `computeResultState`. All three now read `LowerIsWorse`/`lowerIsWorse`.
- **Tests**: `TestEveryTypeParamsRoundTrip` drives every registered type
  through decode → validate → re-encode → apply and fails if `Apply` sets no
  oneof (which would push an agent a spec its dispatch silently drops); a new
  type fails there until it gets a payload in the table.
- Deliberately left alone: the Prometheus `Metrics.Record` switch (driving it
  from the registry means either `internal/store` pulling in Prometheus, or
  renaming every exported series) and `probe.DetectCapabilities` (environment
  detection — `mtr`, monitor mode, `wpa_supplicant` — not a copy of the type
  list).
- Verified end to end against a local server: every validation rule still
  accepts and rejects as before (including the direction-dependent thresholds
  and per-type interval floors), params store normalized, and an agent
  connected over TLS received its config and ran the pushed tests.

## 2026-08-04 — The parts of the server no test could reach

Two pieces of `internal/server` were only reachable through a live gRPC
stream, so neither had a test that called them. Both got a seam a test can
cross, and the tests that had been *simulating* them now drive the real code.

- **`registerAgent` split out of `ControlStream`.** The 180-line stream
  handler opened with the security-relevant half of the server — token check,
  self-enrollment fallback, the per-agent mTLS bind (cert CN must equal the
  agent name), and three writes recording what the agent claims about itself.
  That is now `registerAgent(register, peerCN) (*agentSession, error)`: no
  stream, so a test can drive it against a real store. `ControlStream` keeps
  the registry, metrics, config push and select loop.
- **`TestRegisterAgentAuth`, `TestRegisterAgentEnrollToken` and
  `TestRegisterAgentStoresSelfReport`** are the first tests of any of it:
  missing/empty/unknown token, a valid token with another agent's certificate
  CN, an enroll token recording an unclaimed device instead of opening a
  stream, and the legacy capability list not overwriting the store.
  `TestControlStream_LegacyCapabilitiesNotStored` had a `record := func(...)`
  closure copying the condition out of `ControlStream` — deleted, the real
  function is called now.
- **The alert hysteresis is a pure function.** `decideAlert(state, rule,
  breach, value) (state, action)` holds the whole fire/dead-band/clear
  sequence with no locks, no store and no notifications;
  `evaluateAlerts` reads the state, calls it, writes the state back, and does
  the I/O the action asks for. `checkClearCondition` lost its unused `*Server`
  receiver.
- **`TestHysteresisStateMachine` was asserting against its own copy** of the
  counter logic — and the copy had already drifted from the real function
  (it only cleared the breach counter on resolve, while `evaluateAlerts`
  clears it on any non-breaching sample). It now drives `decideAlert` through
  the sequence, plus two new cases: a resolve must leave the zero state (which
  is what lets `evaluateAlerts` delete the key instead of growing one entry per
  rule|agent|subject forever) and a rule with no clear threshold.
- The two hysteresis maps (`breachCount`/`goodCount`, one mutex between them)
  collapsed into one `alertStates map[string]alertState`.
- Both new tests were checked by mutation: breaking the dead-band reset fails
  `TestHysteresisStateMachine`, and disabling the mTLS CN check fails
  `TestRegisterAgentAuth`.
- Verified end to end against a local server: an agent registered over TLS and
  got its config, a `latency_ms > 0` rule fired on the first result, and
  raising the rule's threshold resolved it on the next one.

## 2026-08-04 — A test type is one entry in the browser too

- **`UI_TYPES` in `app.js`** is the browser's half of the test-type registry:
  per type, its result payload key, params summary, form write/read, result
  detail line, and the numbers its timeline plots. The same nine types had been
  listed in six separate functions (`paramsSummary`, `updateTestParamFields`,
  the `openTestDialog` field population, the test-form submit handler,
  `resultDetails`, `buildSeries`) — miss one when adding a type and the UI
  renders blanks rather than failing.
- All six now look the type up: `updateTestParamFields` toggles the
  `#t-params-*` groups by iterating the table, `openTestDialog` calls every
  type's `write()` (the edited type with its stored params, the rest with
  `{}`, so a hidden field can't carry another type's leftovers), the submit
  handler is one `read()`, `resultDetails` matches on the payload key, and
  `buildSeries` asks the type for its points and unit.
- **`TestEveryTestTypeHasUIEntry`** (`internal/web`) reads the table out of
  `app.js` and fails if a registered type has no entry, or an entry names a
  type the server doesn't register — the same guard idea as
  `TestEveryResultVariantIsRegistered` and `TestEveryIDRouteIsScoped`.
- **`handleListLogs` no longer hand-rolls tenant scoping.** It had its own
  admin/non-admin if-else beside `scope.go` — the one file that is supposed to
  decide this — because server logs carry no tenant. It calls `tenantFilter`
  now and drops the filter only for `source=server`. A tenant user asking for
  another tenant's logs gets 403 instead of being silently rescoped.
- **`TestLogScoping`** covers all six combinations (tenant user's own logs,
  another tenant refused, server logs refused, admin sees all, admin filters to
  one tenant, admin gets server logs despite a tenant filter), and `/api/v1/logs`
  joined `TestListEndpointsAreTenantScoped`.
- Verified against a local server seeded with all nine test types and a
  demo-mode agent: every type's stored params round-trip through its
  `write()`/`read()`, and every stored result renders a detail line and its
  expected series points.
- Deliberately left alone: the Wireless and Path pages naming `wlan_passive`
  and `traceroute` in their result filters (that is what those pages are), and
  the agent list's `baselineCaps` (a curated "which capabilities are worth a
  badge" judgement, not a copy of the type list).

## 2026-08-04 — One shape for a probe

- **`probe.Sense` returns a `*SenseResult`**, not six positional values
  (`(string, []WlanStation, []WlanChannelStat, []WlanNetwork, uint32, error)`)
  — the widest interface in the package, where every caller and every one of
  the three implementations (Linux, non-Linux stub, demo) had to agree on the
  order, and every early error return spelled out `nil, nil, nil, 0`.
  `wlanPassiveResult` takes the sweep too, so its parameter list drops from six
  to four.
- **`probe.DNSQuery` returns an error like its siblings** — but a real one, not
  a nil placeholder: a failed lookup is still a measurement (`Success=false`),
  while an abandoned run (ctx cancelled — agent shutting down, or its config
  changed) has no measurement to report. The scheduler's `if ctx.Err() != nil`
  after the call becomes an ordinary `if err != nil`.
- Deleted `WlanSenseDemo`, a struct nothing used and a near-duplicate of what
  `SenseResult` now is.
- **Tests**: `TestDNSQueryAbandonedRun` pins the one thing `DNSQuery` reports
  as an error (`internal/probe/dns.go` had no test file before);
  `TestWlanSenseDemoMode` now reads the sweep through the struct.
- Verified: builds for darwin and linux, and against a local server an agent in
  WLAN demo mode reported a sweep (5 networks, 8 stations, interface off the
  result) and DNS results with real resolve times.

## 2026-08-04 — SaaS / cloud service tests

- **New `saas` test type**: one test row is one online service. Eight
  services ship in the catalog (`internal/saas`): Microsoft Teams, Microsoft
  365, Webex, Zoom, Google Workspace, AWS, Azure, Google Cloud. Design and
  the decisions behind it are in [doc/plan-saas-tests.md](doc/plan-saas-tests.md).
- **The catalog lives on the server**, not the agent: the stored test carries
  only `{"service": "ms-teams"}`, and `SaasParams.Apply` expands it into the
  pushed `TestSpec` on every config push. Adding or fixing a service is a
  server release — no fleet rollout, the pain the wlan_passive rebuild
  caused. Service ids are permanent (stored tests reference them);
  `TestKnownServiceIDsSurvive` makes deleting one a deliberate act.
- **Endpoint kind is a correctness decision, not a style one.** `resultOK`
  counts an HTTP result OK only on 2xx/3xx, and cloud API front doors answer
  401/403/400 to an unauthenticated GET *correctly* — as `https` entries they
  would have been red forever. So: `https` for user-facing front doors,
  `tcp` for machine APIs (`portal.azure.com:443`, `management.azure.com:443`,
  `storage.googleapis.com:443`, `ec2.amazonaws.com:443`). Every endpoint was
  verified live and against the vendor's own docs — Microsoft's
  machine-readable endpoint feed, Webex's network requirements, Zoom's
  firewall article, Google Workspace's firewall settings.
- **A result's test type now comes from its test definition**, not from the
  payload shape (`testtype.TypeOf` is the fallback for orphaned results).
  saas reuses `HttpResult`/`TcpResult` rather than duplicating their fields,
  so the old rule would have filed every saas result as http and hidden it
  from `?type=saas`. See
  [docs/adr/0001-test-type-from-definition.md](docs/adr/0001-test-type-from-definition.md).
  `handleResult` does the one lookup and hands the definition to
  `evaluateAlerts`, which was querying it again anyway.
- Agent: `runSaas` walks the endpoints sequentially, reusing `runHTTP` and
  `runTCP` — no new probe code. New `saas` capability, reported by every
  agent (needs nothing beyond outbound TCP), so **agents must be updated once**
  before saas tests reach them; after that the catalog is server-only.
  60s interval floor, matching speedtest.
- UI: Service dropdown on the test dialog listing what the service checks,
  fed by the new `GET /api/v1/saas-services`. Results land on the existing
  Results page; the chart series builder learned to read two payload keys
  for the one type that emits both.
- Not built: UDP/STUN media checks (Microsoft publishes Teams media as IP
  ranges with no hostnames, so there is nothing to name — a green saas
  result means sign-in and signalling work, not that a call will),
  per-service rollup verdicts, operator-editable endpoints, vendor feed
  refresh.
- **Tests**: `internal/saas` catalog well-formedness + id permanence;
  `TestHandleResultTypeFromDefinition` pins the type-origin rule including
  the orphaned-result fallback; the registry round-trip test covers the new
  params.
- Verified end to end against a local self-signed server: catalog served,
  test created, a 10s interval and an unknown service both rejected, an agent
  reporting the `saas` capability ran ms-teams (3 https results with real
  timings and cert expiry) and azure (1 https + 2 tcp results), all stored
  under `testType: "saas"` and all OK — including the API hosts that would
  have failed as https. "Run now" triggers a saas run.

## 2026-08-05 — SaaS latency reads TTFB, dashboard tests follow the site filter

- **`saas` primary metric is now time to first byte** (https endpoints;
  tcp keeps connect time). First live results sat above 500 ms and the
  cause was page weight, not the network: a vendor front door redirects
  1-3 times and serves a few hundred KB, and `probe.HTTPCheck` drains up
  to 1 MiB so the transfer is real (measured: `teams.microsoft.com`
  ttfb 304 ms / total 509 ms over 3 redirects and ~230 KB). A threshold on
  the total would fire when Microsoft reworks a landing page. The total is
  still recorded in every result and exposed as the new `total_ms` alert
  metric; the results chart plots TTFB to match.
- **The dashboard's Tests table honours the site filter.** `TenantOverview`
  listed every tenant test regardless of `siteID`, so selecting a site
  showed the other sites' tests as permanent "No data" rows (only their own
  site's agents run them). It now uses `TestsForSite`, and the Tests tile
  counts the same set.
- **Tests**: `TestSaasPrimaryIsTimeToFirstByte` (it cannot join
  `TestPrimaryMetrics`, which also asserts `OfResult` round-trips the
  payload — saas payloads are http/tcp on purpose);
  `TestOverviewTestsScopedToSite`, verified to fail before the fix.

## 2026-08-05 — Traceroute Phase 2, stage 1: a native engine replaces mtr

Design and the decisions behind it: [doc/plan-traceroute-phase2.md](doc/plan-traceroute-phase2.md)
and [docs/adr/0002-native-traceroute-engine.md](docs/adr/0002-native-traceroute-engine.md).

- **The path probe no longer shells out.** `internal/probe/traceroute_linux.go`
  sends ordinary UDP/TCP/ICMP-datagram probes with `IP_TTL` set and reads the
  ICMP time-exceeded replies off the socket error queue (`IP_RECVERR`), so
  path tracing needs **no external tool, no raw sockets and no NET_RAW** on
  Linux. `mtr` is gone from the sensor image, the packages' recommends and
  the compose comments; `parseMTR` and its tests are deleted.
- **Proved before it was built.** A stdlib-only spike ran on tpr06 (x86_64)
  and rp02 (aarch64), as an unprivileged user and inside the distroless image
  with `--cap-drop=ALL`. One limitation found and documented rather than
  worked around: under rootless *bridge* networking the user-mode stack
  terminates TCP locally, so TCP-mode tracing needs host networking (which is
  how agents run); UDP and ICMP are fine either way.
- **Better than parity, measured against mtr on the same host.** ICMP mode
  reproduces mtr's hop sequence and RTTs exactly (hop 1: 0.7 ms both, same
  anonymous hop 7). TCP mode is *better*: mtr reported every intermediate hop
  as `???` because it cannot match ICMP replies to its raw SYN probes here,
  while the error-queue engine resolves all nine hops.
- **Caught by that comparison:** a first cut polled the error queue on a 2 ms
  sleep, which inflated LAN hops from 0.7 ms to 2.3 ms — invisible without an
  A/B, and enough to make a latency threshold meaningless. Replaced with
  `poll(2)` (`golang.org/x/sys/unix`, already an indirect dependency).
- **Paris-style constant flow**: the source port is derived from the test id
  and held fixed across TTLs and runs, so each test always traces the same
  ECMP branch. The spike had already shown the branches differ — TTL 5 was
  `.18` over UDP and `.19` over TCP in the same second.
- **`destination_state`** joins the result: `open` (SYN-ACK), `closed` (RST),
  `filtered`, `unreachable`, `echoed`. `reached`/`status` keep their old
  meaning so stored results and the Path view stay valid; the UI shows only
  the surprising verdicts. **`engine`** records what produced each result.
- **Capability**: traceroute is now baseline on Linux (any agent, slim image
  included); on darwin only demo mode reports it. The old check was `mtr in
  PATH`.
- Verified live on tpr06 for all three protocols: TCP to 1.1.1.1:443 →
  `open`, ICMP → `echoed`, TCP to a closed port → `closed`.
- Stages 2 (ASN enrichment) and 3 (path-change detection) are still to come.

## 2026-08-05 — Traceroute Phase 2, stage 2: hops say whose network they are

- **`internal/asn`** resolves an IP to the AS announcing it, its operator and
  the AS registration country, from embedded tables — the `internal/oui`
  pattern. `GET /api/v1/asn?ips=a,b,c` serves it, batched so a path view
  resolves a whole trace in one request; the Path view gains a **Network**
  column, with hops inside the same AS shown as a continuation so a trace
  reads as "three hops through eww ag, then Cloudflare".
- **The data source changed from the plan.** iptoasn.com (named in the plan,
  CC0) is unreachable from here — Cloudflare serves a "Suspected Phishing"
  block page instead of the file. APNIC publishes the equivalent snapshot
  openly (`thyme.apnic.net/current/data-raw-table` + `data-used-autnums`),
  which is what `internal/asn/gen` now consumes.
- **Bigger than planned: 3.7 MB embedded, not ~2 MB.** The raw table is 1.07M
  prefixes; merging contiguous ranges announced by the same AS collapses it
  to 368k (2.6 MB gz), plus 78k AS names for the ASNs that actually appear
  (1.1 MB gz). Without the merge it would have been unshippable.
- Verified against the real hops of tpr06's own paths: `194.112.158.53` →
  AS3330 eww ag (AT), `89.105.161.39` → AS39555 Stadtwerke Schwaz (AT),
  `1.1.1.1` → AS13335 Cloudflare (US); private hops correctly absent;
  unauthenticated requests get 401.
- **Not verified:** the rendered Path page. The browser extension was not
  connected, so the Network column was checked as data and code, not as
  pixels.
- No agent change and no proto change — server-side only.

## 2026-08-05 — Traceroute Phase 2, stage 3: the path says when it moved

- **Route changes are detected on ingest and stored as events.**
  `internal/server/pathchange.go` compares each traceroute run against what
  earlier runs established, writes a `path_changes` row when they differ, and
  sets `pathChanged` on the result. The Path page gains a **Route changes**
  card (when, which TTL, from/to hop with their networks, and whether the
  change left the network); `GET /api/v1/path-changes` serves it; the
  traceroute type exposes a `path_changed` metric, so alerting on reroutes
  needs no new machinery.
- **Two rules keep it from crying wolf, both from real traces rather than
  guesses.** A hop going silent is not a change (routers rate-limit ICMP —
  tpr06's own paths have a permanently anonymous hop 7). The destination's
  address is not part of the comparison (`www.google.com` answered from three
  different addresses in three consecutive runs).
- **A test caught a third rule we had not thought of.** Comparing against
  only the previous run lets silence mask a change permanently: hop 2 answers
  as A, goes quiet, answers as B — B is compared against `*` and matches, so
  the reroute is never reported. The baseline is now the most recent *answer*
  per TTL across the last 5 runs (`store.TracerouteBaselineFor`).
- **Live traffic found a fourth rule within minutes of deploying.** rp01's
  ICMP path test recorded `89.105.160.18 -> .19` and `.19 -> .18` in
  consecutive runs: ECMP alternation, one false event per minute, exactly the
  noise the design set out to avoid. An address the recent window has already
  seen at that TTL is now treated as alternation rather than a change; the
  first appearance of a genuinely new address still reports. The Paris flow
  pinning that prevents this for tcp/udp cannot work for icmp — an
  unprivileged ICMP datagram socket lets the kernel own the echo id and
  recompute the checksum — so `tcp`/`udp` mode is the recommendation when
  route stability matters.
- **A 6-hour soak then found the first fix was half a fix.** 32 events, of
  which 16 were the same two ECMP branches alternating (a 5-run window is too
  short when a path sits on one branch for six runs) and 13 were the path
  flapping between 13 and 14 hops — a length change has no address to match
  against, so the address check never applied. The window is now 20 runs, and
  a path length the window has already observed is treated as flapping too.
  Growing to a length never seen before is still a change.
- Events are bounded per agent+test (500) like results are, since a flapping
  route is exactly what would otherwise grow the table without limit.
- **Tests**: signature construction, the diff rules (silence, replacement,
  path grew/shrank), AS classification against the embedded table, and
  `TestDetectPathChangeThroughIngest` driving three runs through the real
  ingest path — the one that found the masking bug.
- Server-side only: no agent change. The proto gains `path_changed`, set by
  the server on ingest and never by an agent.
- Phase 2 is complete: native engine (stage 1), ASN enrichment (stage 2),
  route-change detection (stage 3).

## 2026-08-06 — Password change and reset

- `POST /api/v1/users/{id}/password` serves both flows: a user changing their
  own password (current password verified) and an admin resetting anyone's.
  Setting a password deletes every session of that user, so a reset really
  logs them out; a self-change re-issues the caller's own cookie so they stay
  signed in.
- **The two flows differ on API keys**, deliberately: a self-change keeps them
  (you know your own keys are fine), an admin reset **revokes** them. A reset
  is the "this account is in a bad state" path, and keys are separate
  credentials that would otherwise sail straight through it.
- **An admin reset picks the password server-side** and shows it once, the way
  a new API key is shown — no `Welcome123` typed under pressure, and the UI,
  the API and the CLI all behave identically.
- **`-reset-password <username>` on the server binary** recovers a password
  nobody can log in with (the case that previously meant hand-written SQL
  against the production DB). Generates and prints it, admin-reset semantics,
  exit 1 on an unknown user. Safe against a running server: WAL + busy timeout.
- **Failed logins and failed current-password checks are now logged** (username
  + client IP, never the attempted password) and throttled in memory: 10
  failures per minute per username+IP, then `429`. Keyed on both so one noisy
  IP cannot lock a real user out — a lockout would be a denial-of-service on a
  system with no email recovery. `X-Forwarded-For` is deliberately not trusted
  (no proxy in front, and anyone could forge a fresh budget).
- UI: "Change password" on the Access page for your own (with a confirm field —
  the cookie is re-issued, so a typo would only surface at the *next* login),
  a per-row "Reset password" button in the users table for admins.
- Not done, on the roadmap: email-based self-service reset (needs an email
  column, reset tokens, and a public enumeration-safe route), and tenant-scoped
  reset rights, which belong with the roles work.

## Known issues

- The agent logs "Registered with server" right after *sending* the register
  message, before the server accepts it — a rejected agent briefly logs
  success. Pre-existing, not yet fixed.
- **Older deployed agents** (pre-rebuild binaries) will not understand
  `wlan_passive` tests and must be updated to the new build.
