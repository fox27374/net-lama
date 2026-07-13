# Plan: path view polish — drop subway, richer hops table, waterfall fixes

UI-only (internal/web/static/). Current Path cards: status+subway, Hops
table, waterfall, history heatmap.

## 1. Remove the subway visualisation

It duplicates the hops table. Delete renderPathSubway() and its call,
the #pa-subway container, and ALL .path-station/.path-dot/.path-subway
CSS. Move the status banner (#pa-status) into the waterfall card, above
the chart, and reorder cards: 1) waterfall card (status banner + h3 +
chart), 2) Hops table, 3) Path history. The "Viewing run from … Back to
latest" chip keeps working (it lives in #pa-status).

## 2. Waterfall axis: visible labels + unit

In the screenshot the top x-axis renders NO tick labels or name — they
are clipped: the option uses `grid: { height: "70%", top: 10, ... }`,
leaving no room above the plot for a top-positioned axis. In ALL three
metric modes: drop `height: "70%"`, use `grid: { top: 44, bottom: 24,
left: 140, right: 30 }`, keep xAxis position "top" with axisLabel shown
and `name` = "ms" (latency/jitter) / "%" (loss), nameGap small so the
unit sits by the axis. Update the container-height formula to
`rows*28 + 100`.

## 3. Waterfall: negative deltas must not look like bars

Gray blocks (hops whose avg RTT is LOWER than the previous hop — mtr
jitter/ICMP-deprioritization artifacts) currently render as full bars
and read as latency contributions. Change latency mode: for a negative
delta, the stacked delta value becomes 0 (no block) and instead a thin
tick marks the hop's cumulative RTT: one extra `type: "scatter"` series,
symbol "rect", symbolSize [3, 16], data [[cumulative, rowIndex], ...]
only for negative-delta hops, color --muted-solid, no label. Tooltip
(trigger axis) must keep reporting these hops ("faster than previous hop
(jitter): −X ms") — verify the formatter still resolves the row via
params[0].dataIndex with the extra series present.

## 4. Hops table: fewer numbers, more bars

New columns: `# | Host | Latency | Loss | Jitter` (drop Avg/Best/Worst).
Each metric cell = compact value text + inline bar (extend today's
.latency-track pattern; keep cells ~180px wide):

- Latency: text `avg ms`; bar = today's best→worst range with avg marker,
  scaled to max worst RTT of the displayed result.
- Loss: text `N%`; plain fill bar 0–100%, color by thresholds (>=60
  --bad, >=20 --warn, else --ok).
- Jitter: text `X ms`; plain fill bar scaled to the max jitterMs of the
  displayed result (guard max with 0.1), color --accent; "–" when the
  field is absent (older agents).
- No-reply hops: Host shows `* * *`, all three metric cells show muted
  "no reply" / "–" (no bars).

Layout: value text right-aligned in a fixed-width span before the bar so
bars align vertically. Reuse tokens; small font (--text-xs) for values.

## Docs

PROGRESS.md: append to the 2026-07-13 entry. No README/ROADMAP changes.

## Verification

1. make build, go vet ./..., go test ./....
2. Serve check (self-signed TLS, scratch port/DB): app.js has no
   renderPathSubway; index.html has no pa-subway and has the new table
   headers; style.css has no .path-station/.path-dot/.path-subway.
3. Quote in your report: the new grid line, the scatter-series snippet
   for negative deltas, and the new table header row.

## Constraints

No new dependencies; don't touch vendor/. No Go/API changes. Keep the
metric toggle, click-to-inspect, Back to latest, theme re-render
working. Do not commit.
