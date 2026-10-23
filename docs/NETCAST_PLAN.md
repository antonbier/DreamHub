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
