# Net-Lama

Net-Lama places small compute units (e.g. a Raspberry Pi) anywhere in a network to run
network measurements: speedtest, ping, DNS, HTTP(S) and TCP connect checks (and —
planned — WLAN KPI sensing). All configuration lives on a central server with a
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
* **Tests** — define named tests (ping/dns/http/tcp/speedtest) with interval and parameters
* **Sites** — create sites and assign tests to them (pushed live to the site's agents)
* **Agents** — create agents in a site; shows the one-time enrollment token with a
  ready-to-run `podman` command, and the live connection status
* **Results** — recent results filterable by site, agent and test
* **Tenants & Users** — admin only

Everything the UI does goes through the JSON API under `/api/v1`
(cookie session via `POST /api/v1/login`): `tenants`, `users`, `sites`,
`sites/{id}/tests`, `tests`, `agents`, `results?siteId=&agentId=&testId=&limit=`.
Admins pass `?tenantId=` to scope requests; tenant users are scoped automatically.

## Running in containers (podman)

Both components ship as distroless images built from the [Containerfile](Containerfile):

```sh
podman build --target server -t netlama-server .
podman build --target agent  -t netlama-agent .

podman network create netlama

podman run -d --name netlama-server --network netlama \
  -p 9090:9090 -p 50051:50051 \
  -v netlama-data:/data:U \
  -e NETLAMA_ADMIN_PASSWORD=changeme \
  netlama-server

# Create the agent in the UI first to get its token, then:
podman run -d --name netlama-agent --network netlama \
  --sysctl net.ipv4.ping_group_range="0 65535" \
  -e NETLAMA_SERVER=netlama-server:50051 \
  -e NETLAMA_TOKEN=<agent-token> \
  netlama-agent
```

The sysctl allows unprivileged ICMP ping inside the container. For rootless podman,
enable lingering once (`loginctl enable-linger`) so containers survive logout.

## Metrics

The server exposes on `:9090/metrics`, all labeled with `tenant`, `site`, `client`
and `test` (plus `target` for ping, `server`+`query` for DNS):

* `netlama_agent_connected`
* `netlama_speedtest_download_mbps`, `netlama_speedtest_upload_mbps`, `netlama_speedtest_latency_ms`
* `netlama_ping_rtt_min_ms`, `netlama_ping_rtt_avg_ms`, `netlama_ping_rtt_max_ms`, `netlama_ping_packet_loss_percent`
* `netlama_dns_resolve_time_ms`, `netlama_dns_success`
* `netlama_http_total_ms`, `netlama_http_ttfb_ms`, `netlama_http_cert_expiry_days`, `netlama_http_up` (labeled by `url`)
* `netlama_tcp_connect_ms`, `netlama_tcp_up` (labeled by `target`)
* `netlama_results_received_total`, `netlama_test_errors_total`

## Development

```sh
make build   # build server + agent into bin/
make proto   # regenerate protobuf/gRPC code (needs protoc + Go plugins)
make vet     # go vet
```

## Roadmap

* WLAN sensor: measure WLAN KPIs (scan results, RSSI per SSID/BSSID) — port of `legacy/wlan-sensor`
* Traceroute / path test (per-hop RTT and loss)
* TLS for the control stream and HTTPS/secure cookies for the UI
* Password change / user self-service in the UI
* On-demand commands (`RUN_SPEEDTEST`, ...) from the UI (contract already in the proto)
* Charts in the UI (currently tables; use Grafana on the Prometheus metrics for graphs)
* Optional result forwarding (e.g. Splunk HEC, port of `legacy/hec-forwarder`)
