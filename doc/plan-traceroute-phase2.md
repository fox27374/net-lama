# Plan: Traceroute Phase 2 (native engine, ASN enrichment, path-change detection)

Phase 1 shipped an `mtr`-backed path test: per-hop RTT and loss, failure
localization, the subway-line Path view and the history heatmap. Phase 2
replaces the shell-out with a native Go engine and builds the two things
that engine makes possible — knowing what the destination actually said,
and knowing when the path changed.

## Roadmap corrections

The ROADMAP line for this item is out of date in two ways. Fix it when the
work ships, not before (repo convention):

- **rDNS is already done.** `resolveHopNames` in
  `internal/probe/traceroute.go` resolves hop IPs in parallel with a 1500 ms
  budget, stored in `Hop.host_name` (proto field 9), planned in
  `doc/plan-path-rdns.md`. Enrichment in Phase 2 means **ASN/owner/country
  only**.
- **Path history already exists.** The ECharts heatmap with its window
  selector plots per-hop latency/loss over time. What is missing is
  *change detection* — noticing and recording that the route differs — not
  the ability to look at history.

## Design decisions (fixed — do not redesign)

1. **The native engine replaces `mtr` outright.** One engine, one code
   path. This removes the external binary that forces the sensor image, the
   `NET_RAW` juggling (`podman inspect` misreports it on rp02, which broke
   ICMP tracing there on 2026-08-04), and the zombie-`mtr-packet` class of
   bug that once exhausted PIDs on a Pi (fixed repo-wide in 99607b0 by
   `--init`, but the shape of the problem is the shell-out).
2. **Unprivileged sockets first.** Send UDP/TCP probes with `IP_TTL` set on
   ordinary sockets and read the ICMP time-exceeded replies off the socket
   error queue (`IP_RECVERR` + `recvmsg(MSG_ERRQUEUE)`). Raw sockets are
   used only when the process already has them, for ICMP-mode probing.
   **This is the load-bearing assumption of the whole plan and step one is a
   spike to prove it** (see below). If the spike fails, fall back to raw
   sockets throughout and decision 4 changes with it.
3. **Paris-style constant flow.** The flow identifier (UDP checksum /
   TCP source port) is held constant across every TTL of a run and across
   runs, derived from the test id. Under ECMP each run then traces the same
   branch, which is what makes the flat hop list honest and change
   detection meaningful rather than a measurement of hash buckets.
   Enumerating parallel paths (Dublin) is explicitly out of scope: it turns
   the result into a DAG and invalidates the hop list, the subway view, the
   heatmap's per-TTL rows and the alert subject.
4. **Capability detection is platform-aware.** On Linux the unprivileged
   path always works, so `traceroute` becomes a baseline capability
   alongside ping/dns/http — including on the slim distroless image with no
   `NET_RAW`. On darwin/other, report it only when a raw socket actually
   opens. Demo mode keeps reporting it unconditionally.
5. **`destination_state` is a new field**, not a redefinition. `reached`
   and `status` keep exactly today's meaning, so every stored result and
   the whole Path view stay valid. The new field carries what only a native
   engine can see: `open` (SYN-ACK), `closed` (RST), `filtered` (no reply or
   admin-prohibited), `unreachable` (ICMP dest-unreach), `echoed` (ICMP
   echo reply).
6. **Every result records its `engine`** (`"native"`, empty for mtr-built
   agents), the same trick `speedtest` uses for `provider`. A step in a
   latency heatmap then has an explanation instead of being a mystery, and
   the staged rollout (tpr06 first, Pis after) becomes a free A/B: the same
   targets measured both ways at the same time.
7. **Enrichment is server-side and embedded.** `internal/asn` embeds a
   gzipped IP→ASN/owner/country table (iptoasn.com, CC0, ~2 MB gz) and
   resolves at display time behind an endpoint — the `internal/oui` pattern
   (385 KB embedded IEEE registry, `GET /api/v1/oui`). Agents send hop IPs
   as they do today and need no third-party egress, which matters because
   sensors sit in restricted networks. No city-level geo: it needs a
   MaxMind licence and a far bigger database for little value on a
   corporate path.
8. **Change detection runs server-side at ingest** and stores events.
   `handleResult` compares the new signature against the previous run for
   the same test+agent and writes a row when they differ. That gives a
   queryable history, timeline markers, and a numeric metric alert rules
   can watch — from machinery that already exists.
9. **Anonymous hops are wildcards in the signature.** The signature is the
   ordered list of hop IPs; a hop that answered in one run and was silent in
   the other *matches*. ICMP rate-limiting makes hops go quiet constantly,
   and counting that as a reroute would bury the real events. A change is
   recorded only when two runs name different IPs at the same TTL, or the
   path length changes.
10. **Events are classified intra-AS vs inter-AS** using decision 7's data,
    so alerting can default to the changes that moved traffic between
    networks while still recording reroutes inside one operator.

## Stage 0: the socket spike (do this first)

Before any of the below, prove decision 2 on real hardware:

- Small Go program on tpr06 (container, no `NET_RAW`) that sends a UDP
  packet with `IP_TTL=1` to a known target and reads the ICMP
  time-exceeded from the error queue via `unix.Recvmsg` with
  `unix.MSG_ERRQUEUE`, printing the responding router IP.
- Repeat for a TCP `connect()` with a low TTL.
- Repeat inside the **distroless** agent image, which is the environment
  the capability decision depends on.

If either fails, stop and revisit decisions 2 and 4 before building.

## Stage 1: native engine replaces mtr

Agent + proto. The risky stage, deliberately isolated.

### Proto (`proto/netlama.proto` + `make proto`)

```proto
// in TracerouteResult
string destination_state = 9;  // open|closed|filtered|unreachable|echoed
string engine = 10;            // "native"; empty means the old mtr probe
```

### Probe (`internal/probe/traceroute.go`)

- New engine: for each TTL 1..maxHops send `probes` packets with the
  test's constant flow id, collect responders, RTT, loss and jitter per
  hop — the same `TracerouteResult`/`Hop` structs Phase 1 already fills, so
  `parseMTR` and its test go away without anything downstream changing.
- Keep `resolveHopNames` exactly as is (already parallel, already bounded).
- Classify the destination reply into `destination_state`.
- `traceroute_demo.go` gains the two new fields so demo mode stays a
  faithful stand-in.
- The `Traceroute` signature gains the flow id (derived from the test id by
  the caller in `internal/agent/scheduler.go`).

### Capability (`internal/probe/capabilities.go`)

Replace the `exec.LookPath("mtr")` check per decision 4. Platform split
follows the existing `_linux.go`/`_other.go` file convention used by the
WLAN probes.

### Containerfile

Drop `mtr` from the agent-sensor image; keep `iw`.

## Stage 2: ASN enrichment

Server + UI only; no agent change, no proto change.

- `internal/asn/asn.go` + `asn.tsv.gz` (embedded), mirroring
  `internal/oui/oui.go`: parse once into a sorted CIDR table, binary-search
  lookups.
- `GET /api/v1/asn?ip=…` → `{asn, owner, country}`, authenticated, not
  tenant-scoped (it describes the internet, not anyone's data — the same
  reasoning as `/api/v1/test-types` and `/api/v1/oui`). Batch form
  (`?ip=a,b,c`) so the Path view resolves a whole trace in one request.
- Path view: hop rows show owner/ASN next to the rDNS name; consecutive
  hops in one AS are visually grouped.
- `doc/API.md` gets the endpoint.

## Stage 3: path-change detection

Server + UI only.

### Store

```sql
CREATE TABLE IF NOT EXISTS path_changes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id        TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    test_id         TEXT NOT NULL REFERENCES tests(id) ON DELETE CASCADE,
    time            TIMESTAMP NOT NULL,
    first_diff_ttl  INTEGER NOT NULL,
    from_sig        TEXT NOT NULL,   -- ordered hop IPs, '*' for wildcards
    to_sig          TEXT NOT NULL,
    scope           TEXT NOT NULL    -- 'intra-as' | 'inter-as' | 'unknown'
);
CREATE INDEX IF NOT EXISTS idx_path_changes_test
    ON path_changes (test_id, agent_id, time);
```

Bounded like results are (per test+agent cap, reusing the existing prune
pattern), so a flapping path cannot grow the database without limit.

### Server

- Comparison at ingest in `internal/server` (called from `handleResult`,
  which already resolves the test definition since
  `docs/adr/0001-test-type-from-definition.md`).
- Signature per decision 9; classification per decision 10.
- New alert metric on the `traceroute` registry entry: `path_changed`
  (1 on a run that changed, 0 otherwise), so an existing consecutive-breach
  rule can alert on reroutes with no new alerting machinery.

### API + UI

- `GET /api/v1/path-changes?testId=&agentId=&since=` (tenant-scoped like
  results).
- Path page: markers on the history heatmap where changes happened, and an
  event list underneath — old → new hop, first differing TTL, intra/inter-AS
  badge.

## Verification

- `go test ./...`, `make vet`, `make build`, plus the existing
  `TestParseMTR` deleted along with the parser it covers.
- Unit: signature comparison (wildcards match, length change detected,
  different IP at the same TTL detected), destination classification, ASN
  lookup at CIDR boundaries.
- E2E per CLAUDE.md, then real hardware: tpr06 first, watching the same
  targets it already traces (`Path TCP`, `Path ICMP`), comparing native vs
  mtr results before rolling to rp01/rp02.
- Explicit check that the **slim** agent traces without `NET_RAW`, since
  that is the operational payoff of decision 2.

## Not in scope

Dublin-style parallel path enumeration (decision 3), city-level geo
(decision 7), IPv6-specific path features beyond what the engine gets for
free, and MPLS label exposure (`ICMP extensions`) — a possible Phase 3 once
the engine exists.
