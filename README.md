# DreamHub

One place for everything your **Dreambox (DreamOS)** does — recordings, timers, the TV guide, radio, series, recommendations, the remote — in a modern web interface that runs on the box itself, alongside the native WebControl rather than replacing it. Works in any browser and installs as an app on phones and desktops.

## What it is

DreamHub is a **core** (dashboard, global search, notifications, settings in tabs with backup/restore, setup assistant) with **hubs** plugged into it:

- **MediaHub** — recordings and folders, disk space, rename (also in bulk, by naming scheme, into folders), covers, trash
- **SeriesHub** — SerienRecorder: followed series, episode guide with what's missing (TVmaze), what's coming
- **GuideHub** — the TV guide in four views (grid, now & next, tonight, one channel), channel logos, record or switch from any programme
- **RadioHub** — radio stations with what's on, play on the box, radio recordings
- **TimerHub** — timers, disabling/enabling, AutoTimer, conflict detection
- **DiscoveryHub** — AI recommendations from the EPG, optionally matched to your taste, one tap to a timer
- **ControlHub** — a remote with the live picture, on every page; messages to the TV
- **SystemHub** — device info, diagnostics with a box self-test, change log
- **PlexHub** ("Plex Mediathek") — your Plex server's films and series as tiles or a list, watched state synced with Plex, films played on the box's own player; the library stays visible while the server is off
- **AdminHub** — the box's services with restart, logs (live), crash logs — admins only
- later: ChannelHub, StreamHub, AutomationHub (see [ROADMAP.md](ROADMAP.md))

The dashboard is a personal set of tiles ("Now on the box", timers, conflicts, disk space, episodes, your own saved views). Details come from the box's own cover art and DreamMediaInfo first, then TMDB, OMDb (Rotten Tomatoes, IMDb, Metascore) and fanart.tv. Push notifications to the phone go through ntfy. Available in German, Italian and English.

Hubs get their data through **connectors** (WebControl, SerienRecorder, AutoTimer, TMDB, TVmaze, OMDb, fanart.tv, OpenAI, ntfy, Plex). The value is in how they connect: a recommendation becomes a timer, the timer becomes a recording, the recording gets a clean name — all in one place. **Nothing is ever changed without showing you what will change first.** See [CONCEPT.md](CONCEPT.md).

**Using it:** [docs/USER_GUIDE.md](docs/USER_GUIDE.md) — installing on the box, first steps, every part explained, the app, push notifications, troubleshooting.

## Supported boxes

| Box | Package | Status |
| --- | --- | --- |
| Dreambox One, Two (DreamOS, ARM64) | `dreamhub_<version>_arm64.deb` | developed and tested on a Dreambox One (DreamOS 2.6) |
| DM900 UHD, DM920 UHD (DreamOS, ARMv7) | `dreamhub_<version>_armhf.deb` | built, not yet tested on such a box |
| DM7080, DM820, DM520, DM525 (DreamOS, MIPS) | — | not yet: the SQLite driver has no MIPS port |
| Older Dreamboxes without DreamOS | — | not supported (no dpkg, no systemd) |

Packages are attached to each [release](https://github.com/antonbier/DreamHub/releases).

## Tech stack

- **Backend:** Go, statically compiled, runs as a systemd service on port 8080. REST API, Server-Sent Events for live updates, SQLite for DreamHub's own state.
- **Frontend:** Vue 3 + TypeScript + Tailwind, embedded into the Go binary via `go:embed`; an installable web app (manifest + service worker over HTTPS).
- **TV plugin:** a minimal Enigma2 plugin (Python 2.7) — a launcher with a QR code to open DreamHub on your phone.
- **Distribution:** `.deb` packages from the releases; an apt feed is planned.

## Status

**0.1.7** is the current release (0.1.0 was the first). Every hub above works against the real box — confirmed on the reference Dreambox One with the self-test (System → Diagnostics); playing Plex films on the box is still experimental — and against a mock for development, so no Dreambox is needed to work on it. [ROADMAP.md](ROADMAP.md) has exactly what's done and what's open, [CHANGELOG.md](CHANGELOG.md) what changed, [PROJECT_PLAN.md](PROJECT_PLAN.md) why the project exists, and [backend/README.md](backend/README.md) / [frontend/README.md](frontend/README.md) how to run it locally.

## License

[CC BY-NC-SA 3.0](LICENSE), matching the convention of the Enigma2/DreamOS plugin community.
