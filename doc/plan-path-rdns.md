# Plan: reverse-DNS names for traceroute hops

Resolve PTR names for hop IPs on the AGENT (its resolver sees the right
view for private/split-horizon ranges) and show them in the Path UI.
Same end-to-end shape as the jitter round (probe → proto → agent → UI).

## Probe (internal/probe/traceroute.go)

- Keep `mtr -n` exactly as is — probing stays DNS-free.
- Add `HostName string` to the Hop struct.
- New unexported helper `resolveHopNames(ctx, hops)`: for each hop with
  a Host (IP), run `net.DefaultResolver.LookupAddr` in parallel
  (goroutines + WaitGroup), each under a 1500ms context timeout; on
  success take the first name, strip the trailing dot, store in
  HostName; on error/empty leave "". Best-effort: never returns an
  error, never fails the test. Call it from Traceroute() after parseMTR.
- Demo mode (traceroute_demo.go): give the first hop a synthetic name
  (e.g. "gw.demo.lan") and one mid-path hop something like
  "core1.demo-isp.net"; leave the rest empty so the UI fallback is
  exercised.

## Proto + agent

- proto/netlama.proto message Hop: `string host_name = 9;`. Run
  `make proto`; never edit *.pb.go by hand.
- internal/agent/scheduler.go: copy HostName in the probe→proto hop
  conversion (next to JitterMs).
- Server: no change (protojson passes `hostName` through; omitted when
  empty — older agents keep working).

## UI (internal/web/static/)

Display rule everywhere: `display = hop.hostName || hop.host`; when a
hostName exists, the IP remains visible as secondary info.

- Hops table Host cell: hostName on the first line, IP as a second line
  in muted --text-xs mono; bare IP (mono) when no name. No-reply hops
  unchanged ("* * *").
- Waterfall y-axis labels (all three metric modes): `${ttl}  ${display}`,
  keep the existing ~24-char truncation. Tooltips: show hostName and IP
  (e.g. "<strong>name</strong> (ip) (TTL n)"; just "(TTL n)" after the
  IP when no name).
- Heatmap tooltip: same display rule (name + IP when both).
- History note: heatmap rows key on TTL and results key on IP-based
  data — nothing keys on hostName; keep it that way (names can change).

## Tests

Existing go tests must pass (fixtures have no hostName — parseMTR is
untouched by resolution, which happens after parsing). No new DNS test
(network-dependent); resolveHopNames must be trivially skippable: hops
without Host or with Host that is already a name are left alone.

## Docs

PROGRESS.md: append to 2026-07-13. README: update the traceroute test
description only if it enumerates per-hop fields.

## Verification

1. make proto (pb.go in diff), make build, go vet ./..., go test ./....
2. Demo e2e (NETLAMA_TRACEROUTE_DEMO=1, scratch server/DB/ports, JSON
   API tenant/site/agent/test): confirm a stored result hop contains
   "hostName":"gw.demo.lan".
3. Serve check: app.js contains the hostName display logic in the table,
   waterfall labels, and both tooltips.
4. Quote in your report: the resolveHopNames signature + the LookupAddr
   line, the proto field, and the API result snippet with hostName.

## Constraints

No new dependencies; don't touch vendor/. Do not commit.
