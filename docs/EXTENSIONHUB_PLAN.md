# Extension Hub — plan

Status: **decided 2026-10-11, not yet built.** Tracks issue #46. Once a package lands, its status moves to `ROADMAP.md`.

## Why

DreamHub's core keeps growing hubs (MediaHub, ChannelHub, …), but not every useful feature belongs there. Niche, homelab-style ideas — a quick Unraid status check, switching a WLAN socket to wake a server, an Arrr-suite queue view — are each genuinely useful to some households and irrelevant to most. Building them into the core would do to DreamHub what it was explicitly built to avoid elsewhere: a monolith nobody can keep lean. The Extension Hub is how such features ship as **separate, optional packages** instead: installed only by the households that want them, maintained (and versioned) independently of DreamHub's own release cycle, visible in DreamHub's own UI as if they'd always been a hub.

This revisits two calls in `PROJECT_PLAN.md`'s "Out of scope" ("Home automation / smart-home device control"; "Generic infrastructure or container monitoring") the same way the Plex connector revisited "Cross-referencing EPG against an external media library": **not in the core**, but allowed as an optional package nobody has to install, with the same degrade-gracefully rule as any other connector. `PROJECT_PLAN.md` is updated to reflect this.

## Decisions

### Extensions never ship frontend code

The earlier idea (an iframe embedding the extension's own page) was dropped: it looks and behaves like a foreign page inside DreamHub (scrollbars, fonts, spacing never quite match), and it means trusting arbitrary third-party JS to run in the same browser context as the rest of the app. Instead, an extension's backend process describes its UI as **structured data** — a small, fixed vocabulary DreamHub's own frontend renders with its existing components:

- **status** — a label + value (text, a number, good/warn/bad)
- **meter** — a value against a range (disk use, a battery level, …)
- **list** — rows of label + value, optionally with an icon
- **action** — a button that previews and confirms a change **through the existing plan → apply flow** (`PlanPreview.vue`, the same component MediaHub/ChannelHub already use): the extension's backend provides the plan's summary and does the actual work on `apply`, DreamHub just displays it

This keeps every extension visually indistinguishable from a built-in hub, keeps "preview and confirm, always" (`PRINCIPLES.md`) intact for anything an extension changes, and means no extension ever executes code in the browser — only JSON crosses that boundary, the same trust level as any hub-to-core call today.

**Tradeoff, accepted:** an extension can't build a fully custom interface (e.g. an embedded file browser). None of the named examples (Unraid quick actions, eWeLink switches, Arrr queue status) need one — each is a handful of status/meter/list rows and a few confirm-buttons. If something later genuinely needs more, that's a deliberate, separate escape hatch to design then, not the default.

### The contract: versioned, additive, degrades instead of breaking

An extension is its own OS process (own `.deb`, own port, like DreamHub's backend itself) that registers with the core over a small local HTTP handshake, declaring:

- its hub metadata — id, icon, nav group, descriptive + fixed name, the role needed to see it (same fields the core hub contract already uses, `CONCEPT.md`)
- the **schema version** it speaks
- which widget types and fields it uses

The core keeps a registry of widget types per schema version. New versions only *add* optional widget types or fields — an extension written against v1 keeps working forever against a later core, because it simply never sends the newer fields. A structural break (if one ever becomes unavoidable) would get a new major version, with the core supporting both for a transition window rather than cutting over at once.

An extension whose version or widgets the installed DreamHub doesn't understand yet doesn't break anything: it shows up with the **same degraded status every hub already has** — "available / degraded / unavailable, always with a reason and a fix" (`CONCEPT.md`) — e.g. "needs a newer DreamHub" instead of a crash or a blank tile.

### Distribution: one package mechanism, not two

Rather than invent a separate installer for extensions, they ride the **same apt feed/package mechanism AdminHub is getting for DreamHub's own updates** (`docs/ADMINHUB_PLAN.md`, decision D4 for DreamHub's own feed). An extension's `.deb` carries a declaration (the same `X-DreamHub-Category: extension` control-file field `ADMINHUB_PLAN.md` uses for DreamHub's own packages and picon sets) that says "this is a DreamHub extension" and what it registers as; AdminHub's package view then shows "DreamHub extensions" as a filtered, friendlier slice of one general package list, instead of extensions needing their own separate update/remove UI. A second feed (separate from DreamHub's own) hosts them, matching the original idea that e.g. an Arrr-suite extension would live on its own feed, maintained on its own schedule.

### One nav entry for all extensions, not one per extension

An earlier version of this plan had every extension register exactly like a hub — its own icon, its own slot in the navigation. Dropped: install five extensions and the navigation grows by five entries, which is precisely the clutter DreamHub is built to avoid (`CONCEPT.md`, "one place where everything connects," not many separate tools bolted on). Instead:

- Every extension can contribute a **dashboard tile**, through the hub contract's existing tile mechanism (`CONCEPT.md`) — no new extension point needed, hubs already do this.
- Every extension's fuller detail view (its status/meter/list/action widgets) lives as a card **inside one single "Extensions" nav entry** — install any number of extensions, the navigation still only grows by that one entry; what's inside it grows instead.
- A household that genuinely uses one extension constantly can pin it to its own nav slot — opt-in, not automatic.

A more granular idea raised while discussing this — a hub declaring its own **named extension points** (SAP UI5-style: "an extra button slot in this specific list"), so an extension could reach a little deeper than a dashboard tile without reaching into a hub's actual code — is worth keeping in mind for later. Not designed now: real extensions in use would be needed first to know which points are actually worth opening, and each one is itself a small versioned contract to maintain, so it shouldn't be invented speculatively.

### First reference extension: eWeLink/Sonoff

Confirmed as the package to build first once E1–E3 exist: one status (is the socket on), one action (toggle it) — the smallest possible exercise of the whole contract end to end, before anything more complex (Unraid, Arrr) is attempted.

### Isolation

Each extension runs as its own systemd service, under its own non-root user, with standard hardening (`ProtectSystem`, `NoNewPrivileges`, no capabilities it doesn't declare needing). This is the backend-only question "how much do we trust this process" rather than a browser security question — tractable with the same packaging conventions DreamHub's own `.deb` already follows (`ARCHITECTURE.md`'s systemd notes).

### Who may

Installing, updating or removing an extension needs an admin (AdminHub's package management, `admin` role). Whether an *installed* extension's own hub is visible to non-admins depends on the role it declares for itself — e.g. an Unraid-bridge tile might be admin-only, while a "wake the server" button could be fine for anyone, same as any other hub's role field.

## Open questions (resolve before building)

- Exact transport for the registration handshake and for serving live data (plain polling vs. SSE for anything that updates, e.g. a job progress bar) — likely SSE, reusing the event mechanism hubs already have (`internal/events`).
- Where the reverse-proxy boundary sits: does the core proxy an extension's data calls itself (`/api/v1/ext/<name>/...`), or does each extension's process bind only to localhost and the core's own backend is the only thing that ever talks to it? The latter is safer (nothing extension-related is reachable except through DreamHub's own auth) and more consistent with how PlexHub already proxies Plex itself (`plex_play.go`).
## Packages

| # | Package | Contents |
|---|---|---|
| E1 | Contract | registration handshake, widget schema v1 (status, meter, list, action→plan), the generic renderer in the frontend (the single "Extensions" nav entry plus dashboard-tile contribution), version/degrade handling |
| E2 | Distribution | `X-DreamHub-Category: extension` marker in a `.deb`, AdminHub's package view filtering to it, the second feed |
| E3 | Isolation | systemd unit template/hardening for an extension process, the reverse-proxy or localhost-only decision from "Open questions" |
| E4 | Reference extension | eWeLink/Sonoff, built against E1–E3, to prove the contract end to end and document it as the template for others |
| later | Deeper widgets | SSE-backed live values, named per-hub extension points (SAP UI5-style), more widget types as real extensions need them — each additive per the versioning rule above |

Nothing here is built against the mock yet; E1 is the right starting point once prioritized, since nothing else can be tested without it.
