# DreamOS AIO — Architecture Reference

Findings from a read-only inspection of a reference Dreambox One (DreamOS AIO / "Gemini Project" GP4 build): the WebControl web interface with its network traffic observed, and direct on-device inspection (SSH, read-only — no writes, no service restarts, no installs). This is the ground truth for how the platform actually works; treat it as more authoritative than anything inferred from general Enigma2/OpenWebif knowledge elsewhere in this repo.

## System identity

- Enigma2 version: `5.0.0r117-gutemine-2024-08-12`
- Image: "Experimental 2023-04-24", webif version `1.9.1`, front-panel version `1.15`
- Device name reported by the API: `one`
- Tuner: `Si2169D` (combo DVB-S2 / DVB-C / DVB-T2)
- This is a **"Gemini Project" (GP4) build** — confirms the "GP4.2" naming used elsewhere; apt feed names below are all `gp4-*`.
- Primary skin in use: `Shadow-WQHD/skin.xml` (`config.skin.primary_skin`)

## Process architecture

- **WebControl is not a separate web server** — port 80 (and 443, and 9190) are owned directly by the `/usr/bin/enigma2` process itself. It embeds its own HTTP server; there is no lighttpd/nginx/apache in front of it.
- Most plugins run **in-process** inside the single `enigma2` Python interpreter — not as separate OS processes. The one exception observed is a Bluetooth plugin (`geminibluetooth`) running as its own Python process, presumably because it needs continuous background handling.
- This is a useful precedent for DreamHub's own architecture: running the Go backend as a **separate OS process** (as planned, port 8080) is not the platform's default pattern for extensions, but it's a legitimate one — better isolation than the in-process norm, at the cost of not being a "plugin" in the traditional sense. No conflict either way since it's a different port.
- Other services running on the box (background, unrelated to DreamHub but useful context for a full network picture): Samba (`smbd`, 445/139 — explains `.DS_Store` files turning up in `/media/hdd/movie/`, the folder has been browsed from a Mac over SMB), NFS (`rpcbind`/`rpc.mountd`/`rpc.statd`, port 2049), **ZeroTier** (`zerotier-one`, ports 9993/29994 — the box is joined to a ZeroTier virtual network, which is why an interface like `zt5u424jk3` with a `169.254.x.x` address shows up in `/web/deviceinfo`), a CAM emulator (`oscam-cycecplus`, ports 17999/8888, for conditional-access/Pay-TV decryption — unrelated to DreamHub), and `shellinaboxd` bound to `127.0.0.1:4200` only (a browser-based shell, likely what WebControl's "Werkzeuge" section uses internally).
- Streaming port `8001` is confirmed listening (inetd-style, owned by `init` rather than a dedicated daemon) — consistent with the OpenWebif streaming-port convention referenced elsewhere in this repo.

## WebControl's `/web/*` API — reuse this, don't reimplement

This is the single most important finding: **WebControl's API is essentially OpenWebif-API-compatible** — same endpoint names, same `e2...` XML response schema that the wider Enigma2 ecosystem already documents. DreamHub's Go backend should call this API as its primary data source instead of scanning the filesystem or parsing internal caches directly, wherever it covers what's needed.

Confirmed endpoints (all `POST`, all return XML), observed from normal WebControl usage:

| Endpoint | Purpose | Notes |
|---|---|---|
| `/web/session` | session handling | |
| `/web/deviceinfo` | device/system info | Returns enigma2/image/webif versions, tuner model, all network interfaces, HDD model/capacity/free space |
| `/web/getcurrlocation`, `/web/getlocations` | recording folder locations | |
| `/web/getservices`, `/web/subservices` | bouquet/service tree | |
| `/web/getcurrent` | now/next EPG for the current service | |
| `/web/epgnownext` | now/next EPG, service list view | |
| `/web/powerstate` | standby/power state | |
| `/web/movielist` | **recordings list, already parsed** — see schema below | |
| `/web/gettags` | recording tags | |
| `/web/timerlist` | timers — this is exactly the endpoint Phase 7's polling approach already assumed | |

`/web/movielist` response schema (confirmed live, one entry):

```xml
<e2movie>
  <e2servicereference>1:0:0:0:0:0:0:0:0:0:/media/hdd/movie/....ts</e2servicereference>
  <e2title>Bauer sucht Frau International</e2title>
  <e2description>Folge 7</e2description>
  <e2descriptionextended>...</e2descriptionextended>
  <e2servicename>RTL Television</e2servicename>
  <e2time>1780944521</e2time>          <!-- unix timestamp -->
  <e2length>103:16</e2length>           <!-- MM:SS -->
  <e2tags></e2tags>
  <e2filename>//media/hdd/movie/....ts</e2filename>
  <e2filesize>3802689536</e2filesize>
  <e2streamserverseek>False</e2streamserverseek>
</e2movie>
```

**Impact on the roadmap**: FilmHub (Phase 2) and AIO Search (Phase 3) should use `/web/movielist`, `/web/timerlist`, `/web/getservices`, `/web/epgnownext` as their primary data source for *listing/searching*. Direct filesystem/`.ts.meta` access is still needed for *file operations* (rename, move, trim), since the API doesn't do that — but no need to parse `.ts.meta` by hand just to build a browsable list.

## The `/bouqueteditor/*` plugin API

The existing "BouquetEditor" plugin (folder: `WebBouquetEditor`) runs under its own path, not inside `/web/`, and exposes its own endpoints, confirmed from live usage:

- `/bouqueteditor/bouquetlist.htm`, `/bouqueteditor/providerlist.htm`, `/bouqueteditor/servicelist.htm`, `/bouqueteditor/providerservicelist.htm` — list views
- `/bouqueteditor/web/getservices?sRef=...` — takes a standard Enigma2 service-reference query string (e.g. `FROM BOUQUET "userbouquet.favourites.tv" ORDER BY bouquet`)

Only read/list calls were observed (no add/move/remove button was clicked, to avoid touching the real channel list). Confirms the Phase 0 decision: **link to or embed this plugin rather than reimplementing bouquet editing.** Its write endpoints (add/reorder/remove) weren't captured and would need a follow-up pass with write permission explicitly granted.

## Config file conventions

`/etc/enigma2/settings` is a flat `key=value` text file. Confirmed keys relevant to DreamHub:

- `config.osd.language=de_DE` — **the locale format is `<lang>_<COUNTRY>` (e.g. `de_DE`), not a 3-letter ISO 639-2 code.** Corrects the earlier assumption elsewhere in this repo — use this exact format when matching gettext locale directories.
- `config.skin.primary_skin=Shadow-WQHD/skin.xml`, `config.Styles.skin=Shadow-WQHD/skin.xml` — confirms the skin name/path convention.

## Already-installed plugins — check before building

`/usr/lib/enigma2/python/Plugins/Extensions/` on the reference box already contains a large set of plugins. Several overlap directly with planned DreamHub modules and **should be evaluated before writing new code that duplicates them**:

| Installed plugin | Overlaps with | Action needed |
|---|---|---|
| `GraphMultiEPG` | Phase 4 Grid-EPG ("TV guide" view) | Investigate — this may already be the timeline grid EPG view DreamHub planned to build. |
| `EPGSearch` | Phase 3 AIO Search | Investigate — may already cover EPG search; DreamHub's search may only need to add recordings/timers on top. |
| `TvSpielfilm` | Phase 4 Grid-EPG online enrichment | Investigate — a German TV-magazine data integration already exists; may overlap with the planned TMDB enrichment. |
| `AdvancedMovieSelection` | Phase 2 FilmHub grid/list view | Investigate — may already provide a better recordings browser than WebControl's default. |
| `CutListEditor` | Phase 2 FilmHub lossless trimmer | Investigate — may already do cut-marker editing; DreamHub might only need to add the `ffmpeg -c copy` execution step. |
| `GPT-Plugin` | Phase 6 AI Discovery | **Investigate first, specifically** — the name suggests an existing LLM integration. If it already does EPG-based AI recommendations, Phase 6 may be redundant or need to build on top of it instead of starting fresh. |
| `PlexDream` | (out of scope — media browsing) | No action — confirms Plex on the box itself is already covered, consistent with the "out of scope" decision. |

Other plugins present, listed for completeness (no direct overlap identified): `2IB`, `AutoTimer`, `Browser`, `ChannelSelectionPlus`, `DreamExplorer`, `DreamMediaInfo`, `EasyInfo`, `EasyMedia`, `Emission`, `EPGRefresh`, `EventDataManager`, `GithubPluginUpdater`, `HbbTV`, `IMDb`, `MediaPortal`, `MediaRenderer`, `MediaScanner`, `MenuSort`, `MetrixStyle`, `PluginAssistance`, `SkinReloader`, `SvenHPlugins`, `WebAdmin`, `WebInterface`, `WetterCom`/`WetterComComponent`, `mpgz`, `serienrecorder` (confirms Phase 3's SQLite assumption).

## apt feed structure (for Phase 8 packaging)

`/etc/apt/sources.list.d/` on the reference box:

- Active: `gp4-unstable-aio-feed.list`, `gp4-unstable-aarch64-feed.list`, `gp4-unstable-all-feed.list`, `gp4-unstable-allcodes-feed.list`, `gp4-unstable-extraPluginsAarch64-feed.list`, `gp4-unstable-nobody-feed.list`, `kodi-aarch64-feed.list`, `mediaportal.list`, `plugins-boxpirates.list`, `svenh-feed.list`
- Disabled (`.off` suffix, not loaded): `aarch64-feed.list.off`, `all-feed.list.off`, `armv7vehf-neon-vfpv4-feed.list.off`, `dreambox-feed.list.off`, `gutemine.list.off`, `mpgz.list.off`

Third-party plugin feeds (e.g. `svenh-feed.list`, `mediaportal.list`, `plugins-boxpirates.list`) each get their own `.list` file in `sources.list.d/`. A DreamHub feed should follow the same convention: e.g. `/etc/apt/sources.list.d/dreamhub-feed.list` pointing at a self-hosted apt repository, with the `.deb` declaring `ffmpeg` as a `Depends` (see `PROJECT_PLAN.md`).

## Open follow-ups

- Write-permitted pass needed to confirm the `/bouqueteditor/` plugin's actual write endpoints (add/reorder/remove service) — not captured here since only read/list actions were performed.
- The `GPT-Plugin` finding above should be investigated before any Phase 6 work starts.
