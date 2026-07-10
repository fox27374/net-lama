# Plan: alternative speedtest providers (ndt7, cloudflare)

Motivation: the current probe uses the unofficial `showwin/speedtest-go`
client against volunteer speedtest.net servers, and its readings are
sometimes untrustworthy (the probe already retries servers and rejects
implausible values). Add two more trustworthy providers so users can run
them side by side: **ndt7** (M-Lab, official Go client) and **cloudflare**
(speed.cloudflare.com, stdlib-only implementation).

## Design decision (fixed — do not redesign)

One test type (`speedtest`) with a new `provider` parameter, NOT new test
types. Values: `ookla` (default, the existing implementation), `ndt7`,
`cloudflare`. An empty/missing provider means `ookla`, so every existing
test row keeps working unchanged. Because the result shape stays
`SpeedtestResult`, the Prometheus gauges (`netlama_speedtest_*`), alert
rules on latency/throughput, and the Results rendering all keep working
with zero changes — different providers are distinguished by the `test`
label, exactly like two speedtest tests are today.

## Changes

### 1. Proto (`proto/netlama.proto` + `make proto`)

- `SpeedtestParams`: add `string provider = 1;`
- `SpeedtestResult` (the message at ~line 139): add a `string provider`
  field (next free number) so results show where the number came from.
Adding fields is wire-compatible; protoc and the Go plugins are installed.

### 2. Probes (`internal/probe/`)

- `ndt7.go` (new): use the official M-Lab client library
  `github.com/m-lab/ndt7-client-go` (Apache-2; this dependency is
  explicitly approved). Run download then upload, fill a
  `SpeedtestResult`-equivalent struct (server name = the M-Lab server FQDN,
  latency = MinRTT from the measurement if exposed, else 0). If the
  library turns out to drag in something unreasonable or fights the build,
  consult the advisor before hand-rolling anything.
- `cloudflare.go` (new): stdlib-only against `https://speed.cloudflare.com`:
  - latency: time ~5 small GETs (`/__down?bytes=0`), take the median of
    the request round-trips;
  - download: GET `/__down?bytes=<large>` and count bytes read for a fixed
    measurement window (~10s target, honoring ctx cancellation), compute
    Mbps from bytes/elapsed; use a few parallel connections (e.g. 4) to
    not be single-stream-limited;
  - upload: POST `/__up` streaming generated data the same way;
  - server name: use the `cf-meta-colo` / `CF-RAY` colo code from response
    headers if easy, else "Cloudflare".
- Keep `speedtest.go` (ookla) untouched.

### 3. Agent (`internal/agent/scheduler.go`)

`runSpeedtest` dispatches on `spec.GetSpeedtest().GetProvider()`:
`""`/`"ookla"` → existing probe, `"ndt7"`, `"cloudflare"` → new probes.
Set the `provider` field on the result; keep the existing logging shape
(one "Running speedtest" / "Speedtest done|failed" pair, with a provider
attr).

### 4. Server validation + config (`internal/server/config.go`)

- Validation (~line 54): provider must be one of empty/ookla/ndt7/
  cloudflare; keep the 60s-minimum interval rule for all of them.
- Spec building (~line 178): pass the provider through from the stored
  test params.
Check how test params are stored/parsed (JSON in the tests table /
`internal/api` test handlers) and thread `provider` through wherever the
other per-type params (e.g. ping target) flow.

### 5. Web UI (`internal/web/static/`)

- Tests page: when type `speedtest` is selected, show a Provider dropdown
  (Ookla speedtest.net / M-Lab NDT7 / Cloudflare), default Ookla.
- Results page: show the provider (and server name, which already shows)
  in the speedtest result rendering.

### 6. Docs

- README: extend the test-type list — one line per provider incl. the
  trustworthiness rationale (unofficial client + volunteer servers vs
  M-Lab fleet vs Cloudflare edge) and the note that a Pi's TLS throughput
  caps what any provider can measure on fast links.
- doc/API.md: update the speedtest params shape with `provider`.
- ROADMAP.md: add a new checked item under "Tests & monitoring":
  speedtest provider selection (ookla/ndt7/cloudflare); keep wording in
  the style of the other checked items.
- PROGRESS.md: dated entry (2026-07-10).
- No new env vars → no compose changes.

## Verification (required)

1. `make proto`, `make build`, `go vet ./...`, `go test ./...`. Unit-test
   what's testable without network (e.g. cloudflare Mbps computation /
   provider validation); network probes themselves are covered by e2e.
2. E2E per CLAUDE.md (self-signed TLS, scratch ports): create three
   speedtest tests — providers ookla, ndt7, cloudflare — assigned to one
   site, run the agent, and confirm all three deliver results with
   plausible nonzero download AND upload Mbps and the correct provider
   field via `GET /api/v1/results`. This machine has internet access; each
   provider run takes ~20-30s, so give the agent time. If ONE provider's
   servers are unreachable from this network, note it in the report and
   verify the other two — do not fail the whole task on it.
3. Confirm an existing-style test (no provider field) still validates and
   runs as ookla (backward compatibility).

## Constraints

- The ONLY new dependency allowed is `github.com/m-lab/ndt7-client-go`
  (and what it transitively needs). Cloudflare must stay stdlib-only.
- Do not touch the uncommitted logs / API-keys work in the tree — it is
  intentional and deployed. Build on top of it.
- Do not commit; leave changes in the working tree for review.
