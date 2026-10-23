# Box settings — plan

Status: **planned 2026-10-23, researched on both boxes the same day, nothing built.** The owner's decisions are below, including the answers of the same day (end of this file). Both sides were read on 2026-10-23, the VU+ Solo2 and the Dreambox One (`ARCHITECTURE.md`: "OpenWebif features beyond DreamHub's calls", "Dreambox One against the OpenWebif extras", "Box settings on both boxes"). The risk, restart and per-box lists are at the end of this file. Tracked in `ROADMAP.md`, v1.1, "Box features from OpenWebif". Once a package lands, its status moves to `ROADMAP.md`.

## Why

The receiver's own setup menus — recording, timeshift, EPG, channel list, A/V, HDMI-CEC, hard disk, subtitles, time zone, and those of installed plugins — are only reachable with the TV remote, one line at a time. OpenWebif shows them on the web; WebControl on a Dreambox does not. DreamHub can show them on a phone, find a setting by name, explain what a change does, keep a backup, and copy settings from one box to another — which neither the TV nor OpenWebif does.

## How enigma2 keeps its settings (the constraint everything follows from)

enigma2 holds every setting in memory and writes the whole of `/etc/enigma2/settings` on a clean shutdown (`mytest.py`, `configfile.save()`; checked on the Solo2). Editing the file from outside is overwritten then. The TV's setup menus and OpenWebif run **inside** enigma2: they set the value in memory — its notifiers run, so most settings take effect at once — and save. So DreamHub changes settings only from inside enigma2 too:

- **OpenWebif box (Basic)**: its own `/api/config` (sections), `/api/config/<section>` (items) and `/api/saveconfig` (`POST`, `key`, `value`). OpenWebif sets the value and calls the item's `save()`, but not `configfile.save()`: the file is written on the next clean shutdown. A power cut before that loses the change — the preview says so.
- **Dreambox**: WebControl can't set a config value (checked: no such template; `/web/settings` only lists), so DreamHub's own TV plugin, which runs inside enigma2, does it. It gets a small local endpoint (same place as `/dreamhub/play`): list sections and items, set one value, then — in this order, as the TV's own screens do — validate, `elem.value = x`, **`elem.save()`** (notifiers with `call_on_save` run there; `config.osd.language` does), **`elem.changedFinal()`** (the TV list calls it on deselect; other final notifiers, e.g. `usage.hdd_standby`, run only there), then `configfile.save()` and a re-read of the file line. Assigning `elem.value` does not validate on DreamOS (a wrong choice silently becomes the default), so the endpoint checks first. Details: `ARCHITECTURE.md`, "DreamOS config internals". Without `elem.save()` the file would get the old value. DreamOS builds its setup menus from `setup.xml` like VTi and has the same config classes (`ARCHITECTURE.md`).

Never: writing `/etc/enigma2/settings` directly, or stopping enigma2 to do so.

## What is shown

### Sections and items, from the box itself

The list comes from the box, not from DreamHub: the sections and items of enigma2's `setup.xml` (and, on VTi, of the three plugins that ship one), with the box's own titles in the box's language, the item's type (yes/no, choice, number with limits, text, slider, multiple choice) and its current value. On the Solo2 that is 18 sections through OpenWebif (21 in the file); "Recordings" alone has 68 items. **On a Dreambox it is much less: 10 sections and about 90 items** (`usage` has 61) — no plugin has a `setup.xml`, so recording paths, timeshift, HDMI-CEC, network and the plugins' settings are not in it. DreamHub adds a search across every section ("Zeitversatz", "HDMI", "Untertitel").

The form is generated from that answer. DreamHub knows no item in advance except for the risk list below.

### Simple / advanced / expert

Like the TV (`config.usage.setup_level`): every `setup.xml` item has a `level` (0 simple, 1 advanced, 2 expert). DreamHub has its own level switch, starting at the box's; it does not change the box's own level.

OpenWebif filters its answer by the box's level and does not pass the level on. **But `setup.xml` itself is readable over HTTP on both boxes** (`/file?action=download&file=/usr/share/enigma2/setup.xml`, checked), so DreamHub reads the level of every item (and `requires`, and on VTi `guirestart` and `description`) from the file and filters on its own side. What OpenWebif answers still depends on the box's level, so a level above the box's shows only what is in the file but not in the answer: those items are listed read-only ("raise the box's level to change") until S2/the plugin can serve them. Decision for S1: show the box's level only, say so, and use the file for the marks.

### Plugin and extension settings

- **Plugins with a `setup.xml` section** (VTi: HDMI-CEC, AutoShutDown, transcoding on the Solo2) show up by themselves; OpenWebif lists those marked `showOpenWebIF="1"`, all three here. **DreamOS has none** (checked on the One): no plugin there ships a `setup.xml`.
- **Plugins without a `setup.xml`** — all of them on a Dreambox — have their values under `config.plugins.<Name>` (and `config.cec`, `config.Nims`, …) in memory. The TV plugin can walk that tree with types and choices — but with key names instead of titles, so they appear at expert level only, labelled "set in the plugin's own screen; some take effect only after the plugin or the GUI restarts". This is the main body of a Dreambox's settings (907 saved entries on the One, ~90 from `setup.xml`), so S2 is mostly this walk.
- **Plugins with their own files** (checked 2026-10-23, `ARCHITECTURE.md`: "Plugin settings and files"): none of AutoTimer, EPGRefresh, SerienRecorder or AdvancedMovieSelection keeps its *settings* in a file of its own — they are `config.plugins.<Name>.*` (AdvancedMovieSelection: `config.AdvancedMovieSelection.*`) in `/etc/enigma2/settings`, so the walk above covers them. What they keep in files is *data*, not settings: `autotimer.xml`, `epgrefresh.xml`, `SerienRecorder.db`, `AdvancedMovieSelection.conf`. **Data is not part of box settings**: DreamHub never edits those files (the plugin holds them in memory or open), and their content belongs to the hub that manages it (timers, SerienRecorder). Only the Dreambox One has these plugins; the Solo2 has none.
- **Writing a plugin's settings, by plugin**: AutoTimer (`/autotimer/get`, `set`), EPGRefresh (`/epgrefresh/get`, set) and SerienRecorder (`/serienrecorderapi/settings`, `setsettings`) have web routes of their own that apply a change the way the plugin itself does (AutoTimer restarts its poller, EPGRefresh starts or stops its refresh). They persist unevenly: SerienRecorder saves everything, EPGRefresh saves the element but not the file, **AutoTimer neither** — and its timers are written to `autotimer.xml` only with `always_write_config` on (off here). So a plugin's route is used for the change and the TV plugin then `save()`s and `configfile.save()`s the same keys; for AdvancedMovieSelection and plugins without a route, the TV plugin's walk does it alone. The routes give names and values without types (AutoTimer, EPGRefresh) or a plugin-specific JSON (SerienRecorder, with the TV planner login in it — a secret); the types come from the walk.
- **The Gemini plugins (`Plugins/GP4/*`) keep their settings in a second file, `/etc/enigma2/gpsettings`** (323 lines, 30-odd roots such as `config.geminiGlobal`, `gcommander`, `receive`, `vavoo`), through their own `ConfigFile` — not `/etc/enigma2/settings`. `configfile.save()` does not write them, and *when* the plugin does is untraced (compiled code). So the TV plugin's walk must cover elements of this file too, and persisting a change there needs the plugin's own save (`GEMINICONFIGFILE`), not `configfile.save()`. Skin and Styles (`config.skin.*`, `config.Styles.*`) are in the normal `settings`; the look itself is files under `/usr/share/enigma2/<Skin>/`, not a setting. **Tested on the One 2026-10-23** (`ARCHITECTURE.md`, "When the Gemini settings reach `gpsettings`"): changing a Gemini setting in its own screen and saving does not write the file; it is written at the next clean enigma2 shutdown (11 to 16 s before `settings`), and the whole file is rewritten in a different order each time. So after a change DreamHub offers "Restart GUI" to save at once, as for the VU+ — a GUI restart leaves the box on its startup service, not the one watched. Whether the TV plugin can save that `ConfigFile` itself (import from `Plugins.GP4.geminicomm`) is still untried. Gemini keys are shown read-only until S2 has tried that; a snapshot compares `gpsettings` sorted. Their credentials files are never copied (see below).
- **More than one settings file on a Dreambox (checked 2026-10-23):** `/etc/enigma2/settings`, the Gemini `gpsettings` and MediaPortal's `mp_config` (prefix `config_mp.mediaportal.*`) — three `ConfigFile`s. The walk marks each key with the file it belongs to, and persisting a change means saving *that* file; until a way to save the other two is proven they are read-only. The full list of secrets, state keys and own data files is in `ARCHITECTURE.md`, "Plugin data survey on the Dreambox One": the plan's secret handling is an explicit deny-list of real key names (ParentalControl PINs, SerienRecorder IMAP login, Gemini `serienstream`/`tunein`/samba/www logins, `gzerotier.apikey`, …) with name patterns only as a fallback — plain `key` and `auth` are too broad. Plex/Emby/eWeLink logins and `webbrowser.db` cookies are files, not settings, and never part of a snapshot or copy.
- **DreamHub's own extensions** keep their settings in AppsHub → Extensions; the box settings page links there instead of repeating them.

## Changing a setting

### Every change is a plan

One or more changed values become one plan: each line old → new value, with the box's own labels. Confirm applies them one by one, reads each back, and stores the old values (backup, below). Stale check like every plan: a value that changed on the box since the preview is not overwritten — preview again.

### Settings that can make the box unusable

Owner's decision: everything is shown; a risky item is marked, and changing it needs an extra step. The risk list is DreamHub's own, by key prefix, kept in one place and growing as boxes teach us:

- **Picture and sound output** — video mode and rate, port, colour format, HDMI settings (`config.av.*`): a wrong one can leave the TV black.
- **Access to the box itself** — the web interface's port, login and HTTPS (`config.plugins.Webinterface.*`, `config.OpenWebif.*`), and anything DreamHub needs to reach the box: a wrong one locks DreamHub out.
- **Network**, **tuner configuration** (`config.Nims.*`), **parental control and its PIN** (`config.ParentalControl.*`), **softcam**, **deep standby and wake-up** (`config.usage.*shutdown*`, AutoShutDown) — mostly not in `setup.xml`, so they appear only through the plugin tree walk, still marked.

The extra step: the warning says what can go wrong and how to undo it on the TV; then the **profile's PIN** (the one from profiles, `PUT /api/v1/profiles/me/pin`) before the plan can be confirmed; a profile without a PIN gets the warning in red with "Are you sure?" instead.

**Decided** (see the end): for picture output and access settings, the change is **taken back by itself after 60 seconds unless it is confirmed again** from DreamHub — like a PC's "keep these display settings?". If the TV went black or DreamHub can no longer reach the box, nothing needs doing. This is the same kind of safety step ChannelHub already takes (an automatic restore when a change fails), not an automatic change of its own; it only works on a Dreambox (the TV plugin keeps the timer inside enigma2 — over OpenWebif DreamHub can't take a change back if the box no longer answers).

### Settings that take effect only after a restart

Most settings act at once, because the value changes inside enigma2 and its notifiers run, exactly as on the TV. Some don't: the skin, the OSD language on some images, a few A/V and hard disk settings, and plugin settings changed past the plugin's own screen. The box does not say which. So:

1. The **keys that need a GUI restart** mark them before the change ("takes effect after restarting the GUI"). On VTi the box says so itself: `guirestart="yes"` on 44 items of `setup.xml` (list below), read through `/file` — nothing to maintain. DreamOS has no such attribute; there DreamHub keeps a short list of its own (below).
2. After applying a plan with such a key, DreamHub offers **"Restart GUI"** as its own plan. Its preview checks the timers first: restarting enigma2 stops a running recording, so a running one is named and the restart refused unless confirmed for it.
3. For plugin settings changed through the tree walk the line always says "may need a GUI restart".

## The lists (researched 2026-10-23)

Kept in one place in the code (`backend/internal/boxsettings/lists.go` or similar), as key prefixes; they grow as boxes teach us. Sources: `setup.xml` and enigma2's Python source on both boxes, the One's `/web/settings`, the Solo2's `/api/config`.

### Restart (GUI)

- **VTi: from the box** — `guirestart="yes"`, 38 distinct keys: `config.usage.` `channelzap_w_bouquet`, `bqt_change_w_cross`, `favourite_list_w_ok`, `enable_ok_double_click`, `default_zero_double_click_mode`, `default_pip_mode`, `servicelist_*` (alternative mode, mark rec service, rec symbol, two lines, picon, picon_dir, previewpic, service type icon, matrix, name width, event time, servicenumber, hide service name, next event, preview mode), `show_epg_progress_percent`, `movielist_show_cover`, `enable_secondinfobar`, `rc_style`, `picon_dir`, `picon_scale`, `config_icons`, `use_pig`, `use_extended_pig`, `use_extended_pig_channelselection`, `check_for_updates`, `debug_advanced`; `config.seek.smartseek_enable`; `config.misc.db_enabled`, `db_path`. The skin (`config.skin.*`) is not marked in the file but is changed by the skin plugin, which restarts itself.
- **DreamOS: DreamHub's own list.** Known to need it: `config.skin.primary_skin` and `config.Styles.*` (their plugins call `TryQuitMainloop 3`); the web interface — `config.plugins.Webinterface.*` port/http/https/auth — which is applied only by `restartWebserver()` (not a GUI restart, and it drops DreamHub's connection for a moment: say so). `config.osd.language` activates when saved (no restart), but screens already open and a skin's own texts keep the old language until the GUI restarts: "may need a GUI restart". For the 10 core sections the Python source shows no restart need (most have a notifier or are read where used; none says otherwise). For `config.plugins.*` the line always says "may need a GUI restart". To confirm on the One while building S2, not before: `config.misc.use_legacy_virtual_subservices_detection`, `config.usage.max_stream_resolution`, `ts_streaming_buffer`, `non_ts_streaming_buffer` (referenced only where they are defined in Python, so read elsewhere — unclear).

### Risk (extra step)

By key prefix, same on both boxes unless noted. Category **A** = lockout or black screen (gets the 60-second take-back on a Dreambox), **B** = needs the PIN step only.

- **A — picture and sound output**: `config.av.*` (on the One: `videomode.HDMI`, `videoport`, `colorformat`, `tvsystem`, `aspect*`, `policy_*`, `hdr10_support`, `hlg_support`, `allow_10bit`, `allow_12bit`, `sync_mode`, `preferred_modes_only`, …), `config.osd.*` (alpha, brightness, contrast: an invisible menu), `config.rfmod.*` (VTi).
- **A — access to the box**: DreamOS `config.plugins.Webinterface.*` (`http.port`, `https.port`, `*.enabled`, `*.auth`, `localauth`, `anti_hijack`, `streamauth`); VTi `config.OpenWebif.*` — `enabled`, `port`, `streamport`, `https_enabled`, `https_port`, `auth`, `https_auth`, `https_clientcert`, `no_root_access`, `local_access_only`, `vpn_access`, `auth_for_streaming`; and `config.network.*` (VTi has `dhcp`, `ip`, `netmask`, `gateway`, `dns` in `setup.xml`; the One has `config.network.wol_enabled` only, its network lives in the system files). Do not offer the port of the box DreamHub talks to as a plain item at all, only the A-step.
- **B — tuner configuration**: `config.Nims.*` (114 entries on the One), `config.sat.*`, `config.ci.*` (CI slots).
- **B — parental control**: `config.ParentalControl.*`, `config.parental.*` (setup PIN, service PIN: **never show the values**, also not in a plan or log; show "changed").
- **B — power and standby**: `config.usage.inactivity_shutdown*`, `on_long_powerpress`, `on_short_powerpress`, `standby_zaptimer_wakeup`, `config.autoshutdown.*` (VTi plugin), `config.network.wol_enabled`, `config.misc.prev_wakeup*` (state, not offered at all).
- **B — side effects on other devices**: `config.cec.*` (One), `config.hdmicec.*` (VTi): a wrong value switches the TV or the receiver on and off.
- **B — storage**: `config.storage.*`, `config.storage_options.*`, `config.usage.timer_path`, `timeshift_path`, `config.movielist.*` paths, `config.misc.epgcache_filename`: a path that does not exist stops recording.
- **B — softcam / CI and streaming**: `config.plugins.*` of softcam plugins, `config.streamservices.*`, transcoding (`config.plugins.transcodingsetup.*`).
- **Never writable from DreamHub**: values that are passwords or PINs are write-only fields (set, not shown); `config.misc.firstrun`, `startCounter` and the other runtime state below.

### Box-specific (left out when copying, and from the snapshot diff)

- **Identity and hardware**: `config.Nims.*`, `config.sat.*`, `config.ci.*`, `config.inputDevices.*`, `config.rc.*`, `config.fp.*`, `config.lcd.*`, `config.rfmod.*`, `config.network.*`, `config.cec.name`/`config.hdmicec.device_name`, `config.storage.<uuid>.*` and `config.storage_options.*` (disk UUIDs), `config.av.videomode.*`/`videoport`/`colorformat`/`hdr*`/`allow_*` (the TV decides), `config.usage.hdd_standby`, `config.misc.epgcache_filename`, `config.misc.record_*_buffer`, `config.misc.bootprogressbg`, `config.cmdline.*`, `config.timezone.*` only if the user says so.
- **Access**: `config.plugins.Webinterface.*`, `config.OpenWebif.*` (ports and logins).
- **Runtime state, not choices** (also dropped from a snapshot's "differs" list): `config.misc.startCounter`, `firstrun`, `defaultchosen`, `prev_wakeup_time`, `prev_wakeup_time_type`, `rcused`, `initialchannelselection`, `initialharddisknotification`, `missingdefaultstoragenotification`, `fsbl_update_never`, `graph_mepg_current_service`, `config.startupservice.*`, `config.startupserviceleavingstandbymode.*`, `config.servicelist.lastmode`, `config.OpenWebif.webcache.*`.
- **Not on the other box by name**: VTi `config.usage.*` (~230 keys, most with VTi prefixes `servicelist_`, `movielist_`) and `config.autolanguage.*`, `config.subtitles.*` have no counterpart on DreamOS and vice versa (`config.cec.*` vs `config.hdmicec.*`, `config.timezone.region/zone` vs `val`, `config.plugins.Webinterface` vs `config.OpenWebif`). The "same key, same type, choice known to the target" rule drops them into "not on the target"; no mapping table.

### What the snapshot is

`/web/settings` and `/etc/enigma2/settings` are the same thing: the `saved_value`s. **On VTi (and stock enigma2) only values that differ from the default** are written; **DreamOS writes defaults too** (`save_forced`, checked in the source and in the One's file: `usage.hdd_standby=600` is its default), so on the One a missing key means "never saved", not "default", and a restore cannot assume absent = default. A snapshot is therefore a diff against the box's defaults, and a restore sets those keys and **leaves everything else as it is** — it does not put a key back to default unless the snapshot recorded the default explicitly. The snapshot holds PIN and password values: it is treated as a secret (stored like the box passwords, never shown or logged). On VTi OpenWebif has no `/web/settings`; the file comes through `/file?action=download&file=/etc/enigma2/settings` instead.

## Backup and undo

- **Every applied change** keeps the old value per key, box and time in DreamHub's database; "Undo" on a change is a plan that sets the old values back.
- **A full snapshot** before the first change of a day: every setting the box has saved (a Dreambox: `/web/settings`, or `/etc/enigma2/settings` through `/file`; a VU+: the file through `/file`), kept per box, the last few — see "What the snapshot is". Restoring a snapshot is a plan with every differing value — only through enigma2, as above, never by writing the file.
- Snapshots go into DreamHub's own backup (Settings → System).

## Copying settings between boxes

Owner's decision: yes. Pick a source box, a target box and the sections; DreamHub shows every value that differs, as one plan, and applies it to the target like any change. Left out by default, since they belong to one box:

- the "Box-specific" list above (tuner and satellite configuration, network, host name, video output, remote control, front panel, LED, disks, the web interface's port and login, runtime state).

Only keys that exist on both boxes with the same type and a choice the target knows are offered; a VTi box and a Dreambox share many `config.usage.*` keys but not all — the rest is listed as "not on the target". Risky keys keep their extra step.

## Packages

- **S1 — view (OpenWebif)**: sections, items, search, the box's level, the risk and restart marks, read-only. `setup.xml` through `/file` supplies `level`, `guirestart`, `description`. Against the mock (fixtures from the Solo2) and the Solo2.
- **S2 — view (Dreambox)**: the TV plugin's endpoint (the 10 core sections from `setup.xml`, which `/file` already serves, plus the `config.plugins.*` tree walk for everything else; types and choices only the plugin can tell).
- **S2b — plugin routes (Dreambox, after S3)**: apply AutoTimer, EPGRefresh and SerienRecorder changes through their own routes, then persist through the TV plugin (see above). Until then the walk alone writes them.
- **S3 — change**: plans, read-back, the per-change backup and Undo, the PIN step for risky keys; the full snapshot.
- **S4 — restart**: the "Restart GUI" plan with the recording check.
- **S5 — copy between boxes.**
- **S6 — the 60-second take-back** (Dreambox).

## Decided 2026-10-23 (the owner's answers)

1. **The 60-second take-back: yes** (package S6, Dreambox only, see above).
2. **The profile PIN alone** for risky keys. A profile without a PIN gets no extra input, but the warning is shown in red with "Are you sure?" before the plan can be confirmed — never the DreamHub password.
3. **Admins only**, like DeviceHub.
4. **OpenWebif's missing `configfile.save()`**: after applying on a VU+, DreamHub **offers** "Restart GUI" to save at once (the same plan as in S4, with the recording check), next to **"OK"** (keep the change; it is saved on the next clean shutdown) and **"Undo"** (set the old values back). Offered, never done by itself.

## Open for the owner (from the plugin check, 2026-10-23)

1. **Plugin routes or the walk alone?** Recommendation: S2 and S3 use the walk only (one mechanism, same for every plugin); S2b adds the plugins' own routes where a setting needs the plugin's follow-up (restart the AutoTimer poller, start the EPGRefresh run). Say if the follow-up isn't worth a second route.
2. **Should a snapshot include the plugins' data files?** `autotimer.xml` and `epgrefresh.xml` are small and readable through `/file`; `SerienRecorder.db` is a live SQLite file (SerienRecorder has its own backup in `/media/hdd/SR_Backup`). Recommendation: the snapshot holds settings only; AutoTimer/EPGRefresh data belong to the hub that manages them, and SerienRecorder keeps its own backup. Files with credentials (`gemini-www-password.txt`, `geminitelegramtoken.json`) never go in.
3. **Gemini settings (`gpsettings`): read-only first?** Recommendation: yes — show them, but write only after S2 has found the plugin's own save call on the One; they're the image's own plugins (Netcast, Commander, widgets), so a wrong value is more likely to break a screen than the box.

## What is still missing (2026-10-23)

The OpenWebif write path was tested on the Solo2 (1 to 3); nothing has been written on the One yet, and everything about it is read from source, files and GETs. What still needs a real test, on the One in S2:

1. ~~**A harmless write through OpenWebif's `saveconfig`**~~ **Done on the Solo2, 2026-10-23** (`ARCHITECTURE.md`, "Writes on the Solo2"): takes effect at once, reads back, the file stays unchanged.
2. ~~**ConfigSet toggle**~~ **Done**: each call adds or removes the value.
3. ~~**The power-cut claim**~~ **Done** (`killall -9 enigma2`): the unsaved value is gone; a GUI restart (`powerstate?newstate=3`) writes it. This confirms decision 4 above: after a VU+ change DreamHub offers "Restart GUI" to save at once.
4. **How DreamHub's TV plugin on the One can walk `config.*`** (the plugin exists in `Extensions/DreamHub`): list `config.plugins.*` with types, limits and choices, then set and read back one value. This is the basis of S2 and untested.
5. **The save call of the Gemini plugins** (`gpsettings`), and whether `elem.save()` + `configfile.save()` alone is enough for the other plugins on the One (EPGRefresh and AutoTimer settings are the test: they differ in their own route, see above).
6. **Plugin routes**: EPGRefresh's route names for settings and channels (only `/epgrefresh` and `/epgrefresh/get` were called) and when AutoTimer writes `autotimer.xml` without `always_write_config`.
7. **Data files of other plugins**: MediaPortal (`mp_*`), EventDataManager, PlexDream/EmbyDream (`config.plugins.plexdream`, `embydream` probably hold server logins) — check for credentials before the walk shows them; the walk needs a name-based filter (`password`, `token`, `pin`, `login`, `key`) that shows "set" instead of the value.
8. **Web login on** (still open, changes the box) and **other OpenWebif images**.

## S2 design (2026-10-23, from a delegated study; the sketch was run off-box only)

Full evidence in `ARCHITECTURE.md`, "DreamOS config internals". What the design settles:

- **Placement**: pure logic in `enigma2-plugin/src/boxsettings.py` (no Twisted, unit-testable off the box), the Twisted resource in `websettings.py`, registered in `webplay.register()` inside a `try/except` so a bug cannot break `/dreamhub/play`; plugin and backend version bumped together.
- **Routes** under `/dreamhub/settings/`: `GET info` (api version, setup level, language, stores `settings` and `gp` with availability), `GET sections` (the 10 core sections, from the DOM `Screens.Setup` already parsed — headings are items without a body), `GET items?store=&section=|root=&q=&offset=&limit=`, `GET item?store=&path=` (full choice list; listings cap at 64 choices), and in the write release `POST set`. Application errors are HTTP 200 with `{"ok": false, "error": code}` (the Go client keeps only 200 bodies), 405/404 as usual.
- **Item shape**: `store`, `path`, `kind` (choice, bool, integer, number, text, password, pin, directory, slider, multichoice, sequence kinds, selectionNumber, read-only kinds), `secret` (by class `ConfigPassword`/`ConfigPIN` or by the deny-list; value `null` plus `isSet`), `readOnly`, `dirty` (a TV screen holds an unsaved edit), `notifiers [immediate, final]`, `rev`, `default`, `value`, `choices`, `limits`, `step`.
- **Stale check**: `rev` = first 16 hex of sha1 of `tostring(value)|saved_value`, returned by every listing and sent back with `set`; the check and the write run in one reactor turn (atomic). A mismatch answers `stale` with the current item; `dirty` refuses unless `allowDirty`, because `save()` would persist someone's half-finished edit.
- **Write guards**: POST only, `Content-Type: application/json` plus a custom header (forces a CORS preflight that a plain Twisted resource never grants), `Host` an IP literal or the box's own name (DNS rebinding), a kill switch `config.plugins.dreamhub.settingsWrite`, `dryRun` for the preview, `confirmRisk` for the lockout prefixes (`Webinterface.*`, `inputDevices.*.enabled`, `av.videomode.*`, `fans.*`; sent by Go only after the PIN step), hard refusal of runtime-state keys and of `save_disabled` elements, rollback of value and notifiers if a notifier raises, read-back of the element, of `saved_value` and of the file line (`persisted`).
- **Take-back (S6) for category A**: apply in memory only (`value`, `changedFinal()`, no `save()`, no flush); on confirm `save()` and flush; if the box dies first the file still holds the old value — and `saved_value` stays old, so another plugin's `configfile.save()` cannot persist the unconfirmed one. [guess, not run in enigma2]
- **Go side**: a `BoxSettings` interface next to the BouquetEditor one (`SettingsInfo`, `SettingSections`, `SettingItems`, `SettingItem`, `CheckSetting`, `ApplySetting`), typed `*SettingError{Code, Message, Current}`, a mock with the same semantics plus hooks (`NoSettingsPlugin`, `StaleOnce`, `FailSetting`) seeded from recorded fixtures. **The Solo2 maps onto the same interface**: `/api/config` → sections; `/api/config/<key>` → items (`select`→choice, `checkbox`→bool, `number`→integer with limits, `text`, `slider`, `multicheckbox`→multichoice, `unknown` with `result:false`→read-only; strip the " (min - max)" suffix OpenWebif adds to the description); `/api/saveconfig` → set (a multicheckbox is a toggle: diff the lists and send one call per element, reading back in between; ints are clamped silently, so validate first). Secrets show as "unknown" on OpenWebif by accident (it matches the exact class `ConfigText`), not by rule.
- **Test plan, nothing production touched until the last steps**: (0) off-box against the One's real `config.py` (the sketch passed, Python 3 with stubs; it compiles under the box's 2.7) — a test in the plugin directory reading `config.py` from an environment path, skipped if absent; (1) **first deploy read-only**: ship `info`/`sections`/`items`/`item` only, with no write function in the shipped file at all; needs one GUI restart (no running recording or imminent timer; the box then zaps to its startup service); checks: 10 sections, ~87 paths resolve, whole-tree walk well under 1 s, secrets `null`, `settings`/`gpsettings` checksums unchanged, no errors in the log; (2) a dev-only `selftest` store (a standalone `Config()` with one element of every kind, flushed to `/tmp`) registered while a flag file exists; (3) harmless real keys one at a time with restore and a sorted before/after diff of `settings` (candidates: `usage.timerlist_finished_timer_position`, `usage.show_message_when_recording_starts`, `usage.infobar_timeout`, `seek.speeds_slowmotion`); last-resort restore: copy the backup over `settings` **and** `kill -9` enigma2 (a clean shutdown would overwrite it); (4) Gemini: `config.gemininetcast.remove_tmpdir`, the `gp` store read-only until `gpconfset.saveToFile` is proven — a sorted diff must show only that key.

### Open for the owner (from the study)

1. **Plugin-side kill switch** `config.plugins.dreamhub.settingsWrite`: default off, switched on at the TV? The recommendation is yes: with `localauth` and `anti_hijack` off, anything on the LAN can reach the endpoint.
2. **Gemini path form**: `(store: "gp", path: "config.gemininetcast.x")` with the walk rooted at `gpconfset` — fine?
3. **Timezone**: region is never persisted and drives the zone choices; a pair-write, or keep it read-only in the first write release? Recommendation: read-only.
4. **`config.osd.language`**: a free text; writable only after a validator against the language list — in S3 or later?
5. **Batches**: one `changes: [...]` request with a single flush for a multi-line plan, or one request per line? Decide in S3.
6. **Plugins without notifiers**: always "may need a GUI restart" (recommended) or use the notifier count as the hint?
