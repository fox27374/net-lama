# Plan: Dashboard + sidebar navigation restructure

UI restructure. Colors/design tokens are final — do NOT change the token
system, only build with it. Touches internal/web/static/* plus one Go
change (overview series). Keep all existing pages working.

## 1. Navigation: left sidebar instead of top tabs

- Fixed left sidebar (~220px; on <900px viewports collapse to the current
  top-bar behavior or a simple burger — keep it basic).
- Order: Dashboard, Results, Path, Wireless, Logs, Alerts, Tests, Sites,
  Agents; then a "Manage" group label with: Tenants, Users, API Keys.
- Brand at top of sidebar, theme toggle + logout at the bottom.
- Active item: existing accent treatment. Main content area gets full
  remaining width; cards/blocks stack vertically full-width (remove the
  side-by-side .columns usage on all pages; .card becomes full-width).

## 2. Dashboard (rename Overview)

Rename nav id/label Overview→Dashboard everywhere (section id may stay
internally if renaming is risky; label must say "Dashboard"). It stays the
post-login landing view. At the top: a site filter dropdown ("All sites" +
each site) that re-renders every block below.

Block order, each a full-width card:
1. **Stat tiles** (existing counts: sites, agents connected/total, tests,
   active alerts) — unchanged data, restyled as a tile row.
2. **Sites**: one row per site — name, agent count, health rollup computed
   from that site's tests health (healthy / degraded / failing counts as
   ok/warn/bad chips). Site filter narrows to that site.
3. **Alerts**: the active+recent alerts list (reuse Alerts page rendering),
   filtered by site when the filter is set.
4. **Tests**: existing per-test health list BUT replace the "last result"
   column with an inline **sparkline** (see §3) + current value text.
5. **Wireless**: reuse the Wireless page's latest-scan table per agent,
   filtered by site.

Existing separate pages (Alerts, Wireless, ...) remain unchanged.

## 3. Sparklines (Tests block)

Server: extend the overview response's testHealth entries with:
`"series": [..up to 30 numbers, oldest first..], "unit": "ms|Mbps|hops|APs",
"current": <last value>`. In internal/store/overview.go pull the last 30
results per test (site-filtered when siteId given) and extract one primary
metric per type: ping→avg latency ms; dns/http/tcp→duration ms;
speedtest→download Mbps; traceroute→total RTT ms of final hop (or hop
count if RTT absent); wlan_scan→AP count. Failed results → null gap.
Add optional `siteId` query param to GET /api/v1/overview (validated to
belong to the tenant); doc/API.md updated.

Client: render inline SVG polyline (no library), ~160x36px, stroke
var(--cat-1), null gaps break the line, current value + unit right-aligned
(tabular numerals). No axes/grid; a muted dot on the last point.

## 4. ROADMAP.md additions (unchecked, matching style)

Under "Server & UI":
- [ ] Configurable dashboard with widgets (add/remove/reorder blocks)
- [ ] Separate configure vs. view menus for sites, agents, tests, wireless
      and logs
- [ ] Rework the Path view visual design
- [ ] Alert-rule configuration UI as its own menu item
- [ ] Logo for the web UI
- [ ] Version tag reported by server and agent, shown in UI/API

## 5. Docs

README: update the UI pages list (Dashboard, sidebar, Manage group).
PROGRESS.md dated entry (2026-07-12). doc/API.md: overview siteId param +
series fields.

## Verification

1. make build, go vet ./..., go test ./... (add a store test for the
   series extraction happy path + empty).
2. E2E (self-signed TLS, scratch ports): seed tenant/site/agent + a ping
   test via API, run agent ~90s, then GET /api/v1/overview → testHealth
   contains series with ≥2 numeric values and correct unit; with
   &siteId=<other> → empty/filtered. Curl / and confirm sidebar markup,
   Dashboard label, sparkline <svg> rendering code in app.js served.
3. No JS errors: grep app.js changes for references to removed elements.

## Constraints

- No new dependencies. No token/color changes. Do not commit.
- Keep diffs minimal outside static/ + overview code path.
