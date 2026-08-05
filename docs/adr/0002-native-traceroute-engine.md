# Traceroute is a native Go engine on unprivileged sockets, not mtr

Phase 1 traced paths by shelling out to `mtr --json`, which works well but
drags its cost into operations: the agent needs the binary (so path testing
is a sensor-image feature), it needs `NET_RAW` (which `podman inspect`
misreports on at least one of our hosts, silently breaking ICMP tracing
after a container recreate), and every run forks a process whose orphans the
agent must reap. Phase 2 replaces it with an engine in-process that sends
UDP/TCP probes with `IP_TTL` on ordinary sockets and reads the ICMP
time-exceeded replies off the socket error queue (`IP_RECVERR`), falling
back to raw sockets only where they are already available.

## Considered alternatives

- **Keep mtr, add the missing features around it.** Cheapest by far, but
  destination classification (SYN-ACK vs RST vs filtered) and a constant
  ECMP flow are not things `mtr --json` exposes — they are properties of how
  the probes are sent.
- **Native engine on raw sockets**, matching what mtr does. Simplest
  implementation and most capable, but traceroute stays privileged and
  sensor-image-only, which leaves the operational problem exactly where it
  was.
- **Run both engines behind a parameter**, like `speedtest`'s provider.
  Rejected as a permanent arrangement: two implementations of the same
  measurement, indefinitely. Kept only as the transitional signal — results
  record which engine produced them.

## Consequences

Traceroute becomes a baseline capability on Linux, available on the slim
distroless image with no added capabilities; on darwin it depends on raw
sockets being obtainable. Measurements change subtly, so results carry an
`engine` field and the staged rollout doubles as an A/B against the same
targets. The unprivileged path is `x/sys/unix` `recvmsg(MSG_ERRQUEUE)` work
rather than stdlib, so it is proven by a spike on real hardware — including
inside the distroless image — before the rest of the plan is built; if that
fails, the fallback is raw sockets and traceroute stays privileged.
