# Roadmap

## Phase 0 — Spikes (resolve before building)

These questions decide whether specific modules are even feasible and should be answered first.

- [ ] **Verify bouquet editor API** — open WebControl's bouquet editor in a browser, watch the Network tab in devtools while moving/saving. Is there a reusable local endpoint (port 80), or does DreamHub need to parse/write bouquet files itself?
- [ ] **Verify hardware transcoding** — check the box's Addon/Plugin Manager: is a "Transcoding" plugin even offered for the Dreambox One? (The S922X is a decode-only chip per its datasheet — likely no.)
- [ ] **Verify ffmpeg availability** — is ffmpeg preinstalled on the box, or does it need to be provided separately for ARM64/DreamOS (needed for FilmHub's lossless trimmer)?
- [ ] **List all recording sidecar files** — confirm `.ts`, `.ts.meta`, `.ts.ap`, `.ts.sc`, `.ts.cuts`, possibly `.eit` against the current firmware version, so the rename engine doesn't miss any.
- [ ] **Verify Enigma2 language mechanism** — confirm reading `config.osd.language.value` and the correct gettext `.po`/`.mo` loading convention other Enigma2 plugins use, for the plugin's multilingual support.

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
- **Bouquet editor**: implementation depends on the Phase 0 spike result.
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

Target: an `.ipk` package installable via its own opkg feed, instead of a binary manually placed over Telnet. Risk from missing core firmware updates is already low (manufacturer insolvent), but proper packaging remains the goal for maintainability and future updates.
