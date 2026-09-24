# DreamOS AIO — Architecture Reference

Findings from a read-only inspection of a reference Dreambox One (DreamOS AIO / "Gemini Project" GP4 build): the WebControl web interface with its network traffic observed, and direct on-device inspection (SSH, read-only — no writes, no service restarts, no installs). This is the ground truth for how the platform actually works; treat it as more authoritative than anything inferred from general Enigma2/OpenWebif knowledge elsewhere in this repo.

> **Naming note.** This document was written while the roadmap was still organized as "Phase 0–8". The product has since been restructured into hubs (see `CONCEPT.md`). Mapping of the old names used below: Phase 1 → Foundation · Phase 2 / FilmHub → MediaHub · Phase 3 / AIO Search → global search · Phase 4 / TV Control → GuideHub, ChannelHub, ControlHub, SystemHub · Phase 5 / Streaming Bridge → StreamHub · Phase 6 / AI Discovery → DiscoveryHub · Phase 7 / Plugin & Script Hub → AutomationHub · Phase 8 → v1 release & packaging. The findings themselves are unaffected.

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
| `/web/epgnownext` | now/next EPG, service list view | takes `bRef` (bouquet reference), not `sRef` — see below |
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

**Impact on the roadmap**: FilmHub (Phase 2) and AIO Search (Phase 3) should use `/web/movielist`, `/web/timerlist`, `/web/getservices`, `/web/epgnownext` as their primary data source for *listing/searching*. Direct filesystem/`.ts.meta` access is still needed for *renaming* and *trimming*, since the API doesn't do that (it does have `moviedelete`/`moviemove`, but neither fits DreamHub's rules — see "Recording file operations in WebControl" below) — but no need to parse `.ts.meta` by hand just to build a browsable list.

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

`/usr/lib/enigma2/python/Plugins/Extensions/` on the reference box already contains a large set of plugins. Several overlap directly with planned DreamHub modules and **should be evaluated before writing new code that duplicates them**. (Resolved further down in "The other 'overlapping' plugins are native GUI, not web" and "'GPT-Plugin' is not an AI plugin" — the "Action needed" column below reflects the state before that investigation.)

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

## Runtime: systemd, not a bare embedded init

Corrects an earlier assumption in this repo. PID 1 is `/lib/systemd/systemd` — **this box runs full systemd**, not a bare busybox-init/inittab setup as typically assumed for embedded receivers. `enigma2` itself runs as a proper unit:

```ini
# /lib/systemd/system/enigma2.service
[Unit]
Description=enigma2
After=enigma2-environment.service sockets.target
Requires=enigma2-environment.service sockets.target
DefaultDependencies=no

[Service]
EnvironmentFile=/run/enigma2/environment
EnvironmentFile=-/etc/default/enigma2
ExecStart=/usr/bin/enigma2
ExecStopPost=/usr/bin/enigma2-system-state
Restart=on-failure
RestartSec=0
Type=notify

[Install]
WantedBy=graphical.target
```

**Impact on Phase 8 (packaging)**: DreamHub's backend should ship as a proper `dreamhub.service` systemd unit in its `.deb` (`Restart=on-failure`, `WantedBy=multi-user.target` or similar), not a hand-rolled autostart script. This is the standard, reliable persistence mechanism the platform already uses for its own core process — no need to invent anything.

## Runtime: Python 2.7 only

`python --version` → `Python 2.7.13`. **`python3` is not present on this box at all.** The `/usr/bin/enigma2` binary itself is a compiled ARM64 ELF executable (the C++ core); Python 2.7 is the plugin scripting layer. Drop the "Py2/Py3-compatible" hedge used elsewhere in this repo for the Enigma2 plugin — **target Python 2.7 specifically**, it's the only interpreter actually present.

## Filesystem layout notes

- **Picon (channel logo) storage**: `/data/piconpool` — standard PNG files, useful for showing channel logos in DreamHub's UI without needing to source/cache them separately.
- `/media/hdd/` top level (beyond `movie/`): `SR_Backup` and `SR_Cover` (SeriesRecorder's own backup and cover-art cache — likely where some of the `.jpg` cover files found earlier actually originate from), firmware backup archives created by the GPT-Plugin (see below), a `mediaportal/` data folder, and an existing network mount to external NAS storage under `unraidnas/`.
- Cron infrastructure exists (`busybox-cron`, `/etc/cron.daily/`, `/etc/cron/crontabs`) — available as an OS-level scheduling option for Phase 7, as an alternative/complement to the Go backend's own polling loop.
- SSH is served by **dropbear**, not OpenSSH — a lightweight embedded SSH daemon; noted in case key-format quirks ever matter for tooling.

## "GPT-Plugin" is not an AI plugin — false lead, resolved

Read the actual source (`plugin.py`, `GPT_Setup.py`). **"GPT" here stands for GUID Partition Table**, not Generative Pre-trained Transformer. This is a firmware backup/flashing utility: method names include `showFlashDevices`, `showBackupDevices`, `backupFlash`, `flashSD_Card`, `flashBootDevice`, `checkFlashBooted`, `BoxReboot`. It manages flashing/backing up the box's boot partitions to SD/USB — completely unrelated to EPG or AI. **There is no existing AI/LLM plugin on this box.** Phase 6 (AI Discovery) is genuinely new ground, not overlapping with anything installed.

## The other "overlapping" plugins are native GUI, not web — different surface than DreamHub

Read the source structure of all four. Important correction to the earlier framing (issues #7–#9): **all of them are native, remote-control-driven, on-TV-screen plugins** — none expose a web/HTTP interface the way `WebBouquetEditor` or `WebAdmin` do. So they don't make the corresponding DreamHub module redundant; they solve the same underlying problem for a different surface (the TV + remote, not a browser/phone). This validates the "different approach" intuition — DreamHub's actual differentiator (browser-based, phone/tablet-usable control) isn't replicated by any of them. That said, each is a mature, working reference implementation worth reading before writing new logic from scratch:

- **`GraphMultiEPG`**: the classic on-screen timeline grid EPG (`GraphMultiEpg.py`, uses `BouquetSelector`). Reference for the grid logic; DreamHub's Grid-EPG is still a new web build.
- **`EPGSearch`**: on-screen EPG search, invoked via a key shortcut inside EPG screens, integrates with AutoTimer. Reference for search/filter logic.
- **`TvSpielfilm`**: on-screen program guide with its own downloader pulling data from the TV Spielfilm service (`Downloader.py`). Confirms plugins can and do reach out to external APIs from the box — useful precedent for Phase 6's LLM calls.
- **`CutListEditor`**: on-screen cut-marker editor built on the standard `InfoBarCueSheetSupport`/`ServicePositionGauge` components — confirms the standard mechanism for editing `.ts.cuts` files.
- **`AdvancedMovieSelection`**: by far the most extensive — `SearchTMDb.py`, `SearchTMDbSeries.py`, `SearchTVDb.py`, `Rename.py`, `MoveCopy.py`, `TagEditor.py`, `Wastebasket.py`, `DownloadMovies.py`, `MoviePlayer.py`, `FileBrowser.py`, and more. It already does almost everything planned for FilmHub — TMDB/TVDB matching, renaming, moving, tagging, trash — just as an on-screen plugin. **Excellent reference implementation** for the naming/TMDB-matching engine specifically. **License note**: it's released under Creative Commons **Attribution-NonCommercial-ShareAlike 3.0** — fine to learn from and reference its approach, but its actual code cannot be copied into DreamHub (a different license) without complying with that license (attribution, non-commercial, share-alike).

## WebControl plugin namespacing pattern

Confirmed generalizes beyond `/bouqueteditor/`: **`DreamNetcast` runs under its own `/netcast/` path** the same way. `/web/external` is the endpoint WebControl itself calls to enumerate registered web plugins (populates the "WebPlugins" sidebar section). Any future DreamHub-adjacent web plugin should expect to register itself the same way if it wants to appear inside WebControl's own navigation — not required for DreamHub itself, which runs standalone on port 8080, but useful to know the convention exists.

## Standout finding: triple-slot rootfs, not just A/B

`lsblk`/`blkid` reveal the eMMC (`mmcblk0`, 14.6 GB) is partitioned into **three independent, fully labeled root filesystems**, not the usual single rootfs:

```
mmcblk0p5   2G  ext4  PARTLABEL="dreambox-rootfs"    <- currently booted (root=/dev/mmcblk0p5 in /proc/cmdline)
mmcblk0p6   2G  ext4  PARTLABEL="dreambox-rootfs1"    <- alternate slot, unmounted
mmcblk0p7   2G  ext4  PARTLABEL="dreambox-rootfs2"    <- alternate slot, unmounted
mmcblk0p4 7.8G  ext4  mounted at /data (lazy-mounted via autofs)
mmcblk0p1-3     ~250M total, unidentified (likely bootloader/recovery)
mmcblk0boot0/boot1/rpmb   hardware eMMC boot + RPMB areas (secure boot infra, not further inspected)
```

This is exactly what the `GPT-Plugin` (see below — genuinely a **GUID Partition Table** tool, not AI) is built to manage: `flashBootDevice`, `change_boot`, `checkFlashBooted` all operate on this scheme. In effect the box has **three swappable full-OS slots** — flash a new/updated image to a spare slot, boot into it to test, and fall back instantly by switching the active slot if something's broken, all without any network OTA mechanism. This is a more resilient, enthusiast-friendly design than a typical single-rootfs appliance, and is probably the practical reason a dead upstream (no more vendor OTA) is survivable at all for this device: the community can still hand-flash new images to a spare slot via GPT-Plugin. Not directly relevant to what DreamHub builds, but explains a lot about how this platform is actually maintained.

`/data` (`mmcblk0p4`, 7.8 GB, separate from the 2 GB root) is where persistent application data lives (`/data/piconpool`, `/data/mediaportal`, `/data/GPT` backups, etc.) — it's lazy-mounted via `autofs` on first access, not mounted unconditionally at boot. **Implication for DreamHub**: root is small (2 GB) and shared by every installed package — keep the `.deb`'s installed footprint lean, and consider putting any larger persistent state under `/data` rather than assuming generous root space.

## Docker: possible via a Gemini-team plugin, deliberately not used

Correction to an earlier check in this doc: Docker **is** possible on this platform — there's an official Gemini-team Docker plugin available through the feed (explains the `docker` package seen earlier in `dpkg -l`, state `rc` — available/previously removed, just not installed on the reference device at the time of the `no overlay filesystem` check). So the earlier "not possible" conclusion only described the reference device's state at that moment, not a hard platform limit.

DreamHub still doesn't use it, but as a **deliberate choice, not a technical necessity**: on a 2 GB RAM / no-swap device already running enigma2 and a handful of background services (see resource headroom below), adding a Docker daemon is a constant-overhead dependency a single static Go binary simply doesn't need. Simpler, lighter, one less moving part to keep working across the platform's own updates.

## mDNS/Avahi: an existing opportunity for zero-config discovery

`avahi-daemon` is running (confirmed earlier) and already advertises several services (`/etc/avahi/services/`): `ssh`, `sftp-ssh`, `ftp`, `nfs`, and interestingly **`geminiandroid.service`** — almost certainly how an existing companion Android app discovers the box on the LAN via Bonjour/mDNS instead of the user typing in an IP. **DreamHub should do the same**: drop a `dreamhub.service` file into `/etc/avahi/services/` (as part of the `.deb` install) advertising its own `_http._tcp` service on port 8080, so a phone/browser can find it automatically instead of requiring a manual IP entry. Cheap to add, meaningfully nicer UX, and the infrastructure is already running.

## Resource headroom (for capacity planning)

Live snapshot on the reference device (enigma2 + all the services above already running): **1977 MB total RAM, ~1.2 GB "available"** (948 MB free + reclaimable cache), load average ~1.0 on a 6-core chip — healthy headroom for an additional lean Go process. **No swap configured at all** — if DreamHub's backend or embedded PWA assets balloon in memory use, there's no cushion, so keep the resident footprint deliberately small.

## SystemPlugins — the other plugin category

`/usr/lib/enigma2/python/Plugins/SystemPlugins/` is a separate category from `Extensions/` (lower-level, more "OS settings" than user-facing tools). Ones worth knowing about:

- **`SoftwareManager`** — this is what actually implements the apt-backed "Packages" browser seen in the Addon Manager screenshots earlier in this project.
- **`StreamServices`** — confirmed (read `StreamServicesConfig.py`) to be the actual settings screen for the streaming/transcoding ports discussed in Phase 5 (imports `eStreamProcessorFactory`, has `ConfigPassword` fields). This is the authoritative place those settings live if DreamHub ever needs to read or reference them.
- **`NetworkManager`** — the box's own network configuration UI (interfaces, WiFi, static IP).
- **`HdmiCec`** — Enigma2's own CEC handling layer. Relevant to be aware of if DreamHub ever touches CEC (nothing planned currently) so it doesn't fight the existing handler.
- **`UPnP`** — the box already runs a DLNA/UPnP media server. A separate existing way to browse/stream recordings from other devices on the LAN, independent of WebControl or DreamHub.
- **`MediaDatabaseManager`**, **`AutomaticTimerlistCleanup`** — recordings/timer housekeeping, context for Phase 2/7.
- **`SetPasswd`**, **`SkinSelector`**, **`Styles`**, **`Videomode`**, **`VideoTune`**, **`VideoEnhancement`**, **`Satfinder`**, **`PositionerSetup`**, **`InputDeviceManager`**, **`Hotplug`**, **`FSBLUpdater`** — standard device/picture/input settings, no direct DreamHub relevance identified.
- **`StartUpService`**: despite the generic-sounding name, reading its source shows it's about **which TV channel/service launches when the box boots** (`ChannelContextMenu`, service selection) — not a general-purpose "run my own service at startup" mechanism. Corrects an initial assumption; the actual autostart mechanism for DreamHub remains the systemd unit documented above.

## Installed skins

`/usr/share/enigma2/`: `Default-FHD`, `Default-WQHD`, `MetrixStyleHD`, `Shadow-WQHD` (the active one). A separate package `mediaportal-skin-shadow-wqhd` ("by Zombi, only for DreamOS and Amlogic Dreambox") confirms the same skin author also maintains a matching skin specifically for MediaPortal's own UI (see below) — consistent visual identity across both subsystems.

## MediaPortal is a full parallel subsystem, not a small plugin

Worth being explicit about this, since it shows up repeatedly across this investigation (its own `/data/mediaportal/` bundle, its own `ffmpeg-mp`, its own folder under `/media/hdd/mediaportal`) and could otherwise be mistaken for part of the base OS. It's a separate, actively maintained (package date `2026083001` — very recent) third-party extension with its own small ecosystem:

- `enigma2-plugin-extensions-mediaportal` — the core plugin
- `enigma2-plugin-extensions-mediaportal-hosters` — "hosters" in the Kodi-addon sense: modules that pull video from various external streaming/hosting sites
- `curl-mp` — a `curl-impersonate` build (mimics real browser TLS fingerprints — typically needed to get past anti-bot measures on the sites the hosters modules talk to)
- `ffmpeg-mp`, `yt-mp` (a YouTube downloader) — its own bundled media tooling, separate from the system `ffmpeg`
- `mediaportal-skin-shadow-wqhd` — its own matching skin

Treat it the same way as the other already-installed plugins in this document: a separate, mature, actively-maintained system solving overlapping problems (media streaming/aggregation) for the native on-screen surface — not something DreamHub needs to integrate with for its current scope, but worth knowing it exists so it isn't mistaken for a DreamHub concern or for a core OS component.

## Resolved: WebControl auth discrepancy (#13) — `localauth=false` disables it for LAN clients

Confirmed live from a second machine (this Mac, `192.168.1.x`, not the box itself, no session/cookie/credentials ever sent): `curl http://192.168.1.43/web/deviceinfo` returns `200` with full data, no `WWW-Authenticate` challenge. `config.plugins.Webinterface.http.auth=true` is set, but `config.plugins.Webinterface.localauth=false` is also set — this is the explanation. `localauth` is OpenWebif's own toggle for whether auth is *also* enforced for clients on private/RFC1918 address ranges; when it's `false` (the default the reference box ships with), any client whose source IP is in a private range (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `127.0.0.0/8`) bypasses the Basic Auth check entirely, regardless of `http.auth`/`https.auth`. `http.auth=true` still matters for anything reaching the box from outside the LAN (e.g. a port forward or the ZeroTier network, depending on how ZeroTier's address range is treated).

**Practical implication for DreamHub**: don't rely on the device's own `http.auth` to protect `/web/*` for LAN clients — with `localauth=false` (likely the common default), any device on the same LAN/VLAN can call the full read (and, unverified, write) API with zero credentials. This is a property of the box, not something DreamHub's own auth layer touches — DreamHub's Basic Auth/API tokens protect DreamHub's *own* API on :8080, not WebControl's on :80. Users who care about this should be told to set `localauth=true` on the device (native WebControl setting) if they want its API locked down even on their own LAN.

## Resolved: WebAdmin terminal reachability (#18) — page loads without auth, shell login still required

Confirmed live from the same external machine: `curl http://192.168.1.43/webadmin/terminal/` returns `200` with the ShellInABox HTML/JS page (title "Shell In A Box"), no auth challenge — same `localauth=false` LAN-bypass as above, since WebAdmin sits behind the same WebControl auth gate. This means anyone on the LAN can reach the *terminal emulator page* without any DreamHub or WebControl credentials. It does **not** mean an instant root shell: ShellInABox itself still presents a normal Linux login prompt inside that terminal (username/password), so an attacker still needs valid OS credentials (e.g. the box's root password) to actually get a shell — the exposure is that the attack surface (a login prompt reachable pre-auth) exists at all for anyone on the LAN, which it shouldn't given `http.auth=true` is configured. Same fix as above: `localauth=true` on the device.

## Resolved: SerienRecorder web interface API (#10) — reachable, JSON, path is `/serienrecorderapi/*`

`config.plugins.serienRec.enableWebinterface=true` is set. The plugin's own registration code (`SerienRecorderResource.py`, `addWebInterface()`) registers two separate external children with WebControl's Toplevel dispatcher: `serienrecorderapi` (the JSON API, `e2hasgui=False`) and `serienrecorderui` (`e2hasgui=True`, `SerienRecorder` web GUI) — both listed live in `GET /web/external`. Unlike everything else in this document, **this API returns JSON, not XML** (a genuine exception worth remembering). Confirmed working, e.g. `GET /serienrecorderapi/info`:

```json
{"payload": {"apiVersion": "2.12", "channelListeUpToDate": false, "dbVersion": "4.6.0", "image": "5.0.0r117-gutemine-2024-08-12 / Experimental 2023-04-24", "markerLastUpdate": 1778697811, "srVersion": "4.8.0-beta", "stbType": "one"}, "status": true}
```

Other confirmed live children from `getApiList()` in the same file (full list, most look relevant to SeriesHub): `markers`, `changemarkerstatus`, `setmarkerchannels`, `markerseasonsettings`, `markersettings`, `createmarker`, `deletemarker`, `updatemarkers`, `settings`, `cover`, `picon`, `tvdbcover(s)`, `settvdbid`, `searchtvdbid`, `transmissions`, `searchseries`, `activechannels`, `channels`, `timer`, `markertimer`, `addtimers`, `removetimer`, `removetimerbyseason`, `createtimer`, `seriesinfo`, `autocheck`, `log`, `info`, `getChangelog`, `checkforupdate`, `installupdate` — all under `/serienrecorderapi/<name>`. Same `localauth=false` LAN-bypass as WebControl itself applies here too (confirmed no auth prompt). SeriesHub (`ROADMAP.md`) should use `markers` (tracked series), `timer`/`markertimer` (upcoming), and `transmissions` (episodes on disk) as its primary data source instead of reading `SerienRecorder.db` directly, per the original plan.

## `epgnownext` takes `bRef` (bouquet reference), not `sRef`

Corrects an assumption in the earlier endpoint table below: `GET /web/epgnownext?sRef=<service ref>` returns an XML-wrapped `Processing Failed` HTML error, even for a real, currently-playing service reference. The endpoint actually wants `bRef=<bouquet reference>` (the same string `/web/getservices` returns for a bouquet, e.g. `1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "userbouquet.favourites.tv" ORDER BY bouquet`) and returns now/next for every service in that bouquet in one call — matching the "service list view" description already in the table below. Fixed, real response saved as a fixture (`backend/testdata/fixtures/epgnownext.xml`).

## Resolved: device time zone and clock source (#17)

`/etc/enigma2/settings` stores `config.timezone.zone=Europe/Berlin` (an IANA zone name, not just the `de_DE`-style locale used for `config.osd.language`) and `config.timezone.version=2`. The box's local time (`CEST`, UTC+2) matched this Mac's wall-clock time exactly when checked live, confirming the clock is accurate. No NTP daemon, `timedatectl`, or root crontab entry was found on the box — the actual sync mechanism wasn't identified in this pass (Enigma2 boxes commonly derive time from the DVB broadcast stream's TDT/TOT table rather than NTP, which would explain the absence of any NTP-related process or config; not directly confirmed here). Good enough to unblock UTC-storage/local-display work either way: read `config.timezone.zone` for the IANA name DreamHub needs for display formatting, and trust the box's system clock.

## Resolved: Enigma2 picks up recordings renamed outside Enigma2 (#11)

Tested live and reverted immediately, 2026-09-24: renamed all 8 files of a real recording ("Crazy Rich", `.ts` + `.ts.ap` + `.ts.cuts` + `.ts.meta` + `.ts.sc` + `.eit` + `.jpg` + `.backdrop.jpg`) via SSH (`mv`, same base name → new base name + suffix preserved, done in one pass so no window with a half-renamed set), then re-fetched `GET /web/movielist`. Enigma2 picked it up correctly and immediately, no rescan/restart needed: `e2title` ("Crazy Rich") and `e2descriptionextended` came from `.ts.meta`, unaffected by the filename change; `e2filename`/`e2servicereference` updated to the new name; `e2filesize` unchanged. Renamed back to the original name immediately after, confirmed via `movielist` again that the device was left exactly as found. **Conclusion: MediaHub's planned rename engine (`ROADMAP.md`) is safe as designed** — renaming the full sidecar set outside Enigma2 while it's not recording/playing that file is picked up correctly on next listing; no special handling needed beyond the "never touch a file currently being recorded/played" check already planned (#12, resolved above).

## Timer write endpoints: `/web/timeradd`, `/web/timerdelete` (confirmed live)

Confirmed 2026-09-24 by reading the WebInterface plugin's own readable Python source on the device (`/usr/lib/enigma2/python/Plugins/Extensions/WebInterface/`: screen definitions in `web/timeradd.xml`, `web/timerdelete.xml`, implementation in `WebComponents/Sources/Timer.py`, request handling in `webif.py` and `plugin.py`), plus WebControl's own JavaScript (`web-data/statics.js`, `handler.js`, `helpers.js`).

**Exercised live the same day, with the owner's approval, and reverted**: a disabled 5-minute test timer (RTL Television, `POST`, form-encoded, all required fields, a non-ASCII name) was created with `/web/timeradd` (`e2state=True`), appeared in `/web/timerlist` and in `/etc/enigma2/timers.xml` immediately, and was then removed with `/web/timerdelete` using the exact `sRef`/`begin`/`end` from the list (`e2state=True`). Afterwards the timer list was identical to before and `timers.xml` had the same md5 checksum as before the test. Also observed: a disabled timer is reported with `e2state=3` (ended) in the list, and `e2location` reads `None` when no `dirname` is given (the box's default path is used).

Both endpoints exist, alongside `timerchange`, `timeraddbyeventid`, `timerlistwrite`, `timercleanup` and `recordnow`. Both return `<e2simplexmlresult><e2state>True|False</e2state><e2statetext>…</e2statetext></e2simplexmlresult>`.

| Endpoint | Parameters (exactly the ones the screen accepts) |
|---|---|
| `/web/timeradd` | `sRef, begin, end, name, description` (required), `repeated, dirname, tags, eit, disabled, justplay, afterevent` (optional) |
| `/web/timerdelete` | `sRef, begin, end` (all required) |
| `/web/timeraddbyeventid` | `sRef, eventid` (required), `justplay, dirname, tags` (optional) — the box computes begin/end from its EPG **and adds its own margins** (`config.recording.margin_before/after`) |

Behavior that matters for DreamHub:

- **Use `POST`, form-encoded.** Every timer screen is declared `allow_GET=False`. That is only enforced when `config.plugins.Webinterface.anti_hijack=true`, which is the plugin's default; the reference box has it set to `false`, so a `GET` happens to work there. A `GET` with arguments on a default-config box returns `405`. WebControl's own UI always uses `POST`.
- **Send every required parameter, even when it's empty.** Missing parameters arrive as `None`, not as absent keys (`OneTimeElement.handleCommand` in `webif.py`), so the plugin's own "Missing Parameter" checks never fire. A missing `description`, `begin` or `end` makes the plugin throw instead of returning a clean error.
- **`sessionid`** is only checked when `config.plugins.Webinterface.extended_security=true` (it's `false` on the reference box). The token comes from `/web/session` and is tied to client IP plus User-Agent. WebControl's UI always sends it, so DreamHub can do the same for robustness.
- `begin`/`end` are Unix seconds. `begin` in the past is rejected unless the timer is already running (`begin < now < end`). `/web/timeradd` applies **no** margins, since `applyMargin` isn't in the screen's parameter list. DreamHub adds its own, which is consistent with `NewTimer`.
- `afterevent`: `0` nothing, `1` standby, `2` deep standby; anything else means `3` (auto). `disabled`/`justplay`: only `"1"` switches them on. `dirname` defaults to the box's preferred timer path.
- **The box does its own conflict check** (`TimerSanityCheck`, tuner-aware): on a conflict it returns `e2state=False`, `"Conflicting Timer(s) detected! / <name> / …"` and adds nothing. This runs in addition to DreamHub's own plan-time check, not instead of it.
- **Duplicate timers are silently dropped but reported as success.** If a timer with the same `begin` on the same service already exists, `RecordTimer.record()` ignores the new one and returns `None`, and `timeradd` says `"Timer '…' added"`. DreamHub must verify via `/web/timerlist` after applying, not trust `e2state` alone.
- **Persistence is immediate**: `record()` and `removeEntry()` both call `saveTimer()` (`/etc/enigma2/timers.xml`). `config.plugins.Webinterface.autowritetimer` (`false` on the box) doesn't matter.
- **`timerdelete` matches exactly**: the service-reference string and integer `begin`/`end` have to match an entry in `timer_list` or `processed_timers`. Use the values from `/web/timerlist` verbatim. Otherwise it returns `"No matching Timer found"`. **Deleting a running timer aborts the recording in progress** (`entry.abort()`), so the preview has to say so.
- **`timerchange` has a bug in its conflict path**: it mutates the in-memory timer first, then references an undefined variable (`entry`) when resolving conflicts. The resulting exception is caught and reported as a failure, but the timer stays modified in memory. DreamHub shouldn't build on `timerchange` without testing that path.
- Strings in results are translated into the box's OSD language (`_()`), so match on `e2state`, never on `e2statetext`.

## Recording file operations in WebControl (`moviedelete`, `moviemove`)

Corrects the earlier assumption (this document and `ROADMAP.md`) that WebControl has no file-operation endpoints. Confirmed 2026-09-24 from the plugin source (`web/moviedelete.xml`, `web/moviemove.xml`, `WebComponents/Sources/Movie.py`), read only, nothing called:

- **`/web/moviedelete?sRef=`** — a **hard delete** (`offlineOperations().deleteFromDisk(0)`), no trash. Violates "delete means move to trash" (`PRINCIPLES.md`); DreamHub must not use it.
- **`/web/moviemove?sRef=&dirname=&force=&background=`** — moves to another *folder* with the *same* base name (no rename). It uses `os.rename`, so it only works within one filesystem, and it has no rollback: a failure halfway leaves a split set. Suffixes it carries for `.ts`: `.ts.meta .ts.cuts .ts.ap .ts.sc .eit .ts .jpg`. **It leaves `.backdrop.jpg` behind.** DreamHub's own file component is still the right tool.
- **Security note about the box itself:** `MovieWebScreen` is declared `allow_GET=True`, so `moviedelete` accepts a plain `GET` even when `anti_hijack` is on. With `localauth=false`, any web page opened on a LAN device could delete a recording through an image tag, provided it knows the exact file path. The timer endpoints are reachable through a cross-site form `POST` the same way. Nothing DreamHub can fix. The mitigation is `localauth=true` (see below).

## WebControl auth for a backend on the box itself

From `plugin.py` (`HTTPAuthResource.isAuthenticated`): requests from `127.0.0.1`/`::1` skip auth when `config.plugins.Webinterface.streamauth=false` (the reference box's setting). Requests from any configured local network skip it when `localauth=false`. So a user who hardens the box with `localauth=true` keeps DreamHub (calling `127.0.0.1`) working **only as long as `streamauth=false`**. DreamHub therefore needs optional WebControl credentials in its settings and a clear "WebControl requires a password" diagnostic on `401`.

## Timer states and "is it recording right now"

`timer.py`: `StateWaiting=0`, `StatePrepared=1`, `StateRunning=2`, `StateEnded=3`, reported as `e2state` in `/web/timerlist`. A disabled timer is reported as `3`. A timer in state `1`/`2` that isn't `justplay` is about to record or recording, and its target file must not be touched. Recordings started on the remote ("instant record") are timers too (`dontSave=True`) and show up the same way. Playback of a recording is a separate case (`/web/getcurrent`), not covered by the timer list.

## AutoTimer web API (`/autotimer`)

The AutoTimer plugin registers two WebControl children: `/autotimer` (API, XML) and `/autotimereditor` (its own web GUI, like SerienRecorder's). API children: `GET /autotimer` (full XML config, list), `parse` (**runs all AutoTimers and creates timers**), `simulate` (**dry run**, returns `<e2simulatedtimer>` entries with the timers `parse` *would* create, changes nothing), `test`, `edit` (add/edit, many query parameters), `remove?id=`, `get`/`set` (plugin settings), `add_xmltimer`/`upload_xmlconfiguration` (POST). `simulate` → `parse` maps naturally onto plan → apply, but `parse` runs every AutoTimer, not one. The reference box has **no AutoTimers configured** (empty `<autotimer version="8">`); SerienRecorder does that job there. The plugin is optional; DreamHub must work without it.

## BouquetEditor write endpoints (`/bouqueteditor/web/*`)

Closes the open follow-up below; read from `WebBouquetEditor/web/*.xml` only, nothing called. Parameters: `addbouquet(name,mode)`, `removebouquet(sBouquetRef,mode)`, `movebouquet(sBouquetRef,mode,position)`, `addservicetobouquet(sBouquetRef,sRef,sRefBefore,Name,Url)`, `removeservice(sBouquetRef,sRef)`, `moveservice(sBouquetRef,sRef,mode,position)`, `renameservice(sBouquetRef,sRef,sRefBefore,newName,mode,oldsRef)`, `addmarkertobouquet(sBouquetRef,Name,sRefBefore)`, `addprovidertobouquetlist(sProviderRef,mode)`, `addservicetoalternative(sBouquetRef,sCurrentRef,sRef,mode)`, `removealternativeservices(sBouquetRef,sRef)`, `togglelock(sRef,password)`, `backup(Filename)`, `restore(Filename)`. Only relevant for ChannelHub (v1.1), which still links/embeds the plugin's own UI first.

## AdvancedMovieSelection: rename approach and trash convention

Read for approach only (CC BY-NC-SA, no code copied):
- **Rename** (`Rename.py`): title/description are rewritten in lines 2–3 of `.ts.meta`, the rest kept. The file rename globs `<dir>/<basename>*` and replaces the base name in every match, so `.backdrop.jpg` and any other sidecar are carried along. There's no rollback and no in-use check. DreamHub wants the same "all files sharing the base name" rule, plus an explicit preview, a collision check, rollback and the in-use check.
- **Trash** (`Source/Trashcan.py`): deleting renames **only the main file** to `<name>.ts.trash` in place. The sidecars stay, and Enigma2's movie list no longer shows the file. Restore removes the suffix. Emptying deletes the main file plus sidecars. On the reference box: `use_wastebasket=true`, `auto_empty_wastebasket=14` (days, run at 04:00). No `.trash` files existed at inspection time. The old `/media/hdd/movie/trashcan/` folder is empty (2023). If DreamHub uses the same convention, the TV wastebasket and DreamHub see the same trash. AMS is optional, so DreamHub has to implement the convention (list/restore/empty) itself, and AMS's auto-empty is only a bonus.

## journald is volatile

No `/var/log/journal`; `journald.conf` sets `RuntimeMaxUse=64M`, `ForwardToSyslog=yes`. The journal lives in RAM (~16 MB at inspection) and **is lost on every reboot**. Early boot entries carry a 1970 timestamp (before the clock is set). AdminHub has to say "since last boot" and can't promise history. enigma2's own log lines are in the journal (`journalctl -u enigma2`). One enigma2 crash log exists: `/media/hdd/enigma2_crash_1774122941.log` (the name carries a Unix timestamp).

## Real API fixtures captured (#14)

Live responses for every endpoint in the table below, plus `epgnownext` (corrected, see above) and five `serienrecorderapi` endpoints, are saved verbatim under `backend/testdata/fixtures/` (`*.xml` for `/web/*`, `serienrecorder/*.json` for the SerienRecorder API) — captured directly from the reference device on 2026-09-24. `backend/internal/webcontrol/mock` can be extended to load these instead of (or alongside) its current hand-written fixtures for more realistic tests.

## Three plugin tiers, not two

Corrects/extends the earlier plugin survey, which only covered `Extensions/` and `SystemPlugins/`. There's a third, separate category: **`Plugins/GP4/`** — Gemini Project's own **first-party** plugins, distinct from both core Enigma2 and community `Extensions/`:

```
geminiaddonmanager   geminifilebrowser   geminiqbutton
geminibluepanel       geminihwmanager     geminireceive
geminibluetooth        geminiimagebackup   geminiserver
geminicamswitch        geminilocale        geministreamripper
geminicomm             gemininetcast       geminiwebremote
                       gemininetcastwebif   geminiwidgets
                       gemininetscan        geminizerotier
                       geminioscaminfo
```

This resolves several earlier findings cleanly:

- **`geminizerotier`** — ZeroTier isn't something bolted on separately; it's a **built-in, first-party remote-access feature of this image**. Explains the `zt...` interface and running `zerotier-one` process found earlier.
- **`geminiwebremote`** — the actual implementation behind the "WebRemote" feature under WebControl's Steuerung tab.
- **`geminioscaminfo`** / **`geminicamswitch`** — first-party integration with the OSCam CAM emulator found running earlier.
- **`gemininetcast`** / **`gemininetcastwebif`** — "DreamNetcast" in the WebPlugins sidebar; the `webif` half explains the separate `/netcast/` URL namespace observed.
- **`geminiimagebackup`** — likely the underlying engine behind the flash/backup functionality also exposed via the (community-authored) `GPT-Plugin`.
- **Important contrast with `Extensions/`**: GP4 plugins are **closed** — shipped as compiled `.pyo` bytecode, some with compiled native `.so` libraries (`geminiwebremote/webremotetools.so`), no readable `.py` source. Community `Extensions/` plugins (SeriesRecorder, AdvancedMovieSelection, etc.) are plain, readable Python. Treat GP4 the same way as WebControl itself: observe its behavior/API, don't expect to read how it works internally.

So the honest three-tier picture for "what comes from the system" is: **(1)** core Enigma2 + generic `SystemPlugins/` (open, part of the wider Enigma2 ecosystem), **(2)** Gemini Project's own closed `GP4/` layer (this image's actual value-add over plain Enigma2 — remote access, web remote, addon management), **(3)** community `Extensions/` (SeriesRecorder, AdvancedMovieSelection, MediaPortal, etc. — third-party, mostly open, independently maintained, installed via the feed system).

## Package feeds are unsigned (`[trusted=yes]`)

Read `geminilocale/plugin.py` (turns out to be misleadingly named — it's not about UI language at all, it's the component that **auto-detects the box's architecture/image type and writes the apt feed config files** found earlier under `/etc/apt/sources.list.d/`). The line it generates:

```
deb [trusted=yes] <url><type> ./
```

**`[trusted=yes]` means no GPG signature verification** — this platform's own feeds (and by extension, anything installed from them) aren't cryptographically verified. Two implications for DreamHub: packaging doesn't need to deal with GPG signing to match platform convention (lowers the Phase 8 bar), but it's also worth being aware there's no supply-chain verification happening anywhere in this ecosystem, on this device, by default.

## WebAdmin's own Scripts/Terminal/Feed features (Phase 7/8 precedent)

The `WebAdmin` plugin (endpoint namespace `/webadmin/*`) already provides three things directly relevant to later phases:

- **`/webadmin/web/scriptlist`** — a "User Scripts" upload feature (currently empty on the reference device). A real, existing precedent for Phase 7's script hub — worth deciding whether DreamHub complements or duplicates this.
- **`/webadmin/terminal/`** — a full **web-based root shell**, proxying to `shellinaboxd` (confirmed: the browser tab literally retitles to "Shell In A Box"). Not touched beyond confirming it exists — this is a live root shell, exploring further wasn't warranted for a documentation pass.
- **A feed-management UI** with **"Upload new \*-feed.list"** — end users can add a custom apt feed (i.e. DreamHub's own) through this existing web page, no Telnet/SSH required. This is the actual, friendly install path to document for end users in Phase 8, not just "add a file via SSH."

## No JSON API — XML only

Tested directly: `?format=json` and `Accept: application/json` both still return XML from `/web/deviceinfo`. There is no shortcut here — DreamHub's Go backend needs a real XML parser (`encoding/xml` is fine) for anything it pulls from WebControl's API.

## CLI tools available for Phase 7 scripts

Confirmed present: `curl`, `wget`, `python` (2.7), `tar`, `unzip`, `sed`, `awk`. **Confirmed absent**: `jq`, `git`, `rsync`, `sqlite3`. Any bundled example scripts for the Phase 7 script hub should assume only the first list, or declare the others as explicit `.deb` dependencies if needed.

## Open follow-ups

- `/bouqueteditor/` write endpoints: parameters now known from source (see above); behavior still unexercised.
- `AdvancedMovieSelection`'s `Rename.py` and `SearchTMDb*.py` are worth a closer read (not just the file list) before starting Phase 2 — this session only confirmed their existence and rough purpose, not their actual logic.
