# DreamHub — Design Foundations

The visual and interaction foundations of the web UI, extracted from the approved dashboard mockup. The frontend implements these as design tokens and components; when this file and the mockup disagree, this file wins.

**Reference mockup**: [`docs/design/mockup-dashboard-v2.html`](docs/design/mockup-dashboard-v2.html) — open it directly in a browser. It's interactive: edit mode, custom tile builder, language switcher (DE/IT/EN), appearance switch, and the preview-and-confirm dialogs. The UI text in the mockup is the German locale; that's product content, not repository prose. (The mockup loads Source Sans 3 from Google Fonts for convenience — the product bundles the font, see `PRINCIPLES.md`.)

Direction in one sentence: **a good-looking work tool** — information first, calm, desktop-first but fully usable on a phone. See `CONCEPT.md`, "UX direction", for the reasoning.

## Design tokens

All colors are tokens. Dark is the default appearance; light and "follow system" are selectable in the user menu. Status colors are semantic and never used decoratively.

| Token | Dark | Light | Use |
|---|---|---|---|
| `bg` | `#0F1215` | `#F2F3F5` | page ground |
| `side` | `#0B0D10` | `#FAFBFC` | sidebar |
| `tile` | `#161A1F` | `#FFFFFF` | tiles, dialogs, menus |
| `tile-2` | `#1C2127` | `#F4F6F8` | hover, active nav item |
| `field` | `#1A1F25` | `#ECEFF2` | inputs, search |
| `line` | `#252B32` | `#DCE0E5` | borders |
| `line-soft` | `#1E2329` | `#EAEDF0` | row separators |
| `text` | `#E6E9EC` | `#15191D` | primary text |
| `muted` | `#9BA4AE` | `#505A64` | secondary text |
| `faint` | `#6C7580` | `#7B858F` | meta, captions |
| `accent` | `#6AA6E8` | `#1F64C8` | links, actions, focus, primary buttons |
| `onair` | `#F2564A` | `#D23A2E` | live / now marker, progress |
| `warn` | `#E0A23B` | `#B7730F` | conflicts, disk nearly full |
| `ok` | `#4FBF7F` | `#2C8150` | resolved, in sync |

## Typography

- **Source Sans 3** (bundled) for everything; **Source Code Pro** for file names and technical values.
- Scale: page title 26 / 700 · dialog title 21 / 700 · tile title 15.5 / 600 · body 15 / 400 · secondary 13–13.5 · meta 12.5–13 (`faint`) · big numbers 34 / 700.
- Tabular figures (`font-variant-numeric: tabular-nums`) for all times, sizes and counts.
- Sentence case everywhere. **No uppercase letter-spaced micro-labels.**

## Layout

- Sidebar 232 px, collapsible to 60 px (icons only, labels as tooltips); state remembered.
- Content padding 24 px (16 px below 900 px); max content width ~1480 px.
- Dashboard grid: 4 columns, gap 14 px; tiles span 1–4 columns.
- Breakpoints: ≤ 1180 px → 2 columns (spans 3–4 become 2) · ≤ 900 px → sidebar becomes a drawer, no collapsed state · ≤ 640 px → 1 column.
- Radius: 6 px tiles and buttons, 4 px small elements (picons, inputs), 10 px dialogs. Minimal rounding overall.
- CSS uses **logical properties only** (`padding-inline`, `inset-inline-start`, `border-inline-end`, `text-align: start`) — RTL-ready.

## Components

- **Sidebar**: brand + collapse button on top; groups as plain muted text (not uppercase); items with icon + label + optional count (count in `warn` when something needs attention); AdminHub and Settings pinned to the bottom. Generated from the hub registry (group + order). Hub labels are either descriptive ("Recordings") or hub names ("MediaHub"), per user setting — the other name is shown as a tooltip.
- **Top bar**: global search centered (placeholder names what can be searched, `/` hint), then notifications (count badge), language switcher (globe + code, menu with language *names*), user menu (who, role, appearance, settings).
- **Status line** under the top bar: short items with a colored **dot**, not pills (connected, disk, conflicts); the display time zone right-aligned in `faint`.
- **Tile**: header (title + optional meta on the right) and body. One visual weight for all tiles; a tile needing attention gets a `warn`-tinted border and title, nothing louder. In edit mode: dashed border, drag to move, ←/→ buttons, width 1–4, hide.
- **Row** (lists inside tiles): three columns — leading element (time, picon or thumbnail), main text (title + secondary line, ellipsized), trailing element (time range, size or a text action). Row separators in `line-soft`; hover in `tile-2`.
- **Actions**: text actions by default (accent color, subtle hover background). **At most one filled primary button per tile**; dialogs have exactly one primary (confirm) and one secondary (cancel), right-aligned.
- **Preview dialog** (every mutating action): eyebrow "Preview · nothing changed yet", title, then a key–value list of what will happen (with tuner/conflict check shown in `ok`/`warn`), or a before/after diff for renames (monospace, old struck through, new highlighted, list of affected files), or option cards (radio) for choices with a recommended option. Footer note on change log / undo.
- **Toast**: bottom-end, short confirmation of what happened ("6 files renamed · undo in the change log").
- **Picons and thumbnails**: real images from `/data/piconpool` and cover art in the product; the mockup uses placeholders (picon: 44×26 label, thumbnail: 36×52).
- **Conflict timeline**: a mini time axis with both timers as bars, the overlap hatched in `warn`, the "now" line in `onair`.

## Interaction

- Keyboard: `/` or `Cmd/Ctrl+K` search, `g` + letter jumps to a hub, `Esc` closes dialogs, menus and the drawer.
- Focus is always visible (2 px `accent` outline).
- Dialogs focus their primary action on open and return focus on close.
- Motion is minimal (toast fade, drawer slide) and disabled under `prefers-reduced-motion`.

## Avoid

The generated/templated look, specifically: uppercase tracked micro-labels, outlined buttons everywhere, pill chips for status, every block in a heavy rounded box, trendy condensed display fonts, purple gradients, emoji as icons, big Plex-style poster shelves in the default layout (a cover row is an optional tile type).
