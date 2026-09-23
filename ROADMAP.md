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
- [x] Cross-compiles to a static ARM64 Linux binary (`CGO_ENABLED=0 GOOS=linux GOARCH=arm64`), ~11 MB stripped incl. SQLite
- [x] **3a** SQLite store (`internal/store`, pure-Go driver, migrations via `PRAGMA user_version`, file mode 0600): users with **roles**, per-user and global settings, **multiple dashboards per user** (v1 creates one), change log. Data dir `/data/dreamhub` (live) or `./.dreamhub-data` (mock)
- [x] **3a** Schema-driven settings backend (`internal/settings`): typed fields (string, bool, int, enum, timezone, secret, password), per-field validation codes, cross-field checks (e.g. auth can't be enabled without credentials), all-or-nothing updates recorded in the change log without secret values, secrets masked, passwords stored as bcrypt and never returned. `GET`/`PATCH /api/v1/settings`
- [x] **3a** Auth configured through settings instead of env vars; Basic Auth verified with bcrypt, successful checks cached 10 min (bcrypt is slow on the box's ARM cores), cache invalidated on any auth change
- [x] **3a** `/api/v1/health` reports server time in UTC (for the clock check)

- [x] **3b** Connector registry (`internal/connector`): WebControl (real check via `deviceinfo`), SerienRecorder (OK in mock mode, honestly "not implemented" on a real device until #10), OpenAI and TMDB (own settings sections for their keys; "not configured" until a key is set). Reports cached 30 s, recomputed immediately when settings change; checks time out after 2 s
- [x] **3b** Hub registry (`internal/hub`) with the v1 hub descriptors: name, descriptive label, icon, path, group + order, minimum role, requirements (connector / capability / other hub, required or optional) → status *available / degraded / unavailable* with problem and fix codes; one enable switch per hub in settings. `GET /api/v1/navigation` (sidebar per user: role filter, disabled hubs hidden, empty groups omitted, labels mode), `GET /api/v1/hubs` (diagnostics incl. disabled hubs), `GET /api/v1/connectors`
- [x] **3b** Shared wiring in `internal/app`, used by `cmd/dreamhub` and the API tests alike

**Open**:
- [ ] Remaining parts of the hub contract, added as the hubs are built: routes per hub, search providers, tile types, events
- [ ] **3c** Preview-and-confirm mechanism for every change: `plan` → `apply` with plan ID, written to the change log (see `PRINCIPLES.md`)
- [ ] Core event bus (hub events → notifications, later AutomationHub)
- [ ] API tokens (generate/revoke in settings, stored hashed) — Basic Auth works, bearer tokens are rejected until then
- [ ] Dashboards API (read/save layout) on top of the existing store methods
- [ ] Settings UI rendered from the schema (frontend)
- [ ] Web frontend (Vue.js + Tailwind) embedded via `go:embed`, desktop-first and responsive. Source Sans 3 and all assets bundled (no CDN/Google Fonts at runtime). Dark by default, light/system switchable. Not an installable PWA yet (needs HTTPS, see "Later").
- [ ] Navigation: sidebar generated from hub registry (group + order), collapsible; drawer on mobile; no bottom tab bar; labels switchable between descriptive names and hub names
- [ ] Implement the design foundations from `DESIGN.md` (tokens, components) — the approved mockup is `docs/design/mockup-dashboard-v2.html`
- [ ] Dashboard (tiles, decided in #16): show/hide, drag-and-drop reorder, width 1–4 columns, per-user layout; custom tiles as saved views (source + filters + sort + limit + display) built on hub search providers
- [ ] Global search (`/`, `Cmd/Ctrl+K`), notifications inbox (stored as key + params), status strip, box-unreachable banner, live updates via WebSocket
- [ ] i18n: DE/IT/EN locale files, runtime drop-in locales from `/data/dreamhub/locales/`, English fallback, `Intl` for all formatting and sorting, RTL-ready CSS (logical properties), language switcher
- [ ] Time zones: UTC everywhere internally (store already does this); per-user display zone setting exists (box / browser / fixed, validated IANA names) — display logic and hint are frontend work; tests with fixed clock incl. DST days for `Europe/Rome`
- [ ] **3c** Security: CSRF protection for mutating requests (custom header or token), strict CORS, `Host` header check against DNS rebinding
- [ ] **3c** Request IDs in logs and error responses; runtime-switchable log level
- [ ] Keyboard shortcuts (`/`, `g`+letter, arrows, `Esc`), accessible contrast and labels
- [ ] Onboarding wizard
- [ ] Diagnostics: hub status with reason and fix instructions; box clock check
- [ ] mDNS advertisement via a service file in `/etc/avahi/services/` (`avahi-daemon` already runs and is used this way by an existing companion app — see `ARCHITECTURE.md`)
- [ ] Capture real fixtures (#14): at the next device access, fetch every relevant endpoint directly over SSH/HTTP and save the responses as test fixtures, so development continues offline with real data

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
- For every user: device info, diagnostics page (hub status, clock check), backup/restore of DreamHub's own state (incl. dashboards), change log.

### AdminHub (logs)
- Admin role only; bottom of the navigation next to Settings.
- Logs from journald: DreamHub itself (for debugging), enigma2, other services as needed. Live tail over WebSocket; filters for service, level, time range, hub/connector, text.
- DreamHub log level switchable at runtime; debug reverts automatically.
- Request IDs link UI errors to log entries.
- List of enigma2 crash logs (`/media/hdd/enigma2_crash_*.log`).
- Download for support with secrets removed.

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

### AdminHub service status
- Status of enigma2, dreamhub, Samba and other units; restarting is a change and goes through preview-and-confirm.

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

### AdminHub terminal
- Proxy the existing `shellinaboxd` (`127.0.0.1:4200`, used by WebAdmin at `/webadmin/terminal/`) behind DreamHub's auth and CSRF protection — no terminal of our own.
- Off by default; can only be enabled while auth is on; admin only; password re-entered before opening; warning; session start/end in the change log.

### Other
- Plex / Jellyfin / Emby connectors (optional, cached last-known state, graceful degradation when the server is off)
- More AI providers behind the same interface
- Profile/role UI (multi-user), multiple dashboards UI
- Additional languages, incl. RTL (the CSS is prepared for it)
- Installable PWA — needs HTTPS, e.g. via a reverse proxy

## Open items that need device access

Tracked as GitHub issues with the `device-access` label:
- #10 Verify the SerienRecorder web interface API
- #11 Verify Enigma2 picks up recordings renamed outside Enigma2
- #12 Detect in-progress recording/playback before any file operation
- #13 Resolve the WebControl auth discrepancy (`ARCHITECTURE.md`)
- #14 Capture real API responses as test fixtures (done directly on the device, no script needed)
- #15 Investigate the remaining GP4 plugins (`geminiserver`, `geminicomm`, `geministreamripper`)
- #17 Check where Enigma2 stores its time zone and whether NTP is running
- #18 Check whether WebAdmin's existing terminal (`/webadmin/terminal/`) requires a login — if not, anyone on the LAN/ZeroTier network has a root shell

Design decision #16 (home page): **tiles**, as a customizable dashboard — see `CONCEPT.md`.
