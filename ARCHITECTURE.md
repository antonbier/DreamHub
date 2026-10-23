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
| `PlexDream` | PlexHub | A Plex client for the TV remote. Not used: PlexHub (2026-10-05) plays a Plex file with Enigma2's own player instead (a `4097` reference to a local URL DreamHub passes on to the server) — PlexDream is slow, and DreamHub shouldn't depend on another plugin. |

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

**Confirmed 2026-09-25** (`enigma2-plugin/`, WP8): the standalone `/usr/bin/python2` interpreter can `py_compile` a plugin module fine (syntax-level check), and can import plain Python packages like `twisted.web.client` — but every Enigma2-framework module (`Screens.Screen`, `Components.*`, `Plugins.Plugin`, `Tools.Directories`/`LoadPixmap`) fails with `No module named _enigma`, since that's the compiled C++ core only present inside the actual running `enigma2` GUI process. So a plugin's syntax and its plain-Python helper calls can be checked over SSH without touching the box's running state, but anything that actually exercises the Enigma2 framework (skin rendering, `ActionMap`, `ConfigListScreen`) needs installing into `Extensions/` and a GUI restart — no way around that with SSH alone.

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

## Docker: possible via a Gemini-team plugin, now used by AppsHub

Correction to an earlier check in this doc: Docker **is** possible on this platform — there's an official Gemini-team Docker plugin available through the feed (explains the `docker` package seen earlier in `dpkg -l`, state `rc` — available/previously removed, just not installed on the reference device at the time of the `no overlay filesystem` check). So the earlier "not possible" conclusion only described the reference device's state at that moment, not a hard platform limit.

**Revised 2026-10-12**: AppsHub (`docs/APPSHUB_PLAN.md`) now runs self-hosted apps as Docker containers, reusing this same vendor plugin rather than bundling a separate daemon — DreamHub's own backend still never touches a Docker daemon beyond that: a dedicated, isolated helper process (`cmd/dreamhub-dockerd-helper`) is the only thing with socket access, talking a whitelisted subset of the Engine API, the main backend reaching it over loopback only (`backend/internal/dockerd`'s own package doc). The earlier "deliberate choice not to use it" reasoning (a constant-overhead daemon a single static binary doesn't need) was about DreamHub's *own* runtime, which still holds — running optional, user-chosen third-party apps in containers is a different question, decided separately once the owner actually wanted that capability.

**The helper's surface after App12 (compose-defined apps, 2026-10-15)** — still a fixed, whitelisted API (`GET /containers`, `POST /containers`, start, stop, `DELETE`, logs, exec, image-digest, attach), never free-form Docker arguments. `POST /containers` takes one fixed JSON shape (`internal/dockerd/server.go` `createRequest`; an unknown field is a 400, so a raw `HostConfig` or a `binds` list can't be smuggled in). It grew only what a compose file needs, each part checked by the helper itself (`internal/dockerd/validate.go`, `ValidateSpec`, run both in `Server.handleCreate` and in `Exec.Create` — the helper does not take the main backend's word for it):

- **Mounts**: a list of `{hostDir, containerDir, readOnly}` (it was two fixed ones). Allowed host paths: an app's own folders under its storage root (`<root>/<id>`, `<root>/<id>.<name>`) and paths strictly below `/media`. Refused always: `/` and the system directories (`/etc /usr /var /boot /proc /sys /dev /bin /sbin /lib* /run /root /home /opt /srv /data`), any Docker socket, DreamHub's own data directory (`DREAMHUB_DATA_DIR`, default `/data/dreamhub`) including everything that contains it, the apps' storage folder itself, other apps' folders, the app's `<id>.secrets.json`, and any directory that contains a `dreamhub-apps` folder (it would expose every app's data and passwords). The check runs on the path as written (it must be clean: absolute, no `..`, no trailing slash) and again on what the file system resolves it to (symlinks), and both must pass; an unresolvable path (dangling link, loop) is refused. The bases (`/media` by default) can only be changed in the unit's environment (`DREAMHUB_HELPER_MOUNT_BASES`), never by a request.
- **Elevated settings** (`privileged`, `capAdd`, `devices`, `pidHost`, `ipcHost`) exist only inside an `elevated` object and are refused unless it carries `confirmed: true` — the person's explicit extra confirmation, which only AppsHub's install plan sets. Capability names and devices are validated (devices must be clean paths below `/dev`); `/dev/mem`, `/dev/kmem`, `/dev/port`, `/dev/kmsg`, `/dev/root` and the box's flash devices (`mmcblk*`, `mtd*`, `ubi*`, `nand*`) are refused even when confirmed. A confirmation never unlocks a refused mount.
- Env names, sizes, command length, the image reference (no options, spaces or digests) and the display label (`dreamhub.name`, custom apps only) are validated; the network is always the host network.
- A refusal is HTTP 422 with `{code, message, path}`, which the main backend turns back into a typed error — the install plan shows it before anything is pulled.

**The helper's surface after App13 (container list and Docker settings, 2026-10-15)** grew by exactly two read/clean-up calls, both without any parameter a caller could steer: `GET /images/unused` lists the images no container (of any kind, running or stopped) uses, each with the size removing it alone would free (own layers, not shared ones); `POST /images/prune` removes exactly those — the helper sends Docker's own prune with a **fixed filter** (`dangling=false`, i.e. every unused image), and a request carrying a body or a query string is a 400, so there is no way to name an image, a filter or anything else. Containers, volumes, networks and the build cache are never touched, and the daemon itself refuses an image a container uses. An in-process lock keeps a prune from running while `Create` pulls an image no container uses yet. There is deliberately **no** call to change a container's restart policy: **auto-start is DreamHub's own choice** (setting `apps.autostartIds`), carried out once per boot with the existing start call, so every container keeps `on-failure`, at most 5 retries — the crash-loop cap is unchanged. The main backend's calls that can take long (`Create` with its pull, the prune) use a 10-minute timeout instead of 10 seconds. **Edit** needs no new helper call: the containers are removed and created again through the validated create call, so every mount, elevated and device rule applies again to the new values.

**The helper's surface after App14 (updates and rollback, 2026-10-15)**: `POST /containers/{id}/keep-image` tags the image a DreamHub container runs as `dreamhub-rollback/<app>[.<service>]:previous` (the name is built from the container's own labels) and as `…:origin-<hash of the name it was used under>`; `GET /images/kept` lists those; `POST /images/kept/drop[?app=<id>]` removes the rollback names (and the image when nothing else uses it) — the only parameter accepted is a valid app id. `ContainerSpec.FromKept` on the create call skips the pull and gives the kept image its original name back, **only** the name recorded by the origin tag (any other name is refused), so the call cannot be used to rename an image. `POST /images/prune` no longer calls Docker's prune-all: it deletes, one by one and by id, the images `GET /images/unused` lists, and that list never includes a kept image or one a container uses; still no parameters. `GET /containers` carries `restartCount` (from inspect). No new way to name an arbitrary image, container or tag.

**Backups after App15 (2026-10-16)**: a backup file is `backupFile` (settings, dashboard, optional `sections`). Sections implement `BackupSection`: `apps` (folders, `Apps`) and `appList` (`appListSection`: per app `<root>/<id>.app.json` + `<id>.secrets.json`, so the file holds secrets; restore skips installed apps and only accepts `[a-z0-9]+` ids and known roots). `System.BackupDir` keeps saved backups (`POST /system/backup/save`, `…/files`, `…/files/{name}[/restore]`); the file name must match `dreamhub-backup-YYYYMMDD-HHMMSS.json`, files are 0600. `configfs.FS` gained `List(dir)`; apps with files but no containers are `GET /apps/restorable`, installed again by `POST /apps/restorable/{id}/plan` (the normal install plan). Backup reads are `perm.System` (`System.Perms`).

**Still unconfirmed on the reference box** (`docs/APPSHUB_PLAN.md`'s own open questions): whether the vendor's Docker plugin supports cgroup v2 and rootless mode, and the exact Engine API version/path compatibility `internal/dockerd`'s hand-rolled client assumes — built and tested against a faithful fake unix-socket daemon, never against the real plugin.

## mDNS/Avahi: an existing opportunity for zero-config discovery

`avahi-daemon` is running (confirmed earlier) and already advertises several services (`/etc/avahi/services/`): `ssh`, `sftp-ssh`, `ftp`, `nfs`, and interestingly **`geminiandroid.service`** — almost certainly how an existing companion Android app discovers the box on the LAN via Bonjour/mDNS instead of the user typing in an IP. **DreamHub should do the same**: drop a `dreamhub.service` file into `/etc/avahi/services/` (as part of the `.deb` install) advertising its own `_http._tcp` service on port 8080, so a phone/browser can find it automatically instead of requiring a manual IP entry. Cheap to add, meaningfully nicer UX, and the infrastructure is already running.

## Port 8080 on a DM900/DM920 (reported 2026-10-17, not reproduced on the reference box)

A tester's DM900/DM920 (OE 2.5, armhf, DreamHub 0.9.0) could not start DreamHub: `listen tcp :8080: bind: address already in use`, and the owner of the socket was `1/init`, i.e. a systemd socket unit (`netstat -anp`). Most likely the stream server's HLS output (see "Hardware encoder and Dreambox's stream server": HLS default port **8080**), which exists only on boxes with a hardware encoder; not found on the Dreambox One. To confirm on such a box: `systemctl list-sockets | grep 8080`. Decision: DreamHub's default port became **8765** (`PROJECT_PLAN.md`), 8080 stays reserved against apps. Also found there: `dpkg -i` does not install missing dependencies (`apt-get -f install` does).

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

## `epgmulti`'s `endTime` is minutes, not a timestamp

Found running DreamHub on the box (2026-09-24) and confirmed from the plugin source (`WebComponents/Sources/EPG.py`, `getEPGofBouquet(…, multi=True)`): `/web/epgmulti?bRef=&time=&endTime=` hands `endTime` unchanged to `epgcache.lookupEvent` as the `(service, type, begin, minutes)` tuple's last element, so it's a **duration in minutes**. An end timestamp reads as "millions of minutes" and returns the whole EPG. Live, one bouquet of 30 channels: `endTime=<now+1h as timestamp>` → 9,843 events, 6.2 MB, about 25 days; `endTime=60` → 80 events, 66 KB; `endTime=240` → 247 events. The response also includes the event already running at `time`, which can start hours earlier. Corrects the earlier note in `ROADMAP.md` that the endpoint "takes a time window" as start/end timestamps.

## enigma2 answers WebControl requests one at a time

The web server runs inside the single `enigma2` process (see "Process architecture"), and requests queue behind each other. Under a deliberate load test (60 parallel DreamHub requests, each fanning out to WebControl) `/web/timerlist` took up to 5.8 s, and one `/web/deviceinfo` hit DreamHub's 5-second client timeout. In normal use (one request at a time) the same calls take 10–60 ms, `/web/movielist` about 3.5 s. Consequences: DreamHub must never fan out many parallel requests to the box. Anything polling (WP7 live updates) should coalesce concurrent identical requests and cache briefly, and `/web/movielist` is expensive enough to cache.

## DreamHub itself on the box (v1 plan WP2, 2026-09-24)

The static ARM64 binary (12.9 MB, frontend embedded) was run by hand from `/tmp` on the reference box with the owner's approval: live mode against `http://127.0.0.1`, port 8090, data in `/tmp`, device writes off. Results:

- Starts in about 20 ms and detects `Europe/Berlin` from `/etc/localtime`. Every read endpoint answers `200`, the UI and client-side routes load, and the WebControl and SerienRecorder connectors are `ok`.
- **Memory: about 11 MB RSS idle, 21 MB peak** under 60 parallel requests (after the `epgmulti` fix; 40 MB before, while shipping the whole EPG), far under the 150–200 MB budget. The box had about 1.2 GB available throughout.
- `/tmp` is a RAM-backed `tmpfs` (989 MB), fine for a trial but not for data. `/data` is a 7.6 GB ext4 partition, 43 % used.
- The default umask `022` made DreamHub's data directory and SQLite's `-wal`/`-shm` files world-readable. DreamHub now sets umask `077` at startup. A `/data/dreamhub/` from an earlier manual test run (11:16–11:48 that day) still has the old modes and was left untouched.

## `/web/movielist`: folders, the "Processing Failed" tail, and `None` sizes

Confirmed 2026-09-24 against the reference box and its plugin source (`web/movielist.xml`, `WebComponents/Sources/Movie.py`):

- **Without `dirname` the list only covers the top folder, and the subfolder part crashes.** The screen has two parts: `MovieList` (the recordings) and `MovieListSubdir` (the folder's direct subfolders as `<e2locations><e2location>…`). With no `dirname`, the second part runs `pathExists(None)` and the response ends in an HTML "Processing Failed" fragment. That is the "malformed tail" noted earlier, and not any single recording's metadata. With an explicit `dirname=/media/hdd/movie/` the response is complete and valid XML.
- **Most recordings live in subfolders.** The reference box has 36 files in `/media/hdd/movie/` but 773 `.ts` recordings in total, in 173 folders: SerienRecorder's `<Series (year)>/Season N/` folders, plus hand-made `Film/Deutsch/<title>/` trees. Walking every folder through the API took 40 s and returned 913 entries, with enigma2 serving nothing else meanwhile. So MediaHub browses one folder per request.
- **`e2filesize` (and potentially `e2time`) can be the string `None`**, seen on a manually copied `.mkv`. A strict integer field made Go's `encoding/xml` fail on that entry, and everything after it was lost, including the subfolder list. DreamHub reads these fields leniently (`webcontrol.Int64`).
- A recording being played shows up in `/web/getcurrent` with its file path after the ten numeric fields (`1:0:0:0:0:0:0:0:0:0:/media/hdd/movie/x.ts`), the standard Enigma2 form. This wasn't exercised live (it would mean starting playback). A running timer's `e2filename` names the file it writes. MediaHub uses both to refuse touching a file in use.

## MediaHub file operations on the box (v1 plan WP4)

**Exercised live 2026-09-25 with the owner's approval, and reverted.** Through DreamHub's own API on the box (test instance, writes on), a recorded GZSZ episode (8 files, 1.46 GB) was renamed to "DreamHub Schreibtest" and back, moved to the trash and restored. Each step was previewed and applied. enigma2's `/web/movielist` showed each new state immediately. Afterwards the file list of `/media/hdd/movie` (6,191 files, names and sizes) had the same checksum as before, and the change log held the four entries. Earlier, 2026-09-24, previewed on the read-only test instance with nothing applied: A rename preview of a real SerienRecorder recording listed all 8 of its files, including `.backdrop.jpg` and the SerienRecorder cover. Applying was refused (`writes_disabled`), and the recording folders' file list was unchanged afterwards (same checksum). The rules are in `backend/internal/recfs`. No `.ts.trash` files existed on the box. The old `/media/hdd/movie/trashcan/` folder from 2023 is hidden in MediaHub's folder list.

## Real screenshot and remote-control endpoints (confirmed live 2026-09-25)

Found while visually verifying the WP8 Enigma2 plugin — the basis of ControlHub (built since, `ROADMAP.md`), not just of that one test:

- **`GET /screenshot?format=png[&osd=1][&video=1][&res=WxH][&display=1]`** — the WebInterface plugin's real screen-grab (`WebChilds/Screenshot.py`), a top-level path, *not* under `/web/`. Uses `enigma.ePixmap`'s native `setPixmapFromUI`/`setPixmapFromScreen`/`setPixmapFromVideo` — no external binary, always available. With neither `osd` nor `video` given it defaults to `osd` (GUI layer only, transparent where nothing's drawn — renders as white/blank in a typical PNG viewer, not a bug). `osd=1&video=1` composites both, giving a real "what's on the TV" screenshot. Confirmed live: the OSD-only default and the OSD+video composite were both captured successfully, up to ~4MB for a 2560×1440 video frame.
- **`GET /grab?...`** (also top-level, `WebChilds/Screengrab.py`) is a *different*, unrelated endpoint: it shells out to an external `/usr/bin/grab` binary (the "aio-grab" package) which isn't installed on the reference box, so it answers `200` with a plain-text "Grab is not installed" body rather than an image. `/web/grab` (the path initially guessed, by analogy to some OpenWebif builds) doesn't exist on this box's WebInterface plugin at all — `404`. **`/screenshot` is the one that actually works here.**
- **`GET /web/remotecontrol?command=<code>`** simulates a real remote-control keypress (confirmed already reachable, `ARCHITECTURE.md`'s auth section) — standard Linux input-event codes: `139` MENU, `352` OK, `174` EXIT, `103`/`108`/`105`/`106` UP/DOWN/LEFT/RIGHT, `398`/`399`/`400`/`401` RED/GREEN/YELLOW/BLUE, `116` POWER (toggles standby). A key that isn't handled in the box's current state (e.g. anything but POWER while in standby) briefly shows a small "blocked" icon overlay, not an error — the request itself still answers `200`.
- **`GET /web/powerstate`** → `<e2instandby>true|false</e2instandby>` — the reference box was found in standby mid-session (no current service, `/web/getcurrent` all-empty fields); every simulated key was silently ignored until `command=116` woke it. Any future ControlHub screen needs to check this before assuming a blank screenshot means something's wrong.
- **This image's custom "Gemini BluePanel"** (opened by the BLUE key) is *not* the standard Enigma2 Plugin Browser — it's a separate, manually-curated category list (`Extensions-Ordner`/`Plugin-Ordner` → `Downloads`, `System`, etc.) that a newly installed plugin doesn't automatically join. The real, complete list of every registered Extensions plugin is still reachable the standard way: main menu → **Erweiterungen** ("Plugin Browser"). A plugin missing from BluePanel is a Gemini-skin categorization quirk, not a registration failure — confirmed by finding `DreamHub` correctly listed, with its live-translated German description, in the standard Plugin Browser while genuinely absent from BluePanel.

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

## DreamHub installed as a package; live findings (2026-09-28)

`dreamhub_0.1.0-dev_arm64.deb` installed with `dpkg -i` on the reference box, then upgraded twice in place. The service came up (`systemctl is-active dreamhub` → active), every hub reached "available" once enigma2 had finished its restart, and the existing `/data/dreamhub` database from the September manual test was picked up (TMDB and OpenAI keys already set). DreamHub's resident memory after a few minutes of use: **~18 MB** (VmRSS), 8 threads.

- **Upgrades no longer restart enigma2 unless the launcher plugin changed**: `postinst` keeps a checksum of the plugin files in `/data/dreamhub/.plugin.sum` (confirmed: the second upgrade printed "launcher plugin unchanged, enigma2 keeps running" and enigma2's start time stayed the same).
- **`/web/epgsearch` has no result cap.** The earlier note ("at most 256 results") is wrong for this box: `search=a` answered **41 MB in ~11 s**, `ta` 6 MB in ~2 s, `tat` 0.35 MB in ~0.2 s — and enigma2 serves WebControl single-threaded, so a short query stalls the box for everyone. DreamHub now never sends a query shorter than three characters (`webcontrol.MinEPGSearchQuery`).
- **`/web/message`** (read from `web/message.xml` and `WebComponents/Sources/Message.py`): parameters `text`, `type` (0 yes/no, 1 info, 2 warning, 3 error), `timeout` (seconds; ≤ 0 or missing = until closed), `default` (yes/no box only); answers `e2simplexmlresult`. Exactly what DreamHub sends.
- **Remote keys the box handles** (`/usr/share/enigma2/keymap.xml`): KEY_VOLUMEUP/DOWN, KEY_MUTE, KEY_CHANNELUP/DOWN (in the infobar they open the channel list, as on the physical remote), KEY_INFO, KEY_0–9, KEY_TEXT, KEY_AUDIO, KEY_TV, KEY_RADIO, KEY_VIDEO, KEY_REWIND/PLAY/PAUSE/PLAYPAUSE/STOP/FASTFORWARD, KEY_PREVIOUS/NEXT. **There is no KEY_EPG** — on a Dreambox the programme info is INFO (short press), the EPG list the yellow key in the infobar. DreamHub's virtual remote follows this.
- **How SerienRecorder files recordings here**: either a series' own record folder (`/media/hdd/movie/serien/FBI Most Wanted`, with `Season N` subfolders below), or a subfolder of a recording folder named after the series, sometimes with its start year (`Gute Zeiten, schlechte Zeiten (1992)/Season 0`). A marker's record folder can point at a folder that no longer exists. Its recordings' `.ts.meta` carry the episode code in both title and description (`FBI: Most Wanted - S05E07 - Gefangenenübergabe` / `S05E07 - Gefangenenübergabe`). Daily soaps are numbered absolutely (`S00E8615`), which doesn't line up with TVmaze's seasons. Series folders contain a **`folder.jpg`** cover.
- **Cover art next to recordings**: 23 of the 36 recordings in the top folder have `<base>.jpg`, 19 a `<base>.backdrop.jpg`. There are 173 folders and 775 `.ts` files below `/media/hdd/movie`.
- **SerienRecorder's `/serienrecorderapi/seriesinfo?wlid=&fsid=`** returns `{info: {title, year: "2018-", info: "Drama, Crime, Action (42 min)", description (German), cast: [...], rating, seasons_and_episodes}, coverURL}` from SerienRecorder's own server — both parameters required, otherwise an empty payload. Its `rating` (268909 for "FBI") is not a rating. Markers carry no TVDB ID.
- **TVmaze** finds 7 of the 9 followed series by exact name; "FBI: Special Crime Unit" is TVmaze's "FBI" (premiered 2018, German title not among its "akas"), found through the subtitle fallback with the start year; "Sing meinen Song – Das Tauschkonzert" isn't on TVmaze.

## DreamMediaInfo (inspected read-only, 2026-09-28)

`/usr/lib/enigma2/python/Plugins/Extensions/DreamMediaInfo` — readable Python ("coded by zombi, friends and Gemini KI", licensed for DreamOS only, modification and redistribution prohibited: DreamHub only *reads its data*, never its code). It enriches EPG/infobar titles from TMDB with its own key and keeps the results in **SQLite at `config.plugins.dreammediainfo.db_path`** (`/data/dreammediainfo.db`, 75 MB on the reference box; setting in `/etc/enigma2/settings`):

- `media_cache(key, title, type, data, ts)` — key = lower-cased title + `_tv` / `_movie` (films also `_<year>_movie`), with a colon written as ` -` (`fbi - special crime unit_tv`); `data` = TMDB's details as JSON (`id`, `name`/`title`, `overview`, `genres`, `cast`, `vote_average`, `fsk` — the German age rating —, dates). 151 entries, 54 films / 97 series — **titles seen in the EPG, not the recordings** (none of the top folder's recordings is in it). The box's copy has the older schema without `tmdb_id`/`poster_path` columns.
- `episode_cache(key "<tmdb id>|<season>|<lang>", data)` — 12,793 season episode lists; `match_cache(key "<tmdb id>|<episode title>", se "S05E14")` — 2,543 EPG episode titles mapped to their code; `epg_to_tmdb` (empty).
- Images in `/data/dmi_image/` (2,857 files, 351 MB) named after TMDB's image paths (`<path>_backdrop.jpg`, `_logo.png`) — not linked to the entries in this schema.
- Settings on the box include `enable_tmdb=true`, `ttl_hours=96`, `api_langs=de,en`.

DreamHub reads `media_cache` read-only (`internal/dmi`) as its **first** enrichment source; with the TMDB ID from there, TMDB is only asked for the poster, by ID. Confirmed live: Tatort, FBI, Columbo, The Masked Singer, Twister come from DreamMediaInfo with overview, year and FSK, pictures from TMDB. Recordings mostly aren't in DreamMediaInfo, so they keep coming from TMDB — 34 of 35 top-folder recordings found, after reading the title out of Enigma2's default file name (`20230423 2115 - ORF1 HD - Angel Has Fallen.ts`) for the six recordings without a `.ts.meta` title.

`match_cache` (EPG episode title → `S05E14`) is read (`dmi.Reader.EpisodeCode`) to mark which episode a broadcast on a series page is, and whether it's missing — the key's exact spelling (whether the title part is the EPG short description as-is) is still to be confirmed on the box.

## Live write tests (2026-09-28, each reverted)

- Remote keys via `/web/remotecontrol`: INFO opened the event info, EXIT closed it, MUTE twice, VOLUME up then down — all answered `e2result True`.
- `/web/message` with type 1, timeout 5: shown as "Nachricht (3)" with a countdown, closed on its own.
- `.ts.meta` title change through DreamHub's rename plan: `/web/movielist` showed the new title immediately, no reload; after changing it back the file was byte-identical (same MD5), mode 644 kept, no temporary file left.

## Box self-test findings (2026-10-04, release 0.1.0)

Found with DreamHub's own self-test (System → Diagnostics, `internal/hubs/selftest.go`) on the reference box — DreamOS 2.6 (`opendreambox`), systemd **232**. The raw answers are fixtures in `backend/testdata/fixtures/`.

- **Radio**: `/web/getservices?sRef=1:7:2:0:0:0:0:0:0:0:FROM BOUQUET "bouquets.radio" ORDER BY bouquet` lists the radio bouquets, as on other Enigma2 images (`getservices_radio_root.xml`): here just "Favourites (Radio)", which is **empty** (`/etc/enigma2/userbouquet.favourites.radio` has only its `#NAME` line). Every radio service: `sRef=1:7:2:0:0:0:0:0:0:0:(type == 2) || (type == 10) ORDER BY name` — 664 entries in 0.06 s (`getservices_radio_all.xml`), each station once per transponder it's on, plus data services named `.` and services without a name of their own, which Enigma2 calls `<frequency><polarisation> SID 0x<sid>` (`10796V SID 0x3dae`). `/web/epgnownext` over that query works (0.25 s, ~800 KB, 66 stations with a programme at the time).
- **`/web/epgnownext` on an empty bouquet fails**: the WebInterface answers "Processing Failed" inside an `<e2eventlist>` and logs a traceback (`KeyError: u'BouquetReference'` in `WebChilds/Screenpage.py`). Harmless, but DreamHub doesn't ask for an empty bouquet's EPG.
- **Picons**: no logo directly in `/data/piconpool`; it holds picon **sets** as subfolders — `PPH_carbon_220x132`, `PPH_trans_220x132`, `stefanbenno6_transparent_220x132`, `Huminator_mirror-glass_32-bit_220x132` (named by service reference, `1_0_19_…_0_0_0.png`; each matches 34–35 of the first 40 TV favourites), `LicherPils_3d_220x132` (another naming, 0 matches), `netcastpicon` (hashed `.jpg`s), `gemini` (empty). `/usr/share/enigma2/picon` doesn't exist although several settings point there; nothing in `/etc/enigma2/settings` names the set the skin uses.
- **systemd units**: Samba is `smb.service`/`nmb.service` (`samba.service` exists but is inactive), ZeroTier `zerotierone.service`, the softcam `oscam-cycecplus.service`, plus `nfs-server.service`, `avahi-daemon.service`. SSH (`dropbear`) and FTP (`vsftpd`) are socket-activated — their `.service` reads inactive while they work.
- **systemd 232 specifics**: `systemctl show --property=…,NRestarts` exits 1 and stops after the first unit (the property arrived in 235); there's no memory accounting (`MemoryCurrent=18446744073709551615`). `systemctl_show_232.txt`.
- **32-bit ARM binaries run** on the Dreambox One (ARMv8 with AArch32 compatibility): DreamHub's `armhf` build started, opened its SQLite database and answered in mock mode. That says nothing yet about a real DM900/DM920.
- Everything else the self-test asks answered as DreamHub expects: TV bouquets (2, 46 channels in "Free TV", 21 with EPG at the time), `getcurrent`, timers, recording folder with file operations, SerienRecorder 4.8.0-beta (API 2.12, 9 series), AutoTimer (0), DreamMediaInfo's database.

## Live checks during a recording (2026-10-05)

With a SerienRecorder timer recording (state 2) and someone watching another channel, DreamHub 0.1.0 on the box was asked for previews only (no apply):

- Rename, trash and batch rename of the file being written: refused with `409 media_in_use` (`reason: recording`), the batch lists it as skipped. Deleting the running timer: a preview with the `timer.deleteRunning` warning. Disabling it: refused (`timer_toggle_running`).
- Gaps found and fixed in the code afterwards (not yet on the box): MediaHub's list didn't mark the file being written, the dashboard's timer tile didn't mark the running timer, TimerHub's state column shifted for a timer that can't be toggled.
- SSH from the development machine was refused (no key there); HTTP to WebControl and DreamHub worked.

## Plex (owner's server, 2026-10-05)

- The owner's Plex Media Server (on the Unraid machine, behind their own HTTPS reverse proxy) answers `GET /identity` without a token: `{"MediaContainer":{"claimed":true,"machineIdentifier":"…","version":"1.43.4.10903-e5521bd8c"}}`; everything else answers `401` without one. GDM discovery (UDP 32414) from the LAN got no answer while the server was off.
- Plex Media Server 1.43's JSON carries, besides `rating`/`audienceRating` (numbers), list-valued keys of the same names in other case: `Rating` (`[{image, type, value}]`, the rating sources), `Image`, `Guid`, plus `UltraBlurColors`, `Role`, `Director`, `Writer`, `Country`, `Producer`. Go's `encoding/json` matches keys case-insensitively, so a number field tagged `rating` must tolerate the list (`plex.number`). Found on the box with 0.1.5: every `/library/metadata/<id>` answer failed.
- WebInterface's `/web/zap` (`WebComponents/Sources/SwitchService.py`, readable on the box) opens `MoviePlayer` only when the reference's path starts with `/` and the TV infobar is the current dialog; anything else — a `4097` reference to an `http://` URL too — goes through `servicelist.zap`/`nav.playService`, i.e. plays like a channel with the TV infobar and no seeking. Plugins add their own paths to the box's web server with `Plugins.Extensions.WebInterface.WebChilds.Toplevel.addExternalChild((path, resource, name, version, listed))` at `WHERE_SESSIONSTART` (AutoTimer, WebAdmin do so); DreamHub's plugin adds `/dreamhub/play` this way (0.1.7).
- Owner's test (0.1.6): most Plex films play on the box through the local stream; a few report an unknown codec (expected — the box's decoders).
- Confirmed with 0.1.7 (owner's test, 2026-10-05): through `/dreamhub/play` the movie player opens, with seeking, pause and the movie infobar.
- 2026-10-05: release 0.1.5 installed over 0.1.0 with `dpkg -i` (enigma2 restarted by `postinst` for the plugin, no recording running); the self-test afterwards was all green (14 checks). PlexHub reports `canPlay: true` on the box (`/etc/enigma2` present, so the local stream base is set).

## Checks on the box (2026-10-06, 0.1.7, read-only)

- **Resources**: DreamHub's process uses 24 MB RSS (peak 26.5 MB), 11 threads, 5.6 s of CPU time over two hours of uptime with the background syncs running; `/data/dreamhub` is 21 MB (SQLite with WAL, caches, the self-test's raw answers). enigma2 itself: 362 MB. Of 2 GB RAM, 1.2 GB were available.
- **Network**: the box gets its address from ConnMan (`eth0`, default route via the router), plus a ZeroTier interface (`zt…`, 10.147.19.0/24). DNS goes to ConnMan's local resolver (`nameserver 127.0.0.1`).
- **Plex at a public name from inside the LAN**: the owner's Plex domain resolves to the home's own public address (ping 1.2 ms — the router), i.e. the box reaches Plex through NAT loopback. From the reboot at 08:55 until at least 10:25, connections to port 443 there failed with "no route to host" while ping answered and the same address worked from outside; by 10:40 it answered again without anything changed on the box. Cause outside the box (router or reverse proxy). DreamHub's answer: the server's LAN address as a fallback (0.1.8).
- **SerienRecorder and a daily soap**: "Gute Zeiten, schlechte Zeiten" is recorded into `Gute Zeiten, schlechte Zeiten (1992)/Season 0/` with `S00E8622 - <episode title>` in the description — absolute numbering. TVmaze lists the show with years as seasons (2026) but the same absolute numbers (8601–8615 matched the recordings' titles one to one); it lagged about a week behind the broadcasts. SerienRecorder's timers use RTL Television.
- **Simulcasts in the EPG**: an EPG search for the series returns each RTL broadcast seven times — RTL Television, RTL UHD, RTL Austria and four regional variants (Bayern, HB NDS, HH SH, Regional NRW) — with the same event id, start and description: 63 hits for 13 broadcasts.
- **BouquetEditor's page** (`/bouqueteditor/`) sends no `X-Frame-Options` or CSP, so it can be framed; its layout is fixed at about 1000 px wide (unusable on a phone).

## Hardware encoder and Dreambox's stream server (2026-10-06, read-only)

- Kernel 4.9 (Amlogic vendor). Loaded at boot: `encoder` (`/lib/modules/4.9/extra/aml-4.9/frame_sink/encoder/h264/encoder.ko`), device `/dev/amvenc_avc` (char 495:0, root only); dmesg: "amvenc_avc probe start … cma memory pool size: 60 MB" ("reserved memory config fail" just before — it falls back to CMA). The hardware decoders are multi-instance modules (`amvdec_mh264`, `amvdec_mmpeg12`, `amvdec_h265`, …) next to `amvdec_ports` (the vendor's V4L2 decoder wrapper); `/dev/video13–16` are `ionvideo`, `/dev/video10` `amlvideo`.
- ffmpeg lists `h264_v4l2m2m` etc., but no V4L2 encoder device exists — the vendor encoder has its own ioctl interface (Amlogic's userspace `libvpcodec` drives it elsewhere); no such library is on the box.
- enigma2 has `eStreamServer` (log: "[eStreamServer] connectToStreamServer") and `Components/StreamServerControl.py`: a hardware-encoding stream server controlled over D-Bus. Settings `config.streamserver.*`: input "Follow Live", "HDMI Input" or "TV Services" (a background service via `setTarget(2)`, `ref=` in the stream URI switches it), H.264 main/high, 576p/720p/1080p, video 256–20000 kbit/s, audio 32–448 kbit/s (AAC), RTSP (port 554) and HLS (default port **8080** — DreamHub's) outputs. Scene detection and slices only on dm900/dm920. None of these settings is set on the reference box, and the server process itself isn't installed: no unit or binary, no package in the configured feeds (GP4 "pyro-gemini4-unstable" and others; `enigma2-plugin-systemplugins-streamservices` is IPTV playback, unrelated). Port 8001 (`streamproxy.socket`) is the plain, untranscoded TS stream.

## Box checks for 0.4.0 (2026-10-12)

Run against DreamHub 0.4.0 on the reference box, mostly through a throwaway second instance (`DREAMHUB_DATA_DIR=/tmp/…`, port 18080, `DREAMHUB_ENABLE_DEVICE_WRITES=0`), removed afterwards.

- **Packages (AdminHub A2)**: dpkg 1.18.10, apt 1.2.12. `dpkg-query -W -f` with `${X-DreamHub-Category}` works as built (an empty fifth field when a package doesn't set it); DreamHub's own package reads as category `self`. `apt-get upgrade --simulate` lines are `Inst <name> [<old>] (<new> <suite>:<host> [<arch>])`, e.g. `(1.0-r1 allcodes:download.blue-panel.com [all])` — the parser's "version is the first field in the parens" holds. `install -s` of a new package has no `[old]`; `remove -s` prints `Remv <name> [<version>]` and **cascades**: removing `enigma2-plugin-skincomponents-extepgshowhide` would also remove `enigma2-skin-gp4-shadow-wqhd` and `mediaportal-skin-shadow-wqhd`. That is what the A2 install/remove preview must show.
- **Terminal (A4, #36)**: `shellinaboxd` listens on `127.0.0.1:4200`; through DreamHub's proxy the page, `ShellInABox.js`, `styles.css` and a session load, keystrokes go through and the box answers with its own `dreambox login:` prompt — the box's login, not a root shell, sits behind it.
- **systemd 232 and the extension template (E3)**: a test unit from `packaging/extension-template/extension.service` loads and runs; unknown directives are logged and ignored. Ignored on 232: `ProtectKernelLogs`, `ProtectClock`, `ProtectHostname`, `RestrictNamespaces`, `RestrictSUIDSGID`, `LockPersonality`, `RemoveIPC`, `IPAddressDeny`/`IPAddressAllow` — so the loopback-only network restriction doesn't hold (the unit reached the LAN). The kernel (4.9) lacks `CONFIG_NETFILTER_XT_MATCH_OWNER`, so no iptables `--uid-owner` fallback. cgroups are v1.
- **Docker (AppsHub)**: not installed — `docker` 1.13.0 and `geminidocker` are in state `rc` (removed, config left), no `/var/run/docker.sock`. Docker 1.13 is API 1.25; the helper uses unversioned paths. Docker Hub search and manifest checks work from the box itself. Flash (`/`) is 70 % used (560 MB free), the HDD 89 % — both near the thresholds AppsHub checks.
- **Memory**: 2 GB total, `MemAvailable` ~1.17 GB with enigma2 and DreamHub running.
- **StreamHub (#34)**: `/api/v1/stream/live` for the current channel answers in ~0.09 s at ~3.4 Mbit/s; `/api/v1/stream/recording` serves a recording from the HDD at ~70 MB/s; both are clean MPEG-TS (sync byte every 188). Decoding on a phone is still the open part.
- **Uninstall/purge (#22)**: `dpkg -r` keeps `/data/dreamhub`, `dpkg -P` removes it, enigma2 keeps running. Two leftovers found and fixed in 0.4.0's maintainer scripts: enigma2's own compiled `.pyo`/`.pyc` kept the plugin directory alive (a launcher with no backend at the next enigma2 start), and `systemctl disable` in postrm ran after the unit file was gone, leaving `multi-user.target.wants/dreamhub.service` dangling.
- **Package install/remove through DreamHub (A2, later the same evening)**: `apt-get -y install`/`remove` run from DreamHub's process work as-is (htop installed in 0.75 s and removed again). `-o Dpkg::Use-Pty=0` keeps dpkg's "(Reading database ... 65%" progress out of the captured output. `apt-cache dumpavail` lists the feeds' packages in about 0.3 s; some carry no `Section` at all.
- **Docker 29.9.0 static on the box (App7, later the same evening)**: runs on kernel 4.9 with cgroup v1 (deprecation warning only) and the containerd overlayfs snapshotter once `overlay` is loaded; no swap limit or blkio throttling support, memory limits work. Without `overlay`, every container start fails with `mount ... fstype: overlay ... no such device`. The matching module is `kernel-module-overlay 1:4.9.269-dream-219-g6f9ca1e-r0.1` from the official feed (`dreamboxupdate.com/.../dreambox/deb/dreambox`, SHA-256 `49127c28…`), same build date as the kernel (2023-04-20); the Gemini feed's is `1:4.9.113-…`, another build. It's installed on the reference box now. Containers on `--network host` reach the box's own services on 127.0.0.1 (DreamHub answered from inside one) and the LAN; Syncthing's UPnP probe reached the router (which refused).

## Box checks for 0.9.0 — multi-box (2026-10-16)

Reference box (DreamOS, one tuner, DreamHub 0.8.0 running on :8080), no second box. Method: the new build as a throwaway instance next to the running one (`/tmp/dh-dev`, :18080, a copy of the data directory, `DREAMHUB_ENABLE_DEVICE_WRITES=0`), a second throwaway with an empty data directory (:18082) as "the DreamHub on another box". Nothing on the box was changed; both instances were stopped afterwards.

- **Migration** 13–17 on a copy of the real database: ran, `dreamhub.db.pre-migration-13` was written, profiles/PINs/tokens kept.
- **No regression:** every parameterless `GET` of the OpenAPI document answered the same (status and body) on 0.8.0 and the new build, except what is new (`/boxes*`, `/peer/*`, `/openapi.json`), the `box` block in `/navigation`, `boxes` in `/profiles`, and per-instance state (extensions register with the running instance only, self-test results). Logos, channel check and custom logos were identical once the logos directory was copied too.
- **A second "box"** (the same Dreambox added by address): Test names the device, an address that does not answer is `box_unreachable`; timers, recordings, navigation (Basic: `box.full=false`, Apps refused `409 box_not_full`) and the overview answer for both. A recording stream with Range through the hub is byte-identical for both boxes (`206`, size 3 089 625 088); a live stream (RTL, 6 s) through the hub worked for both (about 3.5 MB each).
- **Pairing** with the second throwaway: wrong code `peer_bad_code`, right code pairs, a second use `peer_not_pairing`, the token shows on the other end as "Hub: dreambox", the hub lists that box's capabilities with `peer.state ok`. Pairing against the running 0.8.0 (no peer API) says `peer_not_found`, as it should.
- **Wake-on-LAN:** the packet is sent (`{"sent": true}`); a malformed MAC is `validation_failed`. Not checked: that a box in deep standby wakes (the box was on).
- **Play on another box:** refused with `writes_disabled` while device writes are off, unknown/non-recording paths `404`. **Not done:** actually starting a recording on the movie player (it would switch the TV on) — still open.
- **UI** in headless Chromium against the throwaway (admin session): selector, `/b/main/…` and `/b/zweit/…` for dashboard, timers, guide, recordings, device, control, channels, series, apps, settings → Boxes; no unexpected console errors. Found and fixed: the Device page's two 409s on a Basic box.
- **Found by installing 0.9.0:** `dreamhub-ext-ewelink.service` has `BindsTo=dreamhub.service`, so the package's `systemctl restart dreamhub` stopped it and nothing started it again (extensions empty, self-test `services` warning); `postinst` now starts enabled `dreamhub-ext-*` units after the restart. Also: for about a minute after `enigma2` restarted the box showed `online: false` until the poller's next look.
- **Load:** enigma2 used about 55 more CPU ticks per minute (0.9 %) with four extra pollers (two instances, two boxes each) than without; the throwaway's log had no warnings.

## Box checks for 0.7.0 (2026-10-16)

Run against DreamHub 0.7.0 on the reference box through an admin session (no recording running, TV on Das Erste), through the API and a headless browser on the real UI. The box was rebooted once, Docker was uninstalled and set up again; every app installed for the test was removed again and its data deleted.

- **`.ts.cuts` positions count from the start of the recording, not in raw stream PTS.** `.ts.ap` carries the raw stream clock (first entry 39 000–92 000 s on the recordings checked, with a 33-bit wrap on two of them); every mark and resume position the player itself wrote is within the recording's length (a mark at 295 s in a 62-minute file whose `.ap` starts at 39 432 s). The first version of "Mark in cut list" wrote raw PTS — marks the player could never reach. Fixed: positions are `AP PTS - first AP PTS`. Played on the TV (`/dreamhub/play`, resume point 1855 s, ad break 1881.6 → 2242.8 s): the player jumps the marked OUT→IN region by itself — the resume position written at the stop was 2264.9 s, not ~1903 s. **#44's "the TV skips them" is confirmed.** Two things for later checks: the player resumes from a `.ts.cuts` type 3 entry on its own, overriding the `t` parameter of `/dreamhub/play` (remove the entry to test a start position); and there is no way to read the playback position over HTTP — the resume entry written when the player stops is the probe.
- **Docker apps (App8–App15)**: Vaultwarden (25 s, answers on 8222), Nextcloud 35 with MariaDB LTS (two containers, the database not reachable from the LAN, Nextcloud answers on 8088 and on 8089 after an edit, its `status.php` reports "not installed" — the web installer is the owner's next step), a custom Compose app (`busybox`, exits at once). Update (recreates the container, keeps the previous version), roll back, edit/recreate with the database folder kept, remove (data kept), "bring back" from the stored choices, backup saved on the recording disk (`/media/hdd/dreamhub-backups`), sections per folder with real sizes (Nextcloud 796 MB, its database 169 MB). **Auto-start after a reboot works**: with both apps on, they were running again about 15 s after DreamHub answered. **Uninstall Docker** (16 s) removed containers, services, 210 MB of binaries and the app data; **Set up Docker** afterwards (15 s) brought everything back; the overlay module stays loaded until the next boot after an uninstall. Memory with Nextcloud, MariaDB and Vaultwarden running: 887 MB used of 1977 MB with enigma2 and DreamHub.
- **Crash-loop notice** found a gap: a container that crashes at once uses up its five restarts within seconds, before the 30-second look could see the count grow, so no notice came. A container first seen stopped after at least three restarts now counts; after the fix the inbox got "app.crashLoop" for the test container within 40 s.
- **Packages**: 400 packages listed with their sentences (curated or the package's own), "needed by" for ffmpeg, update list (2 skin packages), feeds listing, an install of the update-all plan preview. `apt-cache madison` knows one version of each upgradable package, so a downgrade could not be tried.
- **Self-test**: 14 checks, all ok. The UI pages load against the real box without script errors (the 404s are pictures of channels without a picon).
- **Not done**: a downgrade (no older version in any feed), an automatic update inside the night window, a restore of a Nextcloud backup on a clean box, a container with a device (`/dev/ttyUSB0`), AdGuard/Pi-hole (port 53 is held by connman's DNS proxy), the phone decoder (#34 needs a phone), WP10's DST day (2026-10-25).

## eWeLink S60TPF on the box (2026-10-16)

The owner's plug (`1002a17548`, firmware 1.2.0, reported as `type=plug`, announces `switches`, so UIID 190): its mDNS announcement decrypts with the cloud's devicekey (AES-128-CBC, key MD5(devicekey), PKCS#7) and carries `switches`, `configure`, `pulses`, `sledOnline`, `rssi`, `current`, `voltage`, `power`, `dayKwh`, `monthKwh` — the last five as whole numbers in hundredths (`voltage` 23442, `power` 901 while Unraid booted). The plug answers `POST :8081/zeroconf/<cmd>` **only when `selfApikey` is present** (otherwise the request hangs) and then always `{"error":400}` for an encrypted body — `info`, `getState`, `switch`, `switches`, with the right key, a wrong key and four key derivations alike — and `401` for an unencrypted one. The cloud accepts the same switch (`POST /v2/device/thing/status`) and the plug follows within two seconds; it answers the consumption-history request (`hundredDaysKwh`) with error 4002. OAuth 2.0 login: the code is valid 30 seconds; `GET /v2/device/thing` returns the devicekey of every own device. DreamHub's extension log: Device → Services lists `dreamhub-ext-ewelink` now.

## Radio text (checked 2026-10-20, #28)

WebControl does not expose radio text: nothing in the WebInterface plugin's `web/` templates or components mentions RDS or radio text, and `/web/getcurrent` for a playing radio station carries none. Enigma2 decodes it only for its own GUI (`Components/Sources/RdsDecoder.py`, `evUpdatedRadioText`).

It is in the stream, though. On a DVB-C radio station (Dlf Kultur, `1:0:2:2BB6:3F3:1:C00000:0:0:0:`, MPEG-1 Layer II, 256 kbit/s) the box's stream proxy (port 8001) delivers frames of 768 bytes that end with the marker byte `0xFD`; the byte before it is the number of RDS bytes (10 at most), stored in reverse order. Strung together they are UECP messages (`FE … FF`, `0xFD` escapes); message element `0x0A` is the radio text: data set, programme service number, length, a flags byte, then up to 64 characters. One frame carries 10 bytes, so a whole line takes up to ~15 s to arrive. A station on another transponder than the one the TV plays cannot be streamed with one free tuner: the proxy answers `SERVICE ERROR:-257` (BAYERN 3 while the TV was on ARD alpha). The box has two DVB-S2 sockets but this install is on cable. Implemented in `backend/internal/radiotext` (fixture: a real stretch of Dlf Kultur with the audio replaced by silence).

## VU+ Solo2 with VTi 15 (checked 2026-10-21, read-only)

First non-Dreambox receiver, the owner's (`192.168.1.90`, WebControl equivalent OpenWebif on port 80, root SSH without password, no web login). Everything below was read with GETs and read-only SSH; nothing on the box was changed.

- **System**: VTi 15.0.04 (Enigma2 `2025-06-23-vti-master`), **OWIF 1.5.2**, OpenVuplus 2.1, kernel 3.13.5, **MIPS** (BMIPS5000, 2 × 1305 MHz), **Python 2.7.9**, BusyBox 1.23, **opkg**, dropbear, **sysvinit** (`/etc/init.d`, no systemd). **559 MB RAM** (not the 1 GB some spec sheets give), rootfs 195 MB with **45 MB free**, recordings on `/media/hdd/movie` (ext3 disk). **Wi-Fi only** in this setup (`wlan0`, 144 Mbit/s). Two BCM7356 DVB-S2 tuners. `config.osd.language=de_DE`.
- **Consequence**: the box is a **Basic** box (MULTIDEVICE_PLAN.md). A Full DreamHub on it would need a MIPS build, an opkg package and an init script, with 45 MB of flash, no systemd and 559 MB of RAM. Not planned.
- **Ports**: 80 (OpenWebif), 8001 (stream), 8002 (transcoding, did not answer), 22, 21, 23, 139/445.
- **Same `/web/*` XML as the Dreambox**, parsed by `webcontrol.HTTPClient` unchanged: `deviceinfo`, `powerstate`, `getcurrent`, `getservices`, `epgnownext` (takes `bRef`), `epgmulti`, `timerlist`, `getlocations`, `movielist` (same `e2movie` fields; about 1 s for 25 recordings), `remotecontrol`, `vol`, `about`; `/file` answers `206 video/MP2T` for a `Range`. `/bouqueteditor/web/getservices` answers too (OpenWebif's own); its write endpoints were not tried.
- **Differences found** (all handled in the client now):
  - `/screenshot` is `404`; **`/grab?format=png`** returns PNG (and `format=jpg` JPEG) with video and OSD. `Screenshot` falls back to it on a 404.
  - `/web/getcurrent` has **no `<e2volume>`**; the volume and mute state come from `/web/vol`.
  - **`/autotimer` answers `200`** with `<e2simplexmlresult><e2state>false</e2state><e2statetext>AutoTimer Plugin not found</e2statetext>`; the real sub-paths (`/autotimer/simulate`) are `404`. The AutoTimer connector took this for "installed"; it now reports `not_installed`.
  - **`/web/session` is `404`** (no session tokens). `/serienrecorderapi/` is `404` (no SerienRecorder). Not tried with a login enabled.
  - Without a signal the box has no time: the clock stood at 1970 (no sat cable, so no transponder time), `epgnownext` answers `None` fields, `epgmulti` is empty, `getcurrent` has no event. Port 8001 accepted a connection and sent nothing (no signal). Both are not defects.
- **Also available**: a JSON API (`/api/about`, `statusinfo`, `getallservices`, `movielist`, …) next to the XML one; not used yet.
- **Whole hub against it** (live mode on a PC, writes off): `deviceinfo`, `timers` (2 tuners), navigation and the screenshot (PNG via `/grab`) work; the box list shows no `autotimer` and no `serienrecorder` capability after the fixes above.
- **With a signal (same day)**: the clock is right (CEST) and the current channel has EPG (other transponders fill over time). Live stream on port 8001 direct and through the hub (MPEG-TS, H.264, first data after 0.4–0.7 s); a recording through the hub with `Range` (206, `Accept-Ranges`); `epgnownext`, `epgmulti`, `epgsearch` (128 hits), `getcurrent` with now/next. **Writes, each undone**: `timeradd` → listed → `timerdelete`; BouquetEditor `addbouquet` → listed → `removebouquet`; the config files were unchanged afterwards except that the plugin rewrote `bouquets.tv` with CRLF line endings.
- **Two things that took the box down**: (1) `/web/epgmulti` with a **negative `endTime`** (a duration passed where an end timestamp belongs) **crashed enigma2** (`/media/hdd/dvbapp2_crash_*.log`; `enigma2.sh` restarted it) — the client now refuses an empty window. (2) The hub's daily channel snapshot (`getservices` for every TV service ordered by name, then one request per provider) took **over 39 s** for the first call (a 15 s client timeout fails it) and the box stopped answering on the network soon after (no ping, no SSH; cause not established — a crash log was not readable at that point). enigma2 answers web requests one at a time, so such a long query also stalls the stream and zapping. The refresh now waits 250 ms between provider reads and stops after three failures in a row.
- **After the reset (same day), Wi-Fi only**: the box was back after a hard reset; `/var/log` is a tmpfs, so nothing survived. Ping, SSH and HTTP had all been dead, which points at the network side (a 2014 `rtl8192eu` USB Wi-Fi driver is the suspect; an `eth0` exists, no cable) rather than enigma2, which `enigma2.sh` restarts by itself. Not proven.
- **Lighter lists, measured**: `/web/getallservices` (XML, every bouquet with its services; also on the Dreambox One) takes **0.4 s** for 251 services, `/api/getallservices` 0.3 s, one bouquet 0.08 s. The query for every service by name that stalled the box was not repeated. On an OpenWebif box (`webifversion` starts with `OWIF`) `webcontrol.HTTPClient` now builds "every channel" from the bouquets (3 requests, 0.9 s for 251 channels, by name, once each) and returns no providers; channels that are in no bouquet are not seen.
- **Tuner state**: OpenWebif's `/web/about` carries `<e2tunerinfo><e2nim>` with `name`, `type`, `live` ("19.2° O / Das Erste HD") and `rec` per tuner; a Dreambox's does not. `Client.TunerStates` reads it; `GET /api/v1/timers` answers `tunerStates` and the timers view shows "Tuner A: live: …, Tuner B: free".
- **Remote and playback** (Wi-Fi, light load): `/web/remotecontrol?command=358` (INFO) opened the VTi guide, the screenshot (`/grab`) shows OSD over the video, EXIT/STOP work. DreamHub's `/dreamhub/play` is absent (Basic box); **`/web/zap?sRef=<recording reference>` plays the recording on the box's TV** (from the start), so `PlayMovie` falls back to it on an OpenWebif box. A start position is not available that way.
- **Compared with the current OpenWebif source** (`E2OpenPlugins/e2openplugin-OpenWebif`, master of 2026-05, read, not run): the parameters and answer fields of everything DreamHub calls match — `timeradd` (`sRef`, `begin`, `end`, `name`, `description`, `repeated`, `dirname`, `tags`, `disabled`, `justplay`, `afterevent`), `timerdelete`, `timerlist` (`e2timebegin`, `e2state`, `e2location`, `e2filename`, …), `epgnownext` (`bRef`), `epgmulti` (`bRef`, `time`, `endTime`), `epgsearch` (`search`), `movielist` (`dirname`, `e2movie` fields), `getcurrent`, `message` (`text`, `type`, `timeout`), `remotecontrol` (`command`), `zap`, `vol`, `powerstate`, `/file`. The source itself says `OPENWEBIFVER = "OWIF 1.5.2"` — the VTi's version is upstream's own, so the `OWIF` prefix that marks an OpenWebif box holds. Routes: `web`, `file`, `grab`, `autotimer`, `bouqueteditor`, `epgrefresh`, `transcoding`, `wol`, `picon`, `terminal`; **no `/screenshot` and no `/web/session`** anywhere. `/grab` runs the external `/usr/bin/grab`; the client now checks it got a picture (a box without the tool is said to answer 200 with a text). `/web/zap` with a recording's reference is meant to play it ("use mediaplayer for recording": it opens the movie player). `/autotimer` returns the config or the same "AutoTimer Plugin not found" stub; `simulate` exists only with the plugin. Not compared: forks with their own changes (OpenATV, OpenPLi, …) and images with `auth_for_streaming` on (the client sends the box login on stream requests too).
- **On the Dreambox One as the hub (2026-10-21, a test build in `/tmp` on port 8791, writes on only for the relay test, production hub untouched)**: the Solo2 added as a Basic box answers every hub endpoint swept (navigation, current, timers with tuner states, recordings, channels, guide, radio) with 200 and no warning. **A recording of the Solo2 played on the One's TV through the hub's relay** (`POST /api/v1/stream/recording/play-on?box=solo2`, target the One): the One's movie player showed it (screenshot checked), the relay reference pointed at the hub's own address. On the One `getservices` for every TV service (3320, ordered by name) takes **0.18 s** and the 182 providers 0.04 s — the query that stalled the Solo2 is no problem on a Dreambox; a 250 ms pause per provider added about 45 s to the first channel list there, so it now applies to the background refresh only (the first request after start took 3.8 s afterwards).
- **OpenWebif features beyond DreamHub's calls (2026-10-23, GETs and read-only SSH, nothing changed; with a signal, on Das Erste HD)** — input for the v1.1 "box features from OpenWebif" item in `ROADMAP.md`:
  - **Box settings**: `/api/config` lists the setup sections OpenWebif shows (18 here: A/V, usage, `vti_movies`, AutoShutDown, automatic audio/subtitle choice, user interface, hard disk, HDMI-CEC, channel list, system, TV/radio, keyboard, buttons, timer, timeshift, transcoding, subtitles, time zone; titles in the box's language). `/api/config/<key>` gives the section's items: `vti_movies` has 68, each with `path` (`config.usage.movielist_show_progress`), `description`, and `data` with `type` (`select`, yes/no, number, text …), `current` and `choices` as `[value, label]` pairs. Enough to build a form without knowing the image. (`/api/config?section=…` ignores the parameter and returns the list.) The source builds the sections from Enigma2's and each plugin's `setup.xml`, filtered by the setup level and `requires`. Writing is `/api/saveconfig` (`POST`, `key`, `value`): it sets `cnf.value` and calls `cnf.save()`, but **not `configfile.save()`**.
  - **Why `/etc/enigma2/settings` can't be edited from outside**: enigma2 keeps every setting in memory and writes the whole file on a clean shutdown (`mytest.py`, `configfile.save()` after `nav shutdown`, line 628 on this box); an outside edit is overwritten then. The on-screen setup menus and OpenWebif run inside enigma2 and change the value in memory, so their notifiers fire at once. OpenWebif's change only reaches the file on the next clean shutdown or the next `configfile.save()` from elsewhere — a power cut loses it. On a Dreambox, the same route is the TV plugin (it runs inside enigma2): set the value, then `configfile.save()` straight away. Whether DreamOS builds its setup menus from `setup.xml` the same way is not checked yet.
  - **`/api/tunersignal`**: `snr` 99, `agc` 91, `ber` 0, `tunertype` DVB-S, `tunernumber` 0, `inStandby`. `snr_db` is also 99 — the percentage, not a dB value; show the percentage. `/web/tunersignal` gives the same as XML (`e2snrdb` "99 dB").
  - **`/api/getaudiotracks`**: four tracks with `index`, `pid`, `description`, `active` (MPEG stereo, without audio description, "Klare Sprache", Dolby Digital 2.0). `/api/getsubtitles`: a DVB subtitle (`deu`, `type` 0) and teletext pages (`type` 1, `mag_nr`, `page_nr`). Switching (`selectaudiotrack`) not tried.
  - **`/api/epgsimilar?sRef=…&eventid=…`**: for a "Tagesschau" event, **71** other airings; for a series episode with "(2/13)" in the title, none — it finds repeats of the same event, not the next episode.
  - **`/api/sleeptimer?cmd=get`**: `enabled` false, `minutes` 30, `action` shutdown. **`/api/powertimer`**: "PowerTimer-Funktion nicht verfügbar" on VTi. **`/api/vpschannels`**: "VPS Plugin nicht gefunden". **`/api/parentcontrollist`**: an empty blacklist.
  - **`/ipkg?command=list_upgradable&format=json`**: answers (two upgrades: `ca-certificates`, `smartmontools`); the list lines are `name - old - new` strings with empty first and last entries. The same controller also installs and removes on a plain request (`command=install|remove`), without a preview.
  - Not checked on a Dreambox yet: which of these WebControl has (the One was off on 2026-10-23).
- **Still open**: behaviour with web login enabled (not switched on: changes the box's settings), wake from deep standby over Wi-Fi, the same checks on a cable connection, other OpenWebif images.

