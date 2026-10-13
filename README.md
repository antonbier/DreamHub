# DreamHub

One place for everything your **Dreambox (DreamOS)** does — recordings, timers, the TV guide, radio, series, recommendations, the remote — in a modern web interface that runs on the box itself, alongside the native WebControl rather than replacing it. Works in any browser and installs as an app on phones and desktops.

## What it is

DreamHub is a **core** (dashboard, global search, notifications, settings in tabs with backup/restore, setup assistant, profiles with an admin and a viewer role) with **hubs** plugged into it:

- **MediaHub** — recordings and folders, disk space, rename (also in bulk, by naming scheme, into folders), covers, trash; finding ad breaks by the channel logo and cutting them out without re-encoding
- **SeriesHub** — SerienRecorder: followed series, episode guide with what's missing (TVmaze), what's coming
- **GuideHub** — the TV guide in four views (grid, now & next, tonight, one channel), channel logos, record or switch from any programme
- **RadioHub** — radio stations with what's on, play on the box, radio recordings
- **Live TV** (StreamHub) — TV channels and recordings right in the browser: the box sends its stream unchanged, your phone or PC decodes it (SD and HD, Dolby sound), no app needed
- **TimerHub** — timers, disabling/enabling, AutoTimer, conflict detection
- **DiscoveryHub** — AI recommendations from the EPG, optionally matched to your taste, one tap to a timer
- **ControlHub** — a remote with the live picture, on every page; messages to the TV
- **DeviceHub** ("Gerät") — device info, diagnostics with a box self-test, change log; for admins also the box's services with restart and logs (live), crash logs, a package browser with update notifications, a script library and a terminal
- **PlexHub** ("Plex Mediathek") — your Plex server's films and series as tiles or a list, watched state synced with Plex, films played on the box's own player; the library stays visible while the server is off
- **ChannelHub** ("Channel list") — your bouquets on the phone: sort, rename, add, remove, headings, new channels after a scan, a channel check and logo sets, with a backup before every change
- **AppsHub** — small third-party bridges as optional extensions (e.g. a smart switch — see [docs/BUILDING_EXTENSIONS.md](docs/BUILDING_EXTENSIONS.md) if you want to build one), and self-hosted apps (AdGuard Home, Pi-hole, Nextcloud with its own database, Vaultwarden, Home Assistant and more) run as Docker containers: browse, install, start/stop, update notifications, logs, a console, and backup — all admin-only
- later: AutomationHub (see [ROADMAP.md](ROADMAP.md))

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

**0.4.0** is the current release (0.1.0 was the first), adding DeviceHub's administration (scripts, terminal, package browser), Extension Hub and AppsHub (optional bridges and self-hosted Docker apps). Every released hub works against the real box — confirmed on the reference Dreambox One with the self-test (Device → Diagnostics), Plex playback on the box included — and against a mock for development, so no Dreambox is needed to work on it. Installing Docker apps needs Docker on the box, which the reference box doesn't have; AppsHub's install path is so far only tested against a mock Docker. [ROADMAP.md](ROADMAP.md) has exactly what's done and what's open, [CHANGELOG.md](CHANGELOG.md) what changed, [PROJECT_PLAN.md](PROJECT_PLAN.md) why the project exists, and [backend/README.md](backend/README.md) / [frontend/README.md](frontend/README.md) how to run it locally.

## License

[CC BY-NC-SA 3.0](LICENSE), matching the convention of the Enigma2/DreamOS plugin community.
