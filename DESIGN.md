# DreamHub — Design Foundations

The visual and interaction foundations of the web UI, extracted from the approved dashboard mockup. The frontend implements these as design tokens and components; when this file and the mockup disagree, this file wins.

**Reference mockup**: [`docs/design/mockup-dashboard-v2.html`](docs/design/mockup-dashboard-v2.html) — open it directly in a browser. It's interactive: edit mode, custom tile builder, language switcher (DE/IT/EN), appearance switch, and the preview-and-confirm dialogs. The UI text in the mockup is the German locale; that's product content, not repository prose. (The mockup loads Source Sans 3 from Google Fonts for convenience — the product bundles the font, see `PRINCIPLES.md`.) The mockup covers the dashboard/shell as originally approved; screens built afterward (search palette, login, onboarding, the settings menu, API tokens, MediaHub/GuideHub/SeriesHub/DiscoveryHub's real screens, Diagnostics) exist only as real Vue components in `frontend/` and as the entries added to "Components" below — there's no separate mockup for them, and this file plus the running app are the reference.

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
- **Status line** under the top bar: short items with a colored **dot**, not pills (connected, disk, conflicts); the *effective* display time zone (per-user box/browser/fixed setting, resolved — not just the browser's own) right-aligned in `faint`.
- **Tile**: header (title + optional meta on the right) and body. One visual weight for all tiles; a tile needing attention gets a `warn`-tinted border and title, nothing louder. In edit mode: dashed border, drag to move, ←/→ buttons, width 1–4, hide.
- **Row** (lists inside tiles): three columns — leading element (time, picon or thumbnail), main text (title + secondary line, ellipsized), trailing element (time range, size or a text action). Row separators in `line-soft`; hover in `tile-2`.
- **Actions**: text actions by default (accent color, subtle hover background). **At most one filled primary button per tile**; dialogs have exactly one primary (confirm) and one secondary (cancel), right-aligned.
- **Preview dialog** (every mutating action): eyebrow "Preview · nothing changed yet", title, then a key–value list of what will happen (with tuner/conflict check shown in `ok`/`warn`), or a before/after diff for renames (monospace, old struck through, new highlighted, list of affected files), or option cards (radio) for choices with a recommended option. Footer note on change log / undo.
- **Toast**: bottom-end, short confirmation of what happened ("6 files renamed · undo in the change log").
- **Picons and thumbnails**: real images from `/data/piconpool` and cover art in the product; the mockup uses placeholders (picon: 44×26 label, thumbnail: 36×52).
- **Conflict timeline**: a mini time axis with both timers as bars, the overlap hatched in `warn`, the "now" line in `onair`.
- **Search palette** (built ahead of the mockup, which only shows the top-bar input): `Cmd/Ctrl+K` or `/` opens a centered modal over a dark scrim — search field with a leading icon, up to 8 results below (icon + title + `faint` subtitle for context, e.g. the contributing hub's name), a keyboard hint footer (`↑↓` navigate, `↵` open, `Esc` close). Same tile/dialog surface (`tile`, `line` border, `lg` radius) as everything else, not a separate visual language.
- **Notifications panel**: the bell's dropdown uses the same list pattern as a tile's `Row` — a small `faint` section label ("Notifications", then "Recent activity" beneath it when there's anything to show), each item with a leading status dot (`warn`/`faint`, matching the sidebar's) and `13px` text. No pills, no unread badges beyond the single dot on the bell icon itself.
- **Login screen** (only ever shown instead of the whole shell, never alongside it): a single centered card, same `tile`/`line`/`lg` surface as a dialog — app name, one line of context, username, password, one primary action. No decoration beyond that; it's the one screen with nothing else competing for attention.
- **Settings field**: label column (fixed width, `muted`, `14px`) beside the control column — a native-styled input/select for text/int/enum, a custom pill-track toggle (`accent` fill when on) for bool, secret/password fields placeholder as "New key"/"New password" and clear themselves on focus so typing never edits a mask. A `12.5px` hint line below where useful (timezone format, allowed-hosts syntax), a `warn`-colored error line when a save is rejected, and a text-action reset link when the value isn't the default. One Save/Cancel pair per section (DESIGN's "at most one primary action" — here, per section, not per tile), not one for the whole page.
- **Onboarding wizard**: a thin multi-segment progress bar (one `accent`/`line` segment per step) above a single centered column, capped at the same content width a dialog would use. Each step is a title, one line of `muted` subtitle, its own content, then Back/Next (and Skip, where a step is genuinely optional) — the same button hierarchy as everywhere else, just without a surrounding tile border.
- **Hub real screens** (MediaHub, GuideHub, SeriesHub, DiscoveryHub — built after the mockup, no separate design file): each opens with the hub's status banner when not fully available (same pattern as the placeholder it replaces), then real content below — banner and content are independent, not either/or, since "degraded" (an optional connector missing) still means the hub mostly works. Recordings use the same three-column `Row` as everywhere else; a non-recording file found alongside real recordings is dimmed (`opacity-60`) and labeled rather than hidden.
- **Timeline grid** (GuideHub): a proportional schedule, not a list — each channel a row, each event an absolutely-positioned block sized by percentage of the visible time window, an hour ruler above, a single `warn`-colored "now" line down through every row. Bouquet and time window (◀ / Now / ▶) are both switchable; the reference device's disk-space-tile bar chart is the only other place a proportional bar already existed, this generalizes the same idea to time.
- **Embedded external UI** (SeriesHub's SerienRecorder panel): a plain tile with a one-line explanation, "Show here" (an inline iframe, lazy-mounted only once toggled on) and "Open in new tab" side by side — DreamHub doesn't restyle or frame the embedded app itself, it's clearly a different, real, external tool inside DreamHub's own chrome.
- **Recommendation card** (DiscoveryHub): the same `Row`-derived shape as a timer, plus the model's one-line reason in `text`, plus a text-action "Create timer" that expands in place into DreamHub's first real preview dialog content (warnings list, then Confirm/Cancel) — no modal, since the card itself already has the room.
- **Inline preview** (`PlanPreview.vue`): the preview dialog's content without the modal, expanded in place under the row or card it belongs to — eyebrow "Preview · nothing changed yet" in `faint`, a one-sentence summary of the change, warnings in `warn`, then Cancel (text) and one primary button, right-aligned. A destructive confirm (delete) uses `onair` instead of `accent`. The primary gets focus when it opens.
- **Timer list** (TimerHub): upcoming timers under plain day headings ("Today", "Tomorrow", then weekday + date in the display zone), finished ones behind a text action. Each timer is a Row: time range in monospace (tinted `warn` when it likely conflicts), name + channel · source · "likely tuner conflict", a state dot + word (`onair` while recording), and a text action that appears on hover (always visible below 900 px). Below 640 px the time moves onto its own line and the state shows as the dot only, so the name keeps the width.
- **Folder browser** (MediaHub): text tabs ("Recordings" / "Trash", the active one underlined in `accent`), then breadcrumbs (the root named "Recordings", earlier segments as text actions), then subfolders as Rows with a folder icon, then recordings. Recordings have a checkbox when file actions are possible, and a selection bar ("n selected · Move to trash") appears above the list. Where file actions aren't possible, one `faint` line says why. The trash tab lists title, original folder and size, with Restore (accent) and Delete for good (`onair`) per row plus "Empty trash".
- **Name suggestions** (MediaHub rename form): a `faint` "Suggest:" label followed by one text action per source ("From the recording", "TMDB", "AI"). A source without its key is disabled, with a tooltip saying where to set it. Results appear as rows (the name in monospace, the source and film/series in `faint` at the end); clicking one fills the name field. The rename still goes through its preview.
- **Enrichment** (posters and ratings — a recording's own cover art on the box first, TMDB otherwise): in Rows, a 36×52 thumbnail as the leading element (a `field`-colored box while loading or when there's no picture, so rows don't jump; only shown at all when enrichment is on), genres at the end of the secondary line, and the rating as "★ 6.9 (310)" before the size — the star in `warn`, the number in `text`, the vote count in `faint`; hidden below five votes. In detail areas (series page, selected programme, recommendation card): poster on the start side, then year · genres · rating in one `muted` line, the overview, and a `faint` "Details and images: TMDB" credit. No big poster shelves (CONCEPT.md) — posters stay small and inside the content they belong to.
- **Custom tile** (dashboard): the built-in tile frame with the user's own title and a `faint` "Custom view" note at the header's end. Body: one `muted` line summarizing the criteria ("Recordings · older than 90 days · largest first → **12 matches, 48 GB**", the count in `text` weight), then Rows (recordings: date, title + channel, size; everything upcoming: weekday + time, title + channel, "–end"; a likely conflict tints the leading time `warn`), or one big number for display "count". A text action "Edit filters" at the bottom. Edit mode adds a red "Remove" beside "Hide" (custom tiles only) and an "Add tile" block with a dashed border at the end of the grid.
- **Custom tile builder**: a centered dialog (tile surface, `lg` radius, 520 px) — eyebrow "Custom tile", title "View with criteria", then Name, Source + Display side by side, a bordered "Conditions" group showing only the conditions the chosen source supports (label column + control), Sort + Show side by side, a live `aria-live` match count, the note that a tile never acts on its own, then Cancel and one primary. Name gets focus on open; Escape or the backdrop cancel.
- **Series detail** (SeriesHub, `/series/{id}`): a text-action back link, the series name as page title with its info line beside it, then a row of four small fact tiles (label in `faint`, value below; the folder in monospace). Below, two columns (one below 1180px): "Planned episodes" (what SerienRecorder has timered) and "In the EPG" (further broadcasts, each with a "Record" text action that expands into the inline preview, or an `ok` dot + "Timer set"). Recordings on disk follow full-width, grouped under plain season headings when the recordings carry `S01E02` codes. A part that couldn't be loaded says so in one `muted` line rather than showing an empty list.
- **Remote** (ControlHub, and the side panel any page opens from the top bar's remote icon): shaped like a physical remote rather than a button grid — round `line`-bordered keys with icons for power (`onair`), mute and playback, bordered rectangular keys for digits and Menu/EPG/Info, pill-shaped volume/channel rockers either side of a round `field`-filled direction pad with a filled `accent` OK in its center, a full-width Back key, and the four color keys as solid `onair`/`ok`/`warn`/`accent` bars (named on hover and for screen readers, never color alone). Keys scale down slightly on press (off under reduced motion). The picture sits beside it (stacked and pinned on top below 900px) and refreshes itself after each key. The side panel is 320 px at the inline end, pushing the page rather than covering it; below 900px it's an overlay drawer like the sidebar's.
- **Batch rename** (MediaHub selection bar → "Rename by scheme"): the inline preview's surface under the selection bar — eyebrow, a small segmented switch for the source ("Recording" / "+ TMDB") at the end, a one-line tally ("5 will be renamed · 1 already named so · 1 skipped"), then one monospace line per recording (old struck through → new in `accent`; left-alone ones `faint` with their reason in sans), a `faint` pointer to the scheme's settings, Cancel and one primary "Rename n". A checkbox "Sort into folders" before the source switch reloads the preview: moving recordings show their target folder before the new name (`FBI - Most Wanted/Staffel 05/…`), and one `muted` line lists the folders that will be created.
- **Episode guide** (series page): one collapsible block per season (newest open), heading with a `muted` tally, rows of code (monospace) · title · first air date · a status dot + word (recorded `ok`, planned `accent`, missing `warn`, still to come/aired `faint`). A `faint` credit names the source.
- **Diagnostics**: hubs grouped under plain section headers (Unavailable, Degraded, Disabled, Available — most urgent first), each hub one `Row`-derived line with its icon, name, and the same translated problem/fix text used in the sidebar and notifications bell. No new visual language, just the sidebar's own status vocabulary laid out flat.

## Interaction

- Keyboard: `/` or `Cmd/Ctrl+K` search, `g` + letter jumps to a hub, `Esc` closes dialogs, menus and the drawer.
- Focus is always visible (2 px `accent` outline).
- Dialogs focus their primary action on open and return focus on close.
- Motion is minimal (toast fade, drawer slide) and disabled under `prefers-reduced-motion`.

## Avoid

The generated/templated look, specifically: uppercase tracked micro-labels, outlined buttons everywhere, pill chips for status, every block in a heavy rounded box, trendy condensed display fonts, purple gradients, emoji as icons, big Plex-style poster shelves in the default layout (a cover row is an optional tile type).
