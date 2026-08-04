# A result's test type comes from its test definition, not its payload shape

`internal/server/server.go` used to derive `results.test_type` from the
result message itself (`testtype.TypeOf`, a switch over the `TestResult`
oneof), which works only while every test type owns a distinct result
message. The `saas` type breaks that assumption deliberately — it reuses
`HttpResult` and `TcpResult` rather than duplicating their timing fields —
so ingest now resolves `result.TestId` to the stored test's type and falls
back to `TypeOf` only when the test is unknown.

## Considered alternatives

- **Give `saas` its own `SaasResult` message.** Keeps `TypeOf` correct,
  but duplicates every HTTP timing field and adds a second store/UI
  rendering path to maintain in parallel with the first.
- **Accept saas results being stored as `http`/`tcp`.** Free, and it
  hollows out the point of a first-class type: `?type=saas` finds nothing
  and the Results page shows service runs as bare HTTP runs.

## Consequences

A test's type is now a property of its definition, which is where it was
always configured — the payload shape is a fallback for orphaned results.
Any future type may reuse an existing result message. The cost is one
lookup on the result ingest path, cached per connected agent.
