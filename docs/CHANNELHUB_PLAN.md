# ChannelHub — plan

Status: **C1–C5 built 2026-10-08 against the mock.** C2/C3 are not released until the BouquetEditor writes have been tried on the reference box ("To confirm on the box" below). Tracks issue #26. Once a package lands, its status moves to `ROADMAP.md`.

## Why

Sorting the channel list, cleaning it up and finding new channels is the one everyday job on the box that has no usable mobile interface. The TV remote is slow for it, and the existing BouquetEditor plugin's web UI works, but it's a legacy desktop page — not something to use on a phone. ChannelHub is that job, done on a phone, with DreamHub's usual safety: preview, confirm, and a backup before every change.

The reference box has a single bouquet ("Favourites"), but other households have several, TV and radio — everything below works for any number.

## Decisions

### Own interface, BouquetEditor's engine

The earlier decision (`CONCEPT.md`, `ROADMAP.md`) was to link to or embed the BouquetEditor plugin's own UI. **Revised:** DreamHub builds its own mobile interface, but every change still goes through the plugin's web API (`/bouqueteditor/web/*`, parameters in `ARCHITECTURE.md`) — DreamHub doesn't parse or write `bouquets.tv`, `userbouquet.*` or `lamedb` itself. That keeps the rule "don't re-implement what the box already exposes": the plugin stays the one thing that writes the channel list, exactly as when it's used on its own. A link to the plugin's own page stays as the expert fallback.

The plugin is optional (`PRINCIPLES.md`, capability detection). Without it, ChannelHub is read-only: browsing bouquets and channels, the channel check and picons work; editing says why it's missing and how to get it.

### Edit as a draft, apply as one plan

On a phone, sorting is many small moves. ChannelHub collects them as a **draft** in the browser — move, remove, add, add a marker, rename — shown live, nothing sent yet. "Review changes" turns the draft into **one plan**: the list of changes in words ("ZDF moved from 14 to 2", "3 channels added", "Sky Cinema removed"), then confirm.

Applying:

1. **Backup first**, always: the plugin's `backup(Filename)` with a DreamHub-named file (`dreamhub-<date>-<time>`), before any change.
2. The changes one by one, through the plugin's endpoints.
3. On any failure: **restore that backup automatically**, and say so. The channel list is never left half-changed.
4. Stale check like every plan: if the bouquet changed between preview and confirm (on the TV, another device), nothing is done — preview again.

Backups made by DreamHub are listed in ChannelHub with **"Restore"** (also a plan); the newest few are kept, older ones are offered for deletion.

### Finding new channels

"New" isn't something the box records. DreamHub keeps its own snapshot of all channels the box knows (reference, name, provider, TV/radio, first seen) and refreshes it when ChannelHub opens and once a day. That gives:

- **New since** a date — typically after a channel scan on the box.
- **In no bouquet** — the channels you have but never see.
- Search by name, filter TV/radio, HD/SD (from the service type), provider.

Adding from there puts the channel into a bouquet at the end or at a chosen position — into the draft, like every other change.

### Channel check

A read-only list of what deserves a look: channels in a bouquet without EPG right now, the same channel twice in one bouquet, channels without a picon in the chosen set, entries that no longer exist after a scan (a reference the box doesn't know anymore). Each line links to the fix in the editor.

### Picons

The box keeps picon **sets** as subfolders of `/data/piconpool`, installed as packages from the Gemini feed (`ARCHITECTURE.md`, "Picons"). ChannelHub shows the installed sets with their coverage of your bouquets ("34 of 40 channels") and lets an admin choose which set DreamHub itself uses for logos everywhere (guide, live TV, radio, remote) — until now DreamHub just took the first match. It doesn't change which set the TV's skin uses (nothing in the box's settings names it).

**Later, after a read-only look at the feed on the box:** list the picon packages the feed offers and install or remove one through plan → apply (`apt-get`). The package names and how they install aren't confirmed yet.

### Bouquets per profile: prepared, no interface

On the TV there's one channel list for everyone, so hiding bouquets per profile has little use today (owner, 2026-10-08). It is **prepared, not built**: a profile can carry a list of allowed bouquets in the data model, and the bouquet lists the hubs serve go through one server-side filter that today lets everything through. The interface comes with a kids role (`docs/archive/PROFILES_PLAN.md`, "Later"), where it matters.

### Who may

A new permission **`channels.manage`** (internal/perm) for editing, backups and the picon set — admins only. Viewers see nothing new: ChannelHub needs `channels.manage` to be visible at all, since browsing the raw channel list isn't something a viewer needs.

### Mobile first

The editor is designed for a phone held in one hand: a long list with large rows (picon, name, now on), drag handles plus "move up/down/to position" buttons (dragging alone isn't accessible and is fiddly on a phone), multi-select for remove/move, a sticky bar with the draft's change count and "Review". On a wide screen the channel finder sits next to the bouquet. `DESIGN.md` gets the entry when it's built.

## Packages

| # | Package | Contents | Needs the box |
|---|---|---|---|
| C1 | Browse | bouquets (TV/radio) and their channels with picons and now/next; the channel snapshot, finder (search, new since, in no bouquet, TV/radio, HD/SD, provider); hub, permission, BouquetEditor detection | read-only check of the all-channels/provider service lists |
| C2 | Edit | draft (move, remove, add, marker, rename), review as one plan, backup → apply → auto-restore on failure, stale check, bouquet create/rename/delete/reorder | first real writes, each one approved by the owner |
| C3 | Backups | DreamHub's backups listed, restore as a plan, pruning | restore tested once |
| C4 | Check & picons | channel check; picon sets with coverage, DreamHub's chosen set | — (installed sets already known) |
| C5 | Prepared | allowed bouquets per profile in the model, the one server-side filter (pass-through) | — |
| later | Picon packages | list/install/remove picon packages from the feed via apt, as plans | read-only look at the feed first |

C1 and C4 can be built and finished against the mock. C2 and C3 are built against the mock too, but aren't released until the BouquetEditor writes have been tried on the reference box, one approved change at a time, and restored.

## Confirmed on the box, read-only (2026-10-09, `spikes/box-checks/channelhub.sh` + the BouquetEditor plugin's own Python source fetched from it)

- `/web/getservices` with the provider/all-services references works as assumed: counted 3320 TV services, 182 providers, 664 radio services, 72 radio providers on the reference box; the first TV bouquet's listing from plain WebControl and from `/bouqueteditor/web/getservices` matched entry for entry.
- `sRefBefore` ("" = at the end) is confirmed directly from `BouquetEditor.py`'s `addServiceToBouquet`/`addMarkerToBouquet`.
- `backup(Filename)` always writes to `/tmp/<name>.tar` — `BACKUP_PATH = "/tmp"`, hardcoded — after sanitizing `name` (anything outside `[A-Za-z0-9_. ]` collapses to a single `_`) and appending `.tar`; it never echoes the resulting path back in its XML answer.
- **Found a real bug from this**: `restore(Filename)` does *not* do the same join — it takes `Filename` as a literal path and just checks `exists()` on it (the plugin's own join call is commented out). `webcontrol.HTTPClient.EditorRestore` was sending the bare backup name straight through, so every real-box restore would have failed with "does not exist" — the auto-restore-on-failure safety net in `channels_edit.go`'s `withBackup` was silently broken for the real device, never caught because it only runs against the mock. **Fixed**: `EditorRestore` now reconstructs `/tmp/<sanitized name>.tar` itself (same commit, `backend/internal/webcontrol/bouqueteditor.go`, test in `bouqueteditor_test.go`).
- `restore()` *does* fully reload: `eDVBDB.removeServices()`, re-reads transponders, `reloadServicelist()`, `reloadBouquets()`, and even refreshes the currently-shown service list screen on the TV if one is open — no restart needed.
- `moveService`/`moveBouquet`'s `position` is passed straight through to the native `eDVBDB` binding (`mutableBouquetList.moveService(ref, position)`) with no Python-visible index arithmetic — reading the source doesn't resolve whether it counts from 0 or 1 with headers included; **still needs a real write** to confirm, as planned.
- Picon sets: several already on disk under `/data/piconpool/*` (1000–2600 files each) and more available as installable feed packages (`gp4-picon-*`, several thousand services' worth across Astra/Hotbird/cable providers) — "Save covers"-style picon fetching has real packages to offer, not just one.

## Still open

- Whether the TV's channel list updates at once after a change, or needs a reload (`/web/servicelistreload`) — covered by the above but worth re-checking after a real write.
- `position`'s base/offset (see above) — the first approved write, one change at a time, per the plan.
