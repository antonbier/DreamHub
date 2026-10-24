# Netcast and IPTV channels — plan

Status: **planned 2026-10-24, packages N1–N5; N1, N2, the box-free half of N5 and N3's read-only half built 2026-10-24** (mock-tested, not yet on a box). N1: read a list or upload one, pick channels, plan and apply into a new or existing bouquet, remember a list and "Compare again", a guide twin with search, logos from `tvg-logo`, a stream test (tab "IPTV" in the channel list). N2: internet and Netcast entries marked in the channel list, played on the TV, and kept out of timers. N5 (built): guide sources — a curated list, an address, a GitHub repository — and the coverage check, in the IPTV tab. N3 (read-only, built): tab "Netcast" in the channel list shows the sources in the box's `netcastserver_*.json` / `netcastepg_*.json` (`GET /channels/netcast/sources`, files through `/file?action=download`, the folder through OpenWebif's `/file?dir=…&pattern=…` — **that listing is not confirmed on either box**; when it fails the three file names seen on the One are tried and the screen says so; the file layout is the README's `serverlist` / `epglist` read generically, logins never leave the backend). Open: N3's writing, the installed-addon check and N3c (need the box), N4, and N5's last step, writing a source into Netcast. Facts are in `ARCHITECTURE.md` ("Netcast and the channel list", "Netcast sources on the One, and what the forum says"). Netcast needs a Dreambox One with the Gemini plugins (Netcast 2.4); the Solo2 has none. Plain IPTV (N1) works on both boxes.

## What it could be

1. **IPTV into the channel list (works on both boxes, no Netcast needed).** Pick an M3U (a URL, or one of the sources Netcast already lists), choose channels, and DreamHub shows the bouquet it would create — plain `4097` references with the stream URL — as a plan: preview, confirm, apply through the BouquetEditor API like ChannelHub does, read back. Nothing automatic. No timer or recording on these channels (a stream has no tuner), so ChannelHub marks them. Play a channel from DreamHub with `/web/zap?sRef=…` (the way recordings are already played).
2. **Netcast sources (One, Gemini only).** List, add, edit and remove `netcastserver_*.json` and `netcastepg_*.json` from the same page, with the fields from the plugin's README — the plugin reads them itself, DreamHub never changes anything else. **Tested 2026-10-23: a new file shows up the next time Netcast's screen is opened, no GUI restart**, and a playlist source is grouped by `group-title`. A plan here is a file write (preview the JSON, confirm, read back); an edit of an existing file keeps a copy of the old one.
3. **Netcast's own bouquet entries** (type `8193`, a made-up DVB identity, the stream URL with the scheme `netcastserver_<id>://`; details in `ARCHITECTURE.md`, "Netcast driven by remote control"): DreamHub can list, move, rename and remove them with the BouquetEditor like any entry, and **can play them: `/web/zap` with the reference starts the stream** (tested). It should not create them for Netcast: the scheme only means something to the plugin, and the plugin refreshes the address. Two limits: `getcurrent` does not report such an entry as the current service (still the previous channel), and the entry has a guide only if its source has an EPG assigned. Without Netcast the same idea works with plain `4097` entries whose reference carries the identity of a DVB service that has events (tested: the real programme showed). A guide for channels with no DVB twin would mean writing events into `epg.db`, which DreamHub does not do.
4. **Netcast radio favourites** (`/netcast/web/*`: list, play, add, edit, remove, settings): possible to drive from DreamHub; low priority.

## Decided 2026-10-24 (the owner's answers)

1. **Where**: IPTV (N1) and the Netcast bouquet entries (N2) are a tab **"IPTV"** in the channel list (ChannelHub); the Netcast sources (N3) are a second tab **"Netcast"** next to it, shown only on a box where Netcast is installed. No new sidebar entry (see "Navigation" below).
2. **Credentials in a URL** (`http://user:pass@…`, `?username=…&password=…`): never shown on screen and never logged, the same way the channel logos already strip `user:pass@` (`channel_logos.go`). Shown as "with login".
3. **Remembering a source: option b** — the source (name, URL, the bouquet it fills, last compared) in DreamHub's database per box; the bouquet on the box stays the truth. Later, for Netcast's own sources, the source is the plugin's `netcastserver_*.json` (option d) behind the same "source" layer.
4. **"Compare again" only on a button** in N1; a background notice ("12 new, 3 gone") later if wanted.
5. **Logos**: `tvg-logo` is an option in the plan (ticked by default), a download from the list's own server, shown in the preview.
6. **An M3U file can be uploaded** as well as a URL; a file source is marked "from a file, no compare".
7. **Guide: DVB twin (A) and an own XMLTV source (C)**; Netcast's TVG-ID assignment (B) is not automated.
8. **Subchannels only where the plugin is installed** (checked per box like the other capabilities); otherwise the option is not shown.
9. **Matching a channel to a guide is partly manual**, and manual matches are remembered per box (see "Linking a guide").

## Packages

- **N1 — IPTV from an M3U (both boxes, mock-testable).** Fetch and parse, pick channels, the bouquet as a plan, apply through BouquetEditor, read back; remembering the source and comparing it again later (below). Built and tested against the mock (`backend/internal/webcontrol/mock`) with recorded M3U fixtures; the box check comes when a box is back.
- **N2 — Netcast and IPTV entries in the channel list (both boxes).** ChannelHub recognises `4097` and `8193` entries: a marker "Internet", no timer button, the Netcast source's name for `netcastserver_<id>://` (read from its JSON through `/file`), play with `/web/zap`, move/rename/remove like any entry. It never creates `8193` entries.
- **N3 — Netcast sources (One, Gemini only; read-only first, writing needs the box).** Listing the `netcastserver_*.json`/`netcastepg_*.json` files and the installed addon packages works read-only (`/file`, the package list DeviceHub already has). Writing a file needs a write route in the TV plugin with the same guards as box settings S3 (kill switch, `Host` check, custom header, read back, copy of the old file). Comes after box settings S3 so both share one write mechanism.
- **N3c — Netcast channels into the channel list (the goal, One).** Pick channels of an installed Netcast source, set the guide and optional extra streams, one plan, see the next section. Needs the tests under "To find out first" and the write path of N3.
- **N4 — Netcast radio favourites.** Later, maybe never; `/netcast/web/*` is XML on port 80 and needs no plugin.
- **N5 — Guide sources.** A list of XMLTV sources (curated, an own URL, a GitHub repository), the coverage check against the chosen channels, and adding a source to Netcast (a `netcastepg_*.json`, so it needs N3's write path). The list and the coverage check need no box and can be built alongside N1.

## N1 in detail

### Reading the M3U

- Fetched by the DreamHub backend (on the box itself), never by the browser (`CLAUDE.md`: no external requests from the UI). `http`/`https` only, no redirects to other schemes, a size cap (a few MB; big provider lists have tens of thousands of entries) and a timeout; gzip accepted. Admins only, like ChannelHub's bouquet edits — fetching a URL the user typed is an SSRF risk on a LAN appliance, so loopback and the box's own addresses are refused.
- `#EXTINF` attributes used: `tvg-id`, `tvg-name`, `tvg-logo`, `group-title`, the display name after the comma. `#EXTVLCOPT` and `#KODIPROP` lines are kept per entry but not used (enigma2's `4097` player can't take headers; shown as "may not play on the box").
- Grouped by `group-title` for choosing, with search; a list over a few thousand entries is shown by group, not as one list.

### The plan

- Target: a new bouquet (name proposed from the source) or an existing one; the position of a new bouquet in the list like ChannelHub's own create.
- Each entry `4097:0:1:0:0:0:0:0:0:0:<URL, ':' as %3a>:<name>` — the form `plex_play.go` already builds. Service type `1` (TV) by default; radio entries (`radio="true"` or audio-only by name) go to a radio bouquet only if the user moves them there.
- **Guide through a DVB twin (optional):** DreamHub suggests, by name, a DVB channel of the box whose identity the entry can carry (tested on the One: the real programme then shows). Every suggestion is visible in the preview and is off until ticked; nothing matched silently.
- Preview lists the bouquet, every entry, the ones skipped (unsupported scheme, duplicates) and the ones "with login"; Confirm applies through the BouquetEditor API, reads the bouquet back and reports differences. The existing bouquet backup of ChannelHub covers Undo.
- **Check before adding (on demand, optional):** "Test streams" opens each chosen URL for a second and reports "answers / doesn't answer / needs login". Only when asked, never in the background, capped in parallel requests.

### Remembering a source (decided: option b)

Recommendation: DreamHub keeps, in its own database per box, only **the source**: name, URL (stored, never shown when it carries a login), the bouquet it fills, and when it was last compared. Not the channels — **the box's bouquet stays the truth**: comparing reads the bouquet back and matches its entries to the fresh M3U by URL, then `tvg-id`, then name.

"Compare again" (a button, never automatic) fetches the list and shows a plan: new channels in the source (offered, not ticked), channels gone from the source (offered for removal, not ticked), changed addresses for the same `tvg-id`/name (offered for update, ticked), renames. Entries the user added or moved on the TV are left alone. If the bouquet was deleted on the box, the source shows "bouquet gone" and offers to create it again.

Alternatives considered: not remembering at all (the user pastes the URL again each time — simplest, but then a compare can't know which bouquet belongs to which list); remembering in the bouquet file (a comment line) — rejected: enigma2 rewrites bouquet files and `#DESCRIPTION` has a meaning of its own.

### Not in N1

Recording or timers on IPTV channels, a guide for channels without a DVB twin (would mean writing `epg.db`), playing IPTV in the browser (the stream proxy only opens tuner references, `stream.go`), Xtream Codes' login API (an M3U URL works for those too), and anything that bypasses a provider's terms: DreamHub only handles URLs the user gives it.

## Netcast channels into the channel list (the goal, N3c)

Pick Netcast channels in DreamHub and have them in the One's channel list, with a guide and optionally more streams per channel.

1. **Source and channels**: DreamHub reads the list the plugin has already cached in `/data/Netcast/server/` (`playlist_netcastserver_<id>` for playlist sources, the source's channel list for API sources such as Pluto and Samsung TV Plus) through `/file` — the same list the TV shows, nothing fetched by DreamHub, no logins in URLs. Search, groups, several sources at once.
2. **Per channel**: the stream (the main one), the guide (below), the logo, and — only with the subchannel plugin — extra streams.
3. **One plan**: the new entries in a bouquet (new or existing), the guide links, the `subchannel.xml` change; Confirm applies through the BouquetEditor API (and the plugin's write route for files), reads back, and the bouquet backup of ChannelHub gives Undo.
4. **The entry format** is the plugin's own (`8193:256:0:<sid>:<tsid>:<onid>:<namespace>:0:0:0:netcastserver_<id>%3a//<URL>:<name>`), so the plugin keeps refreshing the address. **Untested assumption: the plugin accepts an entry DreamHub built.** If it does not, the fallback is a plain `4097` entry with the real URL and a DVB twin (both tested), which loses the plugin's address refresh.

## Linking a guide

A channel's guide is whatever events `epg.db` holds under the channel's identity (`ARCHITECTURE.md`, "Why Netcast channels carry a DVB identity").

- **A — DVB twin (default).** The entry carries the identity of a box service that has events. Matching in three steps: **sure** (the normalised names are equal, e.g. "Das Erste HD": ticked), **suggested** (similar names: shown, not ticked), **by hand** (a search over all services of the box with a filter by bouquet and **the current programme of each candidate**, so a wrong twin shows at once — better than Netcast's alphabetical list full of junk names). Every manual match is remembered per box (channel of the source → service or XMLTV id) and used again at the next compare.
- **C — an own XMLTV source** for channels with no twin (Pluto-only channels): a `netcastepg_*.json` that Netcast loads into `epg.db` as its own source. DreamHub shows which of the chosen channels a source knows (coverage, below) and writes the file (N3's write path).
- **B — Netcast's TVG-ID assignment** is bound to a favourite, kept in the running process until a clean shutdown and drawn by Netcast's overlay: not automated.
- A channel may have no guide; it is marked, not refused. DreamHub never writes events into `epg.db`.

### Guide sources (N5)

- A **curated list** of XMLTV sources in the repository (starting with the ones Netcast already uses, `www.xmltvepg.nl`), an **own URL**, and a **GitHub repository**: paste a repository (or folder) address, DreamHub lists the `.xml`, `.xml.gz` and `.m3u` files in it through GitHub's public contents API and uses `raw.githubusercontent.com` addresses. The same "add a repository" works for playlist sources in N1. Public repositories only; the unauthenticated API allows 60 requests an hour, so listings are cached. **Built (2026-10-24):** `POST /channels/iptv/github` lists a folder's `.xml`, `.xml.gz`, `.m3u` and `.m3u8` files (listings cached 30 minutes), `POST /channels/iptv/guide-coverage` reads only the `<channel>` elements (stopping at the first `<programme>`, 32 MB cap, `.gz` unpacked, a BOM tolerated, `.xz` refused — no decompressor in the build) and matches the chosen channels by `tvg-id`, then by name; coverage results are cached 10 minutes. The curated list is `backend/internal/hubs/guide_sources.json` and holds only files checked to be reachable XMLTV (EPGShare01 DE/AT/CH/IT); `www.xmltvepg.nl` (Netcast's own) could not be reached from the build environment and ships `.xz`, so it is not listed. DreamHub does not search GitHub; which repositories publish a ready guide file (not just a grabber tool that builds one) is to be checked when the list is made.
- **Coverage check**: DreamHub streams the file and reads only the `<channel>` elements (`id`, `display-name`), with a size cap and `.gz`/`.xz` handling, and shows "this source knows 31 of your 40 chosen channels". The fetch is the backend's, never the browser's.
- Adding a source to Netcast writes a `netcastepg_*.json` (fields from the plugin's README). Do not use the separate EPG-Importer next to it (same database).

## Subchannels

The "Subchannel" plugin reads a `subchannel.xml` in `/etc/enigma2`: several streams for one channel, one in the bouquet, the others opened with the green key; the channel name in the stream address must equal the bouquet's and the reference numbers must match, or the guide and picon are lost. **Whether the plugin is installed on the One, and the file's format, are unknown** (no `subchannel.xml` on the box). DreamHub offers "Add another stream" only on a box where the plugin is installed; the plan then writes the bouquet entry and the file together. Without the plugin the option is absent.

## Vavoo

What is known (`ARCHITECTURE.md`, "Samsung TV Plus, Vavoo and the EPG assignment"): in Netcast's screen the green key on a Vavoo channel offered only "Video zu Favoriten hinzufügen", no bouquet entry; the stream is served by a local proxy (`http://127.0.0.1:4323/vavoo?url=…`); the picture was never seen. A forum thread is titled "Bouquet aus Netcast (Vavoo) erstellen", so another version or route may exist: **"cannot be put in a bouquet" holds for the tested route only.**

Idea (owner, 2026-10-24): a page of its own, like Plex and Live TV, where the channel list (from the cached `vavoo.livetv.json`) is searched and a channel is **played on the TV** the way Plex is (`plex_play.go`: a `4097` reference with the URL, `/web/zap`), no bouquet. It depends on whether the proxy runs without Netcast's screen open and whether a reference with the proxy address plays from outside — one test on the One. DreamHub does not talk to Vavoo's servers itself: it only uses what the installed plugin provides, and only for URLs the plugin has made.

## Navigation

The sidebar has Dashboard, three groups (Media, Program, Device) and the bottom section with DeviceHub, AppsHub and Settings — about 15 items. New features go into **tabs of an existing page** (as Box settings went into DeviceHub, and IPTV/Netcast go into the channel list) rather than new sidebar items. If the sidebar still gets too long, the options are (a) groups that fold open and closed, (b) sub-items under a page (its tabs shown in the sidebar when the page is open), (c) fewer, broader pages. Not decided — to be looked at with the whole sidebar once more hubs are in; then `DESIGN.md` gets the result.

## To find out first (on the One, read-only until each step is agreed)

- ~~Does a `4097` channel with a plain URL play with `/web/zap`, and does it show EPG?~~ **Done 2026-10-23**: it plays; the guide comes from the DVB identity in the reference. Bouquet create/add/remove through the BouquetEditor API also done and put back.
- ~~Does Netcast re-read a changed `netcastserver_*.json` without a restart?~~ **Done**: yes, when its screen is opened again.
- ~~What does the plugin write on "green key → bouquet"?~~ **Done**: one bouquet file and one line in `bouquets.tv`, entry type `8193` with `netcastserver_<id>://`.
- Which sources need credentials (`Username`/`Password`) — to be hidden.
- **For N3c/N5/Vavoo:** (1) does the plugin play and keep refreshing an `8193 … netcastserver_<id>://` entry that DreamHub created through the BouquetEditor; (2) the same entry with a DVB twin's identity: does it play and show the real programme; (3) how the plugin picks its made-up identity and whether a DreamHub entry would need the same for `GeminiEPG` events to attach; (4) is the subchannel plugin installed, the format of `subchannel.xml`, re-read without a restart; (5) the cached channel lists in `/data/Netcast/server/`: all fields (id, logo, group), sizes; (6) does the Vavoo proxy run without Netcast's screen open, does a `4097` reference with its address play from `/web/zap`.
- Netcast's sources such as Vavoo, Pluto, Samsung TV Plus, Filmpalast and HD-Filme are separate packages (`libgemini*`, `vavoo-nc`): a Netcast page in DreamHub lists what is installed and offers no source whose package is missing.
- **For N1 on a box:** how many `4097` entries a bouquet takes before the channel list gets slow (big lists), whether `https` streams play on both boxes (the Solo2's VTi image and its player differ), and whether the Solo2's OpenWebif bouquet route takes a `4097` reference the same way.

## Out of scope

Software transcoding (`PROJECT_PLAN.md`), copying anyone's paid IPTV lists, anything that bypasses a provider's terms: DreamHub only handles URLs the user gives it.

## Netcast's own risky actions (seen 2026-10-23)

The EPG-Loader menu on the box has **"Komplette EPG-Datenbank löschen"** (OK, no confirmation seen) next to "Alle EPG-Quellen aktualisieren"; the main menu has **"Picon und Zuweisungen löschen"**. DreamHub does not call them. If it ever offers an EPG refresh for Netcast sources it uses the plugin's refresh, never deletes the database (that one holds the broadcast guide too).

## Findings that change the plan (2026-10-23, later tests)

- **API sources work like playlists once in a bouquet** (Pluto tested): type `8193` entry with `netcastserver_<id>://` in front of the URL, playable with `/web/zap` while Netcast's own screen is closed (start may take several seconds; `getcurrent` named the channel for Pluto, not for the LG entry). So a bouquet that holds Netcast entries can be listed, played and tidied from DreamHub — creating them stays Netcast's job (green key on the TV), because a source's address is resolved by the plugin.
- **A DreamHub page for Netcast could therefore be:** (1) a list of the installed Netcast sources with the installed addon packages (`opkg` through a read-only call), (2) add/edit/remove a playlist or EPG source file, (3) the Netcast bouquets with play/remove, (4) EPG assignment — not tested yet, see below.
- **Tested later the same day** (`ARCHITECTURE.md`, "Samsung TV Plus, Vavoo and the EPG assignment"): Samsung TV Plus works like Pluto (bouquet entry, zap plays, `getcurrent` shows the event and a rewritten reference, a stale name). **Vavoo could not be put in a bouquet by the green key** (favourites only; the stream comes through a local proxy on port 4323) — whether DreamHub can play it by a reference with that proxy address is untested (see "Vavoo"). The **EPG assignment** is per favourite (DVB service or TVG-ID of an XMLTV list), held in the running process until a clean shutdown, and drawn by Netcast's overlay, not by enigma2's EPG — so a DreamHub guide view would not see it. The assignment is not something to automate from outside.
- **Not tested:** the subchannel plugin (no `subchannel.xml` on the box), and whether the plugin re-reads a deleted source file the same way as an added one.

## Still open

0. **A login in an address is visible to admins in the channel list API** once the channel is in a bouquet (the reference holds the address, as the box's own bouquet file does). The list reading, the previews, the plans and the logs never show it; redacting the reference in the channel list would need an opaque handle for every edit that sends a reference back, and was left out.

1. **Vavoo**: only after the test above; whether the page is worth building depends on it.
2. **Which GitHub repositories publish a ready guide file or playlist** — to be checked when the curated list is made.
3. **Channels per bouquet**: a full Pluto or Samsung list has hundreds of entries; how the TV's channel list copes is untested.
4. **Copying an IPTV bouquet to another box** (channel-list copy, M8): plain `4097` entries travel as they are; `8193 netcastserver_` entries only make sense where the same Netcast source exists — the copy plan must say so.
