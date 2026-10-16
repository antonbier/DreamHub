# DreamHub user guide

DreamHub is a web interface for **Dreamboxes running DreamOS** — developed and tested on a Dreambox One; see *What you need* for the others. It runs on the box itself, next to the box's own WebControl, and you use it from a browser — on your phone, tablet or computer. This guide covers installing it, the first steps, and what each part does.

DreamHub changes nothing on its own: every change to recordings, timers or settings on the box is shown to you first ("Preview · nothing changed yet") and only happens when you confirm it. Every applied change is listed under **Device → Overview → Change log**.

---

## 1. Install

### What you need

- A Dreambox running DreamOS:
  - **Dreambox One or Two** (ARM64) — package `dreamhub_<version>_arm64.deb`. Developed and tested on a Dreambox One with DreamOS 2.6 (Gemini/AIO image).
  - **DM900 UHD or DM920 UHD** (ARMv7) — package `dreamhub_<version>_armhf.deb`. Built the same way, but not yet tested on such a box: feedback welcome.
  - The MIPS boxes (DM7080, DM820, DM520, DM525) aren't supported yet, older Dreamboxes without DreamOS not at all.
- The box on your home network. DreamHub uses port **8080**; WebControl stays on port 80.
- About 15 MB on the root partition for the program; its database lives on `/data`.

### Today: install the package by hand

There is no package feed to subscribe to yet (see *Later: from a feed* below), so for now the `.deb` package is installed directly.

1. **Get the package** for your box from the latest release: [github.com/antonbier/DreamHub/releases](https://github.com/antonbier/DreamHub/releases) — `…_arm64.deb` for a Dreambox One/Two, `…_armhf.deb` for a DM900/DM920. (In-between builds of every change are on the repository's **Actions** tab, artifact **`dreamhub-deb`**, for anyone signed in to GitHub.)
2. **Copy it to the box**, for example to `/tmp/` — over the box's network share (Samba), with an SCP program such as WinSCP or Cyberduck (user `root`), or from a USB stick.
3. **Install it.** Open a shell on the box. No SSH client is needed: the box's own **WebAdmin** has a terminal in the browser at `http://<box>/webadmin/terminal/` (log in as `root`). Then run:

   ```sh
   dpkg -i /tmp/dreamhub_*.deb
   ```

   This installs the DreamHub service and starts it, and adds the **DreamHub** plugin to the TV's plugin list. The TV's interface (enigma2) restarts once, so the plugin appears right away; after later updates it only restarts again if the plugin itself changed. A recording running at that moment would be cut, so install when nothing is recording.
4. **Check it:** open DreamHub (next section), then **Device → Diagnostics → Run self-test**. It asks the box everything DreamHub needs and should come back all green.

### Later: from a feed

Once DreamHub is published in its own package feed, installing and updating will work without copying files: in WebAdmin's feed management, use **"Upload new \*-feed.list"** to add DreamHub's feed file, then install `dreamhub` like any other plugin. This guide will name the feed file when it exists.

### Updating

Install the newer `.deb` the same way (`dpkg -i …`). Your settings, keys and dashboard are kept: they live in `/data/dreamhub`, which an update never touches.

### Uninstalling

```sh
dpkg -r dreamhub      # removes the program, keeps your settings in /data/dreamhub
dpkg -P dreamhub      # also deletes /data/dreamhub — settings, keys, change log
```

---

## 2. First steps

### Open DreamHub

Any of these works:

- **From the TV:** open the plugin list (or the blue Extensions menu), choose **DreamHub**, and scan the QR code with your phone.
- **By name:** `http://<box name>.local:8080` — the box announces itself on the network (mDNS). Most phones and computers find it this way.
- **By address:** `http://<box IP>:8080`, e.g. `http://192.168.1.43:8080`.

Tip: put it on your phone's home screen — it then opens like an app. See [section 4](#4-dreamhub-as-an-app).

### The setup assistant

The first visit starts a short assistant: language, connection to the box, a password for DreamHub, which hubs to show, your channel list, and optionally an AI key. Everything in it can be changed later under **Settings**, and **Settings → System → Run setup wizard** starts it again. Settings are split into tabs — General, Recordings, Services, Notifications, Hubs, Access, System — and any setting can be found with **Ctrl+K**.

### Profiles: one per person

Under **Settings → Profiles** an admin adds a profile for each person in the household. Each device asks once "Who's watching?" and remembers the answer; a profile can have a PIN (admins need one as soon as there's more than one profile). **Viewers** watch live TV, recordings and Plex, use the guide and the remote, and plan recordings — they don't rename or delete recordings, change other people's timers, or see the settings beyond their own. Per viewer, an admin can allow "plan recordings" and "rename and delete recordings". **Design the start page** sets the start page every viewer sees until they change their own.

### More than one box

Settings → **Boxes** (admins) adds another Dreambox by its address and, if its web interface asks for one, its login; **Test** tells you whether it answers and whether the login is right. With two or more boxes a **box selector** appears in the top bar, and the start page shows a slim strip with what the other boxes are doing — tap one to switch. Everything below (TV guide, timers, recordings, remote) is then about the selected box, and the address says which (`/b/living-room/guide`), so bookmarks work. DreamHub remembers your last box on each device. Live TV and recordings of another box are streamed through DreamHub, so the box's own login never reaches your browser. Notifications (the bell, and push on your phone) say which box they are about; the bell has an **Only this box** switch for this device.

A box added this way is controlled through its web interface: live TV, guide, timers, channel lists, the remote and watching its recordings. Managing recordings in place, logos, apps and the system tools need DreamHub on that box itself. A box that doesn't answer is greyed in the selector and the status line says so — DreamHub never waits for it. In a profile's settings you can limit which boxes it may use (nothing ticked means all). Removing a box makes DreamHub forget it and what it saved for it; the box itself is not touched.

If a box runs DreamHub itself, you can **pair** the two: on that box open Settings → Boxes → *Pair this box with another DreamHub* and show the code (it also appears on that box's TV); in this DreamHub open the box under Settings → Boxes → Edit and type the code under *DreamHub on this box*. The code works once and for ten minutes. DreamHub then knows the other one, shows its version and tells you if one of the two needs an update. To end a pairing, choose **Unpair** and delete the token named "Hub: …" on the box under Settings → Access. A paired box can also play Plex films; the box fetches them from this DreamHub's address, which is found automatically (Advanced lets you set it).

### API and tokens

For apps and scripts (a DreamDroid-style app, Home Assistant, your own tools): **Settings → Access → API tokens** creates a token — shown once, revocable on its own, acting as the profile that created it (so it sees that profile's boxes and permissions). Send it with every request:

```
curl -H "Authorization: Bearer <token>" "http://dreambox.local:8080/api/v1/boxes"
curl -H "Authorization: Bearer <token>" "http://dreambox.local:8080/api/v1/timers?box=bedroom"
```

- **`box=<id>`** says which box a request is about; `GET /api/v1/boxes` lists the IDs you may use, with what each box can do (`capabilities`). Without it the first box you may use is meant, so a script written for one box keeps working when a second one is added.
- **Changes take two steps.** A call ending in `/plan` changes nothing and returns a preview with an `id`; `POST /api/v1/plans/<id>/apply` makes the change on the box the plan was made for. A plan is valid for ten minutes and once. Requests that change something also need the header `X-DreamHub-Request: 1`.
- **The full description** of every call is `/api/v1/openapi.json` (linked under the token list); load it into a tool such as Swagger UI or a code generator. Request and response shapes are filled in for the most useful calls first (status, boxes, timers, recordings, zap); the rest is listed with what it does.

### Recommended: protect your network access

- **Set a DreamHub password** (Settings → Access → Authentication). Without one, anyone on your home network can use DreamHub.
- **Lock down the box's own web interface too.** By default many boxes let every device on the home network use WebControl (port 80) without a password, even when one is set — DreamHub can't change that for you. In the box's own web interface settings, set **`localauth`** to *on* (`config.plugins.Webinterface.localauth=true`). If you do, enter the box's login under **Settings → Services → WebControl** so DreamHub can still reach it.

### Optional keys

DreamHub works without any of these; each one adds something. All are entered under **Settings**, are stored only on your box, and are never shown again after saving.

| Key | Free? | What it adds |
| --- | --- | --- |
| **TMDB** (themoviedb.org → Settings → API) | yes | Posters, backdrops, ratings, descriptions everywhere; "Save covers"; name suggestions; the guide's film/rating filters |
| **OMDb** (omdbapi.com/apikey.aspx) | yes, 1,000 lookups a day | Rotten Tomatoes, IMDb rating and Metascore in a title's details |
| **fanart.tv** (fanart.tv → profile → API) | yes | The title's logo above the details: a recording's details, a programme's in the TV guide and Recommendations, a series' page. Needs TMDB too (it finds the title), and fanart.tv doesn't have a logo for every title |
| **OpenAI** | paid per use, capped by a daily limit you set | AI recommendations (Recommendations) and AI name suggestions |
| **ntfy** topic | yes | Notifications on your phone — see *Notifications* below |

If the **DreamMediaInfo** plugin is installed on the box, DreamHub uses what it already knows first and only asks TMDB for the rest. Cover pictures that another plugin already saved next to a recording are always preferred.

---

## 3. What's where

### Home

A dashboard of tiles: **Now on the box** (what the box shows, with channel up/down, mute and the remote), upcoming timers, conflicts, disk space, series episodes coming up, hub status. **Edit** rearranges tiles (arrows on each tile move it up or down; with a mouse you can also drag); **Custom tile** builds your own view ("films from TMDB with a rating above 7 on tonight", say).

### Recordings

Every recording folder on the box, folder by folder.

- **Click a title** for its details: picture, ratings, description, and **Play on the TV**.
- **Rename** a recording: all of its files are renamed together; suggestions come from the recording itself, TMDB or AI. You can also change the title the box's own list shows.
- **Select several** (or **Select all**) and:
  - **Rename by scheme** — names follow your scheme from *Settings → Recordings* (e.g. `{title} - S{season:02}E{episode:02} - {episodeTitle}`). With a TMDB key set, a film's missing year and an episode's title are filled in from TMDB by themselves. Tick **Sort into folders** to also move them into folders by your folder scheme (e.g. `{title}/Season {season:02}`); missing folders are created.
  - **Save covers** — writes the poster and backdrop next to recordings that have none, so the TV's own recording list and other plugins show them too (DreamHub shows posters without saving). Existing covers are never replaced.
  - **Move to trash** — nothing is deleted right away; the **Trash** tab restores or deletes for good.

- **Ads** — in a recording's details, **Detect ads** finds the ad breaks by the channel's logo, on the box, in the background (it waits while the box records). The breaks show on a timeline; tap one to watch from there in the browser. **Cut out the ads …** ticks the breaks it is sure about — the uncertain ones are yours to decide — shows what will happen and cuts them out without re-encoding. The original keeps all its files and goes to the trash, so it can be restored. For several recordings at once: select them, then **Detect ads**. The list shows "14 min ads" next to a recording that was checked.

Files are only changed when DreamHub runs on the box itself, and never while a recording is still being written or played.

### Plex library

The films and series of your **Plex** server. Connect it under **Settings → Services → Plex → Sign in with Plex**: open plex.tv/link, enter the code shown, then pick your server and the address the box should use (one in your home network is fastest, or type your own, e.g. a reverse proxy). The server's other addresses are kept as **fallbacks**: if the one in use stops answering, DreamHub switches to the next one that answers as the same server. You can edit them under "The server's other addresses". Choose which libraries to show in the same place.

- Details and series pages show the title's logo from Plex when the server has one.
- **Tiles or list** — switch at the top right; filter by text or by watched state, sort by title, recently added, year or rating.
- **Watched** is Plex's own: marking a film or episode changes it in Plex right away (with **Undo**); a whole season or series shows every episode first.
- **Play on the box** plays the file straight from the Plex server with the box's own player, from the beginning — no Plex app on the box — in the box's movie player, so you can seek and pause with the remote. Whether a file plays depends on its formats, shown in the details.
- When the Plex server is off, the library still shows what DreamHub last saw ("Plex offline · as of …"); playing and marking work again once it answers.
- On **Home**, **Edit** can switch on the tile **Continue watching (Plex)**.

### Series

Your series from **SerienRecorder**. A series' page shows planned episodes, further broadcasts in the guide — each with its episode number and whether you're **missing** it — your recordings by season, and (with TVmaze, on by default) the full episode list with what's recorded, planned, missing or still to come. "Missing" is only shown when your recordings carry episode numbers like `S01E02`. Daily soaps that SerienRecorder numbers straight through (`S00E8622`) are matched by that number; only episodes from your first recording on count as missing. A broadcast shown on several regional channels at once (RTL's, for example) is listed once.

### TV guide

Four views, switched at the top; your choice is remembered:

- **Grid** — the classic timeline, four hours at a time.
- **Now & next** — what's on every channel right now with a progress bar, and what comes next; **Switch** changes the TV's channel. The default on phones.
- **Tonight** — what starts between about 20:00 and 21:30 as picture cards, up to a week ahead.
- **Channel** — one channel's whole day as a list.

**Films only** and **Rated 7+** filter every view (needs TMDB). Any programme opens its details with **Record** and, while it's on, **Switch now**.

### Live TV

TV channels right in the browser — on the phone, tablet or PC, no app needed. Pick a channel from the list; **Watch what the TV shows** next to the channel list plays what the box is showing. The box sends its signal unchanged and your device decodes it, so the box itself does no extra work. **Space** pauses, **M** mutes, **F** or a double click switches to full screen; with several audio tracks (another language, Dolby) you choose one under the picture.

The box has one tuner: another channel only works while the TV and any recording are on the same frequency. Otherwise the picture says so — **Watch what the TV shows** always works. In the **TV guide**, a programme that's on now has **Watch in the browser** too, and **Home** can show a **Live TV** tile with your channels (switch it on under **Edit**). In **Recordings**, **Watch in the browser** plays a recording, with a bar to jump anywhere in it.

Works in current Chrome, Edge, Firefox and Safari. HD channels use your device's own video decoder; older or very small devices may stutter on HD.

### Radio

Your radio stations, with what's on now and next where the station sends programme information. **Play on the box** switches the box to that station — you listen on the TV or whatever is connected to it; the station playing is shown on top. **In the browser** plays a station right on this device instead (like Live TV, it needs the tuner to be free or on the same frequency). Any programme opens its details with **Record**; the radio recordings coming up are listed at the bottom. Stations can also be found through the search at the top.

### Timers

All timers on the box, by day, with likely tuner conflicts marked. **Disable** keeps a timer in the list without recording; **Enable** turns it back on (with a conflict check first). **Delete** removes it — deleting a timer that is recording right now stops that recording, and the preview says so. **Clean up finished** removes every finished one-off timer in one preview (switched-off and repeating timers stay). **AutoTimer** records everything that matches its rules automatically; the page lists them, and **Show what would be recorded** previews what they would add. Each row has one action in sight (switch off/on, or delete when it can't be switched); the rest is under **⋯**.

### Recommendations

AI picks from a bouquet's programme for the next hours, optionally fitted to your taste (what you record and follow — **What gets sent** shows the exact list before anything is sent). The picks stay until you generate new ones, also on other devices. Click a pick for its details and **Create timer**.

### Channel list

Your bouquets, editable on the phone (admins). Move channels (buttons, or drag on a computer), put one on a given number, rename, remove, add from all the channels the box knows, add headings; create, rename and delete bouquets. **All channels** searches everything the box knows (new ones, ones in no bouquet, HD, by provider), **Logos** lists the channels without a logo and what else deserves a look (channels gone after a scan, twice in a bouquet, without programme data), shows the installed logo sets and lets you pick the one DreamHub uses — and, for a single channel, a different logo: from any installed set, your own picture, or one found in the public TV-logos collection (also from the check's "Without a logo" list). That choice only changes DreamHub's screens, never the TV, and **Back to automatic** undoes it. The list always reads the box's current channel list; it does not search for new channels — run a channel scan on the box for that. Every change is collected and shown as one preview ("ZDF HD moves from 14 to 2"); applying makes a backup of the whole channel list first and restores it by itself if the result isn't what was previewed. Older backups can be restored. Needs the BouquetEditor plugin on the box.

### Remote

A remote control for the box in the browser, with a live picture of the TV screen, and **Message to the TV** to show a short text on screen. Opens from the remote icon at the top on every page.

### Device

**Overview** shows the box, its disks, and the **Change log** of every change made through DreamHub. **Diagnostics** runs the box self-test. **Backup and restore** of DreamHub's own settings and dashboard are under **Settings → System** (admins running Docker apps can also include each app's own configuration in that backup).

**Services & logs** (admins only) shows the box's **services** — enigma2, DreamHub, Samba and the others — with their state; **Restart** (or **Start**, when one is down) shows first what it interrupts, such as a recording running right now. Below that, logs — DreamHub's own and the box's — live if you like; **Logs** next to a service shows just its lines, and crash logs.

**Advanced** (admins only) is for people who know their way around a shell: a **script library** (upload a script, run it with a confirm) and a **terminal** — off by default, needs DreamHub's own password protection turned on and re-entered to switch on, with a standing warning while it's on.

### Apps

The page has three tabs in a fixed order — **Packages**, **Extensions**, **Docker** — each opening with one sentence saying what it is. DreamHub remembers the tab you looked at last (per browser).

**Packages** is the box's own software (DreamOS): TV plugins, skins and tools. Search at the top, then the filters **Installed**, **Available** and **Updates**, and category chips (Extensions, Skins, System plugins, Drivers, Channel logos, Kernel modules, System). When updates are waiting, a bar says "3 updates available" with **Update all** — one preview lists every package apt would touch, and one confirmation applies them. Each row has one action (Install, Update or Remove) and a **⋯** menu with the details (version, where the description comes from). Every row says in a sentence what the package is, in your language; where DreamHub has no text of its own it shows the package's own description, and for a package with neither, an optional **Ask the AI** entry appears in **⋯** only if an AI key is set up — nothing needs it. A package that a DreamHub feature uses says "Needed by Recordings" (and so on), and removing one asks first. DreamHub's own list of descriptions is still short; it grows as the box's package feed is read. The gear opens **Package sources (feeds)** — switch one on or off, or add one, each with a preview; adding warns that these feeds are unsigned, so only add one you trust.

**Extensions** are small add-ons for DreamHub itself — for example one that switches a smart plug. Each shows as a card with its own status and actions (never its own web page); an extension that needs something typed in, such as a device key, has a **Settings** button.

**Docker** — optional: extra apps in containers, not needed for normal operation. **Apps** are programs that run in their own Docker container on the box, separate from the TV software. If Docker isn't set up yet, a note says so and points to **Docker settings** (the gear at the top right of the tab), where **Set up Docker** shows what it will do before it does it. A short hand-picked list (AdGuard Home, Pi-hole, Nextcloud, Vaultwarden, Syncthing, Home Assistant, qBittorrent) shows whether each works on your box's processor, its memory and port, and a note for this box; a search for anything else on Docker Hub is folded away (a result can be checked for compatibility or added as your own container). **Install** opens a small form: the **web port** (changeable only where the app's image allows it — otherwise it says so), the **disk** to store the app on (only real disks, never the box's own flash memory) and any field the app requires; **Show advanced options** adds every other field plus free rows for **environment variables**, **ports** and **folders** and a memory limit. A secret field (a password) is hidden while you type and never shown again. Then it shows exactly what will be created — images, ports, the folders on the box, the variables — checked against free memory, disk space and ports, before anything runs. The tab has two views: **My containers** (what is on the box) and **App catalog** (what you can add). In **My containers** each row has the app's picture, name, a status dot, its port, an **Auto-start** switch (start it whenever the box starts; saved at once) and one main button — **Open** while it runs, otherwise **Start**. Behind **⋯**: **Stop**, **Logs**, a **Console** (an interactive shell inside the app — same off-by-default, password-re-entry, standing-warning treatment as the terminal), **Edit** (the same form as at install, filled in; it shows what will change and makes the app again — its data stays), **Back up**, **Update** and **Remove** (its settings are kept). An "Update available" badge and a notification appear when a newer version exists — DreamHub never updates one on its own. Some apps are more than one container — Nextcloud brings its own database, handled together with it and listed under it with its own logs. Apps share the box's network, so each port is the box's port; on the reference box port 53 is already taken by the box's own DNS resolver, so AdGuard Home and Pi-hole can't be installed there. In **Settings → System → Backup** each app folder is listed with its size: a database is in by default, bulk files (Nextcloud's, downloads) only when ticked.

**Add container** (above the list, admins only) is for anything that isn't in the list: paste a **compose file** (`docker-compose.yml`), type an **image name**, or paste an **Unraid template** (the XML; DreamHub bundles no templates and downloads nothing — it only reads the ports, folders and variables). You get the same form as for the list, built from the file: every `${NAME:-default}` in it becomes a field. What is understood: services with image, ports, volumes, environment, command, a memory limit, restart and depends_on; anything else (build, networks, secrets, …) is refused with a reason. Apps use the box's own network, so a port can't be mapped to another one — the app listens on its own port. A folder written `.` or `./name` is kept in the app's storage; any other folder must be below `/media`. **You are responsible for what you run**: DreamHub refuses a container any folder of the box's system (`/`, `/etc`, `/usr`, `/var`, `/boot`, `/proc`, `/sys`, `/dev`), DreamHub's own data, the Docker socket and other apps' data — also through links — and asks for an extra confirmation before it gives a container privileged mode, extra capabilities, devices or the box's process view; it cannot judge the software itself. Containers you added are edited and updated through **⋯** like the others, except ones added before editing existed (remove and add those again).

**Updates.** When something can be updated, a box at the top of the page says how many packages and containers, with **Update all**. The preview lists everything in the order it runs: the containers one after the other, then the packages. Containers go first because an update that does not run goes back to the previous version by itself, so nothing else changes if one fails; the packages follow only when every container is fine. If a step fails, the rest is not started, and the result says what was done, what failed and what was not started. DreamHub itself is not part of it (a note says so); update it with the box's own tools. While a recording runs, or one starts within 15 minutes, nothing is updated: the box says "A recording is running … Updates are possible again at 21:15." **Automatic updates** are off by default: one switch for containers in Docker settings, one for packages in Package settings (only packages that do not restart enigma2); "Only update between" limits them to a time of day (pressing Update yourself is only held back by a recording). Their results appear in the notifications. In a container's **⋯** menu, **Update** updates that one and **Roll back** (shown while the previous version is kept) goes back; Docker settings list the kept versions and can remove them. In a package's menu, **Downgrade to …** appears only if the feed still has an older version. If an app keeps stopping, a notification links to its logs.

**Backup and bringing apps back.** *Settings → System → Backup & restore* can include your apps: tick the apps (and which of their folders) and download the backup, or choose **Save on the box** to keep it on the recording disk, which suits big app data. The apps part also holds the passwords DreamHub made for the apps, so keep the file safe. On a new box, restore the file; the apps then appear under *My containers* as **Apps you can bring back** with *Install again*. You see the usual preview and confirm it. An app you removed earlier is offered the same way.

**Docker settings** (the gear) show whether Docker is running and **Set up Docker** when it isn't. Once it runs they also hold: where **new** apps keep their data (the disks with their free space; apps already installed stay where they are), whether new apps start with the box, the **memory limit per app** (automatic, or your own number for containers that don't state one), how often DreamHub **checks for newer app versions** and whether it tells you in the notifications (nothing is ever updated by itself), and **Clean up unused images** — old images stay on the disk when an app is removed or updated; it shows how much space that is, then lists them and asks before it deletes. Each setting shows **Reset** only when it differs from the default. At the end: **Uninstall Docker**. Uninstalling is a preview like every change: it stops and removes the app containers DreamHub created, switches off Docker and DreamHub's Docker helper, stops loading the overlay kernel module at boot and deletes the Docker program files. It asks one question — **Also delete the app data?** — and by default keeps it, so the apps come back after setting Docker up again. Recordings and the TV software are never touched.


---

## 4. DreamHub as an app

DreamHub can sit on your phone's home screen or your computer's desktop and open in its own window, without the browser's address bar. **Settings → General → DreamHub as an app** tells you what works in the browser you're using, with an **Install** button where the browser offers one.

- **iPhone / iPad:** in Safari, tap **Share → Add to Home Screen**. This works over plain `http://`, no further setup.
- **Android, Windows, Mac, Linux (Chrome, Edge):** browsers only offer to install over **HTTPS**, and DreamHub on the box is plain `http://` (no certificate a phone would trust can exist for a LAN address). Until it's behind HTTPS, use the browser menu's **Add to home screen**: that gives you an icon, just without the app window.

**Putting DreamHub behind HTTPS** (optional) needs a second device that's always on and a name with a real certificate — for example:

- **Tailscale:** on any always-on machine in your network, `tailscale serve --bg http://192.168.1.43:8080` gives `https://<machine>.<tailnet>.ts.net`, reachable from your devices at home and away.
- **A reverse proxy** you already run (Caddy, Nginx Proxy Manager, Traefik on a NAS), with a domain that points to it, forwarding to `http://<box IP>:8080`.

Either way, enter that host name under **Settings → Access → Network → Additional allowed host names**, or DreamHub refuses requests addressed to it (protection against DNS rebinding).

Over HTTPS the browser also decodes HD channels in Live TV with the device's own hardware (it refuses that on plain `http://`), so HD plays more smoothly on phones. Confirmed with Nginx Proxy Manager on an Unraid server (2026-10-08). When the machine running the proxy is off, the HTTPS address is gone too — the box's own `http://<box IP>:8080` keeps working.

**Keep it in your home network.** For access from away, use Tailscale (or another VPN) rather than opening the proxy to the internet. If DreamHub is reachable from outside at all, switch on its login (**Settings → Access**) first: without it, anyone who reaches the address can control the box.

When the box is off or out of reach, the app opens anyway and says so, and connects by itself once the box answers again. Nothing from the box is kept on the phone: timers, recordings and the guide are always live.

---

## 5. Notifications on your phone

*In the browser:* the bell at the top has **Show notifications on this device**. The browser then shows a message when DreamHub is open in a background tab or the installed app — it needs HTTPS (or `localhost`) and your permission. With DreamHub closed, use ntfy below.

DreamHub can send a short message to your phone when something happens, through the free **ntfy** app (iPhone and Android; no account needed):

1. Install **ntfy** on your phone.
2. In DreamHub, **Settings → Notifications**: enter a **topic** — a name that's hard to guess; the page suggests one — and save.
3. In the ntfy app, subscribe to the same topic (server `ntfy.sh` unless you run your own), then press **Send a test** in DreamHub.

Choose which events send a message: a recording finished (on), a recording started (off), **a missing episode of one of your series airs in the next 24 hours** (on), a possible timer conflict (on), the disk is almost full (on), **a service on the box goes down or comes back** (on — Samba, the softcam, enigma2 itself; DreamHub checks every minute and only reports a service that stays down). The bell at the top of DreamHub shows all of these too, with or without push.

Privacy: a message's text (a recording's or series' title, a channel name) passes through the ntfy server. On the public `ntfy.sh` anyone who knows your topic could read it — that's why it should be hard to guess. For full control, run your own ntfy server and enter its address.

---

## 6. When something doesn't work

- **Not sure what works on your box?** **Device → Diagnostics → Run self-test** asks the box everything DreamHub needs, without changing anything, and marks each answer: fine, worth a look, failed, or not applicable. **Download the box's answers** gives a zip that helps when reporting a problem.
- **DreamHub doesn't open:** check that the service runs — in the WebAdmin terminal, `systemctl status dreamhub`. Its log: `journalctl -u dreamhub -n 100`.
- **A hub shows "unavailable" or "degraded":** the sidebar dot and the bell say why and how to fix it (a missing key, a plugin that isn't installed, the box not answering). **Device → Diagnostics** lists all of them.
- **"Could not reach WebControl":** the box's interface is restarting or busy (after an update, enigma2 takes a moment), or it now asks for a login — enter it under **Settings → Services → WebControl**.
- **File actions are missing in Recordings:** they need DreamHub to run on the box itself; the page says so when they're not available.
- **No pictures:** add a TMDB key and make sure *Load pictures, ratings and overviews* is on (Settings → Services → TMDB).
- **More detail for a bug report:** **Device → Services & logs → Logs**, set the level to *debug* for a while (it switches back by itself).

Problems and ideas: the repository's issue tracker on GitHub.
