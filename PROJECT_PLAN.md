# DreamHub — Project Plan

This document is the full context behind DreamHub, written for whoever (human or AI agent) picks up the project next. It was produced from a long planning conversation and is meant to stand on its own — read this before writing any code.

## 1. Core insight

The owner has a Dreambox One (DreamOS AIO) alongside a Sony Bravia running Android TV. Anything "app-like" — photos, music, generic dashboards — Android TV already does better, because those apps are more polished there than anything buildable for Enigma2. The only thing the Dreambox has that Android TV doesn't is **the tuner**: live TV, EPG, recording, timers. Every module in this project earns its place by depending on the tuner or on data only the box has (recordings, SeriesRecorder database) — not by re-implementing something a phone, tablet, or the TV's own apps already do well.

The other running theme: the box's own web interface ("WebControl") is functional but dated, and that's the concrete, felt pain point driving most of this project.

## 2. Ideas considered and rejected

These came up during planning and were explicitly turned down. Don't re-propose them without a new, concrete reason — they weren't overlooked, they were evaluated.

| Idea | Why it was rejected |
|---|---|
| Home Assistant integration (lights, heating, doorbell overlay) | Not used — no Home Assistant setup exists. |
| Sports live-score overlay | Not a sports fan. |
| Prometheus/Grafana monitoring dashboard for the box | Grafana isn't used. |
| Kids' parental control / screen-time limiter | Not currently relevant — kids only watch TV supervised. |
| Live subtitle/translation overlay for foreign channels | Only watches German TV; spouse dislikes subtitles. |
| Family photo screensaver (Immich) on the TV | No perceived value, and the native Immich Android TV app already does this better on the same physical screen. |
| Generic Docker/homelab status dashboard on the TV | Monitoring belongs on phone/tablet, not the TV. Also architecturally broken: it would depend on the Unraid server being reachable, and it isn't on 24/7. |
| EPG/library cross-reference ("have I already got this on Plex", "am I missing an episode") against the Arr-stack | Same Unraid-uptime problem as above — the dependency makes it unreliable by design. |
| Casting a live channel to another screen in the house | Already solved well by the DreamEPG Premium app. |
| Homarr / Plex / Immich style content browsing on the Dreambox itself | Same core insight as above — Android TV's native apps for these are faster and nicer on the same TV. |
| Ad/commercial-break auto-detection in recordings | Ruled not feasible on this hardware by the owner. |
| Push notifications to phone for box events | Rarely useful in practice, and a separate existing "Gemini" automation already covers similar ground. |
| Server-side live transcoding delegated to Unraid ("Space-Saver") | Reworked, not simply cut — see below. |

**AI-based EPG recommendations were *initially* rejected** ("SeriesRecorder already covers what I need") but later reconsidered and accepted — it's in scope as Phase 6, with a scope adjustment (see below) to address the original objection.

**"Space-Saver"** (offload recording transcode jobs to the Unraid server) is explicitly **out of DreamHub's scope**, for the same Unraid-uptime reason as the dashboard ideas. It will be solved separately and independently: a standalone script that re-downloads a properly encoded release via the existing Sonarr/Radarr ("Arr") stack whenever the server happens to be up. It has no dependency on DreamHub and shouldn't be built as part of it.

## 3. Confirmed technical facts

Established during planning research — treat as ground truth, don't re-derive:

- **Hardware**: Dreambox One, DreamOS AIO, Amlogic **S922X** (Quad-Core Cortex-A73 + Dual-Core Cortex-A53, Mali-G52 GPU), 2 GB RAM, 16 GB flash. Strong hardware **decoder** (H.265/VP9 up to 4K60) but **no hardware video encoder** — this chip family comes from the playback-only Android-TV-box world, not the broadcast-SoC world.
- **Web interfaces are two different things**: the native **"Dreambox WebControl"** runs on port 80, is closed-source, and is not meant to be extended (license reasons — don't touch it). **OpenWebif** is a different, open-source project; a community port ("OpenWebIf for DreamOS") exists and can run alongside WebControl on port 90, depending on WebControl staying installed.
- **SeriesRecorder** stores its database as **SQLite** at `/etc/enigma2/SerienRecorder.db` — directly queryable, no need to reverse-engineer a format.
- **Recording sidecar files**: an Enigma2 recording is `.ts` plus `.ts.meta`, `.ts.ap`, `.ts.sc`, `.ts.cuts`, possibly `.eit`. Any rename/organize logic must move *all* of these together or Enigma2 loses cut markers/bookmarks for that recording. The exact current set should be verified on-device (see spike issue).
- **Browser audio codec limitation**: HTML5 `<video>` cannot play **AC3** audio (licensing), and many German broadcasts use AC3. OpenWebif's own "transcoded" streaming path (port 8002) has recurring community-reported bugs ("picture but no sound"). Native apps (DreamEPG Premium, VLC) avoid this because they use their own non-browser decoders (e.g. software AC3 decode), which a browser-confined PWA cannot do without real, CPU-heavy transcoding — and this box has no hardware encoder to lean on (see above).
- **Root/Telnet access** is available on the Dreambox out of the box.
- **No more core firmware (OTA) updates** are expected — the manufacturer is insolvent. This significantly de-risks installing a persistent custom backend: there's no update process to accidentally wipe it. Plugin/skin updates through the Addon Manager only touch their own package's files.
- **Bouquet files** are plain text in Enigma2-specific syntax. WebControl likely edits them through its own internal (undocumented) local API — unverified whether that's reusable (spike issue).
- **ffmpeg availability** on the box is unverified (spike issue).
- **Hardware transcoding plugin availability** for this specific model is unverified, but doubted given the no-hardware-encoder fact above (spike issue).

## 4. Architecture

- **Backend**: Go, statically compiled for ARM64, single binary, port 8080. REST API + WebSockets.
- **Frontend**: PWA (Vue.js + Tailwind), embedded into the Go binary via `go:embed`.
- **Enigma2 TV client**: a minimal Python plugin (Py2/Py3-compatible), display-only, calling `127.0.0.1:8080` — no business logic on the Enigma2 side.
- **Distribution target**: an installable `.ipk` package via a proper opkg feed — not a binary manually placed over Telnet. Persistence risk is already low (see OTA fact above), but proper packaging is still the goal for maintainability and future updates.
- Coexists with native WebControl on port 80; never modifies it.

## 5. New requirements (added after initial planning)

- **Repository language**: everything in this repo — docs, commits, issues, PRs, code comments — is English only, regardless of what language planning happened in. See `CLAUDE.md`.
- **Multilingual product**: the web UI needs a user-switchable language selector (i18n library, e.g. vue-i18n); the Enigma2 plugin must automatically follow DreamOS's own configured system language (`config.osd.language.value`) via gettext, with no separate setting to maintain. This is cross-cutting — touches Phase 1 (i18n scaffolding) and the Enigma2 plugin work specifically.

## 6. Modules / phases

See [`ROADMAP.md`](ROADMAP.md) for the full phase-by-phase breakdown (Phase 0 spikes through Phase 8 packaging), including the specific scope adjustments made during review:

- FilmHub's rename engine must cover all sidecar files, not just `.ts.meta`.
- AIO Search should call Enigma2/WebControl's existing local API for EPG rather than parsing the internal binary cache.
- AI Discovery (Phase 6) sends only the user's selected subset of channels, not the full EPG of every bouquet entry — for recommendation relevance, not primarily cost (a full day's EPG across ~30 channels is roughly 90k tokens, i.e. cents, not a real cost concern).
- TV Control's Grid-EPG only gets TMDB/online enrichment for movie/series genres — daily news/magazine formats have no useful match there.
- The Streaming Bridge does no software transcoding; it offers an inline HTML5 player only for channels with AAC-compatible audio, and a "open in VLC" handoff (`.m3u` link to the native app) for everything else — sidestepping the browser AC3 limitation entirely instead of fighting it.
- The Plugin & Script Hub uses polling (`/api/timerlist` every 30–60s) instead of a deep Enigma2 event hook, which would need real Enigma2 Python plugin development effort disproportionate to the benefit for v1.

## 7. Current status

Planning only. Nothing has been built yet. Phase 0 spikes (see GitHub Issues, milestone "Phase 0 — Spikes") must be answered before starting Phase 1:

1. Does WebControl's bouquet editor expose a reusable local API?
2. Is any hardware transcoding plugin even offered for this model in the Addon Manager?
3. Is ffmpeg available on-device, or does it need to be provided separately?
4. What is the complete, current set of recording sidecar files?
5. What's the correct gettext/`config.osd.language` loading convention for the Enigma2 plugin's multilingual support?

Project board: https://github.com/users/antonbier/projects/1
