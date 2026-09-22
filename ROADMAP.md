# Roadmap

## Phase 0 — Spikes (resolve before building)

- [x] **Bouquet editing** — resolved: don't reimplement. A separate, existing Enigma2 webif plugin ("Bouqueteditor") already provides a complete bouquet editing UI. Link to or embed it; consider re-skinning later.
- [x] **Hardware transcoding** — resolved: confirmed unavailable on this hardware.
- [x] **ffmpeg availability** — resolved: available via the standard feed (package `ffmpeg`, apt/dpkg-based — this DreamOS build uses `apt`/`dpkg`, not opkg, confirmed on-device). Declare it as a dependency in DreamHub's own `.deb` rather than assuming it's preinstalled, so it's pulled in on any device.
- [x] **Enigma2 language mechanism** — resolved: `PluginLanguageDomain`/`PluginLanguagePath` + `gettext.bindtextdomain()` + `language.addCallback()`, `.po`→`.mo` via `msgfmt`. Reference: [`opendreambox/enigma2-plugins`](https://github.com/opendreambox/enigma2-plugins), [`opendreambox/enigma2-po`](https://github.com/opendreambox/enigma2-po).
- [ ] **List all recording sidecar files** — still open. Needs read-only `ls -la` on an existing recording under `/media/hdd/movie/` via Telnet/SSH on the actual device — not verifiable remotely.

## Phase 1 — Base Engine

Go backend (port 8080) with a minimal embedded frontend. Filesystem scanner for `/media/hdd/movie/`, parses `.ts.meta`, exposes results via REST API. Also where PWA i18n scaffolding (language selector, translation file structure) gets set up — see multilingual requirement in `PROJECT_PLAN.md`.

Advertise the backend via mDNS/Bonjour (drop a service file into `/etc/avahi/services/`, `avahi-daemon` is already running and already used this way by an existing companion app — see `ARCHITECTURE.md`) so a phone/browser can find it without a manual IP entry.

## Phase 2 — FilmHub

- Both `AdvancedMovieSelection` and `CutListEditor` are already installed and mature, but are **native on-screen (remote-control) plugins, not web-based** — they don't replace FilmHub's web UI, which is the actual point of this phase. Read `AdvancedMovieSelection`'s `Rename.py`/`SearchTMDb*.py` as a reference for the naming/TMDB-matching logic before writing DreamHub's own (note its CC BY-NC-SA license — reference only, don't copy the code directly). See `ARCHITECTURE.md`.
- Listing should use `/web/movielist` (already-parsed title/description/length/filesize — see `ARCHITECTURE.md`) rather than hand-parsing `.ts.meta` for every recording.
- Custom naming engine covering **all** sidecar files for actual `.ts`-based recordings: `.ts`, `.ts.meta`, `.ts.ap`, `.ts.sc`, `.ts.cuts`, `.eit` — confirmed on-device. (Note: any `.mkv` files sitting in `/media/hdd/movie/` are **not** Dreambox recordings — they were manually copied in separately. The scanner must distinguish real recordings from arbitrary other media sitting in the same folder, and only manage the former.)
- Some recordings already have `.jpg`/`.basename.backdrop.jpg` cover art next to them (fetched by an existing plugin) — reuse/move these on rename too, don't assume they need re-fetching from TMDB.
- Bulk actions via a background queue.
- Lossless trimmer via `ffmpeg -c copy` (note: cuts land on keyframe boundaries, not frame-exact).
- `.ts.meta` is a plain-text file: line 1 service reference, line 2 title, line 3 title+description+genre, line 4 a unix timestamp — parse from here only where the API doesn't already provide the field.

## Phase 3 — AIO Universal Search

- `EPGSearch` is already installed but is a **native on-screen plugin, not web-based** — worth reading as a reference for search/filter logic, doesn't replace this phase (see `ARCHITECTURE.md`).
- Sources: `/web/movielist`, `/web/timerlist`, SeriesRecorder database (confirmed SQLite at `/etc/enigma2/SerienRecorder.db`).
- EPG data should **not** come from parsing `epg.dat` directly (undocumented binary format, breaks across firmware updates) — use `/web/epgnownext`/`/web/getcurrent` (confirmed working, see `ARCHITECTURE.md`) instead.

## Phase 4 — TV Control & WebAdmin (modernized facade)

- Rebuild existing WebControl functionality (OSD screenshot, system message, power menu, live metrics) with a modern UI — the logic already exists, this is purely a UI upgrade.
- `GraphMultiEPG` (timeline grid) and `TvSpielfilm` (enriched program guide) are already installed but are **native on-screen plugins, not web-based** — they're a different surface (TV + remote) than DreamHub's browser/phone-based Grid-EPG, so this phase still has a real reason to exist. Worth reading their source as implementation references first (see `ARCHITECTURE.md`).
- **Grid EPG ("TV guide" view)**: tabular multi-channel overview, for the web UI. Online/AI enrichment (TMDB or similar) **only** for movie/series genres — daily formats (news, magazines) get no useful match.
- **Bouquet editor**: link to/embed the existing "Bouqueteditor" plugin rather than reimplementing (confirmed API surface in `ARCHITECTURE.md`). Re-skin its UI later if needed for visual consistency with the rest of DreamHub.
- The Enigma2 plugin side of this phase must read and follow `config.osd.language` (confirmed format: `de_DE`-style locale, e.g. `config.osd.language=de_DE` — see `ARCHITECTURE.md`) — see multilingual requirement.

## Phase 5 — Live-Streaming Bridge

- No software transcoding (the S922X has no hardware encoder — would be CPU-heavy).
- Inline HTML5 player only for channels with AAC-compatible audio.
- For all other channels (AC3 audio): fall back to **"open in VLC"** (`.m3u` link handed off to the native VLC app), since VLC brings its own audio decoders and sidesteps the browser's codec restriction (no AC3 in HTML5 `<video>`).
- No in-page embedding via browser plugin — both the old NPAPI VLC plugin and Chrome's "VXG" are dead/deprecated.

## Phase 6 — AI Discovery

- Resolved: `GPT-Plugin` was investigated and is unrelated — "GPT" stands for GUID Partition Table (a firmware backup/flash tool), not AI. There is no existing AI/EPG plugin on this box; this phase is genuinely new ground (see `ARCHITECTURE.md`).
- Send the current day's EPG — filtered to the user's selected subset of channels in Settings, for recommendation relevance, not primarily cost — to an LLM API. Recommendation shown as an OSD on a remote button press.
- Rough cost estimate: ~30 channels × ~20 programs/day × ~150 tokens ≈ 90k tokens/day — cents-level, not a real cost concern. The channel filter exists to improve recommendation quality, not to save money.

## Phase 7 — Plugin & Script Hub

- No deep Enigma2 event hook (too much effort for v1).
- Instead: the Go backend polls `/api/timerlist` every 30–60s and detects state changes itself (e.g. "recording finished") — good enough for script triggers.
- Separation: `scripts/` (simple Bash/Python hooks) vs. `plugins/` (deeply integrated backend routes with their own settings tab).

## Phase 8 — Packaging

Target: an installable **`.deb`** package via its own apt feed/repository, instead of a binary manually placed over Telnet. This DreamOS build uses `apt`/`dpkg` (confirmed on-device — the "Packages" UI in the Addon Manager is apt-backed), not opkg. Risk from missing core firmware updates is already low, but proper packaging remains the goal for maintainability and future updates. Declare real package dependencies (e.g. `ffmpeg`) in the `.deb` control file rather than assuming they're preinstalled, so DreamHub installs cleanly on any device, not just a reference box that already happens to have them.

The box runs **full systemd** (confirmed — `enigma2` itself runs as a `.service` unit, see `ARCHITECTURE.md`). Ship the Go backend as a `dreamhub.service` unit (`Restart=on-failure`, started via `WantedBy=multi-user.target`) inside the `.deb`, the same standard mechanism the platform already uses for its own core process — not a custom autostart script.

If DreamHub ever needs GPL-licensed codecs (e.g. H.264 encoding via libx264) that the system `ffmpeg` doesn't have — see confirmed facts below — the existing `ffmpeg-mp` package on this box is a useful precedent: it ships its own fully self-contained ffmpeg build with its own libs under `/data/mediaportal/extra/ffmpeg/`, entirely separate from the system binary. Bundle rather than replace, the same way, if this is ever needed.
