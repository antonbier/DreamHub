# DreamHub

One place for everything your **Dreambox One (DreamOS AIO)** does — recordings, timers, EPG, series, AI recommendations — in a modern web interface, running alongside the native WebControl rather than replacing it.

## What it is

DreamHub is a **core** (home page, global search, notifications, settings, onboarding) with **hubs** plugged into it:

- **MediaHub** — recordings, disk space, rename with preview, trash
- **TimerHub** — timers, AutoTimer, conflict detection
- **GuideHub** — EPG as a TV-guide grid
- **DiscoveryHub** — AI recommendations, one tap to a timer
- **SeriesHub** — SerienRecorder: what you have, what's missing, what's coming
- **SystemHub** — device info, diagnostics, backup, change log
- **AdminHub** — logs (incl. DreamHub's own), later service status and terminal — admins only
- later: RadioHub, ChannelHub, ControlHub, StreamHub, AutomationHub

The home page is a personal, customizable dashboard of tiles. Available in German, Italian and English, extensible by dropping in a locale file.

Hubs get their data through **connectors** (WebControl, SerienRecorder, TMDB, OpenAI, later Plex/Jellyfin/Emby). The value is in how they connect: a recommendation becomes a timer, the timer becomes a recording, the recording gets a clean name — all in one place. Nothing is ever changed without showing you what will change first. See [CONCEPT.md](CONCEPT.md).

## Tech stack

- **Backend:** Go, statically compiled for ARM64, runs as a systemd service on port 8080. REST API + WebSockets.
- **Frontend:** responsive web app (Vue.js + Tailwind), embedded into the Go binary via `go:embed`.
- **TV plugin:** a minimal Enigma2 plugin (Python 2.7) — a launcher with a QR code to open DreamHub on your phone.
- **Distribution:** an installable `.deb` via its own apt feed.

## Status

Pre-release, but the core shell is real and running, not just planned: REST API, SQLite-backed settings/dashboards/change log, auth (Basic Auth + revocable API tokens), the full web UI (sidebar, dashboard tiles, schema-driven settings, global search, notifications, onboarding wizard, status strip), and TimerHub as the first working hub, all buildable and testable against a mock — no Dreambox required. See [ROADMAP.md](ROADMAP.md) for exactly what's done and what's open under **Foundation**, [PROJECT_PLAN.md](PROJECT_PLAN.md) for why the project exists, and [backend/README.md](backend/README.md) / [frontend/README.md](frontend/README.md) to run it locally.

Still open before v1: MediaHub, GuideHub, DiscoveryHub, SeriesHub and the rest of AdminHub (see "v1 — hubs" in `ROADMAP.md`), plus anything in the Foundation's "Open" list that needs real device access (`ARCHITECTURE.md`'s open questions) — the reference Dreambox isn't reachable at the moment, so that work is paused, not forgotten.

## License

[CC BY-NC-SA 3.0](LICENSE), matching the convention of the Enigma2/DreamOS plugin community.
