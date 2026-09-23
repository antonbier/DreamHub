# CLAUDE.md

Instructions for any Claude Code (or other AI coding agent) instance working in this repository.

## Start here

Read in this order:

1. [`PROJECT_PLAN.md`](PROJECT_PLAN.md) — why the project exists, what's out of scope, confirmed technical facts.
2. [`CONCEPT.md`](CONCEPT.md) — the product: core (shell), hubs, connectors, the hub contract, release scope (v1 / v1.1 / later).
3. [`PRINCIPLES.md`](PRINCIPLES.md) — how everything is built. Start with "Preview and confirm — no silent changes".
4. [`ARCHITECTURE.md`](ARCHITECTURE.md) — a read-only inspection of the real reference device: WebControl API, installed plugins, process/service layout, config conventions, partition layout.
5. [`ROADMAP.md`](ROADMAP.md) — technical notes per hub and what's done/open.

`README.md` is the short pitch; `CHANGELOG.md` tracks releases; `backend/README.md` explains how to run and test the backend.

**Do not re-propose items listed as out of scope in `PROJECT_PLAN.md`** without a genuinely new technical argument — they were evaluated and turned down for stated reasons. Treat these documents as ground truth for prior decisions, not as suggestions to second-guess from scratch.

## Language rule

- **All repository content is English**: documentation, commit messages, issue/PR titles and bodies, code comments. No exceptions, even though the project was originally planned in German.
- This is a rule about the *repository*, not the *product* — see multilingual requirement below, which is a separate, runtime concern.

## Multilingual requirement (product feature)

DreamHub itself must support multiple languages at runtime:

- **Web UI**: a user-switchable language selector, independent of the OS. Use an i18n library (e.g. vue-i18n) with per-locale JSON translation strings.
- **Enigma2 plugin**: must follow DreamOS's own configured system language automatically — no separate setting for the user to maintain. Read `config.osd.language.value` (format `de_DE`) and load matching gettext `.po`/`.mo` translations, the same mechanism other Enigma2 plugins use (see `PROJECT_PLAN.md`).

## Architecture constraints (do not violate)

- **Preview and confirm, always.** No change to recordings, timers, bouquets or the system without showing exactly what will change and getting explicit confirmation. Mutating API operations are two-step (`plan` → `apply`). Nothing automatic in v1. See `PRINCIPLES.md`.
- **Respect the core / hubs / connectors boundaries** (`CONCEPT.md`). Hubs reach data only through connectors, talk to other hubs only through the core API or events, and never import another hub's code.
- **Never modify the native WebControl** on port 80. It's Dreambox's own closed-source interface. DreamHub runs alongside it (port 8080).
- **No live dependency on servers that aren't always on** (e.g. the Unraid server hosting Plex). Such connectors are optional, cache their last known state, and degrade gracefully.
- **No software transcoding of live TV.** The Amlogic S922X has no hardware video encoder. See `ROADMAP.md`, StreamHub.
- **Don't re-implement what Enigma2/WebControl already exposes.** Its `/web/*` API is confirmed OpenWebif-compatible (see `ARCHITECTURE.md` for endpoints and schemas); BouquetEditor and SerienRecorder have their own web APIs. Call these instead of re-parsing internal files or binary caches.
- **Existing on-screen plugins are references, not replacements.** `AdvancedMovieSelection`, `GraphMultiEPG`, `EPGSearch`, `TvSpielfilm`, `CutListEditor` are native TV-remote UIs — read them for approach (respect their CC BY-NC-SA license, don't copy code). The "GPT-Plugin" is a GUID Partition Table tool, not AI.

## Repo conventions

- GitHub milestones: **Foundation**, **v1 — hubs**, **v1 — release & packaging**, **v1.1**, **Later** (matching `ROADMAP.md`). Phase 0 (spikes) is closed.
- Issues labeled `device-access` need a reachable Dreambox; everything else should be doable against the mock (`backend/internal/webcontrol/mock`).
- Project board (Kanban): https://github.com/users/antonbier/projects/1
- When a new device fact is confirmed, update `ARCHITECTURE.md`; when a scope or architecture decision is made, update `PROJECT_PLAN.md`, `CONCEPT.md` or `PRINCIPLES.md`. Someone without access to the original planning conversation must be able to reconstruct every decision from these files.
