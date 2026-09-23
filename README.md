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

Planning is complete; the backend foundation has started. See [PROJECT_PLAN.md](PROJECT_PLAN.md) for context, [ROADMAP.md](ROADMAP.md) for progress, and [backend/README.md](backend/README.md) to run it locally (no Dreambox required — it defaults to a mock).

## License

[CC BY-NC-SA 3.0](LICENSE), matching the convention of the Enigma2/DreamOS plugin community.
