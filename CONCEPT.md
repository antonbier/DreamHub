# DreamHub — Product Concept

What DreamHub *is* as a product: its structure, its hubs, and how they fit together. `PROJECT_PLAN.md` covers why the project exists and what's out of scope; `PRINCIPLES.md` covers how everything is built; this document covers what gets built.

## What makes DreamHub distinct

Almost every individual capability DreamHub offers already exists somewhere on this platform: DreamEPG Premium does EPG and timers on a phone, AdvancedMovieSelection renames recordings on the TV, GraphMultiEPG shows a grid EPG on the TV (see `ARCHITECTURE.md`). What doesn't exist anywhere is **one place where all of it connects**. The value is in the flows *across* hubs, not in any single hub:

> The AI recommends *Die Kanzlei, S01/E07* on ONE HD at 21:45 → one click creates the timer (after confirmation) → the recording lands in the MediaHub → DreamHub suggests a clean file name → the user confirms.

> A search for "Tatort" returns, in one result: when it's on (GuideHub), whether a timer is set (TimerHub), which episodes are already on disk (MediaHub), and whether SerienRecorder tracks it (SeriesHub).

When deciding whether a feature belongs in DreamHub, the question is: does it get better by being connected to the rest?

## Three building blocks

| Block | What it is | Examples |
|---|---|---|
| **Core (shell)** | Everything shared across hubs | dashboard, navigation, global search, notifications, settings, onboarding, diagnostics, auth, profiles & roles, i18n, time zones |
| **Hubs** | User-facing feature areas | MediaHub, TimerHub, GuideHub, … (see below) |
| **Connectors** | Data sources and external services | WebControl `/web/*` API, SerienRecorder web API, TMDB, OpenAI, Plex, later Jellyfin/Emby |

Rules between them:

- **Hubs only reach data through connectors, via the core.** A hub never talks HTTP to WebControl itself.
- **Hubs only talk to other hubs through the core API or core events** — never by importing each other's code. When DiscoveryHub creates a timer, it calls TimerHub's API.
- **A new connector extends every hub that can use it.** Adding a Jellyfin connector shouldn't require rewriting the MediaHub.
- **The AI provider is a connector like any other** — it lives in the core, not inside DiscoveryHub, so other hubs can use it later.

## The hub contract

Every hub registers itself with the core by declaring:

- an ID, an icon, a **navigation group and order**, and two names: a **descriptive, localized label** ("Recordings" / "Aufnahmen" / "Registrazioni") and a fixed **hub name** ("MediaHub")
- its API routes (under `/api/v1/<hub>/...`)
- a **settings schema** (fields, types, validation, which fields are secrets) — the core renders the settings UI from this
- optionally a **search provider** for global search (also the data source for custom dashboard tiles)
- optionally **tile types** for the dashboard
- the **events** it emits and listens to (e.g. `recording.finished`, `timer.conflict`)
- its **requirements** (connectors, capabilities like ffmpeg, installed plugins)
- the **role** needed to see it (most hubs: any user; AdminHub: admin only)

From the requirements the core derives each hub's **status**: *available*, *degraded*, or *unavailable* — always with a reason and a fix (see "Diagnostics").

## Hubs

| Hub | Scope | Release |
|---|---|---|
| **MediaHub** | Recordings list, disk space, rename with preview, cover art, trash | v1 (trimming in v1.1) |
| **TimerHub** | Timers, AutoTimer, conflict detection | v1 |
| **GuideHub** | EPG as a TV-guide grid ("TV-Zeitschrift") | v1 (basic) |
| **DiscoveryHub** | AI recommendations, one click → TimerHub (with confirmation); optionally shaped by the user's taste (recordings, followed series, timers, AutoTimers — opt-in, since it sends viewing data to the AI provider) | v1 |
| **SeriesHub** | SerienRecorder: episodes on disk, missing episodes, upcoming | v1 (basic) |
| **SystemHub** | Device info, diagnostics, change log — for every user (backup/restore moved to Settings → System, 2026-10-05) | v1 (basic) |
| **AdminHub** | Logs (v1), service status, terminal (later) — admin role only | v1 (logs) |
| **ControlHub** | Web remote, screenshot, power | v1 — pulled forward from v1.1: verifying the Enigma2 plugin (WP8) needed a real screenshot/remote-control loop anyway, and GuideHub's "switch to this channel now" needs it too (`docs/V1_PLAN.md`) |
| **RadioHub** | Radio services, now playing, radio timers | v1.1 (built early, 2026-10-04) |
| **PlexHub** ("Plex Mediathek") | A Plex server's films and series (tiles or list), watched state in Plex, play on the box — optional, cached while the server is off | built early, 2026-10-05 |
| **ChannelHub** | Bouquets (link/embed the existing BouquetEditor plugin), picons | v1.1 |
| **StreamHub** ("Live-TV") | Live TV and recordings in the browser: the box streams unchanged, the browser decodes (FFmpeg in WebAssembly, WebCodecs) | built early, 2026-10-06 — radio still to come |
| **AutomationHub** | Scripts, webhooks, automations — the only place anything happens automatically | later |

Notes that affect several hubs:

- **Single tuner.** The reference device has exactly one tuner (`Si2169D`, "Tuner A" in `/web/deviceinfo`). Overlapping timers are a real, common conflict — TimerHub's conflict handling and DiscoveryHub's "create timer" flow both have to take this seriously.
- **SerienRecorder has its own web interface** (`config.plugins.serienRec.enableWebinterface=true`). SeriesHub should use its API rather than reading `SerienRecorder.db` directly. Needs verification on the device.
- **Radio in the browser** isn't automatically easier than TV: radio services often use MP2 audio, which browsers don't play natively. StreamHub's player decodes MP2 itself (WebAssembly), so radio can follow.

## AdminHub

A separate hub, visible only to the *admin* role, placed at the bottom of the navigation next to Settings — not inside the hub groups.

- **Logs (v1)** — read-only, so no preview needed:
  - From journald (the box runs systemd): **DreamHub's own logs** (for debugging DreamHub itself), enigma2, and other services as needed.
  - Live tail (Server-Sent Events); filter by service, level, time range, hub/connector (DreamHub logs are structured), free text.
  - **DreamHub's log level switchable at runtime** (info/debug) without a restart; debug automatically reverts after e.g. one hour so logs don't fill up.
  - **Request IDs:** every API request gets an ID; when the UI shows an error it shows the ID, so the matching log entry is found immediately.
  - List of **enigma2 crash logs** (`/media/hdd/enigma2_crash_*.log`) with date.
  - Download for support, with secrets automatically removed.
- **Service status (later in v1.x)** — enigma2, dreamhub, Samba, etc. Restarting a service is a change and goes through preview-and-confirm.
- **Terminal (later)** — not a DreamHub-built terminal: DreamHub proxies the existing `shellinaboxd` (`127.0.0.1:4200`, already used by WebAdmin at `/webadmin/terminal/`) behind its own auth and CSRF protection. Off by default, can only be enabled while auth is on, requires re-entering the password, shows a clear warning, and session start/end is recorded in the change log. This is the one explicit exception to preview-and-confirm (see `PRINCIPLES.md`).
- **Later**: an API overview (OpenAPI) for developers of the mobile apps.

## Core features

### Dashboard (decided: tiles)

The home page is a **personal dashboard made of tiles** (decided from the mockup, issue #16).

- **Customizable in v1**: show/hide tiles, reorder by drag and drop, choose width (1–4 columns). Stored per user, included in backup/restore.
- **Tile types** are contributed by hubs through the hub contract (storage, conflicts, now on air, next timers, recommendations, new recordings, series, box status, …).
- **Custom tiles = saved views with criteria**, e.g. "recordings older than 90 days and larger than 3 GB", "tonight from 20:15 on my favorite channels, movies only", "new episodes of my series this week". A custom tile is *source + filters + sort + limit + display* (list, number, cover row) and reuses the search providers every hub already supplies for global search.
- **Tiles only show — they never act on their own.** Actions inside a tile go through preview-and-confirm like everywhere else. Anything automatic belongs in AutomationHub.
- A Plex-style **cover row** exists as an optional tile type, not in the default layout.
- **Data model: multiple dashboards per user from day one** (name + layout each). v1 shows exactly one and has no switcher — prepared, not built.

### Navigation

- **Desktop**: a persistent left sidebar, collapsible to icons via a button at the top left; the state is remembered.
- **Mobile**: the same sidebar as a drawer opened from the top-left button. **No bottom tab bar** — one navigation concept everywhere.
- **Grouped**, generated from each hub's declared group and order (so regrouping is configuration, not code; user-customizable later):
  - *Dashboard*
  - *Media*: MediaHub, SeriesHub, PlexHub ("Plex Mediathek")
  - *Program*: GuideHub, RadioHub, TimerHub, DiscoveryHub
  - *Device*: ChannelHub, ControlHub, SystemHub
  - bottom: AdminHub (admins only), Settings
- **Labels selectable per user**: descriptive names (default, translated) or hub names (MediaHub, TimerHub, …) — similar to how nzb360 names the services it manages. The other name appears as a tooltip.
- Hubs that are *unavailable* appear greyed out with the reason; hubs that are disabled don't appear.
- **Top bar**: global search in the center; on the right notifications, the **language switcher**, and the user menu.

### Global search

Every hub can contribute a search provider; results are grouped by hub. Keyboard shortcut `/` (also `Cmd/Ctrl+K`).

### Notifications

- Hubs emit events; the core turns relevant ones into notifications: recording finished, timer conflict, disk above 90 %, hub became unavailable, box clock out of sync.
- Shown in an **in-app inbox** and the status strip. No external push in v1.
- Stored as a **message key plus parameters**, not finished text, so each user sees them in their own language — even after switching languages.
- Later, via AutomationHub/webhooks: push, Telegram, ntfy — or a message **on the TV itself** via WebControl's `/web/message`.

### Settings

One central settings menu in tabs — General, Recordings, Services (every connector with its state), Notifications, Hubs, Access, System (backup/restore, diagnostics, setup wizard) — generated from the settings schema, where each section names its tab. Every setting is findable from the global search; "test connection" buttons where a service has one (push). Everything in the UI is also available through the API.

### Onboarding wizard

On first start, re-runnable from settings:

1. Language
2. Auth on/off, password
3. Device detection (`/web/deviceinfo`), capability check, time zone and clock check
4. Choose which hubs to enable
5. AI provider and API key (skippable)
6. Channels to use for recommendations
7. Done — show the QR code / bookmark hint

### Diagnostics / hub status

Every hub that isn't fully *available* shows why and how to fix it — e.g. "SerienRecorder is not installed → install it from the Addon Manager under …". Visible on the hub's own placeholder, a diagnostics page in SystemHub, and the onboarding wizard. Also checks the **box's clock** against the browser (see "Time zones").

### Profiles and roles

Prepared, not built. The data model attaches preferences, language, time zone setting and dashboards to a user from day one, and every user has a **role** (*admin* or *user*). The API carries a user context. v1 has a single user, who is an admin, and no profile switching in the UI.

## Internationalization

- **v1 languages**: German, Italian, English.
- **Web UI**: one JSON file per locale (vue-i18n standard: `de.json`, `it.json`, `en.json`). **TV plugin**: gettext `.po`/`.mo`, as usual on Enigma2.
- **Adding a language without a rebuild**: bundled locales live in the binary; additional locale files dropped into `/data/dreamhub/locales/` are loaded at runtime.
- **Missing strings fall back to English**, never to an empty label.
- **Language switcher** in the top bar/user menu, in settings and in onboarding. **Language names, not flags** ("Deutsch · Italiano · English") — flags represent countries, not languages.
- **Dates, times, numbers, relative times ("in 8 minutes") and sorting** are formatted per locale via the browser's `Intl` APIs — never hardcoded (`23.09.2026` vs. `23/09/2026`; "Ärzte" sorts under A).
- **RTL-ready from the start**: CSS uses logical properties (start/end, not left/right) so a right-to-left language (e.g. Arabic) later is a switch, not a rewrite.
- **Fonts per script**: Source Sans 3 covers Latin (all European languages), Greek, Cyrillic and Vietnamese. For other scripts, add a matching fallback font (e.g. Noto Sans Arabic) that loads only when that language is selected.

## Time zones

- **Everything is UTC internally** — backend, database, API. Conversion happens only for display, in the frontend, per user. WebControl already provides Unix timestamps.
- **Display time zone, one setting per user**: *same as the box* (default — a TV schedule belongs to the box's location), *same as this browser*, or *fixed*. When the displayed zone differs from the browser's, a small hint shows it ("Times in Europe/Rome") — relevant when accessing the box remotely (e.g. via ZeroTier) while traveling.
- **Never derive times from file names.** Recording file names contain local time without a zone (`20260923 1759 - ONE HD - …`), which is ambiguous around daylight-saving changes. Use the API or `.ts.meta`.
- **Durations are computed from timestamps**, never from wall-clock arithmetic (timers across a DST change).
- **AI**: EPG is sent with explicit zone offsets; times in AI responses are validated before they become a timer suggestion.
- **Clock check**: `/api/v1/health` includes the server time; the browser compares and DreamHub warns when the box's clock is off by more than a minute (a wrong clock means wrong timers).
- **Tests** use a fixed clock and include the DST transition days for `Europe/Rome` (South Tyrol's zone).

## Connectors

| Connector | Used by | Release |
|---|---|---|
| WebControl `/web/*` (XML, see `ARCHITECTURE.md`) | almost every hub | v1 |
| SerienRecorder web interface API | SeriesHub, TimerHub, search | v1 (API to be verified on device) |
| journald (local) | AdminHub | v1 |
| BouquetEditor plugin `/bouqueteditor/*` | ChannelHub | v1.1 |
| TMDB | MediaHub, GuideHub | v1 |
| OpenAI | DiscoveryHub (first AI provider) | v1 |
| Other AI providers (Claude, Gemini, local models) | same interface | later |
| Plex | PlexHub ("Plex Mediathek"), search, dashboard tile | built 2026-10-05 |
| Jellyfin / Emby | same shape as Plex | later |

**External media servers are optional and must degrade gracefully.** The reference setup's Plex runs on a server that isn't on 24/7 — with the connector model that's acceptable: DreamHub caches the last known library state and shows "Plex offline, last synced yesterday 21:30"; everything else keeps working. A hub requirement can say so (`hub.Requirement.Cached`): an unreachable connector then only degrades the hub instead of making it unavailable.

## Enigma2 TV plugin

- **v1**: a launcher, not a second UI. Shows a **QR code** on the TV that opens DreamHub on a phone without typing an IP address, shows whether the backend is running and its version, and offers a few basic settings (auth on/off, port). Language follows the system (`config.osd.language`). Same division of labor as the WebControl plugin: basic settings on the TV, everything else in the web UI.
- **Later**: recommendations and notifications shown directly on the TV.

## UX direction: a good-looking work tool

Concrete tokens, type scale, layout and components: [`DESIGN.md`](DESIGN.md), with the approved interactive mockup in `docs/design/`.

**Desktop first** — most use will be on a PC — but fully usable on a phone.

DreamHub is a tool for getting things done, not a media center for browsing. Plex is a reference for imagery, not for the overall feel.

- **Information first**: dense, well-aligned lists and tables with actions directly on the row (on desktop, secondary actions appear on hover).
- **Images support, they don't dominate**: small covers and picons *inside* rows, no big poster shelves (a cover row is an optional tile).
- **Calm**: dark neutral ground, one accent color, status colors (on air, conflict, disk full, OK) used only for status.
- **Avoid the generated look**: no uppercase letter-spaced micro-labels, no outlined buttons everywhere, no pill chips, not every block in a rounded bordered box, no trendy condensed display fonts. At most one primary action per tile; the rest are text actions. Minimal corner rounding.
- **Real content**: picons (`/data/piconpool`), cover art (`SR_Cover`, `.jpg` next to recordings), real air times.
- **Typography**: **Source Sans 3**, bundled with the app (no requests to Google Fonts at runtime — the box must work offline, and the UI shouldn't call out to third parties). Sentence-case headings.
- **Appearance**: **dark by default**, switchable to light or "follow system". No theme system in v1 — all colors are design tokens, so themes or a selectable accent color are cheap to add later.
- **Keyboard**: `/` search, `g` + letter to jump to a hub, arrow keys in lists, `Esc` closes dialogs. Accessible contrast and screen-reader labels from the start.
- **Live**: the dashboard updates over Server-Sent Events (e.g. "recording finished") without reloading.
- **Box unreachable**: a clear banner ("Box not reachable, last connected 14:02"), never empty tiles or raw errors.
- **Units**: GB in decimal (1000), matching how Enigma2 itself reports sizes.
- Layout reference: Sonarr/Radarr (sidebar + dense lists); imagery reference: Plex Web, used sparingly.

## Release scope summary

- **v1**: the complete core (dashboard with customizable and custom tiles, navigation, global search, notifications inbox, settings, onboarding, diagnostics, auth, i18n DE/IT/EN, time zone handling, profile- and role-ready data model) + MediaHub, TimerHub, GuideHub, DiscoveryHub, SeriesHub (basic), SystemHub (basic), AdminHub (logs), ControlHub (remote, screenshot, power — pulled forward from v1.1) + the TV launcher plugin + `.deb` packaging.
- **v1.1**: RadioHub, ChannelHub, MediaHub trimming, AdminHub service status, ControlHub's "message to TV" (`/web/message`). Built early and confirmed on the box: RadioHub, service status, message to TV (release 0.1.0); still to build: ChannelHub, trimming.
- **Later**: AutomationHub, AdminHub terminal, external notification channels, Jellyfin/Emby connectors (Plex built early), more AI providers, profile/role UI, multiple dashboards UI, additional languages incl. RTL.
