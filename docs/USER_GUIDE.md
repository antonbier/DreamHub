# DreamHub user guide

DreamHub is a web interface for the **Dreambox One** (DreamOS, ARM64; other DreamOS ARM64 boxes should work but are untested). It runs on the box itself, next to the box's own WebControl, and you use it from a browser — on your phone, tablet or computer. This guide covers installing it, the first steps, and what each part does.

DreamHub changes nothing on its own: every change to recordings, timers or settings on the box is shown to you first ("Preview · nothing changed yet") and only happens when you confirm it. Every applied change is listed under **System → Change log**.

---

## 1. Install

### What you need

- A Dreambox One running DreamOS (developed on one with the Gemini/AIO image).
- The box on your home network. DreamHub uses port **8080**; WebControl stays on port 80.
- About 15 MB on the root partition for the program; its database lives on `/data`.

### Today: install the package by hand

There is no package feed to subscribe to yet (see *Later: from a feed* below), so for now the `.deb` package is installed directly.

1. **Get the package.** Every change to DreamHub is built automatically on GitHub: open the repository's **Actions** tab, pick the latest successful run on `main`, and download the artifact **`dreamhub-deb`**. It's a ZIP file containing `dreamhub_<version>_arm64.deb`. (You need to be signed in to GitHub to download artifacts.)
2. **Copy it to the box**, for example to `/tmp/` — over the box's network share (Samba), with an SCP program such as WinSCP or Cyberduck (user `root`), or from a USB stick.
3. **Install it.** Open a shell on the box. No SSH client is needed: the box's own **WebAdmin** has a terminal in the browser at `http://<box>/webadmin/terminal/` (log in as `root`). Then run:

   ```sh
   dpkg -i /tmp/dreamhub_*_arm64.deb
   ```

   This installs the DreamHub service and starts it, and adds the **DreamHub** plugin to the TV's plugin list. The TV's interface (enigma2) restarts once, so the plugin appears right away; after later updates it only restarts again if the plugin itself changed.

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

The first visit starts a short assistant: language, connection to the box, a password for DreamHub, which hubs to show, your channel list, and optionally an AI key. Everything in it can be changed later under **Settings**, and **Settings → Run setup assistant** starts it again.

### Recommended: protect your network access

- **Set a DreamHub password** (Settings → Authentication). Without one, anyone on your home network can use DreamHub.
- **Lock down the box's own web interface too.** By default many boxes let every device on the home network use WebControl (port 80) without a password, even when one is set — DreamHub can't change that for you. In the box's own web interface settings, set **`localauth`** to *on* (`config.plugins.Webinterface.localauth=true`). If you do, enter the box's login under **Settings → WebControl** so DreamHub can still reach it.

### Optional keys

DreamHub works without any of these; each one adds something. All are entered under **Settings**, are stored only on your box, and are never shown again after saving.

| Key | Free? | What it adds |
| --- | --- | --- |
| **TMDB** (themoviedb.org → Settings → API) | yes | Posters, backdrops, ratings, descriptions everywhere; "Save covers"; name suggestions; the guide's film/rating filters |
| **OMDb** (omdbapi.com/apikey.aspx) | yes, 1,000 lookups a day | Rotten Tomatoes, IMDb rating and Metascore in a title's details |
| **fanart.tv** (fanart.tv → profile → API) | yes | The title's logo in the details |
| **OpenAI** | paid per use, capped by a daily limit you set | AI recommendations (Recommendations) and AI name suggestions |
| **ntfy** topic | yes | Notifications on your phone — see *Notifications* below |

If the **DreamMediaInfo** plugin is installed on the box, DreamHub uses what it already knows first and only asks TMDB for the rest. Cover pictures that another plugin already saved next to a recording are always preferred.

---

## 3. What's where

### Home

A dashboard of tiles: **Now on the box** (what the box shows, with channel up/down, mute and the remote), upcoming timers, conflicts, disk space, series episodes coming up, hub status. **Edit** rearranges tiles; **Custom tile** builds your own view ("films from TMDB with a rating above 7 on tonight", say).

### Recordings

Every recording folder on the box, folder by folder.

- **Click a title** for its details: picture, ratings, description, and **Play on the TV**.
- **Rename** a recording: all of its files are renamed together; suggestions come from the recording itself, TMDB or AI. You can also change the title the box's own list shows.
- **Select several** (or **Select all**) and:
  - **Rename by scheme** — names follow your scheme from *Settings → Recordings* (e.g. `{title} - S{season:02}E{episode:02} - {episodeTitle}`). Tick **Sort into folders** to also move them into folders by your folder scheme (e.g. `{title}/Season {season:02}`); missing folders are created.
  - **Save covers** — writes the poster and backdrop next to recordings that have none, where Enigma2 and other plugins look for them. Existing covers are never replaced.
  - **Move to trash** — nothing is deleted right away; the **Trash** tab restores or deletes for good.

Files are only changed when DreamHub runs on the box itself, and never while a recording is still being written or played.

### Series

Your series from **SerienRecorder**. A series' page shows planned episodes, further broadcasts in the guide — each with its episode number and whether you're **missing** it — your recordings by season, and (with TVmaze, on by default) the full episode list with what's recorded, planned, missing or still to come. "Missing" is only shown when your recordings carry episode numbers like `S01E02`.

### TV guide

Four views, switched at the top; your choice is remembered:

- **Grid** — the classic timeline, four hours at a time.
- **Now & next** — what's on every channel right now with a progress bar, and what comes next; **Switch** changes the TV's channel. The default on phones.
- **Tonight** — what starts between about 20:00 and 21:30 as picture cards, up to a week ahead.
- **Channel** — one channel's whole day as a list.

**Films only** and **Rated 7+** filter every view (needs TMDB). Any programme opens its details with **Record** and, while it's on, **Switch now**.

### Radio

Your radio stations, with what's on now and next where the station sends programme information. **Play on the box** switches the box to that station — you listen on the TV or whatever is connected to it; the station playing is shown on top. Any programme opens its details with **Record**; the radio recordings coming up are listed at the bottom. Stations can also be found through the search at the top.

### Timers

All timers on the box, by day, with likely tuner conflicts marked. **Disable** keeps a timer in the list without recording; **Enable** turns it back on (with a conflict check first). **Delete** removes it — deleting a timer that is recording right now stops that recording, and the preview says so. **AutoTimer** shows the AutoTimer plugin's searches and a dry run of what they would add.

### Recommendations

AI picks from a bouquet's programme for the next hours, optionally fitted to your taste (what you record and follow — **What gets sent** shows the exact list before anything is sent). The picks stay until you generate new ones, also on other devices. Click a pick for its details and **Create timer**.

### Remote

A remote control for the box in the browser, with a live picture of the TV screen, and **Message to the TV** to show a short text on screen. Opens from the remote icon at the top on every page.

### System and Admin

**System** shows the box, its disks, DreamHub's diagnostics and the **Change log** of every change made through DreamHub. **Admin** shows logs — DreamHub's own and the box's — live if you like.

---

## 4. DreamHub as an app

DreamHub can sit on your phone's home screen or your computer's desktop and open in its own window, without the browser's address bar. **Settings → General → DreamHub as an app** tells you what works in the browser you're using, with an **Install** button where the browser offers one.

- **iPhone / iPad:** in Safari, tap **Share → Add to Home Screen**. This works over plain `http://`, no further setup.
- **Android, Windows, Mac, Linux (Chrome, Edge):** browsers only offer to install over **HTTPS**, and DreamHub on the box is plain `http://` (no certificate a phone would trust can exist for a LAN address). Until it's behind HTTPS, use the browser menu's **Add to home screen**: that gives you an icon, just without the app window.

**Putting DreamHub behind HTTPS** (optional) needs a second device that's always on and a name with a real certificate — for example:

- **Tailscale:** on any always-on machine in your network, `tailscale serve --bg http://192.168.1.43:8080` gives `https://<machine>.<tailnet>.ts.net`, reachable from your devices at home and away.
- **A reverse proxy** you already run (Caddy, Nginx Proxy Manager, Traefik on a NAS), with a domain that points to it, forwarding to `http://<box IP>:8080`.

Either way, enter that host name under **Settings → Network → Additional allowed host names**, or DreamHub refuses requests addressed to it (protection against DNS rebinding).

When the box is off or out of reach, the app opens anyway and says so, and connects by itself once the box answers again. Nothing from the box is kept on the phone: timers, recordings and the guide are always live.

---

## 5. Notifications on your phone

DreamHub can send a short message to your phone when something happens, through the free **ntfy** app (iPhone and Android; no account needed):

1. Install **ntfy** on your phone.
2. In DreamHub, **Settings → Push notifications**: enter a **topic** — a name that's hard to guess; the page suggests one — and save.
3. In the ntfy app, subscribe to the same topic (server `ntfy.sh` unless you run your own), then press **Send a test** in DreamHub.

Choose which events send a message: a recording finished (on), a recording started (off), **a missing episode of one of your series airs in the next 24 hours** (on), a possible timer conflict (on), the disk is almost full (on). The bell at the top of DreamHub shows all of these too, with or without push.

Privacy: a message's text (a recording's or series' title, a channel name) passes through the ntfy server. On the public `ntfy.sh` anyone who knows your topic could read it — that's why it should be hard to guess. For full control, run your own ntfy server and enter its address.

---

## 6. When something doesn't work

- **DreamHub doesn't open:** check that the service runs — in the WebAdmin terminal, `systemctl status dreamhub`. Its log: `journalctl -u dreamhub -n 100`.
- **A hub shows "unavailable" or "degraded":** the sidebar dot and the bell say why and how to fix it (a missing key, a plugin that isn't installed, the box not answering). **System → Diagnostics** lists all of them.
- **"Could not reach WebControl":** the box's interface is restarting or busy (after an update, enigma2 takes a moment), or it now asks for a login — enter it under **Settings → WebControl**.
- **File actions are missing in Recordings:** they need DreamHub to run on the box itself; the page says so when they're not available.
- **No pictures:** add a TMDB key and make sure *Load pictures, ratings and overviews* is on (Settings → TMDB).
- **More detail for a bug report:** **Admin → Logs**, set the level to *debug* for a while (it switches back by itself).

Problems and ideas: the repository's issue tracker on GitHub.
