# Roadmap

Organized by the hub structure described in [`CONCEPT.md`](CONCEPT.md). (Earlier versions of this file were organized as Phase 0–8; the technical notes from those phases have been moved into the matching sections below.)

Releases: **Foundation** → **v1** → **v1.1** → **Later**. GitHub milestones mirror these.

## Phase 0 — Spikes (done)

- [x] **Bouquet editing** — don't reimplement. The existing BouquetEditor webif plugin already provides a complete UI and its own API (`/bouqueteditor/*`, see `ARCHITECTURE.md`).
- [x] **Hardware transcoding** — confirmed unavailable on this hardware.
- [x] **ffmpeg availability** — available via the standard feed (package `ffmpeg`; the box uses `apt`/`dpkg`, not opkg). Declare it as a `.deb` dependency rather than assuming it's preinstalled.
- [x] **Enigma2 language mechanism** — `PluginLanguageDomain`/`PluginLanguagePath` + `gettext.bindtextdomain()` + `language.addCallback()`, `.po`→`.mo` via `msgfmt`. Reference: [`opendreambox/enigma2-plugins`](https://github.com/opendreambox/enigma2-plugins), [`opendreambox/enigma2-po`](https://github.com/opendreambox/enigma2-po).
- [x] **Recording sidecar files** — confirmed on-device: `.ts`, `.ts.meta`, `.ts.ap`, `.ts.sc`, `.ts.cuts`, `.eit`.

## Foundation — backend & core shell

**Done** (built and tested against the mock WebControl client, no device access — see `backend/README.md`):
- [x] Go module, `/api/v1` router, JSON error envelope, structured logging (`internal/api`)
- [x] Basic Auth + API token middleware, toggleable (`internal/auth`); `/api/v1/health` stays public so the TV plugin's version check works with auth enabled
- [x] Environment-based config, mock/live mode switch (`internal/config`)
- [x] WebControl client: real HTTP+XML implementation and a mock (fixtures from `ARCHITECTURE.md`) behind one interface
- [x] `/api/v1/health`, `/api/v1/deviceinfo`, `/api/v1/movies`
- [x] Tests run entirely against the mock
- [x] Cross-compiles to a static ARM64 Linux binary (`CGO_ENABLED=0 GOOS=linux GOARCH=arm64`), ~10 MB

**Open**:
- [ ] Hub registry implementing the hub contract (`CONCEPT.md`): routes, settings schema, search provider, home contributions, events, requirements → status
- [ ] Connector registry; WebControl as the first connector
- [ ] Core event bus (hub events → notifications, later AutomationHub)
- [ ] Preview-and-confirm mechanism for every change: `plan` → `apply` with plan ID, change log (see `PRINCIPLES.md`)
- [ ] SQLite store under `/data/dreamhub/` (`internal/store` is scaffolded but empty); user-aware data model from day one (profiles-ready)
- [ ] Schema-driven central settings (API + UI), secrets masked
- [ ] Web frontend (Vue.js + Tailwind) embedded via `go:embed`, i18n scaffolding with language selector. Responsive web app first — an installable PWA needs HTTPS (see "Later").
- [ ] Home page (timeline or tiles — decided from the mockup), global search, notifications inbox, status strip
- [ ] Onboarding wizard
- [ ] Diagnostics: hub status with reason and fix instructions
- [ ] mDNS advertisement via a service file in `/etc/avahi/services/` (`avahi-daemon` already runs and is used this way by an existing companion app — see `ARCHITECTURE.md`)
- [ ] Fixture-capture script: run once with device access, save real responses from every relevant endpoint as test fixtures, so development can continue offline with real data

## v1 — hubs

### MediaHub
- Listing via `/web/movielist` (already-parsed title/description/length/filesize — see `ARCHITECTURE.md`) rather than hand-parsing `.ts.meta`.
- Disk space overview (the reference device's disk is 88 % full — this is a real, recurring pain point).
- Rename engine covering **all** sidecar files of a `.ts` recording: `.ts`, `.ts.meta`, `.ts.ap`, `.ts.sc`, `.ts.cuts`, `.eit`. Plus existing cover art (`.jpg`, `.backdrop.jpg`) — move it along, don't re-fetch from TMDB.
- The scanner must distinguish real recordings from other media in the same folder: `.mkv` files in `/media/hdd/movie/` on the reference device were copied in manually and are **not** Dreambox recordings — leave them alone.
- Every rename/delete goes through preview-and-confirm; delete means move to trash.
- Never touch a file that is currently being recorded or played — check timer/current-service state first.
- Bulk actions via a background queue.
- `.ts.meta` format: line 1 service reference, line 2 title, line 3 title+description+genre, line 4 unix timestamp — parse only where the API doesn't already provide the field.
- Reference implementation: `AdvancedMovieSelection` (`Rename.py`, `SearchTMDb*.py`) — native on-screen plugin, not web-based, CC BY-NC-SA licensed: read for approach, don't copy code.
- Open (device access needed): does Enigma2's movie list pick up files renamed outside Enigma2 correctly? Probably yes (AdvancedMovieSelection reads the disk directly), but verify.

### TimerHub
- Timers via `/web/timerlist`; AutoTimer.
- **Conflict detection is central**: the reference device has a single tuner, so overlapping timers are common.
- Creating/deleting timers goes through preview-and-confirm, including when triggered from DiscoveryHub.

### GuideHub
- Grid EPG ("TV-Zeitschrift") for the web UI.
- EPG via `/web/epgnownext` / `/web/getcurrent`, **never** by parsing `epg.dat` (undocumented binary format).
- Online enrichment (TMDB) **only** for movie/series genres.
- `GraphMultiEPG` and `TvSpielfilm` are native on-screen references, not replacements.

### DiscoveryHub
- AI recommendations from the day's EPG, filtered to user-selected channels (for relevance; ~90k tokens/day for 30 channels is cents-level).
- Uses the core AI connector — OpenAI first, swappable (see `PRINCIPLES.md`).
- The AI only proposes. "Create timer" hands off to TimerHub with preview and confirmation.
- No existing AI plugin on the box (the "GPT-Plugin" is a GUID Partition Table tool).

### SeriesHub
- Episodes on disk, missing episodes, upcoming episodes for series tracked in SerienRecorder.
- Use SerienRecorder's own web interface API (`config.plugins.serienRec.enableWebinterface=true`) rather than reading `SerienRecorder.db` directly while SerienRecorder may be writing to it. API needs verification on the device.

### SystemHub (basic)
- Device info, diagnostics page (hub status), backup/restore of DreamHub's own state, change log, logs (journald).

### Global search
- Every hub contributes a search provider; results grouped by hub. `EPGSearch` (native on-screen) is a reference for search/filter logic.

### Enigma2 TV plugin (launcher)
- QR code to open DreamHub on a phone, backend status/version (via `/api/v1/health`), basic settings (auth on/off, port).
- Python 2.7 (the only interpreter on the device). Language follows `config.osd.language` (format `de_DE`).

## v1 — release & packaging

- An installable **`.deb`** via DreamHub's own apt feed. The box uses `apt`/`dpkg`; users can add a feed through WebAdmin's "Upload new *-feed.list" page — no SSH required (see `ARCHITECTURE.md`). Platform feeds use `[trusted=yes]` (unsigned).
- Declare real dependencies (e.g. `ffmpeg`) in the `.deb` control file.
- The box runs full systemd: ship the backend as `dreamhub.service` (`Restart=on-failure`, `WantedBy=multi-user.target`). Use `/lib/systemd/system/enigma2.service` as the reference (see `ARCHITECTURE.md`).
- Keep the installed footprint on the 2 GB root partition small; persistent data goes to `/data/dreamhub/`.
- If GPL codecs are ever needed: bundle a self-contained ffmpeg like `ffmpeg-mp` does under `/data/mediaportal/extra/ffmpeg/`, never replace the system one.

## v1.1

### RadioHub
- Radio services, now playing, radio timers. Browser playback belongs in StreamHub (radio often uses MP2 audio, which browsers don't play reliably).

### ChannelHub
- Link to / embed the BouquetEditor plugin (`/bouqueteditor/*`); its write endpoints still need a write-permitted check on the device. Picons from `/data/piconpool`.

### ControlHub
- Web remote, message to TV (`/web/message`), OSD screenshot, power menu — the logic already exists in WebControl, this is a UI layer.

### MediaHub trimming
- Lossless trimming via `ffmpeg -c copy` (cuts land on keyframe boundaries). `CutListEditor` is the on-screen reference for the `.ts.cuts` mechanism.

## Later

### StreamHub
- No software transcoding (no hardware encoder; CPU transcoding is too heavy).
- Browsers don't play raw MPEG-TS natively — needs a library (e.g. mpegts.js) or a remux to HLS/fMP4. On top of that: AC3 audio isn't playable in HTML5 `<video>`, and some channels use HEVC, which browser support covers only partially.
- Fallback for anything the browser can't play: "open in VLC" (`.m3u` link to the native app).
- No browser-plugin embedding (NPAPI VLC and Chrome's "VXG" are dead).

### AutomationHub
- The only place where anything happens automatically — always opt-in.
- Scripts, webhooks, external notification channels (push, Telegram, ntfy, message on the TV).
- Detect state changes by polling `/web/timerlist` every 30–60 s instead of a deep Enigma2 hook.
- Separation: simple `scripts/` hooks vs. deeply integrated `plugins/` with their own settings.
- WebAdmin already has a "User Scripts" feature (`/webadmin/web/scriptlist`) — decide whether to complement or supersede it.

### Other
- Plex / Jellyfin / Emby connectors (optional, cached last-known state, graceful degradation when the server is off)
- More AI providers behind the same interface
- Profile UI (multi-user)
- Installable PWA — needs HTTPS, e.g. via a reverse proxy

## Open items that need device access

Tracked as GitHub issues with the `device-access` label:
- #10 Verify the SerienRecorder web interface API
- #11 Verify Enigma2 picks up recordings renamed outside Enigma2
- #12 Detect in-progress recording/playback before any file operation
- #13 Resolve the WebControl auth discrepancy (`ARCHITECTURE.md`)
- #14 Write and run the fixture-capture script
- #15 Investigate the remaining GP4 plugins (`geminiserver`, `geminicomm`, `geministreamripper`)

Design decision: #16 home page layout (timeline vs. tiles).
