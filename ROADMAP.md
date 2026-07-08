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
- [ ] Agent authentication at the server (mutual auth on top of the token,
      e.g. mTLS or signed enrollment)
- [ ] Encryption for the gRPC control stream (TLS, ideally mTLS)

## Tests & monitoring

- [ ] Perfmon tests **between agents of the same tenant** (agent-to-agent
      throughput/latency, e.g. iperf-style with one agent as reflector)
- [ ] Agent resource monitoring: CPU, memory, storage — reported over the
      stream and visible in the UI/metrics
- [ ] Alerting and thresholding (per test/tenant; notify on failing/degraded)
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

## Server & UI

- [ ] HTTPS for the web UI with Let's Encrypt (ACME) + secure cookies
- [ ] Everything controllable via API (audit the UI-only flows; document the API)
- [ ] Metrics export to OTEL: keep Prometheus, add OTLP push
- [ ] Enhanced logging; logs visible in the web UI (agent + server)
- [ ] Password change / user self-service in the UI
- [ ] On-demand test runs (`RUN_TEST`) from the UI (contract already in the proto)
- [ ] Optional result forwarding (e.g. Splunk HEC, port of `legacy/hec-forwarder`)

## Documentation

- [ ] Documentation website (user guide, API reference, deployment guides)
