# Plan: alerting rework — targets, clear hysteresis, Configuration UI

Two phases. Phase A = backend (schema, engine, notifiers, API). Phase B =
UI (Configuration sidebar group + "Alerts & Alert Rules" page). A coder
implements ONE phase per task; the task prompt says which.

Existing pieces to build on (do not rewrite): alert_rules/alerts tables
(store.go), consecutive-breach engine + webhook notify
(internal/server/alerts.go), API routes in internal/api/alerts.go
(GET/POST/DELETE alert-rules, GET /api/v1/alerts), rules UI block inside
the Alerts section, addColumn migration helper (store.go ~194).

## Phase A — backend

### A1. Schema (internal/store)

- New table `alert_targets`: id TEXT PK, tenant_id (FK tenants, CASCADE),
  name TEXT, type TEXT ('email'|'snmp'|'webhook'|'script'), config TEXT
  (JSON, type-specific), created_at. CRUD store methods, tenant-scoped.
- alert_rules: addColumn `clear_threshold REAL` (nullable — NULL means
  "clear when the fire condition simply stops matching"), addColumn
  `clear_count INTEGER NOT NULL DEFAULT 1`, addColumn `target_ids TEXT
  NOT NULL DEFAULT '[]'` (JSON array of alert_target ids; rules are few,
  no junction table).
- Migration: any rule with a non-empty webhook_url gets a webhook target
  auto-created (name "<rule name> webhook", config {"url": ...}) in its
  tenant, its id added to target_ids, webhook_url set to ''. Keep the
  column itself (harmless).

### A2. Engine hysteresis (internal/server/alerts.go)

- Add goodCount map alongside breachCount (same key scheme).
- On breach: goodCount[key]=0, breachCount++ , fire at >= ForCount (as
  today).
- On non-breach: breachCount[key]=0. If an alert is active: compute the
  CLEAR condition — if clear_threshold is set, the value must satisfy the
  inverse side against clear_threshold (op > or >= → value <
  clear_threshold; op < or <= → value > clear_threshold; op == → ignore
  clear_threshold, use !breach); if not set, !breach suffices. Only when
  the clear condition holds does goodCount++ (else reset it to 0 — a
  sample in the dead band between clear_threshold and threshold resets
  BOTH counters' progress toward firing and clearing... correction: a
  dead-band sample resets goodCount but must NOT increment breachCount).
  Resolve when goodCount >= clear_count, then reset counters.
- "unhealthy" metric: clear condition = result OK, counted the same way.
- Example from the user this must satisfy: ping latency_ms > 100,
  for_count 10, clear_threshold 70, clear_count 10 → fires after 10
  consecutive >100 samples, resolves after 10 consecutive <70 samples;
  samples between 70 and 100 keep the alert firing and reset the clear
  progress.

### A3. Notifiers (new file internal/server/notify.go)

Replace notifyWebhook with a dispatcher: on fire and on resolve, load the
rule's targets and notify each in a goroutine (10s timeout each, errors
logged, never blocks the stream). Shared payload = current webhook JSON.

- webhook: config {"url"} — same POST as today.
- email: config {"to": ["a@b"], "subject" optional}. net/smtp (stdlib):
  global SMTP settings from NEW server options NETLAMA_SMTP_HOST,
  NETLAMA_SMTP_PORT (default 587), NETLAMA_SMTP_USER, NETLAMA_SMTP_PASS,
  NETLAMA_SMTP_FROM, NETLAMA_SMTP_STARTTLS (default 1). Plain-text body
  from the payload. If SMTP_HOST unset → log a warning once per notify.
  Wire flags+env in cmd/server/main.go per repo convention (README, both
  compose files, ROADMAP/PROGRESS).
- script: config {"path", "args": []}. exec.CommandContext, 30s timeout,
  payload JSON on stdin plus NETLAMA_ALERT_STATE/RULE/AGENT/MESSAGE/VALUE
  env vars; log stderr on failure. SECURITY: creating/updating script
  targets is admin-only (enforced in the API layer, A4).
- snmp: config {"host", "port": 162, "community": "public"}. SNMPv2c trap
  via NEW dependency github.com/gosnmp/gosnmp (the only new module).
  Trap OID under 1.3.6.1.4.1.59777 (netlama), varbinds: rule name, state,
  agent, message, value (OctetString/opaque floats as string). Keep it
  minimal — one SendTrap call.
- dashboard: NOT a target type — alerts are always stored and visible in
  the UI regardless of targets (document this in API.md and the Phase B
  UI copy).

### A4. API (internal/api/alerts.go)

- alert-targets: GET list, POST create, PUT /{id}, DELETE /{id}. Tenant
  scoping identical to alert-rules (admins ?tenantId=, tenant users
  auto). type 'script' → 403 for non-admins on create AND update. Config
  validated per type (url/to/host/path present).
- alert-rules: POST accepts clearThreshold (nullable), clearCount,
  targetIds; add PUT /{id} for editing; GET returns the new fields.
  Validate targetIds exist in the same tenant.
- GET /api/v1/alerts unchanged (already supports querying; verify it
  returns state so "current alerts" = state=firing client-side, and
  document a ?state=firing filter if trivial to add).
- doc/API.md: document alert-targets CRUD, new rule fields, the alerts
  query, and the SMTP env vars.

### A5. Tests & verification

- go test: add internal/server/alerts_test.go unit test for the
  hysteresis state machine (fire after N, dead-band keeps firing +
  resets clear progress, resolve after M below clear threshold) — the
  eval logic must be factored so it's testable without a gRPC stream
  (pure function over counters or a small struct).
- make build, go vet ./..., go test ./....
- e2e: start server (scratch DB), create tenant/site/agent/test + a
  webhook target pointing at a local listener + a rule with
  clearThreshold/clearCount via the API; verify the API responses carry
  the new fields. (No live agent needed — API-level only.)
- Quote in the report: the clear-condition code, the migration snippet,
  one API response with targetIds/clearThreshold, and go.mod diff (only
  gosnmp added).

## Phase B — UI (separate task, after A is merged)

- Sidebar (index.html): new group label "Configuration" ABOVE the
  existing "Manage" group, one entry "Alerts & Alert Rules"
  (data-nav="alertcfg"). New section-alertcfg.
- Move the Rules block (table + dlg-rule) from the Alerts section into
  section-alertcfg; Alerts page keeps only active/recent alerts (pure
  view). Update the rule dialog: clear threshold (optional number),
  clear count, and a target multi-select (checkbox list); rules table
  shows condition summary ("latency_ms > 100 ×10, clear < 70 ×10") and
  target names; Edit via the new PUT.
- New "Alert targets" block in section-alertcfg: table (name, type,
  summary e.g. url/to/host/path) + create/edit dialog whose fields switch
  with the selected type; script type option hidden for non-admins; a
  static first row "Dashboard — built-in, always on" (not editable).
- app.js: alertcfg in sections list + reloadSection; loadAlertCfg()
  fetching rules+targets+tests; keep esc() on all user strings; existing
  dialog/token patterns.
- ROADMAP: check off "Alert-rule configuration UI as its own menu item".
  PROGRESS entry. README: update the alerting paragraph (targets, clear
  hysteresis, SMTP vars — verify what it says today).

## Constraints

- Only new dependency: gosnmp (Phase A). Everything else stdlib.
- Do not break existing alerts/rules API consumers: existing fields keep
  their names; new fields are additive.
- Do not commit.
