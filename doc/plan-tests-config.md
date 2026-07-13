# Plan: Tests under Configuration + alert-rule assignment from the test dialog

UI-only (internal/web/static/). Backend stays: rules already belong to a
test (alert_rules.test_id); "assigning a rule to a test" = PUT the rule
with a new testId (endpoint exists). No Go changes.

## 1. Move Tests into the Configuration nav group

index.html sidebar: move the `data-nav="tests"` button out of the main
group into the "Configuration" group, above "Alerts & Alert Rules".
Nothing else about the section changes (hash nav picks it up as-is).

## 2. Tests list: "Alert Rule" column

- loadTests() additionally fetches `/api/v1/alert-rules` + tenantParam()
  (parallel with the tests fetch).
- New column "Alert Rule" between Parameters and the actions column:
  comma-joined names of all rules whose testId === test.id, esc()'d;
  muted "—" when none. Update the thead accordingly.

## 3. Test dialog: assign an alert rule

- Add a labeled control at the bottom of the test form (dlg-test):
  - If at least one alert rule exists in the tenant: a select
    `#t-alert-rule` with option "— none —" plus every rule whose metric
    can apply to the currently selected test type. Applicability map:
    unhealthy → all types; latency_ms → ping, dns, http, tcp,
    traceroute, speedtest; loss_percent → ping; download_mbps /
    upload_mbps → speedtest. Re-filter the options when the test-type
    select changes. If the test being edited already has attached rules,
    preselect the first one.
  - If NO rules exist: hide the select and show a ghost button
    "Create alert rule →" that closes the test dialog, navigates via
    `navTo("alertcfg")` and opens the New-rule dialog (reuse the
    existing btn-new-rule click path); if the test being edited exists
    (edit mode), preselect it in the rule dialog's test dropdown (a
    module-level pending variable, same pattern as pendingResultTest —
    apply + clear it where the rule dialog is populated).
- On save: after the test create/update succeeds (for create, use the id
  from the response), compare: if a rule is selected and its testId !==
  this test id → PUT /api/v1/alert-rules/{ruleId} re-pointing testId to
  this test (send the rule's existing fields unchanged otherwise). If
  "— none —" is selected do NOTHING (no detach semantics — a rule always
  belongs to some test; removing/re-pointing is done on the rules page).
  Mention this in a small muted hint under the control ("Assigning moves
  the rule to this test").
- Reload the tests list after save (existing behavior) so the new column
  reflects the change.

## Docs

PROGRESS.md: append to today's section. README: only if it describes the
sidebar groups (check). No ROADMAP change.

## Verification

1. make build, go vet ./..., go test ./....
2. Serve check (scratch server): / has the tests button inside the
   Configuration group and the "Alert Rule" th; app.js has the
   applicability map, the PUT-on-assign call, and the create-rule
   fallback button wiring.
3. Quote in the report: the new thead row, the applicability map, and
   the PUT call snippet.

## Constraints

No Go/API changes, no new dependencies, don't touch vendor/. Keep
existing test-dialog behavior (params per type) intact. Do not commit.
