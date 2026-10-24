# Netcast and IPTV channels — plan

Status: **planned 2026-10-24, packages N1–N4, nothing built.** Brainstormed 2026-10-23; the owner's first answers are below, open questions at the end. Facts are in `ARCHITECTURE.md` ("Netcast and the channel list", "Netcast sources on the One, and what the forum says"). Netcast needs a Dreambox One with the Gemini plugins (Netcast 2.4); the Solo2 has none. Plain IPTV (N1) works on both boxes.

## What it could be

1. **IPTV into the channel list (works on both boxes, no Netcast needed).** Pick an M3U (a URL, or one of the sources Netcast already lists), choose channels, and DreamHub shows the bouquet it would create — plain `4097` references with the stream URL — as a plan: preview, confirm, apply through the BouquetEditor API like ChannelHub does, read back. Nothing automatic. No timer or recording on these channels (a stream has no tuner), so ChannelHub marks them. Play a channel from DreamHub with `/web/zap?sRef=…` (the way recordings are already played).
2. **Netcast sources (One, Gemini only).** List, add, edit and remove `netcastserver_*.json` and `netcastepg_*.json` from the same page, with the fields from the plugin's README — the plugin reads them itself, DreamHub never changes anything else. **Tested 2026-10-23: a new file shows up the next time Netcast's screen is opened, no GUI restart**, and a playlist source is grouped by `group-title`. A plan here is a file write (preview the JSON, confirm, read back); an edit of an existing file keeps a copy of the old one.
3. **Netcast's own bouquet entries** (type `8193`, a made-up DVB identity, the stream URL with the scheme `netcastserver_<id>://`; details in `ARCHITECTURE.md`, "Netcast driven by remote control"): DreamHub can list, move, rename and remove them with the BouquetEditor like any entry, and **can play them: `/web/zap` with the reference starts the stream** (tested). It should not create them for Netcast: the scheme only means something to the plugin, and the plugin refreshes the address. Two limits: `getcurrent` does not report such an entry as the current service (still the previous channel), and the entry has a guide only if its source has an EPG assigned. Without Netcast the same idea works with plain `4097` entries whose reference carries the identity of a DVB service that has events (tested: the real programme showed). A guide for channels with no DVB twin would mean writing events into `epg.db`, which DreamHub does not do.
4. **Netcast radio favourites** (`/netcast/web/*`: list, play, add, edit, remove, settings): possible to drive from DreamHub; low priority.

## Decided 2026-10-24 (the owner's answers)

1. **Where**: IPTV (N1) and the Netcast bouquet entries (N2) are a tab **"IPTV"** in the channel list (ChannelHub); the Netcast sources (N3) are a second tab **"Netcast"** next to it, shown only on a box where Netcast is installed. No new sidebar entry (see "Navigation" below).
2. **Credentials in a URL** (`http://user:pass@…`, `?username=…&password=…`): never shown on screen and never logged, the same way the channel logos already strip `user:pass@` (`channel_logos.go`). Shown as "with login".

## Packages

- **N1 — IPTV from an M3U (both boxes, mock-testable).** Fetch and parse, pick channels, the bouquet as a plan, apply through BouquetEditor, read back; remembering the source and comparing it again later (below). Built and tested against the mock (`backend/internal/webcontrol/mock`) with recorded M3U fixtures; the box check comes when a box is back.
- **N2 — Netcast and IPTV entries in the channel list (both boxes).** ChannelHub recognises `4097` and `8193` entries: a marker "Internet", no timer button, the Netcast source's name for `netcastserver_<id>://` (read from its JSON through `/file`), play with `/web/zap`, move/rename/remove like any entry. It never creates `8193` entries.
- **N3 — Netcast sources (One, Gemini only, needs the box).** Listing the `netcastserver_*.json`/`netcastepg_*.json` files and the installed addon packages works read-only (`/file`, the package list DeviceHub already has). Writing a file needs a write route in the TV plugin with the same guards as box settings S3 (kill switch, `Host` check, custom header, read back, copy of the old file). Comes after box settings S3 so both share one write mechanism.
- **N4 — Netcast radio favourites.** Later, maybe never; `/netcast/web/*` is XML on port 80 and needs no plugin.

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

### Remembering a source (open question 1)

Recommendation: DreamHub keeps, in its own database per box, only **the source**: name, URL (stored, never shown when it carries a login), the bouquet it fills, and when it was last compared. Not the channels — **the box's bouquet stays the truth**: comparing reads the bouquet back and matches its entries to the fresh M3U by URL, then `tvg-id`, then name.

"Compare again" (a button, never automatic) fetches the list and shows a plan: new channels in the source (offered, not ticked), channels gone from the source (offered for removal, not ticked), changed addresses for the same `tvg-id`/name (offered for update, ticked), renames. Entries the user added or moved on the TV are left alone. If the bouquet was deleted on the box, the source shows "bouquet gone" and offers to create it again.

Alternatives considered: not remembering at all (the user pastes the URL again each time — simplest, but then a compare can't know which bouquet belongs to which list); remembering in the bouquet file (a comment line) — rejected: enigma2 rewrites bouquet files and `#DESCRIPTION` has a meaning of its own.

### Not in N1

Recording or timers on IPTV channels, a guide for channels without a DVB twin (would mean writing `epg.db`), playing IPTV in the browser (the stream proxy only opens tuner references, `stream.go`), Xtream Codes' login API (an M3U URL works for those too), and anything that bypasses a provider's terms: DreamHub only handles URLs the user gives it.

## Navigation

The sidebar has Dashboard, three groups (Media, Program, Device) and the bottom section with DeviceHub, AppsHub and Settings — about 15 items. New features go into **tabs of an existing page** (as Box settings went into DeviceHub, and IPTV/Netcast go into the channel list) rather than new sidebar items. If the sidebar still gets too long, the options are (a) groups that fold open and closed, (b) sub-items under a page (its tabs shown in the sidebar when the page is open), (c) fewer, broader pages. Not decided — to be looked at with the whole sidebar once more hubs are in; then `DESIGN.md` gets the result.

## To find out first (on the One, read-only until each step is agreed)

- ~~Does a `4097` channel with a plain URL play with `/web/zap`, and does it show EPG?~~ **Done 2026-10-23**: it plays; the guide comes from the DVB identity in the reference. Bouquet create/add/remove through the BouquetEditor API also done and put back.
- ~~Does Netcast re-read a changed `netcastserver_*.json` without a restart?~~ **Done**: yes, when its screen is opened again.
- ~~What does the plugin write on "green key → bouquet"?~~ **Done**: one bouquet file and one line in `bouquets.tv`, entry type `8193` with `netcastserver_<id>://`.
- Which sources need credentials (`Username`/`Password`) — to be hidden.
- Netcast's sources such as Vavoo, Pluto, Samsung TV Plus, Filmpalast and HD-Filme are separate packages (`libgemini*`, `vavoo-nc`): a Netcast page in DreamHub lists what is installed and offers no source whose package is missing.
- **For N1 on a box:** how many `4097` entries a bouquet takes before the channel list gets slow (big lists), whether `https` streams play on both boxes (the Solo2's VTi image and its player differ), and whether the Solo2's OpenWebif bouquet route takes a `4097` reference the same way.

## Out of scope

Software transcoding (`PROJECT_PLAN.md`), copying anyone's paid IPTV lists, anything that bypasses a provider's terms: DreamHub only handles URLs the user gives it.

## Netcast's own risky actions (seen 2026-10-23)

The EPG-Loader menu on the box has **"Komplette EPG-Datenbank löschen"** (OK, no confirmation seen) next to "Alle EPG-Quellen aktualisieren"; the main menu has **"Picon und Zuweisungen löschen"**. DreamHub does not call them. If it ever offers an EPG refresh for Netcast sources it uses the plugin's refresh, never deletes the database (that one holds the broadcast guide too).

## Findings that change the plan (2026-10-23, later tests)

- **API sources work like playlists once in a bouquet** (Pluto tested): type `8193` entry with `netcastserver_<id>://` in front of the URL, playable with `/web/zap` while Netcast's own screen is closed (start may take several seconds; `getcurrent` named the channel for Pluto, not for the LG entry). So a bouquet that holds Netcast entries can be listed, played and tidied from DreamHub — creating them stays Netcast's job (green key on the TV), because a source's address is resolved by the plugin.
- **A DreamHub page for Netcast could therefore be:** (1) a list of the installed Netcast sources with the installed addon packages (`opkg` through a read-only call), (2) add/edit/remove a playlist or EPG source file, (3) the Netcast bouquets with play/remove, (4) EPG assignment — not tested yet, see below.
- **Tested later the same day** (`ARCHITECTURE.md`, "Samsung TV Plus, Vavoo and the EPG assignment"): Samsung TV Plus works like Pluto (bouquet entry, zap plays, `getcurrent` shows the event and a rewritten reference, a stale name). **Vavoo cannot be put in a bouquet** (favourites only; the stream comes through a local proxy on port 4323) — so DreamHub can play nothing from Vavoo by reference. The **EPG assignment** is per favourite (DVB service or TVG-ID of an XMLTV list), held in the running process until a clean shutdown, and drawn by Netcast's overlay, not by enigma2's EPG — so a DreamHub guide view would not see it. The assignment is not something to automate from outside.
- **Not tested:** the subchannel plugin (no `subchannel.xml` on the box), and whether the plugin re-reads a deleted source file the same way as an added one.

## Open for the owner (2026-10-24)

1. **Remembering a source**: as recommended above (the source in DreamHub's database, the bouquet as the truth, "Compare again" as a plan)?
2. **A notice when a remembered list changed?** DreamHub could compare in the background (say once a day) and only *tell* ("Pluto list: 12 new, 3 gone") — the change itself stays a plan. Or compare only on the button. Recommendation: only on the button in N1; the notice later if wanted.
3. **Logos**: take `tvg-logo` into DreamHub's channel logos (ChannelHub's Logos tab) for the new entries? It would be a download from the list's own server, shown in the preview. Recommendation: yes, as a ticked option in the plan.
4. **Upload an M3U file** as well as a URL? Recommendation: yes — a file can't be compared again later, so it is shown as "from a file, no compare".
