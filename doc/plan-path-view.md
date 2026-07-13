# Plan: Path view rework (subway line, MTR table, ECharts history heatmap)

UI-only (internal/web/static/), no Go changes. ECharts 5.6.0 is ALREADY
vendored at `internal/web/static/vendor/echarts.min.js` — do not modify or
re-download it; the `//go:embed static` directive picks it up automatically.

## Layout (section-path)

Replace the current three cards with:

1. **Card "Path"**: keep the status banner (`#pa-status`), replace the
   horizontal `#pa-chain` box-and-arrow chain with a **vertical subway
   line**: a left rail (border/pseudo-element) with one "station" row per
   hop — dot on the rail, then `Hop N`, host (mono; `* * *` muted for
   no-reply hops), and right-aligned `avg ms · loss%`. First station is
   the agent, last is the target. Color the dot by the existing loss
   classes (reuse `lossClass()`); failed hop (ttl === failureHop when not
   reached) gets the `--bad` treatment and the rail below it becomes
   dashed to show the break; unreached target station is muted/dashed.
   Pure CSS with tokens — no library, no SVG.

2. **Card "Hops"** (MTR-style table): keep the existing table but replace
   the separate Avg/Best/Worst columns' role as the only latency signal:
   columns become `# | Host | Loss | Avg (ms) | Best | Worst | Latency`.
   The new Latency cell holds an inline range bar: a full-width muted
   track div; inside it a bar spanning best→worst and a small marker at
   avg, all positioned with percentage left/width relative to the max
   worst RTT across all hops (no SVG math — percent-positioned divs).
   Bar color follows the loss class. Loss cell keeps `lossCell()`.

3. **Card "Path history"** (NEW, ECharts heatmap): fetch the last 48
   results (`limit: "48"`, same params otherwise, results come newest
   first — reverse for display) and render a heatmap: x-axis = result
   time (category, HH:MM labels), y-axis = hop TTL (1..max TTL across all
   fetched results, inverted so hop 1 is at the top), cell value =
   avgRttMs. No-reply hops (no host) get no data point — style
   the chart background so gaps read as "no reply". Tooltip per cell:
   host, avg/best/worst ms, loss %. visualMap: continuous, min 0, max =
   nice max of avg RTTs, horizontal at the bottom, using a green→amber→red
   ramp built from the CSS tokens (--ok, --bad; read via
   getComputedStyle at render time like renderHopChart does today).
   If fewer than 2 results, show the existing `.empty` style instead.

Delete the "Latency by hop" bar chart card and `renderHopChart()` — the
range bars and heatmap replace it.

## ECharts wiring

- `index.html`: add `<script src="vendor/echarts.min.js"></script>`
  BEFORE the app.js script tag.
- Init the chart instance lazily inside renderPath's history step (the
  section must be visible when `echarts.init` runs, otherwise width is
  0). Keep ONE instance: `echarts.init` once, store it, `setOption(...,
  true)` on re-render; call `.resize()` on window resize (only when the
  path section is visible).
- Theme: the theme toggle at app.js:18 already re-renders dashboards.
  Read token colors fresh on every render and re-render the heatmap from
  the toggle handler too (keep the fetched results cached in a module
  variable so theme re-render doesn't refetch).
- ECharts text/axis colors: set textStyle/axisLabel colors from the
  --text / --muted tokens so both themes stay readable.

## Docs

- ROADMAP.md: check off "Modify the path view to look more professional"
  (match the existing checked style).
- PROGRESS.md: one dated entry under 2026-07-13.
- CLAUDE.md: in the internal/web bullet, amend "vanilla JS, no build
  step" to note vendored third-party libs live in static/vendor
  (currently ECharts for the path history heatmap).
- README: only if it describes the path view specifically (check).

## Verification

1. `make build`, `go vet ./...`, `go test ./...`.
2. Serve check: start `NETLAMA_ADMIN_PASSWORD=x NETLAMA_TLS_SELF_SIGNED=1
   ./bin/netlama-server -db /tmp/plan-path.db` on scratch ports; curl -k:
   `/vendor/echarts.min.js` returns 200 and ~1MB; `/` contains the vendor
   script tag before app.js; `/app.js` contains the heatmap render
   function and no `renderHopChart`.
3. Quote in your report: the new function names, and confirmation that
   `#pa-latency`'s old card is gone from index.html.

## Constraints

- No other new dependencies; do not touch vendor/echarts.min.js.
- No Go/API changes (the results endpoint already supports limit).
- Keep the Run now / Refresh / agent+test selector behavior untouched.
- Do not commit.
