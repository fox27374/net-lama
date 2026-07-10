# Plan: treat uninterpolated compose placeholders as unset env vars

## Problem (observed on a real deployment, 2026-07-10)

Older podman-compose versions (e.g. the one Debian 12 ships) do NOT
interpolate `${VAR:-default}` in compose `environment:` values. The
container then receives the literal string, e.g. `NETLAMA_TLS=${NETLAMA_TLS:-}`.
Today `envEnabled` treats any non-empty value as true and `envOr` returns
the literal, so behavior becomes accidental: on the observed install, TLS
only worked because two garbage-truthy values happened to cancel out
(`NETLAMA_TLS` literal → TLS on, `NETLAMA_TLS_INSECURE` literal → verify
off). A different combination would fail confusingly. The earlier commit
449b67c fixed exactly this for the two `NETLAMA_*_DEMO` vars only; this
plan generalizes it.

## Change

In BOTH `cmd/agent/main.go` and `cmd/server/main.go` (the helpers are
duplicated there — keep them duplicated, matching the existing structure;
do NOT introduce a new shared package for this):

1. Add a tiny guard used by all env helpers (`envOr`, `envEnabled`, and
   the server's `envIntOr`): if the raw value starts with `${`, treat the
   variable as unset (fall back to default / disabled). Log nothing — these
   helpers run before the logger exists and the situation is common on old
   podman-compose; silence matches how empty values are handled.
2. Remove/absorb the now-redundant special-casing from 449b67c if it lives
   in probe/env.go or similar — one mechanism, not two (check
   internal/probe/env.go: if it has its own env parsing for the DEMO vars,
   apply the same guard there or route it through the same rule; keep the
   change minimal and consistent).
3. Unit tests: the helpers are in package main — if that makes testing
   awkward, a `main_test.go` per cmd is fine (table test: normal value,
   empty, `${NETLAMA_X:-}` literal, `${NETLAMA_X:-default}` literal → all
   placeholder forms behave as unset).
4. README: one short note in the containers section — old podman-compose
   versions pass `${...}` through literally; net-lama treats such values
   as unset, but for full control put every agent variable explicitly in
   `.env`.
5. PROGRESS.md: add a line under 2026-07-10. No ROADMAP change (this is a
   robustness fix, not a feature). No compose changes.

## Verification

1. `make build`, `go vet ./...`, `go test ./...` (including the new tests).
2. Manual check: run the built agent with
   `NETLAMA_TLS='${NETLAMA_TLS:-}' NETLAMA_TOKEN=x ./bin/netlama-agent -server localhost:1`
   and confirm from its startup log line that `tls` is FALSE (placeholder
   treated as unset), whereas `NETLAMA_TLS=1` shows true. No full e2e
   needed — this is env parsing, the startup log is the observable.

## Constraints

- No new dependencies, no proto changes, no behavior change for values
  that don't start with `${`.
- Do not commit; leave changes in the working tree.
