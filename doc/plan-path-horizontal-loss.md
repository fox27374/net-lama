# Plan: horizontal (APM-style) path waterfall + latency/loss metric toggle

UI-only (internal/web/static/), no Go changes. Builds on the current Path
view (subway, hops table, waterfall card, history heatmap). ECharts is
vendored already.

## 1. Waterfall goes horizontal (Splunk-APM style)

Rework renderPathWaterfall to horizontal bars, one ROW per responding hop
(like an APM trace waterfall):

- yAxis: category = hops, `inverse: true` so hop 1 is the TOP row. Label:
  `"<ttl>  <host>"`, truncated via axisLabel formatter to ~24 chars (full
  host stays in the tooltip). Mono-ish small font, --muted-solid color.
- xAxis: value (ms), `position: "top"`, grid lines on.
- Keep the floating-bar stacked trick, just transposed: invisible "base"
  series value = min(prevCumulative, cumulative), visible "delta" series
  = |delta|. So each row's bar starts where the previous hop's cumulative
  RTT ended — reads exactly like the screenshot's span waterfall.
- Colors unchanged: largest positive delta --bad, positive --accent,
  negative --border (tooltip notes "faster than previous hop (jitter)").
- Chart height must scale with rows: `Math.max(180, rows*28 + 70)` px,
  set on the container before init/resize (call chart.resize() after
  changing the height when the instance already exists).
- Bar height ~16px (barWidth), small gap.

## 2. Latency / Loss metric toggle

A segmented control in the Path section header (index.html, next to the
Run now / Refresh buttons): two buttons "Latency" and "Loss" in a pill
container; active one gets accent styling. New CSS classes (.seg-control,
.seg-btn, .seg-btn.active) using existing tokens — check style.css first
and reuse an existing segmented/toggle pattern if one exists.

Module var `paMetric = "latency"`. Clicking a segment sets it and
re-renders the waterfall card AND the history heatmap from cached data
(paDisplayedResult / paHistoryResults) — no refetch. Theme re-render must
respect the current paMetric too.

**Loss mode — waterfall card** (title becomes "Packet loss by hop"):
plain horizontal bars (loss is NOT cumulative — no floating/stacking):
one row per responding hop, bar = lossPercent from 0, xAxis 0–100 (%),
bar color by the existing loss thresholds (>=60 --bad, >=20 --warn, else
--ok). Same row labels/height rules as latency mode. Tooltip: host,
loss %, avg RTT.

**Loss mode — history heatmap** (title "Path history — loss"): same cells
but value = lossPercent, visualMap fixed 0–100 with --ok → --warn → --bad
ramp. Latency mode keeps today's behavior (title "Path history — avg
RTT"). Card titles update from JS when the metric changes (give the h3s
ids).

Click-to-inspect and "Back to latest" must keep working in both modes.

## Docs

PROGRESS.md: dated entry under 2026-07-13 (append to today's section).
No ROADMAP/README changes expected.

## Verification

1. make build, go vet ./..., go test ./....
2. Serve check (self-signed TLS, scratch port/DB): /app.js contains
   paMetric, the segmented-control wiring, and yAxis-category waterfall;
   / contains the two segment buttons and the h3 ids.
3. Quote in your report: the axis-swap lines (yAxis category/inverse,
   xAxis position top), the dynamic height line, and the loss-mode
   visualMap range.

## Constraints

No new dependencies; don't touch vendor/echarts.min.js. No Go/API
changes. Do not commit.
