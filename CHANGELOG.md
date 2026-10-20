# Changelog

All notable changes to DreamHub are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows [Semantic Versioning](https://semver.org/) (see `PRINCIPLES.md`).

## [Unreleased]

### Added
- **Radio text (#28)**: RadioHub shows the line a station sends about what it plays ("Proms London: Mahler's 9th …") under the station that plays — on the box or in the browser. WebControl doesn't expose it; DreamHub reads it out of the station's audio stream (RDS in the MPEG audio frames' ancillary data), listening in for a few seconds at a time. `GET /api/v1/radio/text?ref=`.
- **A package change checks the hubs at once (#47)**: installing, upgrading or removing a package a hub names (the WebInterface, BouquetEditor, SerienRecorder and AutoTimer plugins, `ffmpeg`, `shellinabox`, `enigma2-journald`, the overlay module) has the hubs' status looked at straight away — a hub that became (un)available shows up in the change log without waiting for someone to open the app. The "needed by a hub" list now carries the plugins' real package names, read off the box.
- **Cutting by hand (#27)**: every recording's details have "Cut by hand": name the parts to cut out (m:ss or h:mm:ss), look at each boundary in the browser player, preview, confirm. It is the ad cut's own flow (no re-encoding, the original goes to the trash), and needs no ad detection first — the recording's length is read from the file.
- **Multi-box M8 — copy the channel list from another box**: ChannelHub → Bouquets → "Copy from another box" (shown with two or more boxes). Pick the source box, see per bouquet whether it is created, brought in line or already the same, and which channels this box doesn't know; one confirm. Channels are matched by reference, then by name; bouquets of the same name and kind are made to look like the source's, others stay. Applying backs up the channel list first and checks every bouquet afterwards, like any bouquet edit. API: `POST /api/v1/channels/sync/plan` (`from`, optional `bouquets`).

## [0.9.2] - 2026-10-19

### Security
- **Wrong passwords and tokens are throttled**: after 10 failed attempts within 5 minutes an address gets no further credential checks for 5 minutes (a login that is already open keeps working).

### Fixed
- **Package file modes**: the 0.9.0 and 0.9.1 `.deb` files were built on a machine with umask 0000 and carried 0777 directories and 0666 files, including `/usr/bin/dreamhub` (runs as root), the systemd unit and the TV plugin's Python files — writable by every local user. `build-deb.sh` now sets 0755/0644 itself and refuses a tree with group- or world-writable entries. To repair a box that has the old package: `chmod 755 /usr/bin/dreamhub /usr/bin/dreamhub-dockerd-helper; chmod 644 /lib/systemd/system/dreamhub*.service /etc/avahi/services/dreamhub.service; chmod -R go-w /usr/lib/enigma2/python/Plugins/Extensions/DreamHub` (or just install the next version).
- **"DreamHub is not reachable" when opening the box by a plain host name** (`http://dm920:8765`): the Host check refused every name without a dot except `*.local`. Names without a dot are accepted now (a foreign domain can never be one, so DNS-rebinding protection is unchanged).
- **The TV plugin's QR code pointed to `127.0.0.1`**: it now encodes the box's LAN address with the same port.
- The toggle in the feed list now follows the reading direction (right-to-left languages).
- The mDNS advertisement goes back to port 8765 when the port override in `/etc/default/dreamhub` is removed.

### Changed
- The "not reachable" screen shows the technical reason (address, HTTP status and error code) in small type.
- Log lines: refused requests (421, 403) log at WARN with host, remote address, user agent and origin; every request line carries `host` and `remote`; the start-up line lists the host name and the box's addresses; the duplicate `time` field is gone (journald stamps every line).

## [0.9.1] - 2026-10-19

### Breaking
- **DreamHub's port is now 8765 (was 8080).** On a DM900/DM920 port 8080 is held by DreamOS's stream server (HLS output; systemd owns the socket), so DreamHub could not start there. Open `http://<box>:8765` (the `<box>.local` name follows by itself); update bookmarks, the home-screen app, an HTTPS reverse proxy's target and any API client. Change it with `DREAMHUB_LISTEN_ADDR=:<port>` in `/etc/default/dreamhub` or in the TV plugin's settings.

### Changed
- Installing the package **never restarts enigma2** any more: when the TV plugin changed, the installation says it loads at the next enigma2 restart. (It used to restart enigma2 when nothing was recording — it cannot know whether someone is watching.)
- The TV plugin's **Backend port** setting really changes the port: after a confirmation it writes `/etc/default/dreamhub` and the mDNS advertisement and restarts DreamHub. The port is no longer a second, separate setting inside enigma2.
- The service reads the optional `/etc/default/dreamhub`. The installation says so when the service did not start.
- Started by hand on a box (where `/etc/enigma2` exists), DreamHub uses the real box (`live`), not the demo data (`mock`) — unless `DREAMHUB_MODE` says otherwise.
- UX review before testers: with one box there is no Settings → Boxes tab any more — General ends with an "Another box" card that leads to it, and the tab stays once a second box exists. The developer block under Access (token API, `box=<id>`) is collapsed and, with one box, leaves out `box=`.
- A Basic box's deep link (`/b/<id>/apps`) says "needs DreamHub on the box itself" with a link to Settings → Boxes instead of "That didn't work"; its Device page no longer shows DreamHub's own change log as if it were the box's.
- "Needs attention" only counts real trouble: an integration that was never set up (TMDB, Plex, AI) and pages a Basic box doesn't have are not counted and not repeated per page. The film-database hint appears once, on the dashboard, with a link to the setting; a page that cannot work without a connector that isn't set up still says why.
- With two or more boxes every page says which box it is about (a small coloured line above the page), and the dashboard's strip of other boxes is labelled.
- Connecting a box to the DreamHub on it is worded for two machines: "this DreamHub" and "DreamHub on <box>", numbered steps, **Get a code** / **Connect** / **Disconnect** instead of "DreamHub" three times. On a box with one box only, the way to get a code is the "Another box" card in General.
- Names: **Device** is now **Box admin** (the page that administers the box), "this device" means the browser or "your phone or computer"; the Apps tabs are **System software / Extensions / Apps**, and the Extensions tab says where an extension is installed.
- Settings → Docker: **Address for the Open links** — an app's Open link goes to `http://<address>:<port>`; the address is the one DreamHub was opened with unless set (behind an HTTPS reverse proxy the proxy's address is wrong, the apps' ports are on the box).

### Fixed
- A taken port stops DreamHub before anything else starts, with a message saying what to do. It used to start the database and the background jobs first and log `sql: database is closed` as they were torn down.
- The apps' port check names port 8080 as the stream server's and keeps it reserved, next to 8765.
- A profile restricted to some boxes no longer receives, lists or opens the background jobs of the other boxes (not even as an admin).
- A box that restarted enigma2 could show as offline for up to two minutes: the poller's back-off now stops at one minute (was five).
- A box address with a login (`http://user:pw@host`) keeps it: the login moves to the login fields instead of being dropped; fields filled in at the same time win.
- Settings → Boxes: a second box at an address another box already has is refused (it would be polled twice and split its data), and so is a port beyond 65535; addresses are compared without the default port and in lower case.
- The guide's single-channel day view asked for exactly 24 hours, so on the day the clocks change one hour of programmes was missing; it now runs until the next morning's start.
- `POST /api/v1/boxes` answered `capabilities: null` where the list answers `[]`.

## [0.9.0] - 2026-10-16

Multi-box: DreamHub can steer several Dreamboxes. With one box nothing changes — the selector, the dashboard strip and the other new buttons only show with two or more. Box-checked on the reference box (plan "Box check", `ARCHITECTURE.md`); not tried because only one box exists: a second real box, and starting a recording's playback on another box's TV.

### Fixed
- Updating DreamHub stopped the extensions (they are bound to its service) and left them stopped — the eWeLink plug went missing until a manual start. The package now starts the enabled extensions again after restarting DreamHub.
- Device on a box without DreamHub on it (Basic) asked for the administration (log level, crash logs) on a fresh page load before it knew the box's tier, and logged two 409s.

### Changed
- Multi-box groundwork, M1 (`docs/MULTIDEVICE_PLAN.md`; nothing to see, nothing new for a household with one box): DreamHub now keeps a registry of boxes, and every request is about one of them — the optional `box=<id>` query parameter, the first box when omitted, so existing clients, API tokens and extensions keep working unchanged. Per box: the web-control, SerienRecorder and AutoTimer connections, the connector and hub-status checks, the recording index, the channel snapshot and its backups, AdScan marks and logos, timer owners, the event poller (with back-off while a box does not answer) and the background jobs (a recording on one box no longer holds back another's queue). A plan is applied to the box it was made for. Files, services and packages of the machine DreamHub runs on are offered for that box only.
- Multi-box groundwork, M2 (capabilities per box; still nothing to see with one box): what a box can do is now measured per box — plugins from that box's own checks, machine-level features (recordings in place, packages, services, logs, terminal, Docker, extensions) only for the box DreamHub runs on. The navigation answer carries that list (`box`: id, `full`, `capabilities`); AppsHub is left out of the navigation of any other box, device checks such as `journalctl` no longer count against a box that is another machine, and Device → Administration/Advanced tabs are hidden there.
- Multi-box, M3 (the box selector — **only visible with two or more boxes**): Settings → **Boxes** (admins) adds a further box by address and the login of its web interface — a **Test** button names the device that answers and tells a wrong login from no answer — edits it, or removes it after a preview (DreamHub forgets everything it saved for that box; the box is untouched). A box shows in the top bar's selector (name plus colour, greyed with "offline" when it doesn't answer; if the selected one stops answering, the status line says so). The box is part of the address (`/b/<id>/guide`, bookmarks and the app work); the last choice is remembered per device, and switching loads the page again so nothing of the previous box stays behind. The dashboard gets a slim strip with one line per other box (what it shows, standby, offline, recording ●) — a tap switches. Profiles get a **"Boxes this profile may use"** list (none ticked = all); a profile cannot reach a box it may not use through the API either (`403 box_forbidden`, and a request without `box` means its first allowed box). New: `GET /api/v1/boxes`, `GET /api/v1/boxes/overview`, `POST /api/v1/boxes[/test]`, `PATCH /api/v1/boxes/{id}`, `POST /api/v1/boxes/{id}/remove/plan`. A box added this way is Basic (web interface only): see the plan for what that covers.
- Multi-box, M3a (API for developers): `GET /api/v1/openapi.json` describes the REST API (OpenAPI 3) — the `box` parameter defined once, the plan → apply pattern, and request/response shapes for the most useful calls; Settings → Access → API tokens links it with a curl example, and the user guide has a new section "API and tokens". `GET /api/v1/boxes` now carries each box's capabilities. A test fails when a route is missing from the document (or the document names one that is gone) and checks that every operation either takes `box` and refuses an unknown one, or is marked global.
- Multi-box, M4 (test fixtures): every further box in mock mode (and in the demo) is now a **second mock box** — "wohnzimmer", two tuners, its own timers (no conflict with two tuners), recordings, favourites, guide and live channel — so a box switch visibly changes the content. Fixed: Device on a box without DreamHub on it (Basic) loaded the administration anyway and logged a 409.
- Multi-box, M5 (streams and notifications per box): live TV and recordings of another box already pass through DreamHub (the box's login never reaches the browser); the limit of two simultaneous live streams is now **per box**, so a second box can stream while the first is at its limit. Notifications name the box once there are several boxes — in the bell, in the live feed and in the push text ("Recording finished: Terra X · Wohnzimmer") — and a profile only gets events of the boxes it may use. The bell has an **Only this box** switch (per device) with two or more boxes. Fixed: the bell asked a box without DreamHub on it for app/package updates (409).
- Multi-box, M6 (pairing to the DreamHub on a box): a box that runs DreamHub itself can be **paired**. On that box, Settings → Boxes → "Pair this box with another DreamHub" shows a one-time eight-digit code (also on its TV, when device writes are on; valid ten minutes, once, ended by five wrong tries); in the hub, Settings → Boxes → Edit → "DreamHub on this box" takes the code. The result is an ordinary API token on the box (named "Hub: …", revocable under Settings → Access), kept by the hub. A version handshake (`GET /api/v1/peer/info`, public) says which end is too old instead of failing oddly; a revoked token or a box that is off is shown as such. A paired box's own capabilities join its list in `GET /api/v1/boxes`. New: `POST /api/v1/boxes/{id}/pair`, `DELETE …/pair`, `POST /api/v1/peer/code`, `GET /api/v1/peer/capabilities`, `POST /api/v1/peer/pair`. **`StreamBase` is now per box**: playing a Plex film on a paired box fetches it from the hub's address as seen from that box (found automatically, or set under Advanced); a Basic box still can't play films (no DreamHub player there). Pairing does not yet let the hub manage that box's recordings or system — that follows.
- Multi-box, M9 (switching a box on): a box can get a **power-on method** under Settings → Boxes → Edit — **Wake-on-LAN** (its MAC address; the magic packet is broadcast on the local network) or **an extension's action** (e.g. the smart plug the box hangs on). A box that does not answer then shows a **Switch on** button, in the status line (selected box) and in the dashboard's strip (the others); only with two or more boxes, only for profiles with the remote that may use the box. Wake-on-LAN goes out at once; an extension's action is a plan first, showing the extension's own sentence, because a socket's action may toggle. After switching on, the poller of that box looks every ten seconds for three minutes instead of backing off, and the app refreshes until the box answers. New: `POST /api/v1/boxes/{id}/wake` (`{"sent": true}` or `{"plan": …}`), `wake`/`wakeMethod`/`wakeMac`/`wakeExt`/`wakeAction` in `GET/PATCH /api/v1/boxes` (the details for administrators only); migration 17.
- Multi-box, M7 (a recording of one box on another box's screen): when a recording is open, **Play on <box>** (only with two or more boxes) starts it in the movie player of another box that runs DreamHub — the box DreamHub is installed on, or a paired one — while it is on. DreamHub streams it from the box the recording is on (in place, or through that box's web interface — that box can be Basic), seeking works. The target fetches it without a login from an address that only works with a one-time ticket and only from that box's own address. New: `POST /api/v1/stream/recording/play-on?box=<source>` (`target`, `path`, `title`; needs the remote and a target box the profile may use).
- The database gets its multi-box migrations (multi-box, the profiles' box lists, pairing, the power-on method); before one runs, the old database is copied to `dreamhub.db.pre-migration-<version>` next to it.

## [0.8.0] - 2026-10-16

### Added
- Logs for extensions: the Services list in Device → Administration now includes the Docker services and extensions (eWeLink), each with its Logs button; an extension's refusal text is part of DreamHub's own log line when an apply fails (it only said "unexpected status 502" before).
- Extensions can show things on the start page: a widget flagged `tile` appears in a new **Extensions** tile, under the extension's name — each extension decides what (or nothing); with nothing flagged the tile is not shown (schema 3, `docs/BUILDING_EXTENSIONS.md`). Settings forms can show a sentence (`info`) and an address to open (`link`).
- eWeLink extension 0.3.0: gets the device keys from the owner's own eWeLink developer application (log in once, paste the address you land on — no key hunting), and with that login shows the plug's **power, today's and this month's consumption** (the plug announces them on the LAN; the cloud history is only a fallback), on its card and on the start page. Needs DreamHub 0.8.0. Switching goes through the cloud when the plug refuses the LAN command (the S60TPF does, always); the plug's LAN values are scaled (hundredths).

## [0.7.0] - 2026-10-16

- Clickable demo for a PC (mock data, no Dreambox): `packaging/build-mock.sh` builds zips for Windows, macOS and Linux; CI uploads them as the `dreamhub-demo` artifact. The backend now also compiles for Windows and macOS.

### Added
- Updates and maintenance, App14: one **Updates** summary at the top of the Apps page (and in the notification list) counting package and container updates, with **Update all** — one preview listing every container and package in the order they run (containers first: they roll back by themselves, so a failure leaves the system untouched), a failed step stops the rest and the result says what was done, what failed and what was not started. Updates never run while a recording runs or starts within 15 minutes; a refusal says when it is possible again. Optional "only between HH:MM and HH:MM" and **automatic updates** (off by default; containers in Docker settings, packages in Package settings; packages that restart enigma2 stay manual); results go to the notifications.
- **Roll back** in a container's menu: the version before the last update is kept; an update that does not keep running for a minute goes back by itself. The image clean-up never removes kept versions; Docker settings show them with their own remove button. **Downgrade** in a package's menu where the feed still carries an older version.
- A container that keeps stopping (3 restarts in 10 minutes) puts one notice in the inbox, linking to its logs.
- Backup and restore of apps, App15: a backup can carry the list of installed apps with what was chosen for each (and the passwords DreamHub made for them), next to their folders. After a restore on a new box, **Apps → Docker → My containers** offers "Apps you can bring back" with *Install again* — the ordinary install preview, nothing installs by itself. An app you removed is offered the same way.
- **Save on the box** next to *Download backup* (Settings → System): the backup is kept on the recording disk, listed with restore, download and delete — an app's data can be too big for a phone. Backups are admin-only now (they hold app folders and passwords).
- Docker tab, App13: two views, **My containers** and **App catalog**. A container row shows its picture, name, status dot, port, an "Update available" badge, an **Auto-start** switch and **one** main action (**Open** while it runs, else **Start**); **Stop**, **Logs**, **Console**, **Edit**, **Back up**, **Update** and **Remove** are behind "⋯". **Edit** reopens the install form filled with what was chosen (web port, fields, extra rows, memory; storage shown, since the data stays) and applies it as a plan that makes the containers again — the data folders are kept, secrets are never sent back to the browser (empty means keep), and if the new containers can't be created the previous ones are put back. Your own containers (Add container) are editable and removable too; ones added before this version can't be edited and say so. **Update** is the same plan with the settings as they are (newest image of the same tag). Multi-container apps stay one row with their helpers under it.
- Auto-start: the switch is saved at once (with a short confirmation, no plan). The choice is DreamHub's own: once per boot, as soon as Docker answers, DreamHub starts the apps that have it on (helpers first); an app you stopped by hand stays stopped when only DreamHub restarts. New apps follow a default in Docker settings (on). Containers keep their cap of at most 5 restarts after a crash, so nothing restarts forever.
- Docker settings (gear): storage location for **new** apps (free space shown; installed apps stay where they are), auto-start default, memory limit per app (automatic or custom; used for containers that state none and by the free-memory check), update check (every 6 hours to weekly, default daily, instead of the fixed hourly check) with a switch for telling the notification inbox, and **Clean up unused images** (size first, then a preview with the list, then confirm). They are ordinary admin-only settings (`apps.*`, shown with "Reset" only when changed). The Docker helper learned two fixed calls for the clean-up (list/remove unused images, no parameters).
- Docker apps are defined as Compose (App12, mock only so far): the curated apps (AdGuard Home, Pi-hole, Nextcloud with MariaDB, Vaultwarden, Syncthing, Home Assistant, qBittorrent) are now embedded `docker-compose.yml` files with a thin `x-dreamhub:` layer (descriptions in German, English and Italian, web UI port, form fields) and behave exactly as before — same ports, generated passwords, memory, folders and backup items (`internal/appcatalog`, proven against the former Go data in tests). `internal/compose` reads a strict subset: services with image, ports, volumes, environment, command, memory limit, restart and depends_on; `${VAR:-default}` becomes a form field; everything else (build, networks, secrets, configs, extends, …) is refused with a reason.
- Install form: the simple view asks for the web port, the disk and the fields the app requires; **Show advanced options** adds every field plus free rows for environment variables, ports and folders and a memory limit. Secret fields are masked and never appear in the preview, which now lists exactly what will be created (images, ports, folders on the box, variables, rights).
- **Add container** (admins): paste a compose file, name an image, or import an Unraid template (`<Container version="2">`, read locally — nothing is downloaded and no Unraid templates are bundled) and get the same form. A standing notice says the container is your responsibility. Privileged mode, capabilities, devices and host PID/IPC need an extra explicit confirmation; mounts of `/`, `/etc`, `/usr`, `/var`, `/boot`, `/proc`, `/sys`, `/dev`, DreamHub's own data, the Docker socket, other apps' storage or anything outside the app's own folders and `/media` (also via symlinks or `..`) are always refused — by the install plan and again by the Docker helper.

### Changed
- Packages tab (App11): every package says in a sentence what it is, without an AI — DreamHub's own short descriptions in German, English and Italian (embedded list `internal/pkginfo`), else the package's own description. Categories (Extensions, Skins, System plugins, Drivers, Channel logos, Kernel modules, System) come from the package name. The list has search, the filters Installed / Available / Updates with category chips, a bar "N updates → Update all" (one plan, one preview), one action per row and the details behind "⋯"; "Needed by Recordings" in plain words. The AI explanation is only an optional last entry in "⋯" when a key exists. The list of descriptions is incomplete until the box's feed is read (`go run ./cmd/pkginfo-missing <Packages file>` lists what is missing). Mock only so far.
- Apps page: the tabs are now **Packages**, **Extensions**, **Docker** in that fixed order, each opening with one plain sentence (Docker: "Optional: extra apps in containers — not needed for normal operation."). The page remembers the last tab you opened; old `?tab=apps` / `?tab=system` links lead to Docker / Packages. Package sources (feeds: on/off/add) moved behind a gear on Packages; Docker's setup moved behind a gear on Docker into **Docker settings**.
- New: **Uninstall Docker** in Docker settings — a preview first, then it stops and removes the containers DreamHub created, switches off Docker and DreamHub's Docker helper, stops loading the overlay module at boot and deletes the Docker files. "Also delete the app data?" is off by default, so apps can come back after a reinstall. Recordings and the TV software are not touched. Mock only so far — not yet tried on the box.
- Frontend toolchain moved to the current majors: Vite 8 with @vitejs/plugin-vue 6, Tailwind CSS 4 (configured in CSS, as the Vite plugin; Tailwind 3 look and hover behaviour kept), vue-tsc 3 with TypeScript 6, Pinia 4, Vue Router 5, plus minor bumps (Vue, vue-i18n, fonts). `npm audit` is clean again. Developing the frontend needs Node 20.19+ or 22.12+. No visible change; the UI was compared screen by screen (both themes, English and German, phone and desktop).
- Dashboard edit mode: every tile has "Move up" / "Move down" arrows (44px touch targets), so rearranging works on a phone; drag-and-drop still works with a mouse.
- The sidebar always shows plain page names (Recordings, TV Guide, Live TV, …) — the "Hub names / Descriptive" switch is gone, and no internal hub name (MediaHub, TimerHub, …) appears anywhere on screen, including problem texts and the "Needed by …" notes on packages. Settings and the setup wizard say "Features" instead of "Hubs"; "4 hubs need attention" is "4 things need attention"; the Settings "Hubs" tab is part of General. The navigation group "Device" is "Box"; Device → Administration is "Services & logs".
- Setup wizard: offered to an admin only while the box has no configuration, remembered on the box (new hidden setting `general.setupDone`) so a second phone doesn't get it again, never for viewers, full screen without menus. Its dead Channels step is gone. The sign-in screen and profile picker live at `/login`; PIN profiles show a lock.
- Status banners say what is wrong in a sentence instead of "Degraded" / "Unavailable"; a missing TMDB key is "Film details and ratings are off. Add a TMDB key in Settings to see them."
- Live TV: one "Watch what the TV shows" button instead of the "Same as the TV" pseudo-channel, player controls appear once something plays, channel rows say "Play".
- Channel list: the Check tab is part of Logos ("Channels without a logo", other findings folded below), "Find channels" is "All channels" with one summary sentence; channel logos are looked up by name and show initials when there is none, and no stream address with a password is ever put in a URL.
- TV Guide: "Films only" / "Rated 7+" only appear when TMDB is set up.
- Timers: one action visible per row (switch off/on, or delete), the rest under ⋯; status as a word, long titles wrap; AutoTimer explained in a sentence, "Dry run" is "Show what would be recorded".
- Dashboard edit mode: one way to reorder (drag), no unlabelled width steppers, "Remove from dashboard".
- Remote: the number keys behind a labelled "123" button. Radio: "In the browser" is the first choice, "Play on the box" the quieter second. Recordings rename: one sentence about the file name versus the title in the box's list instead of a checkbox. Notifications: the browser's "blocked" note only after you tried to switch them on.
- The change log, curated app descriptions and the Scripts tab are shown in the interface language; tap targets are at least 44 px; Settings, Device and Apps tab bars fade at the edge instead of clipping on a phone; "Reset to default" only shows when the value differs; byte sizes use a decimal only where it helps.
- Viewers no longer see Apps in the menu (a direct link says only admins can manage apps) and Recordings no longer asks the connectors for them (a 403).

### Fixed
- **Marked ad breaks never reached the player**: "Mark in cut list" wrote positions in the stream's raw clock (tens of thousands of seconds into a 45-minute recording); the player counts from the start of the recording. Found on the box, where the TV now skips a marked break by itself — checked by playing across one. Recordings marked with 0.4.0–0.6.0 carry such entries; mark them again (the old marks do no harm, the player never reaches them).
- A container that crashes at once and runs out of restarts got no notice in the inbox; it does now.
- About said © 2027; it is 2026.

## [0.6.0] - 2026-10-13

### Added
- eWeLink/Sonoff extension (`extensions/ewelink`, its own package `dreamhub-ext-ewelink`): Sonoff devices with LAN control on, shown and switched in AppsHub → Extensions without the eWeLink cloud, switching through the usual preview. Needs each device's key from the eWeLink account; the reference extension for anyone building one.
- AppsHub → System → Packages (was Device → Administration): packages a DreamHub hub uses (ffmpeg for AdScan and trimming, the overlay kernel module for Docker apps) are marked "Needed by …"; a "Needed by DreamHub" filter lists them, including any that are missing, with an install button; and removing one warns which hub stops working.
- AppsHub: apps made of several containers, installed, started, stopped and removed as one — the first is Nextcloud with its own MariaDB, which only listens on the box itself; passwords between them are generated at install and kept for a reinstall. A helper container shows under its app with its own logs.
- Settings → System → Backup: the app folders are listed one by one with their size and a checkbox — a database is in by default, bulk files (Nextcloud's, downloads) are not.

- Extension settings: an extension can declare a settings form (a device key, a name, a language); its card gets a Settings button, DreamHub renders and forwards it, secrets are never sent back. eWeLink uses it for the device key, so it is entered in the UI instead of a config file.
- AppsHub → System: package sources (feeds) can be switched on and off and a new one added, each through the usual preview with a standing warning that these feeds are unsigned; a source apt can't read is put back as it was.
- AppsHub install: choose the web port (for apps whose image can move it) and the disk to store the app on — the box's own flash is never offered; the choice is remembered on the container, so removing, backing up and restoring look in the right place.
- TimerHub: "Clean up finished" removes every finished one-off timer in one preview. PlexHub: "Open in Plex" on an item. MediaHub: the remote-control hint (Next/Previous jump past marked ad breaks) after marking in the cut list.

- ChannelHub: choose the logo of a single channel — from any installed logo set, your own picture, or the public TV-logos collection — from Logos or straight from the check's "Without a logo" list. DreamHub shows it everywhere; the TV is unchanged, and "Back to automatic" undoes it. The check's groups fold, and "Read the list again" now says what it does (reads the box's current list, no channel scan).

### Changed
- AppsHub is now three tabs with one sentence each — Apps (Docker containers), Extensions (DreamHub add-ons), System (the box's own packages and feeds, moved here from Device → Administration). Installed apps show Open and Start/Stop, the rest (logs, console, remove) in a menu; free Docker Hub search is folded away. Device → Advanced holds scripts and the terminal.
- Rename by scheme completes missing years and episode titles from TMDB by itself when a TMDB key is set — the hidden "+ TMDB" switch is gone (the year was never added unless it was switched on). Saving covers now says it makes them visible on the TV too.
- AppsHub's app list reworked: AdGuard Home, Pi-hole, Nextcloud, Vaultwarden, Syncthing, Home Assistant and qBittorrent, each with a short note for this box and its memory limit. Sonarr, Radarr, Prowlarr, Bazarr, Heimdall and WireGuard were removed (Heimdall always needs port 80, WireGuard extra kernel rights DreamHub doesn't give a container). Pi-hole, Vaultwarden and qBittorrent now use ports the box doesn't (8082, 8222, 8095), and every port an app needs — DNS 53 included — is checked before installing.

### Fixed
- qBittorrent was set to port 8090, which is DreamHub's own Docker helper; it now uses 8095.
- Restoring a backup could write an app folder outside AppsHub's storage directory if the file named one like `../x`; such names are now ignored.
- qBittorrent from the app list could never start on the box: its web port was 8080, DreamHub's own.
- The extension packaging template created its user on every install and never removed it on purge (DreamOS has no `getent`), and pointed at a `nologin` shell DreamOS doesn't have.

## [0.5.0] - 2026-10-12

### Added
- AppsHub: "Set up Docker" in the Apps tab — installs Docker (the official release, version and checksum pinned by DreamHub) with everything on `/data`, never on the root flash, plus the matching kernel module, and starts it; shown as a preview first. Apps run on the box's own network (its kernel can't do Docker's own), so each app keeps its own port and a taken one is refused by name.
- Device → Administration → Packages: install, update and remove packages, and "Update all". The preview is apt's own dry run — every package it would touch, including ones nobody picked (on the box, removing one small skin component would take two whole skins with it) — and says when a TV plugin or skin changes. Refused while the box records if enigma2 is touched; DreamHub's own package is left to the box's own tools. A "Not installed" filter browses what the feeds offer. apt's output is shown after applying, and the change log keeps each package's previous version.

## [0.4.0] - 2026-10-12

### Added
- DeviceHub ("Gerät"): SystemHub and AdminHub merged into one nav entry with role-gated tabs — device/disks/diagnostics, a script library (upload, store, run by hand with a confirm and a change-log entry), a terminal (`shellinaboxd` behind DreamHub's own auth, off by default, re-authentication required to turn it on), and a package browser (installed packages, feeds with a trust signal read from `X-DreamHub-Category`, update notifications, an AI explanation of what a package does on request).
- Extension Hub: a versioned, data-only plugin contract so niche or homelab-style features can ship as separate, optional packages — an extension registers over loopback and describes its own UI as structured widgets (status, meter, list, action) rendered with DreamHub's own components; action widgets go through the normal plan → apply flow. A `packaging/extension-template/` for building a future extension's own `.deb` (systemd unit with hardening, control file, postinst/prerm/postrm), and `X-DreamHub-Category` recognized by the feed list and package browser. Demo-only so far — nothing registers against it yet.
- AppsHub: the "Extension Hub" nav entry renamed and widened into "AppsHub" with an "Extensions" tab (unchanged) and a new, empty "Apps" tab — the shell for Docker container management (Unraid-style).
- AppsHub: a separate helper process (`dreamhub-dockerd-helper`) is the only thing that ever holds the Docker socket; DreamHub's own backend would reach it over loopback through a whitelisted subset (list/start/stop/logs/exec, scoped to DreamHub's own labelled containers) once a hub actually calls it. Not packaged or wired into anything the box runs yet.
- AppsHub: the "Apps" tab now shows a short, hand-picked list of self-hosted apps (Pi-hole, AdGuard Home, WireGuard, Syncthing, qBittorrent, Heimdall, Sonarr, Radarr, Prowlarr, Bazarr) with whether each one actually runs on this box's own processor, a search box to look up anything else on Docker Hub, and a "Check compatibility" button for a search result.
- AppsHub: a curated app can now actually be installed — "Install" shows exactly what will happen (the image, its storage location, up to how much RAM, which port) before anything runs, checked first against free memory and free disk space and refused if either is too tight. Installed apps get an "Open" link when they have a web interface, Start/Stop, and Remove (its own stored settings are kept, never deleted along with it). Free-search results still can't be installed, only checked for compatibility.
- AppsHub: an "Update available" badge when a newer image exists for an installed app (also sent to the notification inbox), a "Logs" button per app plus a link to the Docker service's own health in the existing device log viewer, and a "Console" — an interactive shell inside the app's container, with the same off-by-default/password-re-entry/standing-warning posture as the existing device terminal.
- Backup & restore: grew a general, opt-in "sections" mechanism beyond settings — AppsHub's own installed apps are the first, each app's own config included by default, its size shown before downloading.
- MediaHub: ad breaks found by AdScan can be written into the recording's own cut list ("Mark in cut list" in the detail dialog) so the box's own player skips them — the cut-list format is confirmed on the box; that the player really skips the marked ranges still needs a check with the TV.
- Settings: an "About" card (version, copyright, source link) next to Backup & restore.

### Fixed
- PlexHub: a stream that stalled mid-playback could hang forever instead of failing.
- AppsHub on a box without Docker: the Apps tab showed only an error and hid the catalog and search; it now says Docker isn't set up, keeps both usable and disables "Install".
- Backup & restore: on a box without Docker the settings page's backup card broke (an empty apps section came back as `null`).
- Uninstalling (`dpkg -r`) left the TV plugin's directory behind (enigma2's own compiled files), so the next enigma2 start showed a DreamHub launcher with no backend; it also left a dangling systemd link. Both are removed now.

## [0.3.0] - 2026-10-09

### Added
- Radio in the background: listening in the browser keeps going with the phone's screen locked, with the station, what's on and its logo on the lock screen, and play/pause/stop and previous/next station there. Pause and resume on the page too.
- Ad detection in recordings (first part): "Detect ads" in a recording's details or for a selection finds the ad breaks by the channel's logo, on the box, in the background — it waits while the box records. The details show the breaks on a timeline (uncertain ones marked), tap one to watch from there in the browser; the list shows "14 min ads". Nothing in the recording changes yet.
- Cutting the ads out: after detecting them, "Cut out the ads …" in a recording's details — certain breaks are ticked, uncertain ones are yours to decide — shows a preview and cuts in the background without re-encoding. The original keeps all its files and goes to the trash, so it can be restored. The detection results now stay with a recording when it's renamed, trashed or restored. The cut itself is confirmed to play back cleanly through the box's own movie player.
- Radio visualization in the spirit of Winamp: a spectrum with falling peaks, or an oscilloscope, or none — switched with one button, remembered per device, off by default when the system asks for less motion.
- Change log by profile: SystemHub shows who made each change and filters by profile or "DreamHub itself".
- Channel list (ChannelHub, admins): your bouquets and what's in them, every channel the box knows — search, "new since" (after a channel scan), "in no bouquet", TV/radio, HD, provider — a channel check (vanished after a scan, twice in a bouquet, no programme data, no logo), and the installed logo sets with how many of your channels they cover; DreamHub shows logos from the set you choose first. 
- Editing the channel list on the phone: move channels (buttons, or drag on a computer), put one on a given number, rename, remove, add from all the box's channels, add headings; create, rename and delete bouquets. Every edit is collected first and shown as one preview in words ("ZDF HD moves from 14 to 2"). Applying backs up the whole channel list first and checks the result; if anything differs, the backup is restored by itself. Earlier backups can be brought back. Needs the BouquetEditor plugin on the box. Confirmed on a real box with a throwaway test bouquet (create, add, reorder, delete) — nothing in an existing household's bouquets was touched in that test.

### Changed
- Errors in everyday words: "The box doesn't answer right now. Is it switched on?" instead of "DreamHub backend not reachable", with the technical code and request ID behind "Details". The reason is the real one — DreamHub, the box, Plex, the tuner, or the profile's permissions.
- Viewers' sidebar: no status dots, and hubs that can't work at all (e.g. Plex not set up) are left out instead of greyed.

### Fixed
- "Sidebar labels: hub names" did nothing anywhere; now the sidebar, search, notifications, status lists and page headings switch.
- Pressing the same remote key twice quickly reached the box only once.
- A confirmed change kept going if the phone locked its screen mid-apply (a channel-list change could stop halfway, its automatic restore too).
- Checking a new or deleted timer could read the box's list from before the change and report "not created" / "still there".
- Notifications: a DreamHub restart during a recording announced it as just started; zap timers counted as recordings; extending a running recording sent "finished" and "started"; a recording deleted while running was announced without its name.
- A viewer could delete a timer SerienRecorder or AutoTimer made, if it started on the same channel at the same time as one of the viewer's own deleted timers.
- The AI's daily limit could be passed by two requests at the same moment.
- The recording list could time out on a fuller disk (box requests may now take 15 s, not 5).
- The TV plugin's play endpoint only answers POST (any web page on the LAN could start playback through a GET).
- The setup QR code points to https behind a TLS reverse proxy.
- Go 1.27.2 for its HTTP and TLS fixes.
- ChannelHub's backup restore (the automatic rollback after a failed channel-list change) would have failed against the real device — the BouquetEditor plugin's backup and restore handle the file name completely differently, found by reading the plugin's own source off the box, fixed before any real channel-list write.
- ChannelHub: creating a bouquet always rolled itself back on a real device — the plugin appends " (TV)"/" (Radio)" to the name itself, which the check didn't expect. Found and fixed with the first real write on the box.
- ChannelHub: adding a channel with a name always rolled itself back on a real device too — the plugin bakes the name into the stored reference itself, which the result check compared against the clean one it was given. Found and fixed the same session.

## [0.2.0] - 2026-10-07

Includes everything listed under 0.1.9, which was installed on the reference box but never released on its own.

### Added
- Profiles: one per person, picked on each device ("Who's watching?") and remembered there; a profile can have a PIN. Settings → Profiles: your own PIN for everyone, managing profiles for admins. A household with a single profile without PIN sees no change.
- Roles: admin and viewer. Viewers watch live TV, recordings and Plex, use the guide and the remote, and plan recordings; they don't delete or rename recordings, change other people's timers, see settings beyond their own, or the system and admin pages. An admin can switch "plan recordings" and "rename and delete recordings" per viewer. The server enforces all of it, not just the screen.
- A viewer can delete or switch off the timers they made themselves.
- New profiles start with the language and appearance of whoever created them; the profile picker and login speak the device's last language.
- Start page for viewers: an admin designs it (Settings → Profiles → "Design the start page"); every viewer sees it until they change their own, and can go back to it any time in edit mode.
- Dashboard tiles: "Recent recordings" (one tap to watch in the browser), "Now on TV" (the former "Live TV" tile, now showing how far each programme has run), and Plex libraries as a source for custom tiles — recently added, "not seen yet", shown as a row of posters.

### Changed
- Setup hints (the box's clock, hubs missing a key, "Needs attention") are shown to admins only.

## [0.1.9] - not released separately (built 2026-10-06, part of 0.2.0)

### Added
- **Live TV** (StreamHub): TV channels and recordings right in the browser, on phones too, with no app. The box sends its stream unchanged — it can't convert anything — and the browser decodes it: FFmpeg compiled to WebAssembly for MPEG-2 video (SD channels) and MP2/AC3/AAC sound, the device's own decoder (WebCodecs) for H.264. Channel list with what's on, "Same as the TV", audio track choice, volume, full screen. A channel that needs the box's only tuner while it's busy elsewhere says so. In Recordings, "Watch in the browser" plays a recording with a seek bar.
- TV guide: "Watch in the browser" for a programme that's on now. Dashboard: an optional tile "Live TV" with your channels, one click to watch (switch it on in edit mode).
- Radio in the browser: a speaker button next to each station plays it right here, through Live TV's player.
- Plex: "Play on the box" resumes where you left off instead of always starting at the beginning, and reports playback progress back to Plex so "continue watching" stays current — best effort, via the stream proxy's own byte offsets rather than a position read from the box (#30). Films in several parts or versions get a picker in the details dialog instead of always playing the first (#31).
- GuideHub and DiscoveryHub mark a broadcast "Already in Plex" when a film with the same title is already in the library (#32).

### Fixed
- TV guide: "Already in Plex" read and decoded the whole Plex library once per broadcast — a 26-channel, 4-hour grid took 4.2 s on the reference box instead of 0.1 s. The library is now read once per request.
- Live TV: the limit of two streams at once could be bypassed by two requests arriving together.
- TV plugin: a malformed resume position no longer keeps the play call from answering.

## [0.1.8] - 2026-10-06

### Added
- Plex library: the title's logo from Plex (its "clearLogo", transparent PNG) above a film's details and on a series page — no fanart.tv key needed.
- Plex: the server's other addresses (Settings → Services → Plex, "The server's other addresses"). Signing in with Plex fills them in from plex.tv's list, the home-network one first; when the address in use stops answering, DreamHub switches to the next one that answers as the same server. Found on the reference box: after a reboot the server's public address didn't answer the box for over an hour while the server itself was fine.
- SeriesHub: a broadcast shown on several channels at once (RTL's seven regional variants) is listed once, with "also on 6 more channels"; the one kept is the channel a timer or the series' recordings use.

### Changed
- Plex library: "Play on the box" is no longer marked experimental — confirmed on the reference box (movie player, seeking, pause).
- DiscoveryHub: the broadcasts sent to the AI are picked fairly across the bouquet's channels, prime time first. The box lists the EPG channel by channel, so the cap of 300 used to drop a large bouquet's last channels entirely.
- CI: current major versions of GitHub's actions, Node 22.

### Fixed
- SeriesHub: missing episodes of a daily soap were never found. SerienRecorder numbers it absolutely ("S00E8622"), TVmaze files it under years but counts the same way (episode 8622) — confirmed for "Gute Zeiten, schlechte Zeiten" on the reference box. When the numbers and episode titles agree, episodes are matched by number, broadcasts by their episode title, and only episodes from the first recording on count as missing.
- SeriesHub: the list of further broadcasts disappeared exactly when "missing episodes air soon" was shown.

## [0.1.7] - 2026-10-05

### Fixed
- Plex library: a film played on the box opened like a channel — TV infobar, no seeking or pause. WebControl's zap only opens the movie player for files on the box. DreamHub's TV plugin now offers `/dreamhub/play` on the box's web server, which opens the movie player like a recording from the movie list; with an older plugin DreamHub falls back to the plain zap.

## [0.1.6] - 2026-10-05

### Fixed
- Plex library: opening a film or series said "the Plex server doesn't answer" although it did. Newer Plex servers send a list of rating sources ("Rating") next to the rating number ("rating"), and the JSON reader matches names regardless of case, so the whole answer failed. That failure was then taken for "offline", sending every read to the cache for a minute. Ratings are read tolerantly now, only a missing connection or a refused token counts as offline, and an answer that can't be read gets its own message and a log line.

## [0.1.5] - 2026-10-05

### Added
- **Plex library** (PlexHub): your Plex server's films and series as tiles or a list, filtered and sorted; series pages with seasons and episodes. Watched state is Plex's own — one film or episode with Undo, a whole season or series after a preview. Films play on the box straight from the server with Enigma2's own player (experimental). Connect with a code at plex.tv/link (any server, any address); while the server is off, the last known library stays visible. Search finds Plex titles; an optional dashboard tile shows "Continue watching".

### Changed
- Settings are split into tabs: General, Recordings, Services, Notifications, Hubs, Access, System. Each tab has its own address (`/settings/services`); old `/settings#tmdb` links still work. Services shows each connector's state; System now holds backup & restore (moved from SystemHub), Diagnostics and the setup wizard. Settings are findable with Ctrl+K.
- MediaHub marks a recording the box is writing or playing right now ("Recording" / "Playing on the TV") and offers no file actions for it; the dashboard's timer tile marks a running timer. Found during a live recording on the box: the refusals were right, the list just didn't say so up front.

### Fixed
- TimerHub: the state column lines up for timers that can't be toggled (a running one).

## [0.1.0] - 2026-10-04

First release. Packages: `dreamhub_0.1.0_arm64.deb` (Dreambox One/Two — tested on a Dreambox One, DreamOS 2.6) and `dreamhub_0.1.0_armhf.deb` (DM900/DM920 — built, not yet tested on such a box). Install and first steps: `docs/USER_GUIDE.md`.

### What's in it
- **Core**: dashboard of tiles ("Now on the box", timers, conflicts, disk space, episodes, custom saved views), global search over every hub, notifications with push to the phone through ntfy, schema-driven settings with a jump row, setup assistant, sign-in and API tokens, change log of every change, backup/restore, German/Italian/English, dark and light. Installable as an app (manifest, service worker over HTTPS; iOS home screen over plain http); says "can't reach DreamHub" instead of asking for a password when the box is off.
- **MediaHub**: recordings by folder, disk space, rename with a preview of every file, change the title, naming schemes and batch rename, sorting into folders, saving covers next to recordings, trash with restore, details with ratings and "play on the TV".
- **SeriesHub**: SerienRecorder's followed series, a page per series with the episode guide (TVmaze), missing episodes and which broadcast is which episode; push when a missing one airs within 24 h.
- **GuideHub**: grid, now & next, tonight and one channel; films-only and rated-7+ filters; channel logos from the box's picon sets; record (preview) or switch from any programme.
- **RadioHub**: radio stations with what's on now and next, the one playing on top, play on the box, record a programme, radio recordings coming up; "All radio stations" when the radio favourites are empty.
- **TimerHub**: timers by day with conflict estimate, delete, disable/enable, AutoTimer list and dry run.
- **DiscoveryHub**: AI picks from the EPG (OpenAI), optionally matched to your taste, kept for the day, one tap to a timer.
- **ControlHub**: a remote with the live picture, as a panel on every page, keyboard shortcuts, messages to the TV.
- **SystemHub**: device info, diagnostics, the **box self-test** (asks the box everything DreamHub needs, read-only, rates each answer, keeps the raw answers as a zip).
- **AdminHub**: the box's services with restart through preview, log level, logs with live tail, enigma2's crash logs.
- **Enrichment**: the box's own cover art and DreamMediaInfo first, then TMDB (posters, overviews), OMDb (Rotten Tomatoes, IMDb, Metascore), fanart.tv (logos).
- **TV plugin**: a launcher in the Plugin Browser with a QR code to open DreamHub on the phone.

### Fixed on the way to the release (found by the self-test on the reference box)
- Picons: `/data/piconpool` holds picon sets in subfolders — they're searched now (none were found before).
- RadioHub: the radio favourites were empty on the reference box — "All radio stations" (without data services, Enigma2's made-up names and duplicate transponders) is used then.
- AdminHub: the unit names on DreamOS 2.6 (`smb`, `nmb`, `zerotierone`, `oscam-cycecplus`), and systemd 232 failing `systemctl show` when asked for `NRestarts`.
- The box's WebInterface logged a traceback whenever the EPG of an empty bouquet was asked for — DreamHub no longer asks.

### Development history before the release

#### Added (ControlHub, pulled forward from v1.1)
- While visually verifying WP8's Enigma2 plugin, found the WebInterface plugin's real screenshot (`/screenshot`, native `ePixmap` capture) and remote-control (`/web/remotecontrol`, `/web/zap`, `/web/powerstate`) endpoints — genuinely useful beyond that one test, so ControlHub (originally scoped for v1.1) was built now instead of later (decision D8, `docs/archive/V1_PLAN.md`).
- `webcontrol.Client` gains `Screenshot`, `SendKey`, `Zap`, `InStandby`. `GET`/`POST /api/v1/control/*` (`internal/hubs/control.go`) exposes them as direct, immediate actions — not preview-and-confirm, since a virtual remote is inherently a live input device, not a durable data mutation.
- `ControlHubView.vue`: a live screenshot with manual/auto refresh, a standby banner with a wake button, and a virtual remote pad. GuideHub gets a "switch now" button on whatever's airing right now, calling ControlHub's zap directly.

#### Added (v1 plan, WP9: packaging and CI)
- `packaging/build-deb.sh` builds `dreamhub_<version>_arm64.deb`: the cross-compiled backend, `dreamhub.service`, an avahi mDNS service file, and the Enigma2 plugin with its `.po` files pre-compiled to `.mo`. Verified locally via `dpkg-deb --info`/`--contents`; not yet installed live on the reference box.
- `.github/workflows/ci.yml`: backend `go vet`/`go test -race`, frontend type-check/build, then the `.deb` itself, on every push — uploaded as a workflow artifact. Publishing it somewhere a device could actually `apt` from is still open (decision D4).
- `postinst` restarts `enigma2` so the launcher plugin takes effect immediately (same as installing any other Extensions plugin); `postrm` only deletes `/data/dreamhub` on `purge`, never on a plain removal.

#### Confirmed live (v1 plan, WP8)
- The Enigma2 launcher plugin installed on the reference box and enigma2 restarted cleanly: no traceback anywhere in the boot journal, the service came back within 15s, and Python's own bytecode cache (`.pyo`) shows the boot-time plugin scan really did import and run the plugin's registration and gettext/config setup.
- **Visually confirmed afterwards**: discovered the WebInterface plugin's real screenshot endpoint (`/screenshot`, not the unrelated `/grab`) and used it together with `/web/remotecontrol` key simulation to actually navigate to and open the plugin. It shows correctly in Enigma2's standard Plugin Browser with its gettext-translated German description; opening it renders the real screen (QR code, buttons) and the backend's own log shows the exact `GET /api/v1/health`/`GET /api/v1/qr.png` calls landing at that moment. It doesn't appear in this image's custom "Gemini BluePanel" quick-launcher — that turned out to use its own separate category list, not a DreamHub issue.
- Found, root-caused and fixed a real bug this surfaced: the status text showed "backend not reachable" despite the backend log proving the call succeeded, because `gotHealth`'s bare `except Exception` swallowed the actual error. Logging it instead of swallowing it surfaced the real cause on the next attempt: `json.loads()` returns Python 2 `unicode`, and Enigma2's SWIG-wrapped `eLabel.setText()` needs a plain `str` — raises `TypeError` on `unicode`. Fixed with a small `_toStr()` helper; **re-verified live**, the screen now correctly shows "Backend läuft (v0.1.0-dev)".

#### Added (v1 plan, WP8: Enigma2 launcher plugin; WP7: event bus, SSE live updates)
- The core event bus (`internal/events`): a poller diffs `/web/timerlist`/`/web/deviceinfo` snapshots into `recording.started`/`recording.finished`/`disk.aboveThreshold` events (disk with hysteresis so it can't spam one line every poll while hovering right at 90%), persisted and fanned out live. `GET /api/v1/notifications` (+ mark-read) and `GET /api/v1/events/stream` (SSE). `NotificationsBell.vue`'s "Recent activity" is now this real, persisted inbox instead of change-log entries filtered down to one type.
- Live updates over Server-Sent Events rather than a literal WebSocket — see `api.handleEventsStream`'s doc comment for why (server→client only, no new dependency, and the frontend needs its own stream reader either way since DreamHub's auth header isn't something the browser's native `EventSource` can send). AdminHub's log viewer gets a "Live" toggle on top of this (`GET /api/v1/admin/logs/stream`).
- Found and fixed while building the SSE stream: `httpx`'s request-logging middleware wrapped every response in a type that silently dropped `http.Flusher`, which would have made any streaming response hang indefinitely in production despite passing in Go's own `httptest` environment used by earlier tests.
- The Enigma2 launcher plugin (`enigma2-plugin/`): a QR code (`GET /api/v1/qr.png`, rendered server-side via `github.com/skip2/go-qrcode` so the Python 2.7 side needs no QR library), backend status/version with a mismatch warning, and basic settings (port, whether the backend expects a password). Written and syntax-verified against the reference box's own Python 2.7.13, but not yet installed into a live Enigma2 GUI session — see the module's README for the exact gap.

#### Added (v1 plan, WP6: AdminHub logs; WP7: request coalescing)
- AdminHub screen: DreamHub's own runtime log level (debug for 60 minutes, back to info), a journald log viewer filtered and bounded server-side (unit, minimum severity, text — `internal/journald`, real `journalctl -o json` client + mock), and enigma2's crash logs with view-in-browser and secrets redacted on read. The journal is stated as "since last boot only" — it's RAM-backed and cleared on every reboot (`ARCHITECTURE.md`).
- Live tail over WebSocket is still open (needs WP7's event bus); the screen reads a bounded snapshot with its own refresh button for now.
- Diagnostics folded into SystemHub as a second tab instead of its own top-level page — `CONCEPT.md` already described it that way. Old `/diagnostics` links redirect to `/system?tab=diagnostics`.
- `webcontrol.HTTPClient` now coalesces concurrent identical `GET`s: several callers asking for the same path at once (e.g. dashboard tiles loading together) make one real request to the box instead of one each, addressing enigma2's single-threaded request handling. Deliberately not a time-based cache on top of that — it would also apply to a plan's own apply-time freshness check and could silently weaken preview-and-confirm's staleness guarantee.

#### Added (v1 plan, WP5: SystemHub)
- SystemHub screen: device info (`GET /api/v1/deviceinfo`), a link into the existing Diagnostics page rather than a second copy of it, and the change log with a client-side filter by action/summary.
- Backup and restore of DreamHub's own state (settings and the dashboard layout) as one JSON file, through preview-and-confirm like any other change: `GET /api/v1/system/backup` downloads it, `POST /api/v1/system/backup/restore` previews exactly which settings and whether the dashboard would change, `POST /api/v1/plans/{id}/apply` applies it. API keys and passwords are never included in a backup — they're excluded at export, not just masked, so the file itself can't unlock the box.

#### Added / changed (v1 plan, phase A — WP1: correct box writes)
- Timer writes verified live on the reference box and **enabled by default** in live mode (`DREAMHUB_ENABLE_DEVICE_WRITES=0` switches them off). Writes are form-encoded `POST`s with every parameter. Boxes with the WebInterface's default `anti_hijack` refuse `GET`, and a missing parameter crashes the plugin.
- Every apply is verified against `/web/timerlist`, because the box reports a silently dropped duplicate as success (`409 timer_not_created`). A refusal by the box, e.g. its own tuner conflict check, comes back as `409 box_rejected` with the box's text.
- Delete timers through preview-and-confirm (`POST /api/v1/timers/delete/plan`). The preview warns when the timer is recording right now, since deleting it stops the recording.
- Conflict warnings are now a transponder- and delivery-system-aware estimate (`internal/tuner`), and multi-tuner, FBC and mixed sat/cable boxes are covered by tests. On a single tuner, two channels on the same transponder are no longer flagged. The duplicate warning follows the box's own rule (same channel, same start).
- Optional box login (Settings → WebControl) for boxes that enforce auth for DreamHub. It's used for SerienRecorder's API too. A `401` shows as "asks for a password" instead of "unreachable".
- A SerienRecorder that isn't installed now shows as `not_installed` with an install hint instead of a connection problem. Optional plugins never break DreamHub (`docs/archive/V1_PLAN.md`).
- `docs/archive/V1_PLAN.md`: the plan for v1 (work packages, open decisions, the tuner proposal).

#### Added / fixed (v1 plan, WP4: MediaHub files)
- MediaHub browses **every** recording folder. Before, only the top folder was visible (36 of 773 recordings on the reference box), because the box crashes on the subfolder part of `/web/movielist` without an explicit `dirname`.
- Fixed: a `None` file size (a manually copied `.mkv`) made the listing lose that entry and everything after it.
- Rename a recording: every file shown before → after, all or nothing. Move to trash, single or bulk, in AdvancedMovieSelection's `.ts.trash` convention, so it's the same trash the TV shows; it works without AMS too. Restore, and delete for good with the total size named.
- Nothing is touched while it's being recorded or played; this is checked at preview and again at apply. File operations only run when DreamHub is on the box, and `DREAMHUB_ENABLE_DEVICE_WRITES=0` switches them off too.
- Sizes are shown in decimal units (GB), as Enigma2 reports them.
- Name suggestions for renaming, from three sources: the recording's own data (always), TMDB (with a key, in the UI language, episode titles for series) and the AI (with a key, counts against the daily AI limit). DreamHub always builds the name itself (`Title - S01E07 - Episode` / `Title (Year)`, SMB-safe); the AI never writes a file name.
- Verified live on the reference box (approved, reverted): rename and back, trash and restore of a real recording; file list byte-identical afterwards.

#### Added (v1 plan, WP3: TimerHub)
- TimerHub screen: timers by day, state, source (SerienRecorder/AutoTimer, from the box's tags), a likely-conflict marker, and deleting through preview-and-confirm.
- Record a programme straight from the TV guide grid. The preview shows the recording window including margins.
- GuideHub search: global search finds programmes through the box's own EPG search, and a result opens the grid at that time.
- AutoTimer (optional plugin), read-only: its rules, its own dry run, and a link to its editor. Without the plugin TimerHub stays fully available.
- Dashboard tile "Conflicts". A shared inline preview component (`PlanPreview.vue`). A refusal by the box now shows the box's own reason everywhere, DiscoveryHub included.
- GuideHub's timeline positions use logical properties (RTL-ready).

#### Fixed (v1 plan, phase A — WP2: first run on the box)
- GuideHub's grid shipped the whole EPG (~25 days, about 2 MB per page): `/web/epgmulti`'s `endTime` is a duration in minutes, not an end timestamp. Now 52 KB and 0.1 s for a 4-hour page.
- DreamHub's data directory and SQLite's `-wal`/`-shm` files were world-readable. It now sets umask `077` at startup.

#### Added (DiscoveryHub, Diagnostics, time zones, keyboard shortcuts, step 10)
- DiscoveryHub: AI recommendations from a chosen bouquet's EPG (`GET /api/v1/discovery/status`, `POST /api/v1/discovery/recommendations`). New `internal/ai` package (real OpenAI client + mock) — grounded in real EPG data by index, not free-text, so the model can't hallucinate a service reference that would create a timer for the wrong program. Daily budget (`ai.dailyLimit`) now actually enforced via a new `ai_usage` store table (previously an unenforced setting). "Create timer" is a plain call to TimerHub's own plan/apply endpoints — DreamHub's first real preview-and-confirm UI, showing conflict/duplicate warnings before confirming. Nothing calls the AI automatically; generating is always an explicit, budget-visible click.
- Diagnostics page (`/diagnostics`): every hub the user may see, including disabled ones, grouped by Unavailable/Degraded/Disabled/Available with the same translated problem/fix text as everywhere else — no new backend, `GET /api/v1/hubs` already existed for exactly this.
- Time zones: the long-existing per-user display-zone setting (box/browser/fixed) is now actually used everywhere a time is shown, via a new `useDisplayTimeZone` composable; `GET /api/v1/health` gained `serverTimeZone` (read from `/etc/localtime`) so "box" mode has something to resolve to.
- Keyboard shortcuts: `g` + letter global navigation to every hub and Settings.

#### Added (GuideHub and SeriesHub, step 9)
- GuideHub: a real TV-magazine-style timeline grid, not just now/next — `GET /api/v1/guide/bouquets`, `GET /api/v1/guide/grid?bouquet=&start=&end=` (`/web/epgmulti`, a genuine time window per channel), grouped server-side. `GuideHubView.vue`: a proportional timeline with an hour ruler and a "now" line, switchable bouquet and switchable time window (◀ / Now / ▶). Verified live against the reference device across two real bouquets.
- SeriesHub: a new `internal/serienrecorder` package (real HTTP+JSON client and mock, mirroring `internal/webcontrol`'s pattern) talks to SerienRecorder's own API, confirmed reachable in the previous device-access session (#10). Tracked series and upcoming episodes (`GET /api/v1/series/markers`, `GET /api/v1/series/upcoming`), `SeriesHubView.vue`. The SerienRecorder connector now does a real reachability check instead of unconditionally reporting "not implemented" on a real device. A link to SerienRecorder's own web GUI ("open in new tab" or an inline, lazy-mounted iframe) rather than rebuilding its marker/timer editing — verified live, the real app loads and works.
- Both hubs are search providers; a new dashboard tile shows upcoming episodes (`series.upcoming`).

#### Added (MediaHub read-only screen, device-access investigation, step 8)
- Device access session 2026-09-24: the reference Dreambox came back online after an extended gap. Captured real `/web/*` and SerienRecorder API responses as test fixtures (`backend/testdata/fixtures/`); resolved all remaining `device-access`-labeled roadmap items (#10–#18) — see `ARCHITECTURE.md` and GitHub issue #19 for details.
- MediaHub, first real screens (`GET /api/v1/media/movies`, `GET /api/v1/media/disk`, `MediaHubView.vue`): recordings list with real vs. manually-copied media distinguished (`isRecording`), disk usage overview (WebControl's human-formatted HDD sizes parsed into a percentage via `webcontrol.ParseSize`). Read-only — rename/delete need their own preview-and-confirm design (`ROADMAP.md`).
- MediaHub is the hub contract's second search provider (after TimerHub); a new dashboard tile shows disk usage.
- Confirmed live against the reference device (#11): Enigma2 picks up recordings renamed outside Enigma2 correctly.

#### Added (API tokens, hub contract, status strip, step 7)
- API tokens: generate (secret shown once), list, revoke (`internal/apitoken`, `GET`/`POST /api/v1/tokens`, `DELETE /api/v1/tokens/{id}`). `Authorization: Bearer dh_...` now works as an alternative to Basic Auth on any endpoint. Managed from Settings → Authentication.
- Hub contract, first real implementations: search providers (`internal/hub/search.go` — TimerHub is the first) power real hub-content results in the search palette; status-change events (`internal/hub/watcher.go`) write a `hub.statusChanged` entry to the change log whenever a hub's level actually transitions, surfaced as "Recent activity" in the notifications bell.
- Status strip under the top bar: connected, clock skew, hub issue count, and the browser's time zone — replaces the earlier ad hoc "backend unreachable" banner.

#### Added (login, search, notifications, onboarding, step 6)
- Login screen: the app manages Basic Auth itself (`GET /api/v1/whoami`), since only `/api/v1/*` sits behind auth, not the SPA shell, so the browser never had a native prompt to react to. Fixes the gap found while testing step 5's settings menu.
- Global search: a `Cmd/Ctrl+K` / `/` command palette over the sidebar's pages. Per-hub content search is still open, pending hub search providers.
- Notifications: the top bar bell shows current issues (degraded/unavailable hubs, box clock skew) — a live snapshot, not yet the persisted event history CONCEPT.md describes.
- Onboarding wizard: language, sign-in, device detection + clock check, hub enable switches, AI key (skippable), channels (honest "not built yet"), and a finish screen with a QR code. Runs once automatically, re-runnable from Settings.

#### Added (settings menu & dashboard tiles, step 5)
- Dashboards API (`internal/dashboard`, `GET`/`PUT /api/v1/dashboard`): a fixed catalog of built-in tile types backed by existing endpoints, reconciled on read/write, saved directly and logged like settings.
- Dashboard tiles: show/hide, drag-and-drop reorder, width 1–4, persisted per user; two real tiles — upcoming timers and hubs needing attention.
- Settings menu rendered generically from `GET /api/v1/settings` — every section and field, per-type controls, per-field validation errors, reset-to-default, per-section save; saving language/appearance/sidebar labels here updates the running UI immediately, not only after a reload.
- Found while testing: enabling Basic Auth from the new settings menu has no way to actually sign back in from the web UI yet (tracked in `ROADMAP.md`).

#### Added (frontend, step 4)
- Frontend scaffold (Vue 3 + TypeScript + Tailwind + vue-i18n + Pinia, `frontend/`), built into and embedded by the Go backend via `go:embed` with an SPA fallback for client-side routes (`backend/internal/webui`).
- Core shell per `DESIGN.md`: design tokens, bundled Source Sans 3 / Source Code Pro, a self-drawn icon set, collapsible sidebar with a drawer below 900px, top bar with search/notifications shown as explained-not-built placeholders.
- Sidebar driven entirely by `GET /api/v1/navigation`, including translated hub status/problem/fix explanations and the descriptive/hub label switch.
- Dashboard route with the tokened responsive tile grid and an honest empty state; a placeholder route per hub showing its live status.
- i18n: bundled German/Italian/English locales, language switcher, appearance (dark/light/system) and sidebar label mode wired to real settings via `PATCH /api/v1/settings`.

#### Added (backend foundation, step 3c)
- Preview-and-confirm: single-use, expiring, owner-bound plans with stale detection; `POST /api/v1/plans/{id}/apply`; applied plans recorded in the change log.
- TimerHub: list timers and preview a new one with margins, single-tuner-aware conflict warnings and duplicate detection (`/api/v1/timers`, `/api/v1/timers/plan`).
- SystemHub change log (`/api/v1/changes`) and AdminHub runtime log level (`/api/v1/admin/log-level`, admin only, debug reverts automatically).
- CSRF protection (`X-DreamHub-Request` header, foreign origins rejected), Host-header check against DNS rebinding (`network.allowedHosts`), request IDs in headers, error bodies and logs.
- Writing to a real box is implemented but off by default until the endpoint is verified on the device (`DREAMHUB_ENABLE_DEVICE_WRITES=1`).

#### Added (backend foundation, step 3b)
- Connector registry with cached, time-limited checks: WebControl, SerienRecorder (mock only for now), OpenAI and TMDB with their own settings sections.
- Hub registry with the v1 hubs, requirements and status evaluation (available / degraded / unavailable, with problem and fix codes), plus an enable switch per hub.
- `GET /api/v1/navigation`, `GET /api/v1/hubs`, `GET /api/v1/connectors`.

#### Added (backend foundation, step 3a)
- SQLite store (pure Go, statically cross-compilable): users with roles, global and per-user settings, multiple dashboards per user in the data model, change log; database file restricted to the owner.
- Schema-driven settings with typed fields, validation codes per field, cross-field checks, masked secrets, bcrypt-hashed passwords; `GET`/`PATCH /api/v1/settings`.
- Auth is now configured through settings (Basic Auth with bcrypt, cached successful checks) instead of an environment variable.
- `/api/v1/health` reports the server time in UTC.

#### Changed
- Concept extended: customizable tile dashboard (decided over a timeline), sidebar navigation, AdminHub (logs in v1, terminal later), roles and multiple dashboards in the data model, internationalization (DE/IT/EN, drop-in locales, RTL-ready), time zone handling (UTC inside), UX direction ("good-looking work tool", desktop-first, Source Sans 3 bundled, dark by default), LAN security (CSRF, DNS rebinding).
- Product restructured into a core (shell), hubs and connectors — see `CONCEPT.md`. `ROADMAP.md` is now organized by hub and release (Foundation / v1 / v1.1 / Later) instead of Phase 0–8.
- New principles: preview and confirm for every change (two-step `plan` → `apply` API, change log, trash instead of delete), schema-driven central settings, hub status with fix instructions, AI provider as a swappable connector, profiles-ready data model.

#### Fixed
- `/api/v1/health` is now public when auth is enabled, so the TV plugin's version check works without credentials. Added tests for Basic Auth and API tokens.

#### Added
- Backend scaffold (Go): `/api/v1` router with health/deviceinfo/movies endpoints, JSON error envelope, structured logging, Basic Auth + API token middleware, environment-based config, and a WebControl client with both a real HTTP/XML implementation and a mock backed by fixtures from `ARCHITECTURE.md`. Tests run entirely against the mock — no device required. Confirmed cross-compiling to a static ARM64 Linux binary.

Still pre-release — see `PROJECT_PLAN.md`, `ARCHITECTURE.md`, `PRINCIPLES.md`, `ROADMAP.md` for full context.
