# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Net-Lama is a network monitoring system: small sensor agents (Raspberry Pi class)
run tests (ping, DNS, HTTP, TCP, speedtest, traceroute, WLAN scan) and stream
results to a central multi-tenant server with a web UI, JSON API, SQLite storage
and Prometheus metrics. `legacy/` holds the original Python prototype — reference
only, never modify it.

## Commands

```sh
make build          # builds bin/netlama-server and bin/netlama-agent
make vet            # go vet ./...
make pi             # cross-compile the agent for Raspberry Pi (arm64 + armv7)
make proto          # regenerate proto/*.pb.go after editing proto/netlama.proto (needs protoc + Go plugins)

go test ./...                                     # all tests (only internal/probe has tests so far)
go test ./internal/probe -run TestParseMTR -v     # a single test
```

Quick local run: `NETLAMA_ADMIN_PASSWORD=changeme ./bin/netlama-server -db netlama.db`
(UI at :9090, gRPC at :50051), create tenant → site → agent in the UI to get a
token, then `./bin/netlama-agent -server localhost:50051 -token <token>`.
CGO is not required (SQLite via modernc.org/sqlite).

## Architecture

Everything hangs off **one gRPC bidi stream** (`ControlStream` in
`proto/netlama.proto`): the agent dials out, authenticates with its token
(first message), receives its config, and streams results up. The server pushes
config changes and commands (e.g. `RUN_TEST`) down the same stream — agents are
never dialed into, so they work behind NAT. Messages are `oneof` payloads; adding
a test type or command means touching the proto, `make proto`, then both sides.

- `cmd/server`, `cmd/agent` — flag/env parsing and wiring only; logic lives in `internal/`
- `internal/server` — gRPC stream handling, connected-agent registry, config push,
  alert-rule evaluation (`alerts.go`), Prometheus metrics, TLS/mTLS setup
  (`tls.go`, `mtls.go`)
- `internal/agent` — stream client with reconnect/backoff + test scheduler
- `internal/probe` — one file per test type; `*_demo.go` variants emit synthetic
  data when `NETLAMA_WLAN_DEMO=1` / `NETLAMA_TRACEROUTE_DEMO=1`
- `internal/store` — all SQLite access (schema created in `store.go`)
- `internal/api` — JSON REST API under `/api/v1`, cookie sessions; admins scope
  with `?tenantId=`, tenant users are scoped automatically
- `internal/web` — the UI is a single embedded static page
  (`static/index.html` + `app.js`, vanilla JS, no build step); it talks only to
  the JSON API, so every UI feature needs an API endpoint first

Multi-tenant data model: tenant → sites → agents; **tests** are named reusable
definitions assigned to sites; every agent of a site runs the site's tests.
Changes are pushed live to affected connected agents (`PushConfigs`).

## Conventions

- **New server/agent options**: wire flag + `NETLAMA_*` env var in the respective
  `cmd/*/main.go`, and update all of: README, ROADMAP.md checkbox, **both**
  compose files (`compose.yaml` and `compose.sensor.yaml`). Prefer a
  zero-external-dependency default (e.g. self-signed cert, built-in agent CA).
- Completed roadmap items get checked off in ROADMAP.md and a dated entry in
  PROGRESS.md.
- The traceroute and WLAN probes shell out to `mtr` and `iw` and need raw
  sockets — locally they usually fail; use the demo env vars above, or
  `compose.sensor.yaml` (agent-sensor image, host network, NET_RAW/NET_ADMIN)
  for real runs.
- End-to-end verification pattern: build both binaries, start the server with
  `NETLAMA_TLS_SELF_SIGNED=1`, create tenant/site/agent via the JSON API
  (login cookie from `POST /api/v1/login`; the agent token is in the
  create-agent response), then run an agent against it.

## Gotchas

- The agent logs "Registered with server" immediately after *sending* the
  register message, before the server accepts — a rejected agent briefly logs
  success. Known issue; don't trust that line in tests.
- TLS: one cert covers gRPC + HTTPS UI. A plaintext agent cannot connect to a
  TLS server. mTLS (`NETLAMA_MTLS=1`) additionally requires the agent client
  cert CN to equal the agent name (enforced in `ControlStream`).
