# Plan: path latency waterfall + history click-to-inspect

UI-only (internal/web/static/), no Go changes. Builds on the reworked Path
view (subway line, hops table, ECharts history heatmap). ECharts is already
vendored.

## 1. Waterfall card "Latency contribution by hop"

New card between the Hops table and Path history. ECharts bar chart showing
where round-trip latency is added along the path of the displayed result:

- Traceroute avg RTTs are CUMULATIVE per hop. Contribution of hop i =
  avgRttMs[i] − (cumulative avg of the previous responding hop).
  No-reply hops (no host) contribute nothing: skip them but carry the
  previous cumulative forward (they appear as a gap on the x-axis — still
  add their TTL to the axis so hop numbers stay honest, with no bar).
- Render as a floating waterfall using the standard ECharts two-series
  stacked-bar trick: an invisible "base" series (stack placeholder,
  itemStyle transparent, tooltip disabled) with value min(prevCum, cum),
  and a visible "delta" series with value |cum − prevCum|.
- Colors (read CSS tokens at render time like renderPathHeatmap):
  positive delta = --accent; THE single largest positive delta = --bad
  (that's the "this hop hurts" answer); negative delta = --border/muted
  (jitter/asymmetric return path — label it in the tooltip as "faster than
  previous hop (jitter)").
- Tooltip per hop: host, +delta ms (or −), cumulative avg RTT ms.
- x-axis = hop TTL, y-axis ms. Axis/text colors from tokens, same as the
  heatmap. Height ~260px.
- Fewer than 2 responding hops → show '.empty' text instead of a chart.

## 2. Click a heatmap cell → inspect that run

Clicking a Path-history cell loads that exact result into everything above
(status banner, subway line, hops table, waterfall):

- Refactor: extract the "render one result" part of renderPath() into
  `renderPathResult(r, agent)` (status banner, subway, hop table rows,
  waterfall). renderPath() fetches latest+history as today, then calls it
  with results[0], then renders the heatmap.
- Fix the heatmap's category key while here: use the raw `r.time` string
  as the x-axis category value and format it to HH:MM only for display
  (axisLabel formatter + tooltip). This removes the current fragile
  find-by-formatted-time lookup AND makes click handling exact:
  `paHeatmapInstance.on("click", ...)` → params.data[0] is the exact
  r.time → find the result in the cached paHistoryResults → call
  renderPathResult with it. Attach the click handler once, at init.
- Viewing indicator: when a historical run is displayed, prepend a chip to
  the status banner: `Viewing run from <locale datetime> — <button
  id="pa-back-latest" class="ghost">Back to latest</button>`; the button
  re-renders the latest cached result (no refetch). Refresh / Run now /
  agent/test change always reset to latest (they already re-run
  renderPath).
- Keep ONE waterfall ECharts instance with the same lazy-init /
  setOption(option, true) / dispose-on-empty / resize / theme-re-render
  pattern the heatmap uses. Small shared helper is fine if it stays
  simple; don't over-abstract. Theme toggle must re-render BOTH charts
  (waterfall re-renders via re-rendering the currently displayed result —
  cache it in a module variable, e.g. paDisplayedResult).

## Docs

- PROGRESS.md: one dated entry under 2026-07-13 (append to today's
  section if present).
- ROADMAP/README: no changes expected (verify README doesn't describe the
  path cards; update only if wrong).

## Verification

1. make build, go vet ./..., go test ./....
2. Serve check (self-signed TLS, scratch port, scratch DB): /app.js
   contains renderPathResult, the waterfall render function, and
   paHeatmapInstance.on("click"; index.html contains the new waterfall
   card container.
3. Quote in your report: the new function names and the line implementing
   the largest-delta highlight.

## Constraints

- No new dependencies; do not touch vendor/echarts.min.js.
- No Go/API changes. Do not commit.
