# v1 plan (draft for discussion)

> **Status 2026-10-04 (release 0.1.0):** this plan was written on 2026-09-24 and its "Where we are" describes that day. Since then WP1–WP9 are done and confirmed on the reference box, ControlHub, RadioHub and AdminHub's service status were built early, and live updates use Server-Sent Events rather than WebSockets. What's left is WP10 (hardening) and the feed (D4). `ROADMAP.md` is the authoritative status; the decisions below still hold.

Plan for getting from today's state to a released **DreamHub 1.0.0**. It builds on `ROADMAP.md` (authoritative status) and the device findings of 2026-09-24 in `ARCHITECTURE.md`. Once the open decisions below are settled, the work packages become GitHub issues under the existing milestones and this file is folded into `ROADMAP.md`.

## Where we are

- **Built**: the whole core shell (auth, API tokens, settings, dashboard, navigation, search, notifications snapshot, onboarding, diagnostics, i18n DE/IT/EN, time zones), preview-and-confirm (`internal/plan`), and read-only screens for MediaHub, GuideHub, SeriesHub, DiscoveryHub. DiscoveryHub → TimerHub "create timer" is the one real write flow.
- **Not built**: any TimerHub/SystemHub/AdminHub screen (placeholder `HubView.vue`), every MediaHub write, the TV plugin (`enigma2-plugin/` is a README), packaging, live updates/event bus.
- **Newly confirmed on the device**: `/web/timeradd` and `/web/timerdelete` work (live test, reverted). WebControl's `moviedelete` is a hard delete and `moviemove` can't rename, so MediaHub keeps its own file component. AdvancedMovieSelection's trash convention is known. journald is volatile. Details in `ARCHITECTURE.md`.

## Cross-cutting rules for all of v1

1. **Optional plugins never break DreamHub.** AdvancedMovieSelection, AutoTimer, SerienRecorder and WebBouquetEditor may not be installed. Each one is an optional, runtime-detected capability. A missing one turns the dependent feature *degraded/unavailable* with reason and fix, never an error screen. Where DreamHub adopts a plugin's convention (AMS trash), it works without the plugin. Mocks get a "not installed" variant and tests cover it.
2. **Device writes are verified, not trusted.** After every apply, read the state back (e.g. `/web/timerlist`). `e2state=True` alone isn't proof (the box reports a dropped duplicate timer as success).
3. **Anything that needs the box gets a live check before the WP is closed**, with the owner's approval and reverted afterwards, following the precedent set for #11 and the timer test.
4. The definition of done in `PRINCIPLES.md` applies to every WP: docs, error envelope, a device-free test, resource budget.

## Decisions needed before starting

| # | Question | Recommendation |
|---|---|---|
| D1 | **Trash**: adopt AMS's `.trash` suffix convention, or DreamHub's own trash folder? | **AMS convention.** The TV wastebasket and DreamHub then show the same trash, and AMS's auto-empty keeps working. DreamHub implements list/restore/empty itself, so it also works without AMS. |
| D2 | **Rename suggestions** | **Decided and built (WP4)**: three suggestion sources, all optional, all ending in the same preview: (1) template from metadata (`.ts.meta`/EPG/SerienRecorder), (2) TMDB matching, (3) the AI provider (OpenAI first). The AI follows DiscoveryHub's grounding rule: it gets the recording's real metadata and returns structured fields (title, season, episode, episode title, year). DreamHub builds the file name from those fields with its own template and sanitizer, never from free text. Counted against `ai.dailyLimit`, always an explicit click, bulk requests batched. |
| D3 | **AutoTimer in v1**: list + dry-run + link to `/autotimereditor`, or full editing? | List + `simulate` + link, like SerienRecorder's GUI link. Full editing duplicates a working UI. |
| D4 | **Where the apt feed is hosted** | **Deferred** — decided much later, before WP9. |
| D5 | **Enable/disable a timer** in v1? `timerchange` has a buggy conflict path. | **Decided 2026-10-04: both, without `timerchange`** — delete + add with only `disabled` changed, all other fields carried over, full preview (enabling shows the conflict estimate), the original put back if the box refuses; refused while the timer records. |
| D7 | **Exact conflict dry run** via DreamHub's own plugin (see "Tuners")? | Yes, but after layer 2. Build it with the TV plugin (WP8). Until then the transponder-aware estimate plus the box's verdict are enough. |
| D6 | **Release gate**: how long to dogfood on the reference box before 1.0.0? | Two weeks on the box with no data-affecting bug. |
| D8 | **Pull ControlHub into v1**, or leave it for v1.1 as originally scoped (`CONCEPT.md`)? | **Pulled into v1** (decided 2026-09-25): verifying WP8's Enigma2 plugin needed a real screenshot/remote-control loop anyway (discovered `/screenshot`, `/web/remotecontrol`, `/web/zap`, `/web/powerstate` — see `ARCHITECTURE.md`), and GuideHub's "switch to this channel now" needs it too. Scoped to what's already confirmed working: screenshot, remote pad, power, zap. "Message to TV" (`/web/message`) stays v1.1 — not yet researched. |

## Tuners: beyond the single-tuner reference box

The reference box has one tuner, but the Dreambox world has many more: several tuners (e.g. three on a DM7080), twin tuners, **FBC tuners** (one input, up to 8 demodulators, typically listed as 8 frontends in `/web/deviceinfo` even though they share an input), mixed delivery systems (satellite + cable + terrestrial), Unicable/JESS setups. DreamHub's current check (`peak concurrent timers + 1 > number of frontends`) is wrong in both directions:

- **Too strict**: one tuner can record several channels on the **same transponder** at once (e.g. RTL and VOX). Today that's flagged as a conflict.
- **Too lax**: 8 FBC frontends aren't 8 independent tuners, and a DVB-C tuner can't record a satellite channel.

The box itself gets this right. `TimerSanityCheck` simulates a real tuner allocation for every overlapping timer (`recordService(…, simulate).start(True)`), and `/web/timeradd` refuses conflicting timers atomically. Proposal, in three layers:

1. **The box is the final arbiter.** At apply, a conflict answer from the box means nothing was created. DreamHub shows the conflicting timers from the box's answer and suggests fixes. This already works and is kept.
2. **DreamHub's preview becomes transponder- and type-aware, and says it's an estimate.** Timers are grouped by transponder, taken from the service reference (`TSID:ONID:namespace`, fields 5–7). Timers on the same transponder count as one tuner slot. The delivery system comes from the namespace (`0xFFFF…` cable, `0xEEEE…` terrestrial, otherwise a satellite orbital position) and is matched against each frontend's capabilities parsed from `e2model` (e.g. `DVB-S2 / DVB-C / DVB-T2`). The preview says "likely conflict" or "no conflict expected", never "impossible". A "likely conflict" still allows applying, since the box decides and nothing changes if it refuses.
3. **Optional, exact dry run (D7).** DreamHub's own Enigma2 plugin runs inside enigma2 anyway. It could register a tiny WebControl child (e.g. `/dreamhub/timercheck`) that runs the box's own `TimerSanityCheck` for a proposed timer **without adding it**. The preview would then show exactly what the box will say. This is a thin adapter to box-native logic, not business logic in the plugin.

To test it without owning such boxes: mock fixtures for a triple-tuner box, an FBC box (8 frontends, one input) and a mixed S/C box, plus a table-driven test per case. Real `deviceinfo` samples from other models are welcome from users.

## Work packages

Order matters where noted. Everything else can run in parallel.

### WP1 — WebControl client: correct writes *(first, small)*
- All mutating calls as form-encoded `POST`, every required parameter always sent. Optionally send `sessionid` from `/web/session` so boxes with `extended_security=true` work too.
- `DeleteTimer(sRef, begin, end)`; `DisableTimer` if D5 says so.
- Map the box's own conflict answer (`e2state=False`) to a proper error. Never match on `e2statetext`, which is translated.
- Align DreamHub's duplicate warning with the box's rule (same service + same **begin**, not begin+end). Otherwise a "clean" preview gets silently dropped by the box.
- Conflict estimate layers 1–2 from "Tuners" above (transponder- and delivery-system-aware), with the multi-tuner/FBC mock fixtures.
- Optional WebControl credentials in settings + a "WebControl needs a password" diagnostic on `401` (needed once a user sets `localauth=true`, see `ARCHITECTURE.md`).
- Enable device writes by default in live mode now that they're verified. Keep a setting to switch them off.

### WP2 — Run DreamHub on the box *(done 2026-09-24, see `ARCHITECTURE.md`, "DreamHub itself on the box")*
- Result: 11 MB idle / 21 MB peak, every read endpoint and the UI work live. Found and fixed: `epgmulti`'s `endTime` is minutes (the grid shipped the whole EPG), and world-readable data files (umask). Learned: enigma2 serializes WebControl requests, which feeds into WP7.
- Copy the ARM64 binary to `/tmp` on the reference box by hand (owner approval) and run it on a spare port against `127.0.0.1`. Measure RSS against the 150–200 MB budget, and check `/data` autofs, file permissions and `/etc/localtime`.
- Without this, MediaHub's file operations can't be tested for real: they need local access to `/media/hdd`.
- Result: a short "how to run a dev build on the box" section in `backend/README.md`.

### WP3 — TimerHub complete *(done 2026-09-24 except D5; see ROADMAP.md, TimerHub)*
- `TimerHubView.vue`: timers grouped by day, status (waiting / preparing / recording / done / disabled), conflict highlighting, source hint where detectable (e.g. SerienRecorder).
- Delete with preview. A running timer shows an explicit "this stops the recording in progress" warning.
- Create from GuideHub: click an EPG event → the existing plan/apply flow (send `eit` so the box links the EPG event).
- AutoTimer per D3, as an optional capability (hidden/degraded when the plugin is missing).
- GuideHub search provider (open item), dashboard tile "conflicts".

### WP4 — MediaHub writes *(file engine, rename, trash and UI done 2026-09-24; name suggestions (D2) and `.ts.meta` title edits open — see ROADMAP.md, MediaHub)*
- New file component (real: `os.Rename` on-device; mock: in-memory FS). Rule: all files sharing the recording's base name (`.ts`, `.ts.meta`, `.ts.ap`, `.ts.sc`, `.ts.cuts`, `.eit`, `.jpg`, `.backdrop.jpg`, whatever else exists), non-recordings left alone.
- **In-use check** before any operation: timers in state 1/2 targeting that file, and the currently played service (`/web/getcurrent`).
- **Rename** with preview (every file before → after), collision check, rollback on partial failure, optional title/description update in `.ts.meta`, name suggestions per D2 (metadata template, TMDB, AI — each optional, each degrading cleanly when unconfigured).
- **Trash** per D1: move to trash, list, restore, empty (each with preview). Trash size is shown on the disk tile.
- Bulk actions as one plan with many changes, executed by a small background queue with progress. Each file set stays all-or-nothing.
- Change log entries detailed enough for a later undo.

### WP5 — SystemHub *(done 2026-09-25, filtering the change log by user deferred until profiles exist — see ROADMAP.md, SystemHub)*
- Screen: device info, the existing diagnostics, change log UI (filter by action/user).
- Backup/restore of DreamHub's own state (settings incl. dashboards, tokens excluded or flagged) as one file. Restore goes through preview-and-confirm.

### WP6 — AdminHub logs *(mostly done 2026-09-25, live tail deferred to WP7 — see ROADMAP.md, AdminHub)*
- journald via `journalctl -o json` (exec, bounded): DreamHub, enigma2, other units. Filters for unit, level, text. Live tail over Server-Sent Events (reuses WP7; built).
- UI states clearly that the journal is **since last boot** (volatile, see `ARCHITECTURE.md`).
- enigma2 crash logs list + view, support download with secrets removed.

### WP7 — Core leftovers for v1 *(event bus, SSE live updates and in-flight coalescing done 2026-09-25 — see ROADMAP.md, step 7; criteria tiles, runtime locales, accessibility and complete Italian still open)*
- Never fan out parallel requests to the box (enigma2 serializes them, see `ARCHITECTURE.md`): coalesce identical in-flight requests ([x], `webcontrol.HTTPClient.coalescedGet`), short caches, `/web/movielist` cached — a time-based cache is deliberately deferred: it would also apply to a plan's own apply-time freshness check (`internal/plan`'s Fingerprint/Current) and could silently weaken preview-and-confirm's staleness guarantee, so it needs a cache-bypass path for that check specifically, not a blanket TTL.
- [x] Live updates (Server-Sent Events, not WebSockets — server → client only) + a minimal event bus. Events come from polling `/web/timerlist`/`movielist` (detection, not automation): `recording.started/finished`, disk above 90 % — `internal/events`. Persisted notifications (key + params, `store`'s `notifications` table), inbox in the bell (`stores/notifications.ts`). Implemented as **Server-Sent Events**, not a literal WebSocket: everything here only ever flows server → client, so SSE gets the identical result with no new dependency for a hand-rolled handshake — see `api.handleEventsStream`'s doc comment for the full reasoning, including why the frontend can't use the browser's native `EventSource` (it can't carry DreamHub's own auth header). `timer.conflict` isn't in the poller yet (still open, needs `internal/tuner` wired in without duplicating TimerHub's own estimate).
- Custom/criteria tiles on top of the search providers (the mechanics exist).
- Runtime drop-in locales from `/data/dreamhub/locales/`, accessibility pass, complete Italian strings.

### WP8 — TV launcher plugin *(fully confirmed live on the reference box 2026-09-25 — screen renders, backend calls land, gettext translates, status text shows correctly after fixing a real unicode/setText bug found via live logging — see ROADMAP.md, "Enigma2 TV plugin")*
- If D7 goes ahead: the `/dreamhub/timercheck` dry-run adapter lives here.
- [x] Python 2.7, gettext DE/IT/EN via `config.osd.language`.
- [x] The QR code is rendered **by the backend** (`/api/v1/qr.png`, local-only), so the plugin needs no QR library and holds no logic.
- [x] Shows backend status/version (`/api/v1/health`), mismatch message. Basic settings: auth on/off, port.

### WP9 — Packaging & release *(last)* *(done: `.deb` + CI 2026-09-25, installed live 2026-09-28, release 0.1.0 2026-10-04 — see ROADMAP.md, "v1 — release & packaging")*
- [x] `.deb` (arm64): binary, `dreamhub.service`, avahi service file, TV plugin, `postinst`/`prerm`/`postrm`, data in `/data/dreamhub`, `Depends` per `ROADMAP.md`.
- [x] CI: tests + ARM64 build + `.deb` on every push. Feed publishing per D4 (decided later) — still open.
- [x] Live on the reference box: install and in-place upgrades (2026-09-28, 2026-10-04). Uninstall/purge not run — the box keeps DreamHub.
- [x] User docs (`docs/USER_GUIDE.md`): install from the release, first steps, recommended `localauth=true`; the feed route once the feed exists.

### WP10 — Hardening & release candidate
- Resource measurement on the box under real use, security pass (CSRF, Host, plan ownership, file paths), DST test days, a run with none of the optional plugins installed.
- Dogfooding per D6 → 1.0.0, `CHANGELOG.md`.

## Suggested sequence

```
Phase A   WP1 ─┬─ WP2
               │
Phase B        ├─ WP3 (TimerHub) ── WP4 (MediaHub) ──┐
               ├─ WP5 (SystemHub) ── WP6 (AdminHub) ─┤  (WP6 needs WP7's live stream)
               └─ WP7 (core leftovers) ──────────────┤
Phase C                             WP8 (TV plugin) ─┤
Phase D                                  WP9 ── WP10 ─┴─> 1.0.0
```

## Out of v1 (unchanged)

RadioHub, ChannelHub (BouquetEditor), ControlHub, trimming, service status → v1.1 (since built early: ControlHub, RadioHub, service status). StreamHub, AutomationHub, terminal, external notifications, Plex/Jellyfin/Emby, more AI providers → later (`CONCEPT.md`).
