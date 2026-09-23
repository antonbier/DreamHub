# DreamHub — Project Plan

Full technical context for DreamHub, written for whoever picks up the project next. Read this before writing any code. Then read [`CONCEPT.md`](CONCEPT.md) (the product structure: core, hubs, connectors) and [`ARCHITECTURE.md`](ARCHITECTURE.md) (the actual platform, confirmed via a read-only inspection of a reference device).

## 1. Core insight

This device's one advantage over any general-purpose smart TV platform paired with it is **the tuner**: live TV, EPG, recordings, timers. Every hub in this project earns its place by depending on the tuner or on data only this box has (recordings, SeriesRecorder database) — or by connecting such data with something else.

**What makes DreamHub distinct is integration.** Most individual capabilities already exist somewhere on the platform (DreamEPG Premium, AdvancedMovieSelection, GraphMultiEPG — see `ARCHITECTURE.md`). None of them connects everything in one place: AI recommendation → timer → recording → rename, or one search across EPG, timers, recordings and series. That's the product. See `CONCEPT.md`.

The other driver: the box's native web interface ("WebControl") is functional but dated. Modernizing that experience — without touching the Enigma2 core — is the concrete goal.

## 2. Out of scope

Evaluated and explicitly not part of this project. Don't re-propose these without a new, concrete technical reason:

- Home automation / smart-home device control.
- Generic infrastructure or container monitoring shown on the TV — belongs on a phone/tablet, and any such feature would depend on an external server that isn't guaranteed to be reachable at all times, which breaks the "must work reliably" requirement below.
- ~~Cross-referencing EPG against an external media library~~ — **revised**: now allowed, but only as an *optional* connector (Plex/Jellyfin/Emby, "Later" in `ROADMAP.md`) that caches the last known state and degrades gracefully when the server is off. It must never be something the rest of DreamHub depends on.
- Casting a live channel to another screen — already solved by existing third-party software.
- Photo/media browsing, sports scores, subtitle/translation overlays, parental controls — not part of this project's focus.
- External push notifications in v1 — **revised**: an in-app notifications inbox is part of the core; external channels (push, Telegram, ntfy) come later via AutomationHub.
- Commercial-break auto-detection in recordings — evaluated, not considered feasible on this hardware.
- Live transcoding delegated to an external server ("Space-Saver") — moved out of this project entirely. Solved as a fully separate, independent script with no dependency on DreamHub.

## 3. Confirmed technical facts

Established during planning research — treat as ground truth, don't re-derive:

- **Hardware**: Dreambox One, DreamOS AIO, Amlogic **S922X** (Quad-Core Cortex-A73 + Dual-Core Cortex-A53, Mali-G52 GPU), 2 GB RAM, 16 GB flash. Strong hardware **decoder** (H.265/VP9 up to 4K60) but **no hardware video encoder** — this chip family comes from the playback-only Android-TV-box world, not the broadcast-SoC world.
- **Web interfaces are two different things**: the native **"Dreambox WebControl"** runs on port 80, is closed-source, and is not meant to be extended (license reasons — don't touch it). **OpenWebif** is a different, open-source project; a community port ("OpenWebIf for DreamOS") exists and can run alongside WebControl on port 90, depending on WebControl staying installed.
- **SeriesRecorder** stores its database as **SQLite** at `/etc/enigma2/SerienRecorder.db`, and also has **its own web interface** (`config.plugins.serienRec.enableWebinterface=true`). Prefer its API over reading the database file while SerienRecorder may be writing to it (to be verified on the device).
- **Recording sidecar files — confirmed on-device**: `.ts` recordings have `.ts`, `.ts.meta`, `.ts.ap`, `.ts.sc`, `.ts.cuts`, `.eit`. Some recordings also have `.jpg`/`.backdrop.jpg` cover art from an existing plugin — carry these along on rename too. `.ts.meta` is plain text: line 1 service reference, line 2 title, line 3 title+description+genre, line 4 a unix timestamp. (`.mkv` files found in `/media/hdd/movie/` are **not** recordings — they were copied in manually. DreamHub must leave non-recording files alone.)
- **Single tuner**: the reference device has one tuner (`Si2169D`, DVB-S2/C/T2). Overlapping timers are a common, real conflict.
- **Browser audio codec limitation**: HTML5 `<video>` cannot play **AC3** audio (licensing), and many German broadcasts use AC3. OpenWebif's own "transcoded" streaming path (port 8002) has recurring community-reported bugs ("picture but no sound"). Native apps avoid this because they use their own non-browser decoders (e.g. software AC3 decode), which a browser-confined PWA cannot do without real, CPU-heavy transcoding — and this box has no hardware encoder to lean on (see above).
- **Root/Telnet access** is available on the Dreambox out of the box.
- **No more core firmware (OTA) updates** are expected for this model. This significantly de-risks installing a persistent custom backend: there's no update process to accidentally wipe it. Plugin/skin updates through the Addon Manager only touch their own package's files.
- **Bouquet editing is already solved**: a separate, existing Enigma2 webif plugin ("Bouqueteditor") provides a complete bouquet editing UI (satellite/provider/service browsing, drag-style reordering, markers). Don't reimplement — link to or embed it, optionally re-skin later for visual consistency with DreamHub.
- **Package manager is apt/dpkg**, not opkg — confirmed on-device (`/usr/bin/dpkg`, `/usr/bin/apt`; `opkg` also exists but apt/dpkg is what the Addon Manager's "Packages" UI actually uses). DreamHub's own package must be a `.deb`.
- **ffmpeg is available** via the standard feed (package `ffmpeg` 4.1.9, confirmed present and installed on the reference device). It's a GPL-free build: no `libx264`, `libmp3lame`, or `libvpx`, but the native (non-GPL) `aac`, `ac3`, and `eac3` encoders are present. Fine for the lossless `-c copy` use case in FilmHub, which needs no encoder at all. DreamHub's own `.deb` should still declare `ffmpeg` as a dependency so it's pulled in automatically on any device, not just the reference box it was verified on.
- **`ffmpeg-mp`** (seen in the Addon Manager) is a separate, fully self-contained ffmpeg build bundled under `/data/mediaportal/extra/ffmpeg/` for MediaPortal's own use — not on `PATH`, doesn't replace the system ffmpeg. Useful precedent if DreamHub ever needs GPL codecs the system build lacks: bundle your own binary+libs the same way, don't try to replace the system one. Not needed for anything currently planned.
- Confirmed encoder list includes an `h264_v4l2m2m` entry (V4L2 hardware encoder wrapper) despite the no-hardware-encoder conclusion elsewhere in this doc. This is only a compiled-in wrapper registration, not proof the underlying kernel driver actually supports hardware encode on this SoC — the owner's own testing says hardware transcoding doesn't work in practice. Treat as unresolved/low-priority; would need an actual write-permitted test to confirm either way, not done here since this check was read-only.
- **No hardware transcoding** — confirmed unavailable on this hardware, consistent with the no-hardware-encoder fact above. Any streaming approach must assume software-only, and therefore avoid transcoding entirely (see Phase 5).
- **Enigma2 plugin localization mechanism confirmed**: define `PluginLanguageDomain` / `PluginLanguagePath` in the plugin, call `gettext.bindtextdomain()`, register with `language.addCallback()`; `.po` files compile to `.mo` via `msgfmt` into `locale/<lang>/LC_MESSAGES/<domain>.mo`. Reference implementation: [`opendreambox/enigma2-plugins`](https://github.com/opendreambox/enigma2-plugins) (DreamOS-specific) and [`opendreambox/enigma2-po`](https://github.com/opendreambox/enigma2-po) for real translation files. **Confirmed on-device**: `config.osd.language` is stored as a `<lang>_<COUNTRY>` locale string, e.g. `config.osd.language=de_DE` — not a 3-letter ISO code as might be assumed from generic Enigma2 docs.
## 4. Architecture

- **Three building blocks**: a core (shell), hubs (feature areas), and connectors (data sources, including the AI provider). Hubs only reach data through connectors and only talk to each other through the core. See `CONCEPT.md`.
- **Backend**: Go, statically compiled for ARM64, single binary, port 8080. REST API + WebSockets.
- **Frontend**: responsive web app (Vue.js + Tailwind), embedded into the Go binary via `go:embed`. Not an installable PWA in v1: service workers require HTTPS, and plain `http://<box-ip>:8080` on the LAN isn't a secure context. PWA comes later with HTTPS (e.g. via a reverse proxy).
- **Enigma2 TV client**: a minimal Python plugin, display-only, calling `127.0.0.1:8080` — no business logic on the Enigma2 side. Target **Python 2.7 specifically** — confirmed on-device as the only interpreter present (`python3` doesn't exist on this box; `enigma2` itself is a compiled ARM64 binary, Python 2.7 is just the plugin layer).
- **Distribution target**: an installable `.deb` package via a proper apt feed — not a binary manually placed over Telnet. Persistence risk is already low (see OTA fact above), but proper packaging is still the goal for maintainability and future updates.
- Coexists with native WebControl on port 80; never modifies it.
- Must not depend on any external server that isn't guaranteed to be running — any feature touching one must be an on-demand/batch action, never something the core UI relies on being reachable.

## 5. Requirements

- **Repository language**: everything in this repo — docs, commits, issues, PRs, code comments — is English only. See `CLAUDE.md`.
- **Multilingual product**: the web UI needs a user-switchable language selector (i18n library, e.g. vue-i18n); the Enigma2 plugin must automatically follow DreamOS's own configured system language (`config.osd.language.value`) via gettext, with no separate setting to maintain. This is cross-cutting — set up in the Foundation, used by every hub.
- **No silent changes**: in v1, every change to recordings, timers or the system happens only through the web UI, shows exactly what will change, and requires explicit confirmation. Nothing happens automatically; automation comes later via AutomationHub, opt-in. See `PRINCIPLES.md`.

## 6. Hubs and releases

See [`CONCEPT.md`](CONCEPT.md) for the hub list and release scope (v1 / v1.1 / later) and [`ROADMAP.md`](ROADMAP.md) for the technical notes per hub. Earlier planning organized this as "Phase 0–8" with modules named FilmHub, AIO Search, TV Control, Streaming Bridge, AI Discovery and Plugin & Script Hub; these map to MediaHub, global search, GuideHub/ChannelHub/ControlHub/SystemHub, StreamHub, DiscoveryHub and AutomationHub.

## 7. Current status

Phase 0 spikes are resolved. The backend foundation has started (Go scaffold, API, auth, mock/live WebControl client, tests — see `backend/README.md`). Next: a home page mockup (timeline vs. tiles) to decide the UX direction before building the frontend.

Project board: https://github.com/users/antonbier/projects/1
