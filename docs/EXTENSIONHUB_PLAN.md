# Extension Hub — plan

Status: **decided 2026-10-11; E1 (the contract) built 2026-10-12, E2 (distribution) and E3 (isolation/packaging template) built the same day.** Tracks issue #46. Only E4 (a real reference extension, eWeLink/Sonoff) is still open — deliberately deferred, it needs real hardware and account, not just more code. [`docs/BUILDING_EXTENSIONS.md`](BUILDING_EXTENSIONS.md) is the developer-facing how-to this plan's own decisions led to.

**2026-10-12: widened and renamed to AppsHub** (`docs/APPSHUB_PLAN.md`, built in full the same day — App1 through App6) — Docker container management turned out to need its own full hub (forms, not widgets), not something E1's widget contract could stretch to cover. Everything on this page still stands exactly as built: E1's widget cards became one tab ("Extensions") inside the renamed AppsHub nav entry, same contract, same code, just a different wrapper around it.

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

## Open questions — resolved while building E1 (2026-10-12)

- **Reverse-proxy boundary: resolved as the localhost-only option.** An extension's process binds only to its own localhost address; DreamHub's backend is the only thing that ever calls it (`internal/extension.HTTPClient`). There's no `/api/v1/ext/<name>/...` passthrough — the core calls `GET {baseUrl}/widgets`, `POST {baseUrl}/actions/{id}/plan` and `POST {baseUrl}/actions/{id}/apply` itself and reshapes the answer into its own `GET /api/v1/extensions` response. Nothing extension-related is reachable except through DreamHub's own auth.
- **Registration transport: a plain HTTP handshake, loopback-checked the same way `Plex.Stream` already is** (`hubs/plex_play.go`) — `POST /api/v1/extensions/register`, mounted as a public route (no DreamHub login; an extension's process has none) but rejecting anything whose `RemoteAddr` isn't loopback, and whose declared `baseUrl` isn't a loopback address either.
- **Live data: plain polling for v1, not SSE.** `GET /api/v1/extensions` calls every registered extension's `/widgets` fresh, no caching — simple, and enough for the kind of state these widgets show (a socket on/off, a disk filling up). SSE-backed live values stay in the "later" tier below; nothing here closes the door on it, since a future `WidgetType` is additive.
- **New, not anticipated when this doc was written: `plan.Plan` needed a `summary` field.** Every built-in hub's own `PlanPreview` slot is text the frontend itself builds from a local i18n key, with the plan's `changes`/`warnings` as the only structured data from the server — but an extension owns no key in DreamHub's own locale files, so it has nothing to render instead. `plan.Draft.Summary` (already used for the change-log entry) is now also carried through to the client (`plan.Plan.Summary`, omitted by every other hub's response) and shown verbatim by `ExtensionsHubView.vue`.

## Packages

| # | Package | Contents |
|---|---|---|
| E1 | Contract | **built 2026-10-12.** See "Built" below. |
| E2 | Distribution | **built 2026-10-12** (the field + filtering + feed convention; standing up a real feed server is operational, not code — stays open until E4 needs one). See "Built" below. |
| E3 | Isolation | **built 2026-10-12** (the systemd unit + packaging templates; not yet confirmed against the reference box's systemd 232). See "Built" below. |
| E4 | Reference extension | eWeLink/Sonoff — deliberately **not built this round**: needs a real device and account to build and verify against, not assumptions about its cloud API |
| later | Deeper widgets | SSE-backed live values, named per-hub extension points (SAP UI5-style), more widget types as real extensions need them — each additive per the versioning rule above |

### E1, built 2026-10-12

- **`internal/extension`**: `Widget` (status/meter/list/action, `SchemaVersion = 1`), `Registration`, the `Client` interface (`Widgets`/`PlanAction`/`Apply`), `HTTPClient` (the real implementation, localhost-only), `Registry` (in-memory, like `plan.Service` — an extension re-registers after a restart rather than this surviving one). `./mock`: a demo "smart socket" (one status, one toggle action) — deliberately the same smallest shape planned for the real eWeLink/Sonoff reference extension (E4), registered directly into the registry in mock mode (`app.go`) since there's no separate process to run one from yet.
- **`internal/hubs.Extensions`**: `GET /api/v1/extensions` (every registered extension's card: widgets fetched fresh, status available/degraded/unavailable with a reason, same vocabulary every hub already uses — unreachable or an unsupported schema version degrades that one card, never the whole hub), `POST /api/v1/extensions/{id}/actions/{action}/plan` (forwards to the extension, wraps the answer in a real `plan.Service` plan — stale-refused exactly like every other hub's actions), `Register` (the public, loopback-only handshake).
- **Hub registration**: `IDExtensions` in `hub/v1.go`, no `Requires` of its own (zero extensions installed is "available", just empty — never "unavailable"), `perm.Watch` (the nav entry is for everyone; an individual admin-only extension's card is filtered out for a non-admin inside the handler instead, via `Registration.Admin`).
- **Frontend**: `ExtensionsHubView.vue`, the one generic renderer for all four widget types, a new "extension" icon (`Icon.vue`), `/extensions` route, `hub.extensions`/`extensions.*`/`problem.extension.*` in all three locales.
- Dashboard-tile contribution (mentioned in E1's original scope) **not built this round** — no real extension exists yet to prove it against; revisit with E4.
- All backend tests green (`internal/extension`, new `internal/api/extensions_test.go` covering the demo extension, the full preview→apply→stale cycle, the loopback checks and the unsupported-schema-version degrade), `go vet` clean, `npm run build` green.

### E2 + E3, built 2026-10-12

- **E2, the `X-DreamHub-Category` marker**: `internal/apt.ParseDpkgQuery` now also reads `${X-DreamHub-Category}` via `dpkg-query`'s own field substitution, preferring it over apt's plain `Section` when a package sets it — a package that doesn't (almost everything on the box) falls back exactly as before. DreamHub's own `packaging/control.in` declares `X-DreamHub-Category: self`, so the mechanism is proven against a package this repo actually builds, not just planned. AdminHub's existing `GET /api/v1/admin/packages?category=` filter (built as part of A2, `docs/ADMINHUB_PLAN.md`) already works for `category=extension` the moment any package declares it — no separate "DreamHub extensions" UI was needed, the generic category filter already is that view.
- **E2, the second feed**: `internal/apt/feeds.go`'s `knownFeeds` already recognized `dreamhub-extensions-feed` as a trusted name (added during A2). Actually hosting that feed is operational work with nothing to build against yet — stays open, tracked here rather than in code.
- **E3, isolation**: `packaging/extension-template/` — a `control.template`, `extension.service`, and `postinst`/`prerm`/`postrm` templates for a future extension's own `.deb` to fill in and build from, mirroring DreamHub's own `packaging/` conventions (the `@PLACEHOLDER@` + `sed` pattern from `build-deb.sh`, the "purge only" data-removal rule from DreamHub's own `postrm`). The unit runs the extension as its own dedicated, unprivileged system user (created/removed by the postinst/postrm templates) with systemd sandboxing: no new privileges, every capability dropped, and network restricted to loopback only (`RestrictAddressFamilies`, `IPAddressAllow=localhost`) — consistent with E1's localhost-only decision, nothing an extension's process does should ever need the LAN or internet directly.
  - **Not yet confirmed on the reference box**: several hardening directives (`ProtectKernelLogs`, `ProtectClock`, `ProtectHostname`, `RestrictRealtime`) need a newer systemd than the reference box's confirmed systemd 232 (the same version that already caused a real gap once, `internal/systemd`'s `NRestarts` property during A1). systemd's documented behavior for an unrecognized directive is to warn and ignore it, not fail the unit — but that's an assumption, not a box-confirmed fact, and is called out as such in `packaging/extension-template/README.md`.
  - This is template/packaging work with no real extension to build from yet — not wired into CI or `build-deb.sh`, since there's nothing today that would consume it.
