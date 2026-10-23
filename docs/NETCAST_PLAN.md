# Netcast and IPTV channels — idea

Status: **idea, brainstormed 2026-10-23, nothing built.** Facts are in `ARCHITECTURE.md` ("Netcast and the channel list", "Netcast sources on the One, and what the forum says"). Dreambox One with the Gemini plugins only (Netcast 2.4); the Solo2 has none.

## What it could be

1. **IPTV into the channel list (works on both boxes, no Netcast needed).** Pick an M3U (a URL, or one of the sources Netcast already lists), choose channels, and DreamHub shows the bouquet it would create — plain `4097` references with the stream URL — as a plan: preview, confirm, apply through the BouquetEditor API like ChannelHub does, read back. Nothing automatic. No timer or recording on these channels (a stream has no tuner), so ChannelHub marks them. Play a channel from DreamHub with `/web/zap?sRef=…` (the way recordings are already played).
2. **Netcast sources (One, Gemini only).** List, add, edit and remove `netcastserver_*.json` and `netcastepg_*.json` from the same page, with the fields from the plugin's README — the plugin reads them itself, DreamHub never changes anything else. Open: whether the plugin notices a changed file without a GUI restart (to try on the One).
3. **Netcast's own channel entries** (the placeholder references with name `None`): only listed and moved/removed by the BouquetEditor like any bouquet entry; DreamHub does not create them — only the plugin can resolve them.
4. **Netcast radio favourites** (`/netcast/web/*`: list, play, add, edit, remove, settings): possible to drive from DreamHub; low priority.

## To find out first (on the One, read-only until each step is agreed)

- Does a `4097` channel with a plain URL (from an M3U) play with `/web/zap`, and does it show EPG? (One test channel, put back.)
- Does Netcast re-read a changed `netcastserver_*.json` without a restart?
- What does the plugin do on "green key → bouquet": the exact lines it writes (compare the bouquet before and after one add).
- Which sources need credentials (`Username`/`Password`) — to be hidden.

## Out of scope

Software transcoding (`PROJECT_PLAN.md`), copying anyone's paid IPTV lists, anything that bypasses a provider's terms: DreamHub only handles URLs the user gives it.
