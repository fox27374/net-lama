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
- [x] Agent resource monitoring: CPU, memory, storage — reported over the
      stream and visible in the UI/metrics (host-level readings; per-cgroup /
      container-scoped readings a possible later refinement)
- [x] Agent self-health: explainable health status (healthy/degraded/unhealthy/
      unknown) computed server-side from agent self-metrics, connection stability,
      and error logs; badge in UI; Prometheus gauge; `/api/v1/agents` health field
- [x] Alerting and thresholding: per-test rules (unhealthy, or latency/loss/
      throughput thresholds) with consecutive-breach counts, per-target alert
      state, webhook notification, Alerts UI + nav badge. Later: email/SMTP,
      more metrics, maintenance windows, per-rule scoping to sites/agents.
- [x] WLAN Phase 1: interface inventory + managed-mode AP/SSID scan (agent reports
      wireless interfaces, per-agent interface selection, periodic scan, Wireless UI)
- [x] WLAN Phase 2: monitor-mode client sensing — per-station MAC/RSSI/SNR/rate/MCS
      per SSID; needs a monitor-capable adapter and a rootful container with host
      network + NET_ADMIN/NET_RAW; capture via gopacket/afpacket + radiotap/Dot11
- [x] Traceroute / path analysis Phase 1: mtr-based path test (TCP/ICMP/UDP),
      per-hop RTT and loss, failure localization, hop-chain Path visualization
- [ ] SaaS / cloud service tests: reachability and quality checks for online
      services (MS Teams, Webex, social networks) and cloud platforms
      (AWS, Azure, GCP) — curated endpoint sets per service
- [ ] Host logs from the agent: fetch/tail selected log files or journald
      from the sensor host, viewable on the server
- [ ] Packet capture (wireshark-style) on the agent: start a filtered
      capture from the UI, display a summary or download the pcap from
      the server
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
- [ ] Roles and permissions: finer-grained access than the current
      admin / tenant-user split (e.g. read-only viewer, per-site operator)
- [x] On-demand test runs (`RUN_TEST`) from the UI — "Run now" on the Path and
      Results pages
- [ ] Optional result forwarding (e.g. Splunk HEC, port of `legacy/hec-forwarder`)
- [x] Dashboard deep-links: every dashboard block links to its page
      (e.g. clicking agents opens the Agents page, sites the Sites page)
- [ ] Configurable dashboard with widgets (add/remove/reorder blocks)
- [x] Separate configure vs. view menus for sites, agents, tests, wireless
      and logs (sidebar split: viewing pages on top, Sites/Agents/Tests/Alert
      rules under Configuration)
- [x] Modify the path view to look more professional (vertical subway line,
      MTR-style latency bars in table, path-history heatmap)
- [x] Alert-rule configuration UI as its own menu item
- [x] Logo for the web UI (theme-aware transparent logo in sidebar/login +
      light/dark favicons)
- [ ] Version tag reported by server and agent, shown in UI/API
- [ ] Configurable result retention: time-based (e.g. "keep 30 days") and/or
      per-test caps instead of the fixed 5000-results-per-agent limit, which
      is shared across all of an agent's tests (a chatty 1-minute test crowds
      out slower ones)
- [ ] Path history window selector (e.g. last 48 runs / 24h / 7d) — the
      results API already supports `since`, so the heatmap can query by time
      instead of a fixed run count

## Documentation

- [ ] Documentation website (user guide, API reference, deployment guides)
- [ ] OpenAPI spec (`doc/openapi.yaml`) generated from the API surface, plus
      an embedded Swagger UI page served by the server (e.g. `/api/docs`,
      vendored static assets, works with API-key auth)
