# Plan: SaaS / cloud service tests (`saas` test type)

Motivation: "is Teams broken from the Berlin office?" is the question a
branch-office sensor exists to answer, and today it takes an operator
hand-building four `http`/`tcp` tests per service from endpoints they had
to look up themselves. A `saas` test type turns that into picking a
service from a dropdown: the server ships a curated catalog of what each
service actually depends on, and one test row represents one service.

## Design decisions (fixed — do not redesign)

1. **First-class test type**, not sugar over `http`/`tcp`. One test row =
   one service, so the thing operators reason about ("MS Teams") is the
   thing the model names, schedules, and alerts on.
2. **The catalog lives on the server.** `TestSpec` carries the expanded
   endpoint list; the agent runs whatever it is handed and knows nothing
   about services. Adding or fixing a service is a server release —
   deployed agents never need updating for it (the fleet rollout the
   `wlan_passive` rebuild forced is the failure mode being avoided).
3. **Endpoint kinds: `https` and `tcp` only.** Both are served by existing
   probes (`probe.HTTPCheck`, `probe.TCPConnect`) and need no privileges,
   so any container agent can run them.
   `ponytail: no UDP kind, so the media path is not checked at all —
   Microsoft's endpoint feed gives Teams media as UDP 3478-3481 against IP
   ranges with no hostnames, so there is not even a TCP fallback to stand
   in for it. Add a STUN binding probe (~40 stdlib lines, new "udp" kind)
   when someone needs an honest call-path answer.`
4. **No new result message.** `https` endpoints emit `HttpResult`, `tcp`
   endpoints emit `TcpResult`, one result per endpoint, exactly as
   `runPings` emits one per target. No duplicated timing fields, no second
   UI renderer.
5. **Stored params are only `{service}`.** Endpoints resolve from the
   catalog at push time, so a catalog fix reaches existing tests without
   editing them.
6. **Service ids are permanent.** Endpoints inside a service may be fixed,
   added or removed freely; a shipped service id is never renamed or
   removed, because stored tests reference it. Enforced by discipline and
   a comment at the top of the catalog, not by code.
7. **New `saas` capability.** Agents must be updated once before the type
   works anywhere; the server's capability filter means an old agent never
   receives a spec it would silently ignore. After that one rollout,
   decision 2 holds.
8. **`Primary` is latency in ms** (`total_ms` for https, `connect_ms` for
   tcp), same unit and threshold semantics as `http`/`tcp`. Outages are
   covered by the existing type-independent `unhealthy` alert rule.
   `Subject` is the url/target, so alerts land per endpoint instead of
   flapping across a service.
9. **No service-level verdict in v1.** A rollup ("MS Teams is degraded:
   1 of 3 endpoints down") needs aggregation across results, which nothing
   in the registry does today — every `Metric` reads a single result.
   Phase 2, together with a Services page.
10. **60s interval floor, endpoints run sequentially.** Same floor
    `speedtest` uses, so a fleet cannot hammer a vendor's login endpoint
    every five seconds.

## Changes

### 1. Proto (`proto/netlama.proto` + `make proto`)

```proto
message SaasParams {
  // service is the catalog id ("ms-teams"); carried for labelling and
  // logs. endpoints is the server's expansion of it — the agent runs
  // these and never consults a catalog of its own.
  string service = 1;
  repeated SaasEndpoint endpoints = 2;
}

message SaasEndpoint {
  string kind = 1;    // "https" | "tcp"
  string target = 2;  // URL for https, host:port for tcp
}
```

Plus `SaasParams saas = 15;` in the `TestSpec` params oneof. No result
message: `HttpResult` and `TcpResult` are reused as-is.

### 2. Catalog (`internal/saas/catalog.go`, new)

A package-level map, `map[string]Service`, with `Service{ID, Name,
Endpoints []Endpoint}`. Header comment carries decision 6 verbatim.

**Which kind an endpoint gets is not a style choice.** `resultOK`
(`internal/server/server.go:522`) counts an HTTP result as OK only on
2xx/3xx, so a machine API that answers `401`/`403`/`400` to an
unauthenticated GET — which is the correct, healthy behaviour of
`management.azure.com`, `portal.azure.com`, `storage.googleapis.com` and
`webexapis.com` — would be red forever as an `https` endpoint. Rule:

- **`https`** for user-facing front doors that answer 2xx/3xx unauthenticated.
- **`tcp`** for machine APIs that answer 4xx unauthenticated. Connect-only
  loses the status and TLS timings, but never lies.

`ponytail: per-endpoint expected-status in the catalog would let the API
hosts be checked over https properly. It needs resultOK to know the test
type, which becomes possible once change 4 lands. Do it when someone
wants TLS expiry/TTFB on the API front doors.`

v1 catalog — every entry verified live on 2026-08-04 (status shown) and
against the vendor's own documentation:

| id | Name | endpoints | source |
|----|------|-----------|--------|
| `ms-teams` | Microsoft Teams | https `teams.microsoft.com` (302), https `teams.cloud.microsoft` (302), https `login.microsoftonline.com` (302) | Microsoft 365 endpoint feed, Skype+Common, `required: true` |
| `m365` | Microsoft 365 | https `outlook.office365.com` (301), https `graph.microsoft.com` (301), https `login.microsoftonline.com` (302) | same feed; `outlook.office365.com` is category Optimize |
| `webex` | Cisco Webex | https `web.webex.com` (200), https `webexapis.com/v1/ping` (200) | Webex network requirements: `*.webex.com`, `*.webexapis.com` |
| `zoom` | Zoom | https `zoom.us` (301), https `api.zoom.us` (200) | Zoom firewall doc: `zoom.us`, `*.zoom.us` |
| `google-workspace` | Google Workspace | https `mail.google.com` (301), https `accounts.google.com` (302), https `drive.google.com` (302) | Google Workspace firewall/proxy settings |
| `aws` | Amazon Web Services | https `console.aws.amazon.com` (302), https `signin.aws.amazon.com` (302), tcp `ec2.amazonaws.com:443` | no vendor allowlist doc exists; console/signin front doors + the API front door |
| `azure` | Microsoft Azure | https `login.microsoftonline.com` (302), tcp `portal.azure.com:443` (403 over https), tcp `management.azure.com:443` (400 over https) | login endpoint from the M365 feed; portal/ARM are 4xx-by-design |
| `gcp` | Google Cloud | https `console.cloud.google.com` (302), tcp `storage.googleapis.com:443` (400 over https) | console front door; storage API is 4xx-by-design |

**Teams media cannot be checked in v1.** Microsoft's endpoint feed lists
the Teams media path (id 11, category Optimize) as UDP 3478–3481 against
IP ranges with **no hostnames at all** — there is nothing to resolve and
nothing meaningful to TCP-connect to. This is decision 3's ceiling made
concrete: a green `ms-teams` test means signalling and sign-in work, not
that a call will.

Status-page hosts (`azure.status.microsoft`, `status.cloud.google.com`,
`health.aws.amazon.com`) all answer 200 and were deliberately **not**
included: they run on separate CDN infrastructure and say nothing about
whether the service works from this site.

No vendor feed fetching, no refresh scheduler, no override file: these are
hostnames that change on a scale of years.

### 3. Registry (`internal/testtype/`)

`specs.go` gains one entry:

```go
register(&Spec{
    Type:               "saas",
    Unit:               "ms",
    MinIntervalSeconds: 60,
    NewParams:          func() Params { return &SaasParams{} },
    Primary:            saasLatency,   // total_ms or connect_ms
    Metrics: map[string]Metric{
        "latency_ms":       saasLatency,
        "cert_expiry_days": ...,       // http endpoints only
    },
    Subject: saasSubject,              // url or target
})
```

`params.go` gains `SaasParams{Service string}`: `Validate` rejects a
service id that is not in the catalog (typo via the API), `Apply` expands
it into the proto oneof. This is where the catalog is read — server-side,
on every push and every validation, per decision 5.

### 4. Result type resolution (`internal/server/server.go`)

`handleResult` derives `results.test_type` from the payload shape via
`testtype.TypeOf`, which would file every saas result as `http`/`tcp` and
break the `?type=saas` filter and the Results page. It must instead
resolve `result.TestId` to the stored test's type, falling back to
`TypeOf` when the test is unknown (deleted mid-flight, unclaimed agent).
Cache the lookup per connected agent — the spec set is already pushed to
each one. See `docs/adr/0001-test-type-from-definition.md`.

### 5. Agent (`internal/agent/scheduler.go`, `internal/probe/capabilities.go`)

- `runTest` gains `case *pb.TestSpec_Saas: a.runSaas(...)`.
- `runSaas` walks `params.Endpoints` sequentially: `https` →
  `probe.HTTPCheck(ctx, target, 10, false)` → `TestResult_Http`;
  `tcp` → `probe.TCPConnect(ctx, target, 5)` → `TestResult_Tcp`. Failures
  are recorded as results with `Error` set, like every other probe;
  an abandoned run (`ctx.Err() != nil`) returns without reporting.
- `DetectCapabilities` appends `"saas"` unconditionally — it needs nothing
  beyond outbound TCP, like `http`, `tcp` and `perfmon`.

### 6. API (`internal/api/saas.go`, new + route in `api.go`)

`GET /api/v1/saas-services` → the catalog as JSON (`id`, `name`,
`endpoints[{kind, target}]`). Authenticated, not tenant-scoped: this is
the shape of the software, not anyone's data — same reasoning as
`GET /api/v1/test-types`. Documented in `doc/API.md`.

### 7. UI (`internal/web/static/app.js`)

- One entry in the browser's test-type registry.
- Create/edit test form: when type is `saas`, show a **Service** dropdown
  populated from `/api/v1/saas-services` (fetched once, like the type
  list). No other fields.
- Results page needs nothing new: it already renders `HttpResult` and
  `TcpResult` rows and per-subject series.

### 8. Docs on ship (CLAUDE.md convention)

README (new test type + the `saas` capability), ROADMAP checkbox,
dated PROGRESS entry. No new server/agent option, so the compose files
are untouched.

## Verification

- `go test ./...`, `make vet`, `make build`.
- Registry test coverage already walks every registered type
  (`testtype_test.go`); the new entry must satisfy it.
- Unit check for the catalog expansion: a stored `{service:"ms-teams"}`
  validates, expands to the catalog's endpoint list, and an unknown id
  fails validation.
- E2E per CLAUDE.md: build both binaries, self-signed server, create
  tenant/site/agent via the JSON API, create a `saas` test for `ms-teams`,
  run an agent, confirm results land with `test_type = "saas"` and render
  on the Results page.

## Not in v1

UDP/STUN media checks (decision 3), per-service verdict and a Services
page (decision 9), operator-editable or subsettable endpoint lists,
vendor endpoint-feed refresh, social networks (a guest-wifi/filtering
question, not "is our business SaaS reachable").
