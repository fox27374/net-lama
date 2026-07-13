# Plan: per-hop jitter metric + honest no-reply hops

Two changes: parse per-hop jitter (mtr StDev) end-to-end and add "Jitter"
to the Path metric toggle; stop rendering no-reply hops as "100% loss".

## 1. Jitter end-to-end (proto + agent + UI)

- `internal/probe/traceroute.go`: add `StDev float64 \`json:"StDev"\`` to
  mtrReport hubs; add `JitterMs float64` to Hop; fill it in parseMTR.
- `internal/probe/traceroute_demo.go`: emit a small synthetic JitterMs
  per hop (e.g. 0.2–3 ms, keep the existing jitter randomness idea) so
  demo mode exercises the UI.
- `proto/netlama.proto`: `double jitter_ms = 8;` in message Hop. Run
  `make proto` (protoc + Go plugins are installed). Never edit *.pb.go
  by hand.
- Agent: find where probe Hop fields are copied into the proto Hop
  (grep for WorstRttMs in internal/agent or cmd/agent) and copy JitterMs
  too. Server side needs no change (results are stored as protojson —
  the new field flows through as `jitterMs`, camelCase, omitted when 0).
- `go test ./internal/probe`: extend the existing TestParseMTR fixture
  JSON with StDev values and assert JitterMs on the parsed hops.

## 2. UI: "Jitter" in the metric toggle

- index.html: third segment button `data-metric="jitter"` labeled
  "Jitter" between Latency and Loss.
- app.js: paMetric gains "jitter". Titles: waterfall card "Jitter by
  hop", history card "Path history — jitter".
- Waterfall card in jitter mode: same as loss mode (plain horizontal
  bars, NOT floating/cumulative), value = hop.jitterMs (treat
  undefined/missing as 0), xAxis in ms with dynamic niceMax, bar color
  --accent (no thresholds — we have no jitter thresholds yet). Tooltip:
  host, jitter ms, avg RTT ms.
- Heatmap in jitter mode: cell value = jitterMs, visualMap 0..niceMax of
  the fetched data (NOT fixed 0-100), same ok→warn→bad ramp as latency.
- Hops table: add a "Jitter (ms)" column (after Worst) showing
  fmt(h.jitterMs); "–" for no-reply hops or when the field is absent
  (older agents don't send it — must not render NaN/undefined).
- Older results without jitterMs: charts render zeros/gaps gracefully,
  never `undefined` in tooltips ("–" instead).

## 3. No-reply hops: "no reply", not 100% loss

In the hops table (renderPathResult), no-reply hops (no h.host)
currently render `lossCell(h.lossPercent)` → "100%", which reads as an
outage. Render a muted "no reply" instead in the Loss cell for anon
hops. (Subway already says "no reply"; heatmap/waterfall already skip
them — verify, change nothing there.)

## Docs

PROGRESS.md entry under 2026-07-13 (append to today's section). README:
check the traceroute test description — if it lists measured per-hop
fields, add jitter; otherwise no change. No ROADMAP change.

## Verification

1. make proto (proto/*.pb.go regenerated and included in the diff),
   make build, go vet ./..., go test ./... (TestParseMTR extended).
2. Demo e2e: start server (self-signed TLS, scratch DB/ports) and an
   agent with NETLAMA_TRACEROUTE_DEMO=1; create tenant/site/agent/
   traceroute-test via the JSON API (login cookie; agent token from the
   create-agent response); wait for one result; GET /api/v1/results and
   confirm hops contain nonzero "jitterMs".
3. Serve check: /app.js contains the jitter segment handling and the
   no-reply loss cell; / contains the third seg-btn.
4. Quote in your report: the parseMTR jitter line, the proto field, the
   jitterMs assertion from the test, and the API result snippet showing
   jitterMs.

## Constraints

No new dependencies. Do not commit. Do not touch vendor/.
