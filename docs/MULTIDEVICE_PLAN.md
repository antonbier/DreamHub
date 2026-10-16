# Multi-box — plan

Status: **brainstormed and decided 2026-10-16 (six rounds with the owner); nothing built yet; M1 is the first thing to build, against the mock, shipping only after a check on the reference box.** Replaces the "one box" assumption that runs through the backend (`app.Options.WebControl` is injected once into every hub, the poller and the jobs blocker).

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
