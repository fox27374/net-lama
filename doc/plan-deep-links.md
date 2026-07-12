# Plan: dashboard deep-links

Every dashboard block links to its page. UI-only (internal/web/static/),
no Go/API changes. Use existing tokens/styles; keep showSection() as the
navigation mechanism.

## Links

1. **Stat tiles**: whole tile clickable — Sites→sites, Agents→agents,
   Tests→tests, Active alerts→alerts. Hover: subtle surface shift
   (existing --transition); cursor pointer; tiles become buttons or get
   role="link" + tabindex + Enter handling; :focus-visible ring (token
   treatment already exists).
2. **Block headers**: each dashboard card header (Sites, Alerts, Tests,
   Wireless) gets a small "View all →" link on the right (muted, accent
   on hover) navigating to the corresponding page.
3. **Sites rows** → sites page. **Alerts rows** → alerts page.
4. **Tests rows** → Results page with the test preselected: navigate to
   results, set the Results page's test filter to the clicked testId
   (and site filter to the dashboard's site filter if set), then trigger
   its reload. Inspect how the Results filters are wired and reuse that
   path — do not duplicate fetch logic.
5. **Wireless rows** → wireless page.

## Implementation notes

- One helper, e.g. `navTo(section, presets?)`: applies optional filter
  presets after showSection, keeping logic in one place.
- Rows: cursor pointer + hover background (there may already be a hover
  style — reuse); don't break existing in-row interactive elements
  (none expected on dashboard rows today).
- Accessibility: rows get tabindex="0" + Enter key triggers the same
  handler; tiles/links are real <button>/<a>-like elements with labels.

## Docs

ROADMAP: check off the dashboard deep-links item (match style).
PROGRESS.md: one line under 2026-07-12. README: only if it mentions
dashboard behavior in a way that becomes wrong (check; likely no change).

## Verification

1. make build, go vet ./..., go test ./... (should be unaffected).
2. Serve check (self-signed TLS, scratch port): curl / and app.js —
   confirm the navTo helper, tile click wiring, and "View all" markup
   are served.
3. Static sanity: every navTo target is in the sections array; the
   Results filter preset uses the actual element IDs from the Results
   page (quote them in the report).

## Constraints

No new dependencies. No layout redesign — only adding affordances.
Do not commit.
