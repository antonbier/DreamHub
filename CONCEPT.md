# DreamHub — Product Concept

What DreamHub *is* as a product: its structure, its hubs, and how they fit together. `PROJECT_PLAN.md` covers why the project exists and what's out of scope; `PRINCIPLES.md` covers how everything is built; this document covers what gets built.

## What makes DreamHub distinct

Almost every individual capability DreamHub offers already exists somewhere on this platform: DreamEPG Premium does EPG and timers on a phone, AdvancedMovieSelection renames recordings on the TV, GraphMultiEPG shows a grid EPG on the TV (see `ARCHITECTURE.md`). What doesn't exist anywhere is **one place where all of it connects**. The value is in the flows *across* hubs, not in any single hub:

> The AI recommends *Die Kanzlei, S01/E07* on ONE HD at 21:45 → one tap creates the timer (after confirmation) → the recording lands in the MediaHub → DreamHub suggests a clean file name → the user confirms.

> A search for "Tatort" returns, in one result: when it's on (GuideHub), whether a timer is set (TimerHub), which episodes are already on disk (MediaHub), and whether SerienRecorder tracks it (SeriesHub).

When deciding whether a feature belongs in DreamHub, the question is: does it get better by being connected to the rest?

## Three building blocks

| Block | What it is | Examples |
|---|---|---|
| **Core (shell)** | Everything shared across hubs | home page, global search, notifications, settings, onboarding, diagnostics, auth, profiles |
| **Hubs** | User-facing feature areas | MediaHub, TimerHub, GuideHub, … (see below) |
| **Connectors** | Data sources and external services | WebControl `/web/*` API, SerienRecorder web API, TMDB, OpenAI, later Plex/Jellyfin/Emby |

Rules between them:

- **Hubs only reach data through connectors, via the core.** A hub never talks HTTP to WebControl itself.
- **Hubs only talk to other hubs through the core API or core events** — never by importing each other's code. When DiscoveryHub creates a timer, it calls TimerHub's API.
- **A new connector extends every hub that can use it.** Adding a Jellyfin connector shouldn't require rewriting the MediaHub.
- **The AI provider is a connector like any other** — it lives in the core, not inside DiscoveryHub, so other hubs (e.g. MediaHub name suggestions, GuideHub summaries) can use it later.

## The hub contract

Every hub registers itself with the core by declaring:

- an ID, a localized name, an icon, and a navigation entry
- its API routes (under `/api/v1/<hub>/...`)
- a **settings schema** (fields, types, validation, which fields are secrets) — the core renders the settings UI from this
- optionally a **search provider** for global search
- optionally **home contributions** (timeline entries or tiles, depending on which home variant wins — see below)
- the **events** it emits and listens to (e.g. `recording.finished`, `timer.conflict`)
- its **requirements** (connectors, capabilities like ffmpeg, installed plugins)

From the requirements the core derives each hub's **status**: *available*, *degraded*, or *unavailable* — always with a reason and a fix (see "Diagnostics" below).

## Hubs

| Hub | Scope | Release |
|---|---|---|
| **MediaHub** | Recordings list, disk space overview, rename with preview, cover art, trash | v1 (trimming in v1.1) |
| **TimerHub** | Timers, AutoTimer, conflict detection | v1 |
| **GuideHub** | EPG as a TV-guide grid ("TV-Zeitschrift") | v1 (basic) |
| **DiscoveryHub** | AI recommendations, one tap → TimerHub (with confirmation) | v1 |
| **SeriesHub** | SerienRecorder: episodes on disk, missing episodes, upcoming | v1 (basic) |
| **SystemHub** | Device info, diagnostics, backup/restore, logs, change log | v1 (basic) |
| **RadioHub** | Radio services, now playing, radio timers | v1.1 |
| **ChannelHub** | Bouquets (link/embed the existing BouquetEditor plugin), picons | v1.1 |
| **ControlHub** | Web remote, message to TV, screenshot, power | v1.1 |
| **StreamHub** | Live TV / radio in the browser | later |
| **AutomationHub** | Scripts, webhooks, automations — the only place anything happens automatically | later |

Notes that affect several hubs:

- **Single tuner.** The reference device has exactly one tuner (`Si2169D`, "Tuner A" in `/web/deviceinfo`). Overlapping timers are a real, common conflict, not an edge case — TimerHub's conflict handling and DiscoveryHub's "create timer" flow both have to take this seriously.
- **SerienRecorder has its own web interface** (`config.plugins.serienRec.enableWebinterface=true`). SeriesHub should use its API rather than reading `SerienRecorder.db` directly while SerienRecorder may be writing to it. Needs verification on the device.
- **Radio in the browser** isn't automatically easier than TV: radio services often use MP2 audio, which browsers don't reliably play. Playback belongs in StreamHub, later.

## Core features

### Home page

Two candidate designs, to be decided from a side-by-side mockup:

- **"Today" timeline** — one chronological view of the day: what's on now, what's recording tonight, recommendations, what finished recording. TV is inherently time-based, which makes this feel native to the subject.
- **Tiles** — one tile per hub (storage, next timers, recommendations, new recordings, series, now on air). More familiar, but at risk of the generic dashboard look DreamHub explicitly wants to avoid.

Either way, a thin **status strip** at the top: disk space, timer conflicts, unread notifications.

### Global search

Every hub can contribute a search provider; results are grouped by hub. Keyboard shortcut (`/` or `Cmd+K`). Possibly also in the TV plugin later.

### Notifications

- Hubs emit events; the core turns relevant ones into notifications: recording finished, timer conflict, disk above 90 %, hub became unavailable.
- Shown in an **in-app inbox** and the status strip. No external push in v1.
- Later, via AutomationHub/webhooks: push, Telegram, ntfy — or a message **on the TV itself** via WebControl's existing `/web/message`.

### Settings

One central settings menu, organized as: general (language, auth, network, AI provider) first, then one section per hub, generated from each hub's settings schema. Includes search within settings and "test connection" buttons (e.g. validate an API key). Everything visible in the UI is also available through the API.

### Onboarding wizard

On first start, re-runnable from settings:

1. Language
2. Auth on/off, password
3. Device detection (`/web/deviceinfo`) and capability check
4. Choose which hubs to enable
5. AI provider and API key (skippable)
6. Channels to use for recommendations
7. Done — show the QR code / bookmark hint

Enigma2 has its own start wizard (`startwizard.xml` in `/usr/share/enigma2`), so this is a pattern users of the platform already know.

### Diagnostics / hub status

Every hub that isn't fully *available* shows why and how to fix it — e.g. "SerienRecorder is not installed → install it from the Addon Manager under …". Visible in three places: the hub's own placeholder, a diagnostics page in SystemHub, and the onboarding wizard. A one-click "install package" is possible later, but it's a change to the system like any other and therefore needs preview and confirmation (see `PRINCIPLES.md`). In v1, instructions are enough.

### Profiles

Prepared, not built. The data model attaches preferences to a user from day one and the API carries a user context, but v1 has a single user and no profile switching in the UI. This keeps multi-user (per-person language, recommendations, login) possible later without a data migration.

## Connectors

| Connector | Used by | Release |
|---|---|---|
| WebControl `/web/*` (XML, see `ARCHITECTURE.md`) | almost every hub | v1 |
| SerienRecorder web interface API | SeriesHub, TimerHub, search | v1 (API to be verified on device) |
| BouquetEditor plugin `/bouqueteditor/*` | ChannelHub | v1.1 |
| TMDB | MediaHub, GuideHub | v1 |
| OpenAI | DiscoveryHub (first AI provider) | v1 |
| Other AI providers (Claude, Gemini, local models) | same interface | later |
| Plex / Jellyfin / Emby | MediaHub, search | later |

**External media servers are optional and must degrade gracefully.** The reference setup's Plex runs on a server that isn't on 24/7 — the same reason this idea was rejected once before. With the connector model it's acceptable: DreamHub caches the last known library state and shows "Plex offline, last synced yesterday 21:30"; everything else keeps working.

## Enigma2 TV plugin

- **v1**: a launcher, not a second UI. Shows a **QR code** on the TV that opens DreamHub on a phone without typing an IP address, shows whether the backend is running and its version, and offers a few basic settings (auth on/off, port). Language follows the system (`config.osd.language`). Same division of labor as the WebControl plugin: basic settings on the TV, everything else in the web UI.
- **Later**: recommendations and notifications shown directly on the TV.

## UX direction

DreamHub must not look AI-generated or templated (no purple gradients, shadowed cards everywhere, emoji icons, centered-everything). It has its own material to work with:

- **Real content instead of placeholders**: picons (`/data/piconpool`), cover art (`SR_Cover`, `.jpg` files next to recordings), real air times.
- **TV-guide conventions**: a time grid like a printed TV magazine, a red "on air" marker, tabular figures for times.
- **Density for daily users** — a tool, not a landing page.
- Optionally a dark theme related to the Shadow-WQHD skin, so the TV and the web UI feel like one family.
- Mobile-first (phone/tablet is the main use), but usable on desktop.

## Release scope summary

- **v1**: the complete core (home, search, notifications inbox, settings, onboarding, diagnostics, auth, profile-ready data model) + MediaHub, TimerHub, GuideHub, DiscoveryHub, SeriesHub (basic), SystemHub (basic) + the TV launcher plugin + `.deb` packaging.
- **v1.1**: RadioHub, ChannelHub, ControlHub, MediaHub trimming.
- **Later**: StreamHub, AutomationHub, external notification channels, Plex/Jellyfin/Emby connectors, more AI providers, profile UI, installable PWA (needs HTTPS — e.g. via a reverse proxy).
