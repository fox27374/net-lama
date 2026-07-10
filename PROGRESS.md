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

## Known issues

- The agent logs "Registered with server" right after *sending* the register
  message, before the server accepts it — a rejected agent briefly logs
  success. Pre-existing, not yet fixed.
