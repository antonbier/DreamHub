# DreamHub

One place for everything your **Dreambox (DreamOS)** — and, as a second box, a **VU+ or other OpenWebif receiver** — does — recordings, timers, the TV guide, radio, series, recommendations, the remote — in a modern web interface that runs on the box itself, alongside the native WebControl rather than replacing it. Works in any browser and installs as an app on phones and desktops.

## What it is

DreamHub is a **core** (dashboard, global search, notifications, settings in tabs with backup/restore, setup assistant, profiles with an admin and a viewer role) with **hubs** plugged into it:

- **MediaHub** — recordings and folders, disk space, rename (also in bulk, by naming scheme, into folders), covers, trash; finding ad breaks by the channel logo, cutting them out (or any part you name) without re-encoding, and flagging recordings that stopped early
- **SeriesHub** — SerienRecorder: followed series, episode guide with what's missing (TVmaze), what's coming
- **GuideHub** — the TV guide in four views (grid, now & next, tonight, one channel), channel logos, record or switch from any programme
- **RadioHub** — radio stations with what's on and the song or line the station sends, play on the box or in the browser, radio recordings
- **Live TV** (StreamHub) — TV channels and recordings right in the browser: the box sends its stream unchanged, your phone or PC decodes it (SD and HD, Dolby sound), no app needed
- **TimerHub** — timers, disabling/enabling, AutoTimer, conflict detection
- **DiscoveryHub** — AI recommendations from the EPG, optionally matched to your taste, one tap to a timer
- **ControlHub** — a remote with the live picture, on every page; messages to the TV
- **DeviceHub** ("Gerät") — device info, diagnostics with a box self-test, change log; for admins also the box's services with restart and logs (live), crash logs, a package browser with update notifications, a script library and a terminal
- **PlexHub** ("Plex Mediathek") — your Plex server's films and series as tiles or a list, watched state synced with Plex, films played on the box's own player; the library stays visible while the server is off
- **ChannelHub** ("Channel list") — your bouquets on the phone: sort, rename, add, remove, headings, new channels after a scan, a channel check and logo sets, with a backup before every change, and a copy of the list from another box
- **AppsHub** — small third-party bridges as optional extensions (e.g. a smart switch — see [docs/BUILDING_EXTENSIONS.md](docs/BUILDING_EXTENSIONS.md) if you want to build one), and self-hosted apps (AdGuard Home, Pi-hole, Nextcloud with its own database, Vaultwarden, Home Assistant and more) run as Docker containers: browse, install, start/stop, update notifications, logs, a console, and backup — all admin-only
- later: AutomationHub (see [ROADMAP.md](ROADMAP.md))

The dashboard is a personal set of tiles ("Now on the box", timers, conflicts, disk space, episodes, your own saved views). Details come from the box's own cover art and DreamMediaInfo first, then TMDB, OMDb (Rotten Tomatoes, IMDb, Metascore) and fanart.tv. Push notifications to the phone go through ntfy. Available in German, Italian and English.

Hubs get their data through **connectors** (WebControl, SerienRecorder, AutoTimer, TMDB, TVmaze, OMDb, fanart.tv, OpenAI, ntfy, Plex). The value is in how they connect: a recommendation becomes a timer, the timer becomes a recording, the recording gets a clean name — all in one place. **Nothing is ever changed without showing you what will change first.** See [CONCEPT.md](CONCEPT.md).

**Using it:** [docs/USER_GUIDE.md](docs/USER_GUIDE.md) — installing on the box, first steps, every part explained, the app, push notifications, troubleshooting.

## Supported boxes

DreamHub runs **on** a Dreambox, or on any machine in your network (a server, a Raspberry Pi) that reaches the receivers. Receivers are used in one of two ways: **Full** — DreamHub runs on the box itself — or **Basic** — DreamHub reaches the box over the network through its web interface, nothing is installed on it.

| Receiver | Image | DreamHub runs on it (Full) | As a box in DreamHub (Basic) |
| --- | --- | --- | --- |
| Dreambox One, Two (ARM64) | DreamOS | `dreamhub_<version>_arm64.deb` — developed and tested on a Dreambox One (DreamOS 2.6) | yes (WebControl) |
| DM900 UHD, DM920 UHD (ARMv7) | DreamOS | `dreamhub_<version>_armhf.deb` — built, not yet tested on such a box | yes (WebControl), not yet tested |
| DM7080, DM820, DM520, DM525 (MIPS) | DreamOS | not yet: the SQLite driver has no MIPS port | yes (WebControl), not yet tested |
| Older Dreamboxes | without DreamOS | no (no dpkg, no systemd) | not tested |
| **VU+ Solo2** | **VTi 15.0.04** (OpenWebif OWIF 1.5.2) | no (45 MB of free flash, no systemd) | **yes — checked** (details below) |
| Other VU+ (Duo2, Solo SE, Zero, 4K models …) | VTi | no | expected to work like the Solo2, not tested |
| Other Enigma2 receivers (Gigablue, Xtrend, Vu+, …) | OpenATV, OpenPLi, OpenVision, … with OpenWebif | no | expected to work (OpenWebif answers the same way), **not tested** — reports welcome |

**What a Basic box does** (checked on the Solo2 with VTi 15, against a Dreambox One as the main box):

| Feature | Dreambox (WebControl) | VU+ / OpenWebif (Basic) |
| --- | --- | --- |
| Live TV in the browser, zapping, now & next, the guide | yes | yes |
| Timers: list, add, delete, conflict hint | yes | yes; the view also shows what each tuner does |
| Channel lists (bouquets) | yes | yes, through OpenWebif's BouquetEditor (adding and removing a bouquet tried) |
| Remote, screenshot | yes | yes (the screenshot through `/grab`; needs the `grab` tool) |
| Recordings: list, watch in the browser with seeking | yes | yes, read through the box's web interface |
| Play a recording on the box's own TV | yes (DreamHub's TV plugin) | yes, from the start (no start position) |
| Play a recording of another box on this box | yes | as the source: yes; as the target: no |
| Recordings: rename, move, trash, ad cut, cutting by hand | yes | no — needs DreamHub on the box |
| AutoTimer, SerienRecorder | where installed | where installed (VTi 15 has neither) |
| Logos, packages, services, logs, terminal, Docker apps | yes | no — needs DreamHub on the box |
| Wake from standby | WebControl | Wake-on-LAN or a smart plug (not tested from deep standby) |

Not tested yet: a web login switched on at the receiver, the browser player and live TV in a browser against the Solo2 (the streams were checked as bytes and codecs: H.264, MP2 and AC3, like a Dreambox's), messages to the TV, and a Wi-Fi-only box under load (the Solo2 was on Wi-Fi, and dropped off the network once during the checks, cause not established). A **cable connection is recommended** for a receiver that is only reachable by Wi-Fi. How the receiver was checked: [ARCHITECTURE.md](ARCHITECTURE.md), "VU+ Solo2 with VTi 15"; the open points: [#56](https://github.com/antonbier/DreamHub/issues/56).

Packages are attached to each [release](https://github.com/antonbier/DreamHub/releases).

## Tech stack

- **Backend:** Go, statically compiled, runs as a systemd service on port 8765. REST API, Server-Sent Events for live updates, SQLite for DreamHub's own state.
- **Frontend:** Vue 3 + TypeScript + Tailwind, embedded into the Go binary via `go:embed`; an installable web app (manifest + service worker over HTTPS).
- **TV plugin:** a minimal Enigma2 plugin (Python 2.7) — a launcher with a QR code to open DreamHub on your phone.
- **Distribution:** `.deb` packages from the releases; an apt feed is planned.

## Status

**0.9.6** is the current release (0.1.0 was the first): Netcast channels (Samsung TV Plus, Pluto and playlist sources) into a bouquet as the plugin's own entries, the Box settings tab fixed on a Dreambox, radio and phone fixes; before it **0.9.5**: internet channels (an M3U list, a file or a GitHub repository into a bouquet, marked in the channel list), Netcast's sources on a Dreambox One (list, add, change, remove — behind a switch on the TV), the box's own settings in view-only form, the tuner's reception and other airings in the guide; before it **0.9.4**: VU+ and other OpenWebif receivers as Basic boxes next to a Dreambox (checked on a VU+ Solo2 with VTi 15), what each tuner does, a lighter channel list on slow receivers, and a fix that kept a crashing request from reaching the box; before it the TV plugin's QR code and screens fixed, radio text, cutting by hand, copying a channel list between boxes, several Dreamboxes behind one selector (pairing, Wake-on-LAN or a smart plug to switch one on, a recording of one box on another's screen), extensions that show their own values on the start page, Docker apps with an install form, updates with roll back and backups, marked ad breaks the TV really skips, and many small clarity fixes. Every released hub works against the real box — confirmed on the reference Dreambox One with the self-test (Device → Diagnostics), Plex playback on the box included — and against a mock for development, so no Dreambox is needed to work on it. Installing Docker apps needs Docker on the box, which AppsHub can set up itself. [ROADMAP.md](ROADMAP.md) has exactly what's done and what's open, [CHANGELOG.md](CHANGELOG.md) what changed, [PROJECT_PLAN.md](PROJECT_PLAN.md) why the project exists, and [backend/README.md](backend/README.md) / [frontend/README.md](frontend/README.md) how to run it locally.

## License

[CC BY-NC-SA 3.0](LICENSE), matching the convention of the Enigma2/DreamOS plugin community.
