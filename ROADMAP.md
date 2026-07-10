# Roadmap / Backlog

Planned work, roughly grouped. Not ordered by priority yet.

## Deployment & packaging

- [ ] Kubernetes deployment for server and agent — probably a Helm chart
- [ ] Native agent install packages: `.deb` and `.rpm` (systemd unit, config in `/etc/netlama`)
- [ ] Option to change the web UI port independently of the API/metrics port
- [ ] Option to disable the web UI entirely (API/headless mode)

## Agent lifecycle & connectivity

- [ ] Zero-touch enrollment: agent automatically finds its server (public DNS
      discovery) and connects through a WireGuard tunnel
- [ ] Unclaimed state: a new agent connects without a site and waits until
      someone assigns it to a site in the UI (replaces pre-issued tokens as the
      only flow)
- [x] Encryption for the gRPC control stream (TLS): one cert for gRPC + HTTPS
      UI, self-signed auto-generation or provided cert/key, agent verify via
      CA/system-roots or skip-verify, secure cookies
- [x] Agent authentication at the server: per-agent mTLS (client certs) on top
      of the token — gRPC requires client certs (built-in agent CA or provided
      bundle), cert CN bound to the agent name, `-issue-agent-cert` helper

## Tests & monitoring

- [ ] Perfmon tests **between agents of the same tenant** (agent-to-agent
      throughput/latency, e.g. iperf-style with one agent as reflector)
- [ ] Agent resource monitoring: CPU, memory, storage — reported over the
      stream and visible in the UI/metrics
- [x] Alerting and thresholding: per-test rules (unhealthy, or latency/loss/
      throughput thresholds) with consecutive-breach counts, per-target alert
      state, webhook notification, Alerts UI + nav badge. Later: email/SMTP,
      more metrics, maintenance windows, per-rule scoping to sites/agents.
- [x] WLAN Phase 1: interface inventory + managed-mode AP/SSID scan (agent reports
      wireless interfaces, per-agent interface selection, periodic scan, Wireless UI)
- [ ] WLAN Phase 2: monitor-mode client sensing — per-station MAC/RSSI/SNR/rate/MCS
      per SSID; needs a monitor-capable adapter and a rootful container with host
      network + NET_ADMIN/NET_RAW; capture via gopacket/afpacket + radiotap/Dot11
- [x] Traceroute / path analysis Phase 1: mtr-based path test (TCP/ICMP/UDP),
      per-hop RTT and loss, failure localization, hop-chain Path visualization
- [ ] Traceroute Phase 2: native-Go engine (precise SYN-ACK/RST/filtered
      classification at the destination), Paris/Dublin stable paths for ECMP,
      hop enrichment (rDNS + ASN/owner + geo), path-change/history detection
- [x] Speedtest provider selection: `speedtest` tests take a `provider`
      param — `ookla` (default, existing speedtest-go client), `ndt7`
      (M-Lab's official Go client) or `cloudflare` (speed.cloudflare.com,
      stdlib-only) — so trustworthiness can be cross-checked across
      independently operated fleets
- [x] Capability detection and reporting: agents report which test types they can
      run (detection-based: available external tools like `mtr`, `iw`, or demo
      mode enabled); server filters tests to agents' capabilities and surfaces
      them in the UI; backward-compatible with agents that haven't reported
      capabilities (pushed all tests)

## Server & UI

- [x] HTTPS for the web UI + secure cookies (via the shared TLS cert)
- [ ] ACME/Let's Encrypt automation for public deployments (autocert)
- [x] Everything controllable via API (audited: every UI flow already went
      through `/api/v1`, so GUI/API parity already existed) plus API-key
      (`nlk_...` bearer token) authentication for scripted/CI use alongside
      the session cookie, self-service API Keys UI page, and a full API
      reference (`doc/API.md`)
- [ ] API key expiry / scopes (currently keys never expire and carry the
      full privileges of the owning user)
- [ ] Metrics export to OTEL: keep Prometheus, add OTLP push
- [x] Enhanced logging Phase 1: server and agent `log/slog` output (Info+) teed
      into SQLite (bounded per-scope history via `NETLAMA_LOG_HISTORY`), agent
      logs shipped over the existing control stream (buffered while
      disconnected), Logs UI page with agent/level/source filters and
      auto-refresh. Later: log download, DEBUG-level capture, retention by age.
- [ ] Password change / user self-service in the UI
- [x] On-demand test runs (`RUN_TEST`) from the UI — "Run now" on the Path and
      Results pages
- [ ] Optional result forwarding (e.g. Splunk HEC, port of `legacy/hec-forwarder`)

## Documentation

- [ ] Documentation website (user guide, API reference, deployment guides)
- [ ] OpenAPI spec (`doc/openapi.yaml`) generated from the API surface, plus
      an embedded Swagger UI page served by the server (e.g. `/api/docs`,
      vendored static assets, works with API-key auth)
