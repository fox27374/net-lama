# Net-Lama

Net-Lama places small compute units (e.g. a Raspberry Pi) anywhere in a network to run
network measurements: speedtest, ping, DNS, HTTP(S), TCP connect, traceroute path
analysis and WLAN AP scanning. All configuration lives on a central server with a
multi-tenant web UI; the sensors dial out, authenticate with a token, receive their
configuration and stream results back.

This is the Go rebuild of the original Python prototype (preserved in [legacy/](legacy/)).

## Architecture

```
+-------------+       gRPC bidi stream        +----------------------+
|   agent     | <---------------------------> |        server        |
| (Pi sensor) |  token auth, config down,     |----------------------|
+-------------+  results up (:50051)          | web UI + JSON API    | <-- browser (:9090)
                                              | Prometheus /metrics  | <-- Prometheus (:9090)
                                              | SQLite (tenants,     |
                                              |  users, agents,      |
                                              |  configs, results)   |
                                              +----------------------+
```

* The **agent** opens a single outbound gRPC connection, registers with its **agent token**
  and keeps the stream alive (automatic reconnect with backoff). Works from behind
  NAT/firewalls — no inbound access to the sensor needed.
* The **server** is multi-tenant. Within a tenant: **tests** are named, reusable
  definitions (type, interval, parameters); tests are assigned to **sites**; every site
  has one or more **agents** that execute the tests assigned to their site. Changes to
  tests or assignments are **pushed live** to the affected connected agents.
* Results record which test produced them and are persisted in SQLite (bounded history,
  filterable by site/agent/test in the UI) and exported as Prometheus metrics labeled
  with `tenant`, `site`, `client` and `test`.

## Components

| Path | Description |
|------|-------------|
| `cmd/server` | Central server: gRPC control endpoint, web UI, JSON API, SQLite, Prometheus exporter |
| `cmd/agent` | Sensor agent: runs speedtest/ping/DNS on schedule, streams results |
| `internal/probe` | Test implementations (speedtest.net, ICMP ping, timed DNS lookups) |
| `internal/store` | SQLite persistence (tenants, users, sessions, agents, results) |
| `internal/api` | JSON REST API under `/api/v1` |
| `internal/web` | Embedded single-page web UI |
| `proto/` | gRPC/protobuf contract between agent and server |
| `legacy/` | The original Python implementation, kept as reference |

## Quick start

```sh
make build

# Start the server; UI at http://localhost:9090
NETLAMA_ADMIN_PASSWORD=changeme ./bin/netlama-server -db netlama.db

# In the UI: create a tenant, then an agent -> you get a one-time token

# Start the agent with that token
./bin/netlama-agent -server myserver:50051 -token <agent-token>
```

On first start with an empty database the server creates an `admin` user; the password
comes from `NETLAMA_ADMIN_PASSWORD` or is generated and printed in the log.

Server flags/env: `-grpc`/`NETLAMA_GRPC_ADDR` (default `:50051`), `-http`/`NETLAMA_HTTP_ADDR`
(default `:9090`), `-db`/`NETLAMA_DB`. Agent: `-server`/`NETLAMA_SERVER`,
`-token`/`NETLAMA_TOKEN`, `-id`/`NETLAMA_CLIENT_ID` (informational, defaults to hostname).
Set `DEBUG=1` for debug logging. Cross-compile the agent for a Raspberry Pi with `make pi`.

## Web UI and API

The UI (login with username/password, dark/light theme) has pages for:

* **Overview** — the tenant landing page: site/agent/test counts and per-test health
  (healthy / degraded / failing / no data); click a test to jump to its results
* **Tests** — define named tests (ping/dns/http/tcp/traceroute/wlan_scan/speedtest) with interval and parameters
* **Wireless** — per agent: pick its WLAN sensor interface and view the nearby access
  points (SSID, BSSID, band, channel, RSSI, security) from its latest scan
* **Path** — traceroute path visualization: the hop chain from an agent to a target
  (TCP/ICMP/UDP), per-hop latency and loss, and where a failing path breaks
* **Alerts** — define rules (a test is unhealthy, or a metric such as latency/loss
  crosses a threshold for N consecutive runs) with optional webhook notification;
  see active and recent alerts, with a firing count badge in the nav

The Path and Results pages also have a **Run now** button to trigger a test on a
specific agent immediately instead of waiting for its interval.
* **Sites** — create sites and assign tests to them (pushed live to the site's agents)
* **Agents** — create agents in a site; shows the one-time enrollment token with a
  ready-to-run `podman` command, and the live connection status
* **Results** — recent results filterable by site, agent and test
* **Tenants & Users** — admin only

Everything the UI does goes through the JSON API under `/api/v1`
(cookie session via `POST /api/v1/login`): `tenants`, `users`, `sites`,
`sites/{id}/tests`, `tests`, `agents`, `results?siteId=&agentId=&testId=&limit=`.
Admins pass `?tenantId=` to scope requests; tenant users are scoped automatically.

## Running in containers

Multi-arch images (amd64 + arm64, so they run on a Raspberry Pi) are published to
GHCR by [CI](.github/workflows/containers.yml) on every push to `main`:
`ghcr.io/fox27374/netlama-server` and `ghcr.io/fox27374/netlama-agent`
(`:latest`, `:vX.Y.Z` on tags, `:sha-...`).

The easiest way to run them is the [compose.yaml](compose.yaml)
(docker compose or podman-compose):

```sh
# 1. Start the server; UI at :9090
NETLAMA_ADMIN_PASSWORD=changeme docker compose up -d server

# 2. In the UI: create a tenant, site, tests and an agent -> copy the token
echo "NETLAMA_TOKEN=<agent-token>" >> .env

# 3. Start the agent
docker compose up -d
```

To build locally instead, use the [Containerfile](Containerfile):
`podman build --target server -t netlama-server .` (same for `agent`), and point
compose at them via `NETLAMA_SERVER_IMAGE`/`NETLAMA_AGENT_IMAGE` in `.env`.

The agent's sysctl in the compose file allows unprivileged ICMP ping inside the
container; the *host* must also allow it (`sysctl net.ipv4.ping_group_range` — wide
open on Debian/RPi OS, needs `0 2147483647` in `/etc/sysctl.d/` on Ubuntu). For
rootless podman, enable lingering once (`loginctl enable-linger`) so containers
survive logout.

### Sensor agents (WLAN scan and traceroute)

The WLAN scan and traceroute probes shell out to external tools (`iw`, `mtr`) that
are **not** in the default distroless agent image and need raw-socket access. Use
the ready-made [compose.sensor.yaml](compose.sensor.yaml), which runs the agent as:

1. **the sensor image** (`agent-sensor` target — Debian-slim with `iw` + `mtr`);
2. with **`cap_add: [NET_RAW, NET_ADMIN]`** — `NET_RAW` for traceroute (custom-TTL
   packets + receiving ICMP), `NET_ADMIN` for WLAN scanning; and
3. with **`network_mode: host`**. This is required for path tracing: rootless
   user-mode network stacks (`slirp4netns` / `pasta`) NAT everything, so the agent
   would only ever see the destination and *none* of the intermediate routers. With
   host networking the probe packets traverse the real routing table.

This runs **rootless — no sudo/rootful needed** (verified with podman-compose). Keep
`net.ipv4.ping_group_range` open on the host (as for ping), and `loginctl
enable-linger` so the containers survive logout.

```sh
podman build --target agent-sensor -t netlama-agent-sensor .   # or pull from GHCR
NETLAMA_AGENT_IMAGE=netlama-agent-sensor \
  podman-compose -f compose.sensor.yaml up -d
```

Because it uses host networking, the agent reaches the server on the published host
port (`NETLAMA_SERVER=127.0.0.1:50051`, the compose default). Note that TCP-mode
traceroute shows only hops that return ICMP for TCP-SYN probes — many networks
answer for ICMP-mode traceroute but not TCP, so an ICMP test often shows a fuller
path while a TCP test better reflects the real application path and port reachability.

Until the above is in place, set `NETLAMA_WLAN_DEMO=1` and/or
`NETLAMA_TRACEROUTE_DEMO=1` to emit synthetic data so you can use the Wireless and
Path UIs on a host without a radio or raw-socket access. Monitor-mode client
sensing and native-Go traceroute are later phases — see [ROADMAP.md](ROADMAP.md).

## Metrics

The server exposes on `:9090/metrics`, all labeled with `tenant`, `site`, `client`
and `test` (plus `target` for ping, `server`+`query` for DNS):

* `netlama_agent_connected`
* `netlama_speedtest_download_mbps`, `netlama_speedtest_upload_mbps`, `netlama_speedtest_latency_ms`
* `netlama_ping_rtt_min_ms`, `netlama_ping_rtt_avg_ms`, `netlama_ping_rtt_max_ms`, `netlama_ping_packet_loss_percent`
* `netlama_dns_resolve_time_ms`, `netlama_dns_success`
* `netlama_http_total_ms`, `netlama_http_ttfb_ms`, `netlama_http_cert_expiry_days`, `netlama_http_up` (labeled by `url`)
* `netlama_tcp_connect_ms`, `netlama_tcp_up` (labeled by `target`)
* `netlama_wlan_aps_visible` (labeled by `interface`)
* `netlama_path_rtt_ms`, `netlama_path_hops`, `netlama_path_reached` (labeled by `target`)
* `netlama_results_received_total`, `netlama_test_errors_total`

## Development

```sh
make build   # build server + agent into bin/
make proto   # regenerate protobuf/gRPC code (needs protoc + Go plugins)
make vet     # go vet
```

## Roadmap

See [ROADMAP.md](ROADMAP.md) for the full backlog (Kubernetes/Helm, zero-touch agent
enrollment via DNS + WireGuard, TLS/mTLS, agent-to-agent perfmon, alerting, OTEL
export, native packages, WLAN sensing, and more).
