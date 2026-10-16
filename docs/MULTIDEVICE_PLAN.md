# Multi-box — plan

Status: **brainstormed and decided 2026-10-16 (six rounds with the owner). M1, M2 and M3 are built against the mock and merged to `main` (see "… as built"); nothing is released before the check of M1 on the reference box; M3a, M4 and M5 are built too; M6 onwards not started.** Replaces the "one box" assumption that runs through the backend (`app.Options.WebControl` is injected once into every hub, the poller and the jobs blocker).

## Goal

Control more than one Dreambox from one DreamHub: a **box selector** in the header, and everything below it (guide, timers, recordings, remote) refers to the selected box. A household with one box must see **nothing new** — that is an acceptance criterion, not a nice-to-have.

Not goals (for now): non-Dreambox receivers (VU+ and other OpenWebif boxes — *prepared for, not built*), StreamHub multiview (parked: single tuner, untestable), cross-box actions beyond the two below.

## Decisions

### Vocabulary
The UI says **"Box"** everywhere ("Active box", "Boxen", "Box attivo") — the word already runs through `de/en/it.json` ("il box"). `DeviceHub` stays the name of the system administration of one box.

### Two states the user sees, nothing else
- **Basic** — only the box's web interface is used: live TV, zapping, guide, timers, bouquets, remote control, screenshots, watching its recordings. Needs no installation on the box.
- **Full** — DreamHub runs on that box and is paired: everything the main box can do (recording management, logos, AdScan, packages, logs, …). The add-box form says what is missing in Basic and how to get DreamHub onto the box.

The hub may itself run off-box (e.g. on Unraid): then *every* box is remote and **the local box is not a special case** — capabilities decide, not "is this the machine I run on".

A slim **agent** (same binary, build tag, no embedded UI, few hubs) is *not* user-visible and *not* built now. It is only kept possible: the pairing and the "local capabilities" API are designed so a full DreamHub and a future slim variant can both answer them. Whether it is needed depends on the flash size of the Dreambox One (to be measured).

### Adding a box
Manual only (name, colour, address, web-interface login, **Test** button; a 401 is reported clearly). Admins only; the hub only ever talks to registered boxes (no free URL pass-through).
Pairing to Full: the unpaired DreamHub on the other box shows a one-time **code on that box's TV and its status page**; entering address + code in the hub proves physical presence and yields a long-lived token. The web-interface password is a separate thing (the Basic login) and is never part of the pairing.

### Identity
Each box has its own **ID** (stable slug, never the IP, never changed by renaming). On connect the hub compares the box's own device identity and warns "a different box answers at this address" on mismatch (DHCP).

### Capabilities
Per box a capability list (`recordings.local`, `packages`, `docker`, `tvplugin`, `serienrecorder`, `autotimer`, `dmi`, …) measured, not assumed. A menu item the box can't have *and never will* (Docker on a Basic box) disappears; one that is surprising to miss is **greyed with one sentence** ("This box has no AutoTimer").

### Who sees which box
Profile editor: a **"Boxes this profile may use"** checklist, default all. With one allowed box the selector isn't shown. The last box is remembered **per device (browser)**, the profile can set a default box. The box is part of the URL (`/b/<id>/guide`) so bookmarks and the PWA work.

### Dashboard
No second dashboard. The dashboard of the selected box, plus — only with ≥2 allowed boxes — a slim strip with one line per other box ("Living room: ARD · Bedroom: standby, recording ●"); tap switches.

### Offline boxes
A box that doesn't answer is greyed in the selector; the UI never waits for it. Polling backs off. If the **selected** box stops answering, the UI shows a notice (no automatic jump to another box). Optional per box **power-on method**: none / Wake-on-LAN (MAC) / extension (e.g. the eWeLink plug via an extension action). Later package.

### Streams
Browser streams from a remote box run **through the hub**: one path for LAN, phone and reverse proxy, no mixed content, and the box's login never reaches the browser. A "direct" switch can be added later if the hub's load ever matters.

### Data scope
- **Global:** profiles, PINs, roles, language, Plex, TMDB, AI, push keys.
- **Per box:** name/colour, address/credentials, picons (channel **logos are shared by channel name** across boxes, a box can override one), channel change log, AdScan marks, recording folders, timer owners (key becomes `(box, timer)`), AppsHub.
- Existing data is assigned to the first box in a one-time migration, **after an automatic backup**.
- Notifications (recording started/failed, …) come for **all allowed boxes** with the box name in the text, with a switch to restrict them to the active box.

### Cross-box features (priority from the owner)
1. **Search across boxes** (EPG, recordings) — later.
2. **Watch a recording of box A on box B** (hub streams it, B plays it through its `/dreamhub/play` plugin → B must be Full, and B must be able to reach the hub's LAN address; the same `StreamBase` problem Plex has, solved once for both).
3. **Keep the channel list in sync** (ChannelHub: read A's bouquets, apply as a plan to B) — only if it turns out to be a small step on top of ChannelHub's existing plans.
Everything else (record on the box with a free tuner, move/copy recordings) is a later, larger topic.

### REST API and tokens
`/api/v1` stays; the box is an **optional `box=<id>` query parameter** on every request (a query parameter because `EventSource`, `<img>` picons and `<video src>` streams can't send headers). Omitted = the profile's default box, or the first box — so existing clients, tokens and extensions keep working unchanged. A bearer token is already bound to a profile, so it **inherits that profile's box list**; restricting a token to a single box can come later. `GET /api/v1/boxes` lists the boxes the caller may use, with capabilities.

PRINCIPLES.md promises an OpenAPI spec ("generated from the Go code, not hand-written docs that drift"), but **none exists yet**. Plan: one `openapi.json` with `box` defined once as a shared parameter, served by the backend and linked from Settings → Authentication next to the tokens (with a curl example), the plan → apply two-step documented as a pattern. The router uses plain `net/http` patterns plus a permission table, so the route list is introspectable: the spec is kept in step by a **test that fails when a route is missing from it** (the same test makes M1 prove every route resolves a box). Audience: **external developers** (DreamDroid-style apps, Home Assistant) and perhaps a DreamHub app of our own one day — so the spec and a short **USER_GUIDE section "API and tokens"** (create a token, call with `Authorization: Bearer`, the `box` parameter, plan → apply) are part of the package. Schemas are filled in from the most useful endpoints first (status, boxes, timers, recordings, zap).

## Things that are easy to get wrong (each needs a test)
- **Service references differ per box** (satellite, provider, flags). Sync and cross-box playback match channels by **name / ONID-TSID-SID**, never by the reference string.
- **Caches and coalescing** (guide grid, recording index, the web-control client's request coalescing) are keyed by box. Otherwise one box's data shows up in another's view.
- **Frontend:** on a box switch every store and the service-worker cache of API responses is dropped; SSE events of a non-selected box never touch the open views.
- **Old API paths, API tokens, extensions** keep working and mean "the first box"; new paths carry the box ID.
- `StreamBase` ("how the box reaches this DreamHub", `127.0.0.1:8080`) is wrong for any box other than the hub's own host → per box setting / detection.
- Hub ↔ Full-box version skew: a version handshake says what doesn't work instead of failing oddly.
- Removing a box is a **plan with confirmation** and asks whether to keep or delete its data.
- **The same programme recorded on two boxes** is spotted across boxes by SeriesHub and the recommendations (a hint, nothing more).
- **A hub without a box of its own** (Unraid) is tested from M1 on, not added afterwards.
- Per-box plugin presence (SerienRecorder, AutoTimer, DMI) goes through capabilities.
- The jobs blocker ("don't trim/scan while recording") is per box.
- Terminal and package management on a remote box need the Full tier; not offered in Basic.
- Accessibility: the selector is keyboard/screen-reader usable, and a box is identified by **name plus colour**, never colour alone.
- Docs: `de/en/it` strings, USER_GUIDE, ARCHITECTURE, CHANGELOG; the second mock box for tests and screenshots.

## Non-Dreambox receivers (VU+ and OpenWebif boxes)
Not a goal, not broken by anything here: no new hard-wired Dreambox assumptions (paths and folders from configuration, capabilities instead of "enigma2 does X"), and `webcontrol.Client` stays the seam. The owner has a VU+ Solo 2; a **read-only probe script** (which endpoints answer, response shapes, folder layout, tools present, Python and init system) is deferred but would give the fact base for a capability list. The Solo 2 is MIPS (`GOARCH=mipsle` cross-compiles).

## Packages

| # | Package | Visible? |
|---|---|---|
| M1 | **Box registry + box-aware wiring.** Box ID, one `webcontrol.Client` per box, hubs resolve the active box per request, caches/coalescing/poller per box, owners keyed `(box, timer)`, migration with backup, old API paths = first box. | No — mock + tests + one box check |
| M2 | **Capabilities** per box (measured), hubs and nav honour them, greyed/hidden rules. | Mostly no |
| M3 | **Selector, URL, profiles' box list, offline handling, dashboard strip.** Settings → Boxes: add (manual + Test), edit, remove-as-plan. | Yes |
| M3a | **OpenAPI spec + route/spec parity test**, API section in Settings → Authentication, `GET /api/v1/boxes`. | Yes (docs/settings) |
| M4 | **Second mock box**, headless-browser pass, screenshots, docs and `de/en/it`. | Test/docs |
| M5 | **Streams of remote boxes through the hub**; notifications per box with name and switch. | Yes |
| M6 | **Pairing to Full:** unpaired mode with code on TV/status page, token, version handshake, local-capabilities API; `StreamBase` per box. | Yes |
| M7 | **Watch a recording of box A on box B.** | Yes |
| M8 | **Channel list sync** (A → B as a plan), scheduled by effort. | Yes |
| M9 | **Power-on method** (WoL, extension action — pairs with E5). | Yes |
| later | Slim agent variant · VU+/OpenWebif probe and capability list · search across boxes · tuner-aware recording · move/copy between boxes. | — |

M1 is the risky one (it touches nearly every hub) and has nothing to look at, so it ships on its own and is checked on the reference box before anything is built on top.

## M1 as built (2026-10-16)

How the wiring was done, so M2 onwards can build on it without re-reading the diff.

- **The box travels in the context** (`internal/box`): `box.With/ID/Key` and `box.WithRemote/Remote`. The API layer (`api/boxes.go`, `withBox`) reads `?box=`, answers `404 box_not_found` for an unknown one, and puts the ID in the request context. Nothing is passed as an argument.
- **Hubs did not change their dependency.** They still hold one `webcontrol.Client`; it is now `boxes.Registry.WebControl()`, a client that forwards each call to the box the context names (the first box without one). The same goes for SerienRecorder and AutoTimer (`boxes/series.go`); a box without a client for them answers "plugin not installed". The router also implements `BouquetEditor` and the raw GET of the self-test, and says "not available" for a box whose client lacks them.
- **Registry** (`internal/boxes`): the first box ("main") comes from the options (configuration and the WebControl settings, as before); further boxes are rows of the new `boxes` table, built with `Options.BoxFactory` (HTTP clients with that row's address and login in live mode, another mock in mock mode). `Entry.Local` says DreamHub runs on that machine; only the first box is local.
- **Local resources belong to the local box only.** For a non-local box `box.Remote(ctx)` is true: file operations, in-place recording reads, the recording index, ad detection and Plex "play on the box" (the `StreamBase` is the hub's own loopback) are off (`hubs.onBox` replaces `recfs.Available` at every decision point), and `/api/v1/admin/*` and `/api/v1/apps/*` answer `409 box_not_full`. M2 turns this into capabilities.
- **Per box:** connector reports (`connector.Registry` cache), hub status transitions (`StatusWatcher.DiffBox`), `Media` recording index, `Channels` refresh time and its daily refresh, the poller (`events.Fleet`: one `Poller` per box, back-off up to 5 minutes, events tagged with `box` once there is more than one box), jobs (`jobs.box_id`: a job runs against its box; the recording blocker is asked per box).
- **Tables keyed by box** (migration 14, existing rows → "main"): `timer_owners`, `channel_snapshot`, `channel_backups`, `adscan_logos`, `adscan_results`, `jobs`. The store reads the box from the context (`box.Key`), so its API did not change. Before the migration runs on an existing database it is copied to `dreamhub.db.pre-migration-13`.
- **Plans** remember their box (`plan.Service.Create` now takes the request context): applying a plan acts on the box it was made for, whatever `box=` the apply request carries.
- **Not done in M1 (by design):** no way to add a box from the UI or API (M3), no `GET /api/v1/boxes` (M3a), no profile box list, no capability list (M2), SeriesHub's missing-episode watch and the admin watches still look at the first box only, notifications are tagged but not yet worded per box (M5), picons are still searched on DreamHub's own disk, `StreamBase` is not yet per box (M6).
- **Checked:** `go test ./...` (new: two mock boxes through the API, plan-to-box, owners, remote box without local file operations/administration, a hub with no file system at all, job queue per box, poller back-off/fleet/tagging, migration from a real schema-13 database incl. the backup); the new binary on a data directory written by the previous release (responses of timers, guide, media, navigation, hubs, connectors, deviceinfo, channels identical to the old binary's in mock mode).
- **Box check before release** (reference box, read-only unless noted): install a dev build next to the running one (README, "Trying a build on the box") on a copy of the data directory; open every hub once and compare with 0.8.0; confirm `dreamhub.db.pre-migration-13` exists and the dashboard, profiles, timer owners, channel "new since", AdScan marks and notifications are as before; leave it running for a few minutes and confirm no extra load on enigma2 (one poller, 30 s); do not apply plans while a recording runs.

## M2 as built (2026-10-16)

- **Capabilities are measured per box** (`boxes.Registry.Capabilities`): `webcontrol`, `serienrecorder`, `autotimer` come from that box's connector reports (already per box since M1); the machine-level ones (`recordings.local`, `packages`, `services`, `logs`, `terminal`, `docker`, `extensions`) are present exactly when DreamHub runs on that box (`Entry.Local`) — the Basic/Full tiers of the plan. The BouquetEditor plugin is deliberately not in the list: probing it is a network call, ChannelHub already reports it itself.
- **Hubs honour them** (`hub.StatusesFor(ctx, …)`): a `Requirement{Capability}` measures DreamHub's own machine, so it is skipped for a remote box (no false "journalctl missing" on a Basic box); `Descriptor.NeedsFull` (AppsHub) makes a hub `Hidden` — unavailable with `box.notFull` and left out of the navigation, search and tile sources — because that box will never offer it. SeriesHub and friends keep their greyed-with-a-sentence behaviour through the connector problems.
- **The UI** gets `navigation.box` (`id`, `full`, `capabilities`) with the navigation answer; `useNavigationStore().isFull` hides Device → Administration and Advanced on a Basic box. More hide/grey rules follow with the selector (M3), when a second box can actually be picked.
- Tests: `hub` (`TestStatusesFor_RemoteBox`), `boxes` (`TestCapabilities_MeasuredPerBox`), `api` (`TestBox_NavigationOfABasicBox`).

## M3 as built (2026-10-16)

- **Registry at runtime:** `boxes.Manager` (Test / Add / Update / Remove) over the registry and the `boxes` table; the connections come from the same `BoxFactory` as at start-up. Adding **tests first** — an unreachable box (`box_unreachable`) or a wrong login (`box_unauthorized`, from `webcontrol.ErrUnauthorized`) is refused, so the list never holds a box that was never reachable. The ID is a slug of the name, unique, kept on renaming. The first box can be renamed and recoloured but not removed (its address is the configuration; removing it would leave no default).
- **Removing** is a plan; applying deletes the box's rows (`store.DeleteBox`) and the profiles' references. *Keep the data* was dropped from the plan on purpose (remove concepts rather than add options — the data is meaningless without the box). A profile whose only box is removed is **not** widened to "all": it falls back to the first box.
- **Reachability:** `Poller.OnReach` reports every poll to `Registry.SetOnline`; `Fleet` now also stops the poller of a removed box. A box counts as online until polled. Because of the back-off (30 s up to 5 min) a box that came back can still show as offline for a few minutes — the selector is greyed, never blocked.
- **Profiles:** `users.boxes` (migration 15, JSON list, empty = all), `profile.Input.Boxes`. `withBox` refuses a box the profile may not use (`403 box_forbidden`) and, without `box`, picks the first allowed one. Box management is `settings.global`.
- **The UI:** the box is the router's *base* (`/b/<id>`, `lib/box.ts`), chosen from the address when the page loads; `client.ts`, the event stream and the TS player append `box=<id>` to every API call (except whoami/health/profiles). **Switching reloads the page** — simpler and safer than flushing every store (the plan's "drop every store"), and the service worker never caches `/api/`. An address without a box opens the remembered one (localStorage, per device). Selector in the top bar and strip on the dashboard only with ≥2 allowed boxes (`useBoxesStore().multi`); the status line names an unreachable selected box. Settings → Boxes (admins) with Test; profile editor lists boxes with ≥2.
- **Not in M3:** per-box settings screens (rec dirs, AdScan, logos still global/first-box), per-box notification wording and the "only this box" switch (M5), a default box per profile (the last choice per device is remembered instead), reordering boxes, the OpenAPI spec (M3a), a second mock box with a different fixture (M4: the added mock boxes are identical copies, so screenshots show the same programme).
- **Tests:** `api` (add/test/edit/remove incl. wrong login and unreachable, profile restriction incl. the removal fallback, overview and offline), `events` (reach report, poller of a removed box stops), `store` (migration test finds the multi-box step by content).

## M3a as built (2026-10-16)

- **The spec** is `internal/api/openapi.json` (OpenAPI 3.0, embedded, served at `GET /api/v1/openapi.json` behind login). Written by hand, not generated from handler code — the Go handlers have no typed response structs to generate from; the parity test is what keeps it honest. `servers` is `/api/v1`, so paths are short. `box` is a shared component parameter; an operation that does not depend on the box (settings, tokens, profiles, box management, dashboard, plans/apply, Plex, backup, …) says `x-box: global` instead. Plan routes are described as "changes nothing, returns a plan"; `POST /plans/{id}/apply` documents that it acts on the plan's own box.
- **Schemas** exist for status (`whoami`, `deviceinfo`), boxes, timers (list, create/delete plan), recordings (`media/movies`), now/next, current service, zap, plans and tokens; everything else lists summary, parameters and a generic body. More are filled in as needed.
- **Tests** (`api/openapi_test.go`): parity both ways against `env.routes` (plus the few public routes; the terminal WebSocket proxy is left out on purpose); every operation has a summary and either the `Box` parameter or `x-box: global`; every box-taking operation, called with `?box=no-such-box`, answers `404 box_not_found` — the proof that every route resolves a box.
- **`GET /api/v1/boxes`** gained `capabilities` (measured for allowed, online boxes; empty for an offline one so the list never waits).
- **UI:** a "For developers" block under the token list (curl example with this page's address, link to the spec); strings in de/en/it. USER_GUIDE "API and tokens".
- **Not done:** a rendered API viewer inside DreamHub (the spec is the artefact; Swagger UI etc. load it), per-box tokens, schemas for the long tail of endpoints.

## M4 as built (2026-10-16)

- **The second mock box** is `mock.NewSecond()` (`webcontrol/mock/second.go`): a `fixture` (device info with two tuners and a 1 TB disk, recordings, providers, favourites, evening programme, live channel) that the existing mock consults when set; the reference fixture is untouched. In mock mode `BoxFactory` returns it for every added box, so screenshots and the demo show different content per box. The channels (ProSieben, SAT.1, VOX) and recordings (Terra X, Tagesthemen) exist on that box only — a view that mixes boxes up shows foreign names.
- **Checked** in headless Chromium against the mock (main + "Wohnzimmer"): dashboard with strip ("Wohnzimmer · ZDF HD"), timers ("2 tuners", own timers, no conflict), guide, recordings (own disk and list), Device overview and change log. Found and fixed: the Device view called the administration endpoints on a Basic box (409 in the console).
- **Tests:** `mock` (second box differs everywhere), `api` (`TestBox_SecondMockBoxHasItsOwnContent`).
- **Seen, not changed (later):** the change log is global, not per box (the plan wants it per box); mock recording sizes are 9 bytes (fixture file contents) on both boxes; the AutoTimer fixture is the same on both.

## M5 as built (2026-10-16)

- **Streams** needed almost nothing: since M1 `Stream` talks to the box-routing `WebControl`, so `stream/live` and `stream/recording` of a remote box already run hub → box, with the box's login only in the hub's request. What was missing: `maxLiveStreams` (2) was one counter for the whole hub; it is now **per box** (`Stream.acquire(ctx)`), because each box has its own tuners and uplink. Tests: `api` (`TestBox_StreamsOfARemoteBoxPassThroughTheHub`: live and a recording that exists on the second box only, Range-less), `hubs` (`TestStream_LiveLimitIsPerBox`).
- **Notifications** carry the box: the poller already tags events with `box` once there are several; an untagged one (a hub's watch) is about the first box (`boxes.Registry.EventBox`). The notification view gets `box {id,name,color}` — only with ≥2 boxes. A restricted profile sees only events of its boxes, in the list, the unread count and the live feed (`viewFor`). Push appends the box name to the body (`Notifier.BoxName`, again only with ≥2 boxes).
- **"Only this box"** is a checkbox in the bell (≥2 boxes), stored per device in localStorage and applied in the browser — not a setting. Push has no "active box", so the switch does not touch it (push always names the box).
- **Fixed on the way:** the bell's update summary called `/apps/updates` on a Basic box (409).
- **Not done:** the stream hub's picons for a remote box still come from DreamHub's own disk (shared by channel name); push for a *single* box restriction per profile (push is global); the "direct" stream switch.
- Checked in headless Chromium against the mock (main + Wohnzimmer): bell with box names, switch, no new console errors.
