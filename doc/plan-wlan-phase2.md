# Plan: WLAN Phase 2 — monitor-mode sensing (`wlan_sense`)

Roadmap item: "WLAN Phase 2: monitor-mode client sensing — per-station
MAC/RSSI/SNR/rate/MCS per SSID; capture via gopacket/afpacket +
radiotap/Dot11". Scope agreed with the user: **Phase 2 core + channel
utilization** (airtime busy stats from `iw survey dump`). Target hardware
exists: MT7612U (`mt76x2u`) on a Pi at ataltrp01, monitor-capable, dedicated
to sensing (`wlan1`; onboard `wlan0` stays managed). Deployment will be a
rootful podman container with host network + NET_ADMIN/NET_RAW — done by the
operator later, NOT part of this implementation task.

## New test type: `wlan_sense`

One sweep per scheduled run: hop across channels, capture in monitor mode,
aggregate stations and per-channel airtime, emit one result.

### Proto (proto/netlama.proto, then `make proto`)

- `TestSpec.params` oneof += `WlanSenseParams wlan_sense = <next>;`
- `TestResult.result` oneof += `WlanSenseResult wlan_sense = <next>;`

```proto
// WlanSenseParams configures a monitor-mode sweep. The interface is the
// per-agent Config.wlan_interface (must be monitor-capable).
message WlanSenseParams {
  repeated uint32 channels = 1; // empty = all channels the phy supports
  uint32 dwell_ms = 2;          // per-channel capture time, default 400
}

message WlanSenseResult {
  string interface = 1;
  bool demo = 2;                    // synthetic (NETLAMA_WLAN_SENSE_DEMO)
  repeated WlanStation stations = 3;
  repeated WlanChannelStat channels = 4;
  uint32 sweep_ms = 5;              // total sweep duration
}

message WlanStation {
  string mac = 1;
  string bssid = 2;        // empty for probe-only stations
  string ssid = 3;         // resolved from beacons when known
  int32 rssi_dbm = 4;      // last seen
  int32 rssi_avg_dbm = 5;
  double rate_mbps = 6;    // last observed data rate (0 = unknown)
  int32 mcs = 7;           // -1 = unknown/legacy
  uint32 frames = 8;
  bool probe_only = 9;     // only seen probing, not associated
  int64 last_seen_ms = 10; // unix ms
}

message WlanChannelStat {
  uint32 channel = 1;
  uint32 freq_mhz = 2;
  uint64 active_ms = 3;
  uint64 busy_ms = 4;
  double utilization_pct = 5; // busy/active*100, 0 if survey unavailable
  uint32 frames = 6;
}
```

Old agents ignore unknown oneof fields; capability-aware dispatch already
prevents `wlan_sense` tests reaching agents that don't claim it.

### Agent probe (internal/probe/)

Files: `wlansense.go` (shared types + demo + iw helpers), capture in
`wlansense_linux.go` (`//go:build linux`), stub `wlansense_other.go`
(`//go:build !linux`) returning a clear error — **`make build` on macOS and
`make pi` cross-compiles must both pass**. Demo data behind
`NETLAMA_WLAN_SENSE_DEMO=1` following the existing `*_demo.go` pattern
(realistic: ~8-15 stations across 2-4 BSSs, 2.4+5 GHz channels with varied
utilization).

Capture flow (linux):
1. Remember the interface's current type (`iw dev <if> info`); set monitor:
   `iw dev <if> set type monitor` (interface down/up around it as needed via
   `ip link`). Restore the original type afterwards (defer), skip restore if
   it already was monitor.
2. Channel list: from params; if empty, derive from `iw phy <phy> channels`
   (skip disabled/radar-only). Sort 2.4 GHz first.
3. Per channel: `iw dev <if> set channel <n>`, then capture for `dwell_ms`
   via `github.com/gopacket/gopacket` + `.../afpacket` (AF_PACKET, no CGO).
   Parse RadioTap + Dot11 layers:
   - Beacons/probe responses → BSSID → SSID + channel map.
   - Data frames → station = transmitter that is not the BSSID
     (handle ToDS/FromDS address ordering); collect RSSI
     (RadioTap DBMAntennaSignal), rate (RadioTap Rate legacy, or MCS/VHT
     fields → Mbps lookup; MCS index when present), frame count.
   - Probe requests → stations with `probe_only=true`, `bssid=""`.
   - Ignore FCS-failed frames if flagged.
4. After the sweep: `iw dev <if> survey dump` → per-frequency active/busy
   time (mt76 reports these); compute utilization_pct. Parser must tolerate
   missing survey data (utilization 0).
5. Aggregate per station MAC (avg RSSI incremental, last rate/MCS, frames).
   Cap stations at ~256 per sweep to bound the result size.

`iw`/`ip` are shelled out like Phase 1 (`wifi.go` has parsing precedent —
reuse its `channelAndBand` helper for freq→channel).

### Capabilities (internal/probe/capabilities.go)

Claim `wlan_sense` when demo env is set, OR (a monitor-capable wireless
interface exists AND the process looks privileged — euid 0 or CAP_NET_ADMIN;
a simple euid check is acceptable). Extend the interface inventory with
monitor support: parse `iw phy` "Supported interface modes" for `monitor`
(the existing parseIWDev only marks interfaces already IN monitor type).

### Agent wiring (internal/agent/scheduler.go)

New `TestSpec_WlanSense` case mirroring `runWlanScan` (uses the same
per-agent `wlan_interface` selection; error result when the interface isn't
set/capable).

### Server side

- `internal/server/config.go`: validate params (`dwell_ms` 100–2000 default
  400; channels sanity 1–177), test type registered everywhere type lists
  exist (ValidateTestDef, metric applicability, capability filtering just
  works by type string).
- `internal/store/overview.go`: primary metric for `wlan_sense` = **max
  channel utilization_pct** (unit "%"), higher-is-worse (default direction)
  so state thresholds (green/orange/red bands) work unchanged. Series/current
  via a new extract function on the payload.
- Prometheus (wherever Phase 1 wlan gauges live): add
  `netlama_wlan_stations{tenant,site,agent,test}` (count) and
  `netlama_wlan_channel_utilization_pct{...,channel}`.
- Alert rules: `unhealthy` and `state` metrics apply automatically; add
  `wlan_sense` to METRIC_APPLICABILITY in app.js for those two only.

### UI (internal/web/static/, vanilla JS)

Wireless page gains two blocks fed by the latest `wlan_sense` result per
agent (results API already returns payloads):
- **Clients**: table MAC / SSID (or "probing") / RSSI dBm (colored by
  signal: ≥-60 ok, -75..-60 warn, <-75 bad) / rate Mbps / MCS / frames /
  last seen. Group or sort by SSID.
- **Channel utilization**: per-channel horizontal bars (utilization %,
  label "ch N (freq) — busy X%/frames Y"), split 2.4/5 GHz.
Tests dialog: new type "WLAN sensing (monitor mode)" with channels
(comma-separated, blank = all) and dwell ms inputs; BAND_UNIT += `wlan_sense:
"%"`. Empty state hints that a monitor-capable interface + privileged agent
is required.

### Housekeeping (CLAUDE.md conventions)

- `NETLAMA_WLAN_SENSE_DEMO` documented in README + **both** compose files.
- README test-type table row; doc/API.md params/result shapes.
- ROADMAP: tick WLAN Phase 2. PROGRESS.md dated entry (2026-07-17).
- `make proto` after editing the proto (protoc 34.1 + plugins are installed).

### Verification (all must pass)

- `make build` (darwin!), `make pi` (arm64+armv7 cross), `go vet ./...`,
  `go test ./...`.
- Unit tests: radiotap/Dot11 aggregation with synthetic frames (build frames
  via gopacket serialization or hex fixtures), survey-dump parser, channel
  list parser, demo generator sanity.
- E2E (demo): server `NETLAMA_TLS_SELF_SIGNED=1` + agent
  `NETLAMA_WLAN_SENSE_DEMO=1`, seed tenant/site/agent + `wlan_sense` test via
  JSON API, confirm results arrive, overview shows utilization as primary
  metric, capability list includes wlan_sense. Clean up processes.

### Out of scope (later phases / operator)

- Deployment to ataltrp01 (rootful podman, done by operator after CI images).
- Retry/FCS-error link quality metrics; 6 GHz; deauth detection.
- MAC randomization de-duplication.
