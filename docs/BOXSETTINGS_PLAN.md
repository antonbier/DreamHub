# Box settings — plan

Status: **planned 2026-10-23, nothing built.** The owner's decisions are below; the open questions at the end are still his to answer. The OpenWebif side was read on the VU+ Solo2 (`ARCHITECTURE.md`, "VU+ Solo2 with VTi 15", "OpenWebif features beyond DreamHub's calls"); the Dreambox side is not checked yet (the One was off). Tracked in `ROADMAP.md`, v1.1, "Box features from OpenWebif". Once a package lands, its status moves to `ROADMAP.md`.

## Why

The receiver's own setup menus — recording, timeshift, EPG, channel list, A/V, HDMI-CEC, hard disk, subtitles, time zone, and those of installed plugins — are only reachable with the TV remote, one line at a time. OpenWebif shows them on the web; WebControl on a Dreambox does not. DreamHub can show them on a phone, find a setting by name, explain what a change does, keep a backup, and copy settings from one box to another — which neither the TV nor OpenWebif does.

## How enigma2 keeps its settings (the constraint everything follows from)

enigma2 holds every setting in memory and writes the whole of `/etc/enigma2/settings` on a clean shutdown (`mytest.py`, `configfile.save()`; checked on the Solo2). Editing the file from outside is overwritten then. The TV's setup menus and OpenWebif run **inside** enigma2: they set the value in memory — its notifiers run, so most settings take effect at once — and save. So DreamHub changes settings only from inside enigma2 too:

- **OpenWebif box (Basic)**: its own `/api/config` (sections), `/api/config/<section>` (items) and `/api/saveconfig` (`POST`, `key`, `value`). OpenWebif sets the value and calls the item's `save()`, but not `configfile.save()`: the file is written on the next clean shutdown. A power cut before that loses the change — the preview says so.
- **Dreambox**: DreamHub's own TV plugin, which runs inside enigma2. It gets a small local endpoint (same place as `/dreamhub/play`): list sections and items, set one value, then `configfile.save()` at once. Not built until the One is checked: whether DreamOS builds its setup menus from `setup.xml` the same way, and what its enigma2 calls the config classes.

Never: writing `/etc/enigma2/settings` directly, or stopping enigma2 to do so.

## What is shown

### Sections and items, from the box itself

The list comes from the box, not from DreamHub: the sections and items of enigma2's and each plugin's `setup.xml`, with the box's own titles in the box's language, the item's type (yes/no, choice, number with limits, text, slider, multiple choice) and its current value. On the Solo2 that is 18 sections; "Recordings" alone has 68 items. DreamHub adds a search across every section ("Zeitversatz", "HDMI", "Untertitel").

The form is generated from that answer. DreamHub knows no item in advance except for the risk list below.

### Simple / advanced / expert

Like the TV (`config.usage.setup_level`): every `setup.xml` item has a `level` (0 simple, 1 advanced, 2 expert). DreamHub has its own level switch, starting at the box's; it does not change the box's own level.

On OpenWebif the answer is already filtered by the box's level, so a higher level than the box's can't be shown there — the switch then says so. The TV plugin on a Dreambox can return the level per item and filter in DreamHub.

### Plugin and extension settings

- **Plugins with a `setup.xml` section** show up by themselves, as soon as the plugin is installed — on a Dreambox through the TV plugin. OpenWebif only lists plugin sections marked `showOpenWebif="1"` (on the Solo2: AutoShutDown, transcoding), so on a VU+ most plugin settings stay hidden.
- **Plugins without a `setup.xml`** (many keep their own setup screen) have their values under `config.plugins.<Name>` in memory. The TV plugin can walk that tree with types and choices — but with key names instead of titles, so they appear at expert level only, labelled "set in the plugin's own screen; some take effect only after the plugin or the GUI restarts".
- **DreamHub's own extensions** keep their settings in AppsHub → Extensions; the box settings page links there instead of repeating them.

## Changing a setting

### Every change is a plan

One or more changed values become one plan: each line old → new value, with the box's own labels. Confirm applies them one by one, reads each back, and stores the old values (backup, below). Stale check like every plan: a value that changed on the box since the preview is not overwritten — preview again.

### Settings that can make the box unusable

Owner's decision: everything is shown; a risky item is marked, and changing it needs an extra step. The risk list is DreamHub's own, by key prefix, kept in one place and growing as boxes teach us:

- **Picture and sound output** — video mode and rate, port, colour format, HDMI settings (`config.av.*`): a wrong one can leave the TV black.
- **Access to the box itself** — the web interface's port, login and HTTPS (`config.plugins.Webinterface.*`, `config.OpenWebif.*`), and anything DreamHub needs to reach the box: a wrong one locks DreamHub out.
- **Network**, **tuner configuration** (`config.Nims.*`), **parental control and its PIN** (`config.ParentalControl.*`), **softcam**, **deep standby and wake-up** (`config.usage.*shutdown*`, AutoShutDown) — mostly not in `setup.xml`, so they appear only through the plugin tree walk, still marked.

The extra step: the warning says what can go wrong and how to undo it on the TV; then the **profile's PIN** (the one from profiles, `PUT /api/v1/profiles/me/pin`; without one, the DreamHub password) before the plan can be confirmed.

**Proposed on top** (needs the owner's yes, see questions): for picture output and access settings, the change is **taken back by itself after 60 seconds unless it is confirmed again** from DreamHub — like a PC's "keep these display settings?". If the TV went black or DreamHub can no longer reach the box, nothing needs doing. This is the same kind of safety step ChannelHub already takes (an automatic restore when a change fails), not an automatic change of its own; it only works on a Dreambox (the TV plugin keeps the timer inside enigma2 — over OpenWebif DreamHub can't take a change back if the box no longer answers).

### Settings that take effect only after a restart

Most settings act at once, because the value changes inside enigma2 and its notifiers run, exactly as on the TV. Some don't: the skin, the OSD language on some images, a few A/V and hard disk settings, and plugin settings changed past the plugin's own screen. The box does not say which. So:

1. A **list of known keys** that need a GUI restart, in the same place as the risk list, marks them before the change ("takes effect after restarting the GUI").
2. After applying a plan with such a key, DreamHub offers **"Restart GUI"** as its own plan. Its preview checks the timers first: restarting enigma2 stops a running recording, so a running one is named and the restart refused unless confirmed for it.
3. For plugin settings changed through the tree walk the line always says "may need a GUI restart".

## Backup and undo

- **Every applied change** keeps the old value per key, box and time in DreamHub's database; "Undo" on a change is a plan that sets the old values back.
- **A full snapshot** before the first change of a day: every setting the box has saved (`/etc/enigma2/settings` through the TV plugin; OpenWebif's `/web/settings` on a VU+), kept per box, the last few. Restoring a snapshot is a plan with every differing value — only through enigma2, as above, never by writing the file.
- Snapshots go into DreamHub's own backup (Settings → System).

## Copying settings between boxes

Owner's decision: yes. Pick a source box, a target box and the sections; DreamHub shows every value that differs, as one plan, and applies it to the target like any change. Left out by default, since they belong to one box:

- tuner and satellite configuration, network, host name, video output (the TVs differ), the remote control's type, front panel and LED, hard disk standby, the web interface's port and login, everything under `config.misc.*`.

Only keys that exist on both boxes with the same type and a choice the target knows are offered; a VTi box and a Dreambox share many `config.usage.*` keys but not all — the rest is listed as "not on the target". Risky keys keep their extra step.

## Packages

- **S1 — view (OpenWebif)**: sections, items, search, the level switch, the risk and restart marks, read-only. Against the mock (fixtures from the Solo2) and the Solo2.
- **S2 — view (Dreambox)**: the TV plugin's endpoint, after the check on the One; plugin settings through the tree walk.
- **S3 — change**: plans, read-back, the per-change backup and Undo, the PIN step for risky keys; the full snapshot.
- **S4 — restart**: the "Restart GUI" plan with the recording check.
- **S5 — copy between boxes.**
- **S6 — the 60-second take-back**, if the owner wants it.

## Open questions

1. **The 60-second take-back** for picture output and access settings: wanted? It is a step the box takes on its own if nobody confirms.
2. **PIN or password**: the profile PIN for risky keys, and the DreamHub password when the profile has none — or always the password?
3. **Who may change box settings**: admins only (like DeviceHub), or a separate permission?
4. **OpenWebif's missing `configfile.save()`**: accept that a VU+ keeps a change only after a clean shutdown (said in the preview), or offer a "Restart GUI" there too, which saves on the way?
