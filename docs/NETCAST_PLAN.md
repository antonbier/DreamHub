# Netcast and IPTV channels — idea

Status: **idea, brainstormed 2026-10-23, nothing built.** Facts are in `ARCHITECTURE.md` ("Netcast and the channel list", "Netcast sources on the One, and what the forum says"). Dreambox One with the Gemini plugins only (Netcast 2.4); the Solo2 has none.

## What it could be

1. **IPTV into the channel list (works on both boxes, no Netcast needed).** Pick an M3U (a URL, or one of the sources Netcast already lists), choose channels, and DreamHub shows the bouquet it would create — plain `4097` references with the stream URL — as a plan: preview, confirm, apply through the BouquetEditor API like ChannelHub does, read back. Nothing automatic. No timer or recording on these channels (a stream has no tuner), so ChannelHub marks them. Play a channel from DreamHub with `/web/zap?sRef=…` (the way recordings are already played).
2. **Netcast sources (One, Gemini only).** List, add, edit and remove `netcastserver_*.json` and `netcastepg_*.json` from the same page, with the fields from the plugin's README — the plugin reads them itself, DreamHub never changes anything else. **Tested 2026-10-23: a new file shows up the next time Netcast's screen is opened, no GUI restart**, and a playlist source is grouped by `group-title`. A plan here is a file write (preview the JSON, confirm, read back); an edit of an existing file keeps a copy of the old one.
3. **Netcast's own bouquet entries** (type `8193`, a made-up DVB identity, the stream URL with the scheme `netcastserver_<id>://`; details in `ARCHITECTURE.md`, "Netcast driven by remote control"): DreamHub can list, move, rename and remove them with the BouquetEditor like any entry, and **can play them: `/web/zap` with the reference starts the stream** (tested). It should not create them for Netcast: the scheme only means something to the plugin, and the plugin refreshes the address. Two limits: `getcurrent` does not report such an entry as the current service (still the previous channel), and the entry has a guide only if its source has an EPG assigned. Without Netcast the same idea works with plain `4097` entries whose reference carries the identity of a DVB service that has events (tested: the real programme showed). A guide for channels with no DVB twin would mean writing events into `epg.db`, which DreamHub does not do.
4. **Netcast radio favourites** (`/netcast/web/*`: list, play, add, edit, remove, settings): possible to drive from DreamHub; low priority.

## To find out first (on the One, read-only until each step is agreed)

- ~~Does a `4097` channel with a plain URL play with `/web/zap`, and does it show EPG?~~ **Done 2026-10-23**: it plays; the guide comes from the DVB identity in the reference. Bouquet create/add/remove through the BouquetEditor API also done and put back.
- ~~Does Netcast re-read a changed `netcastserver_*.json` without a restart?~~ **Done**: yes, when its screen is opened again.
- ~~What does the plugin write on "green key → bouquet"?~~ **Done**: one bouquet file and one line in `bouquets.tv`, entry type `8193` with `netcastserver_<id>://`.
- Which sources need credentials (`Username`/`Password`) — to be hidden.
- Netcast's sources such as Vavoo, Pluto, Samsung TV Plus, Filmpalast and HD-Filme are separate packages (`libgemini*`, `vavoo-nc`): a Netcast page in DreamHub lists what is installed and offers no source whose package is missing.

## Out of scope

Software transcoding (`PROJECT_PLAN.md`), copying anyone's paid IPTV lists, anything that bypasses a provider's terms: DreamHub only handles URLs the user gives it.

## Netcast's own risky actions (seen 2026-10-23)

The EPG-Loader menu on the box has **"Komplette EPG-Datenbank löschen"** (OK, no confirmation seen) next to "Alle EPG-Quellen aktualisieren"; the main menu has **"Picon und Zuweisungen löschen"**. DreamHub does not call them. If it ever offers an EPG refresh for Netcast sources it uses the plugin's refresh, never deletes the database (that one holds the broadcast guide too).

## Findings that change the plan (2026-10-23, later tests)

- **API sources work like playlists once in a bouquet** (Pluto tested): type `8193` entry with `netcastserver_<id>://` in front of the URL, playable with `/web/zap` while Netcast's own screen is closed (start may take several seconds; `getcurrent` named the channel for Pluto, not for the LG entry). So a bouquet that holds Netcast entries can be listed, played and tidied from DreamHub — creating them stays Netcast's job (green key on the TV), because a source's address is resolved by the plugin.
- **A DreamHub page for Netcast could therefore be:** (1) a list of the installed Netcast sources with the installed addon packages (`opkg` through a read-only call), (2) add/edit/remove a playlist or EPG source file, (3) the Netcast bouquets with play/remove, (4) EPG assignment — not tested yet, see below.
- **Tested later the same day** (`ARCHITECTURE.md`, "Samsung TV Plus, Vavoo and the EPG assignment"): Samsung TV Plus works like Pluto (bouquet entry, zap plays, `getcurrent` shows the event and a rewritten reference, a stale name). **Vavoo cannot be put in a bouquet** (favourites only; the stream comes through a local proxy on port 4323) — so DreamHub can play nothing from Vavoo by reference. The **EPG assignment** is per favourite (DVB service or TVG-ID of an XMLTV list), held in the running process until a clean shutdown, and drawn by Netcast's overlay, not by enigma2's EPG — so a DreamHub guide view would not see it. The assignment is not something to automate from outside.
- **Not tested:** the subchannel plugin (no `subchannel.xml` on the box), and whether the plugin re-reads a deleted source file the same way as an added one.
