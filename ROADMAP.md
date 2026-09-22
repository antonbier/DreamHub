# Roadmap

## Phase 0 — Spikes (resolve before building)

- [x] **Bouquet editing** — resolved: don't reimplement. A separate, existing Enigma2 webif plugin ("Bouqueteditor") already provides a complete bouquet editing UI. Link to or embed it; consider re-skinning later.
- [x] **Hardware transcoding** — resolved: confirmed unavailable on this hardware.
- [x] **ffmpeg availability** — resolved: available via the standard feed (package `ffmpeg`). Declare it as a dependency in DreamHub's own `.ipk` rather than assuming it's preinstalled, so it's pulled in on any device.
- [x] **Enigma2 language mechanism** — resolved: `PluginLanguageDomain`/`PluginLanguagePath` + `gettext.bindtextdomain()` + `language.addCallback()`, `.po`→`.mo` via `msgfmt`. Reference: [`opendreambox/enigma2-plugins`](https://github.com/opendreambox/enigma2-plugins), [`opendreambox/enigma2-po`](https://github.com/opendreambox/enigma2-po).
- [ ] **List all recording sidecar files** — still open. Needs read-only `ls -la` on an existing recording under `/media/hdd/movie/` via Telnet/SSH on the actual device — not verifiable remotely.

## Phase 1 — Base Engine

Go backend (port 8080) with a minimal embedded frontend. Filesystem scanner for `/media/hdd/movie/`, parses `.ts.meta`, exposes results via REST API. Also where PWA i18n scaffolding (language selector, translation file structure) gets set up — see multilingual requirement in `PROJECT_PLAN.md`.

## Phase 2 — FilmHub

- Grid/list view for recordings.
- Custom naming engine covering **all** sidecar files (not just `.ts.meta` — see Phase 0).
- Bulk actions via a background queue.
- Lossless trimmer via `ffmpeg -c copy` (note: cuts land on keyframe boundaries, not frame-exact).

## Phase 3 — AIO Universal Search

- Sources: local recordings, timers/AutoTimer, SeriesRecorder database (confirmed SQLite at `/etc/enigma2/SerienRecorder.db`).
- EPG data should **not** come from parsing `epg.dat` directly (undocumented binary format, breaks across firmware updates) — query Enigma2/WebControl's existing local API instead.

## Phase 4 — TV Control & WebAdmin (modernized facade)

- Rebuild existing WebControl functionality (OSD screenshot, system message, power menu, live metrics) with a modern UI — the logic already exists, this is purely a UI upgrade.
- **Grid EPG ("TV guide" view)**: tabular multi-channel overview. Online/AI enrichment (TMDB or similar) **only** for movie/series genres — daily formats (news, magazines) get no useful match.
- **Bouquet editor**: link to/embed the existing "Bouqueteditor" plugin rather than reimplementing (see Phase 0). Re-skin its UI later if needed for visual consistency with the rest of DreamHub.
- The Enigma2 plugin side of this phase must read and follow `config.osd.language.value` — see multilingual requirement.

## Phase 5 — Live-Streaming Bridge

- No software transcoding (the S922X has no hardware encoder — would be CPU-heavy).
- Inline HTML5 player only for channels with AAC-compatible audio.
- For all other channels (AC3 audio): fall back to **"open in VLC"** (`.m3u` link handed off to the native VLC app), since VLC brings its own audio decoders and sidesteps the browser's codec restriction (no AC3 in HTML5 `<video>`).
- No in-page embedding via browser plugin — both the old NPAPI VLC plugin and Chrome's "VXG" are dead/deprecated.

## Phase 6 — AI Discovery

- Send the current day's EPG — filtered to the user's selected subset of channels in Settings, for recommendation relevance, not primarily cost — to an LLM API. Recommendation shown as an OSD on a remote button press.
- Rough cost estimate: ~30 channels × ~20 programs/day × ~150 tokens ≈ 90k tokens/day — cents-level, not a real cost concern. The channel filter exists to improve recommendation quality, not to save money.

## Phase 7 — Plugin & Script Hub

- No deep Enigma2 event hook (too much effort for v1).
- Instead: the Go backend polls `/api/timerlist` every 30–60s and detects state changes itself (e.g. "recording finished") — good enough for script triggers.
- Separation: `scripts/` (simple Bash/Python hooks) vs. `plugins/` (deeply integrated backend routes with their own settings tab).

## Phase 8 — Packaging

Target: an `.ipk` package installable via its own opkg feed, instead of a binary manually placed over Telnet. Risk from missing core firmware updates is already low, but proper packaging remains the goal for maintainability and future updates. Declare real package dependencies (e.g. `ffmpeg`) in the `.ipk` control file rather than assuming they're preinstalled, so DreamHub installs cleanly on any device, not just a reference box that already happens to have them.
