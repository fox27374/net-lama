# Plan: UI design-system pass (tokens, spacing rhythm, color coherence)

Goal: the UI currently looks "added together" — 70 raw hex values in
style.css (most outside :root), and gaps/padding using ad-hoc values
(0.3/0.35/0.4/0.45/0.5/0.8/0.9/1.1rem). This pass introduces a strict
token system and sweeps every rule onto it. Pure refactor of
internal/web/static/ (style.css, small class touch-ups in index.html /
app.js) — NO functional/JS-logic changes, NO Go changes.

Derived with the ui-ux-pro-max design engine (ops-dashboard pattern,
dense scale, dark+status-color strategy), adapted: keep BOTH themes
(light default + dark), keep indigo accent (green stays reserved for
"healthy" status in a monitoring tool), keep system fonts (UI must work
offline — no webfont imports).

## 1. Token system (replace/extend the existing `:root` blocks)

Spacing scale (the ONLY spacing values allowed; 4px rhythm):

```css
--space-1: 4px;  --space-2: 8px;  --space-3: 12px;
--space-4: 16px; --space-5: 24px; --space-6: 32px;
```

Radius scale: `--radius-sm: 6px; --radius-md: 10px; --radius-lg: 14px;`
pills keep `999px`. One elevation pair only: keep the existing
`--shadow` for cards, add `--shadow-pop` (menus/dialogs); nothing else
may define its own box-shadow.

Type scale (rem): `--text-xs: .72rem; --text-sm: .8rem;
--text-md: .9rem; --text-lg: 1.05rem; --text-xl: 1.35rem;` — sweep all
font-size values onto the nearest step. Add `font-variant-numeric:
tabular-nums` to all table cells and stat/metric values.

Status colors (semantic, both themes): keep `--ok`/`--ok-bg`,
`--bad`/`--bad-bg`, ADD `--warn: #d98a00; --warn-bg: #d98a0015;`
(dark theme variants: slightly lighter/desaturated). The existing
`.cap-warning` and any amber/degraded styling must use them.

Categorical ramp for type/series chips — 8 hues as tokens, defined
once, with the alpha pattern the chips already use (solid color,
border 55, bg 12):

```css
--cat-1: #4f7cf7; /* ping        (blue)   */
--cat-2: #a06ee0; /* dns         (purple) */
--cat-3: #2fa670; /* http        (green-teal) */
--cat-4: #4fb8c9; /* tcp         (cyan)   */
--cat-5: #e0a03a; /* speedtest   (amber)  */
--cat-6: #c96bc9; /* traceroute  (magenta)*/
--cat-7: #35a08e; /* wlan_scan   (teal)   */
--cat-8: #7f8ea3; /* fallback / unknown   */
```

Dark theme: raise lightness of each by ~8-12% so contrast vs dark
surfaces stays ≥ 3:1 (chips are large text/graphics). Every
`.chip.type-*` rule becomes `color: var(--cat-N); border-color:
color-mix(in srgb, var(--cat-N) 35%, transparent); background:
color-mix(in srgb, var(--cat-N) 8%, transparent);` — delete the
hardcoded per-type hex rules, including the `--series-*` block if it
can reuse the same ramp (charts/series may map --series-N to --cat-N).

## 2. Sweep rules

- After the pass, `grep -cE '#[0-9a-fA-F]{3,8}' style.css` counts hex
  ONLY inside the two `:root` theme blocks. No raw hex anywhere else —
  use tokens or `color-mix()` on tokens.
- All `gap`/`padding`/`margin` use `--space-*` (or `0`). Map each
  existing value to the nearest step (0.3-0.45rem → --space-2 or
  --space-1 where genuinely tight; 0.8-1.1rem → --space-4;
  2rem+ page padding → --space-6). Table cell padding: --space-2
  vertical / --space-3 horizontal (dense dashboard density).
- All border-radius from the radius scale (pills exempt).
- Interactive elements: consistent hover (background shift via
  color-mix, 150ms ease-out transition) and a visible `:focus-visible`
  ring (`outline: 2px solid var(--accent); outline-offset: 2px`) on
  buttons, nav items, inputs, row actions. One shared transition token:
  `--transition: 150ms ease-out;`.
- Nav: the active page indicator must be unmistakable (accent text +
  left border or bg tint — pick one treatment, apply to all nav items).
- `@media (prefers-reduced-motion: reduce)`: disable transitions.
- Both themes must be updated together — every new token gets a value
  in the light AND dark `:root` blocks; verify chips/status/warn are
  legible in both.

## 3. Explicitly out of scope

No layout restructuring, no new pages/sections, no JS behavior changes
(class renames in template strings are fine), no font imports, no
removal of either theme, no Go/proto/API changes.

## 4. Verification (required)

1. `make build` (embeds the static files), `go vet ./...`,
   `go test ./...`.
2. Mechanical checks on internal/web/static/style.css:
   - hex colors appear only within the `:root` blocks (show the grep
     proving it);
   - no `gap:`/`padding:`/`margin:` value outside the --space scale
     except `0`/`auto` (show a grep of remaining literal values);
   - `.chip.type-*` rules contain no hex.
3. Serve check: start the built server (self-signed TLS, scratch port),
   curl `/style.css` and confirm the token block and one swept rule are
   served; curl `/` and `/app.js` unchanged-behavior sanity (page still
   references the same section ids).
4. Contrast spot-check (arithmetic, not eyeball): compute WCAG contrast
   of --fg on --surface and each --cat-N on --surface for BOTH themes
   (a tiny python snippet is fine); every --cat-N ≥ 3:1, body text
   ≥ 4.5:1. Adjust lightness until they pass and note final values.

## Constraints

- Do not commit; leave changes in the working tree for review.
- If color-mix() feels risky for older browsers, static fallback values
  computed at the same ratios are acceptable — but pick ONE approach
  consistently.
