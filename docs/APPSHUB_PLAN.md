# AppsHub — plan

Status: **decided 2026-10-12 (brainstorm with the owner); App1 (the rename/shell) built the same day.** No issue opened yet — ask before creating one. Widens and renames Extension Hub (`docs/EXTENSIONHUB_PLAN.md`, #46, E1 already built 2026-10-12); once a package lands, its status moves to `ROADMAP.md`.

## Why

Extension Hub's E1 contract (status/meter/list/action widgets, a loopback-only registration handshake) is deliberately narrow — built for small, third-party bridges like an eWeLink switch. But the owner also wants something categorically different: running real self-hosted apps (Pi-hole, AdGuard, and whatever else arm64/armhf Docker images exist for) directly on the box, Unraid-style — its own config forms, its own web UI, not a few status widgets. That doesn't fit the widget contract and shouldn't be forced into it. It's **not an extension** in E1's sense; it's genuinely its own first-party hub (own Vue views, own backend routes) — it just belongs *conceptually* next to Extension Hub, since both are "optional additions to the base install," not core TV/recording functionality.

Resolution: rename the nav entry to **AppsHub**, following the exact precedent `docs/ADMINHUB_PLAN.md` already set merging SystemHub+AdminHub into DeviceHub — one nav entry, several tabs, each tab allowed to be a genuinely different kind of feature:

- **Extensions tab**: E1's existing widget-based cards (unchanged, already built).
- **Apps tab**: this plan — Docker container management.

## Decisions

### Not installed is not degraded

Zero apps installed, or Docker itself not installed, is **available, just empty** with an install prompt — the same choice already made for zero registered extensions. "Degraded" is reserved for *is installed but something's actually wrong* (the daemon isn't responding, a container is crash-looping). Reuses the existing `connector.Report` state vocabulary (`OK`/`Unreachable`/`NotConfigured`-equivalent) rather than inventing new states — Docker becomes a connector like any other.

### Docker access is isolated from DreamHub's own backend process

The Docker socket is root-equivalent access to the whole box (a privileged container can trivially escape to host root) — giving it directly to DreamHub's main backend would turn any bug or dependency vulnerability in DreamHub itself into a full box compromise, regardless of network topology (LAN-only + reverse-proxy + auth protects against an external attacker reaching the API at all, but does nothing for a bug *inside* an already-authenticated call, or a malicious image trying to pivot). Mitigation, reusing Extension Hub's own isolation pattern (E3):

- A small, separate helper process is the only thing with Docker socket access — DreamHub's main backend talks to *it* over loopback, never to the socket directly.
- The helper only allows a fixed, whitelisted subset of the Docker API (list/start/stop/logs/exec for containers under a DreamHub-owned label prefix) — the same approach tools like Tecnativa's `docker-socket-proxy` take. Hard denials regardless: no `--privileged`, no mounting outside the app's own designated storage path, no re-exposing the socket into a container.
- Whether the reference box's bundled Docker (the vendor's own GP4 plugin, not DreamHub's build) supports **rootless mode** as a further layer is unconfirmed — worth checking on the box, not assumed.

### Curated list, verified multi-arch, no AI and no ongoing manual curation of taste

The list is deliberately **lean and purely technical** — name, category, a verified `arm64`/`armhf` manifest check, nothing requiring subjective, ongoing upkeep (e.g. no "is this suited to an intermittently-on box" judgment — that's the owner's own call, not DreamHub's to make or maintain). **linuxserver.io** is the best first source: they build genuinely multi-arch images as policy, unlike a lot of Docker Hub content that's `amd64`-only. This is also the first concrete instance of a new, explicit project-wide rule (`PRINCIPLES.md`, "AI is always optional"): the list is maintained as plain project data, never AI-generated or AI-kept-current, and the whole feature works with zero AI involvement.

Free search beyond the curated list stays possible (**not locked to a curated allowlist** — any `arm64`/`armhf`-available image on Docker Hub should be installable), but checks the image's own multi-arch manifest **lazily, on click** rather than pre-filtering every search result (would need one manifest lookup per result — slow). A short-TTL cache (not permanent — a maintainer can add arm64 support later) avoids repeat lookups for a few popular searches without pretending a stale negative result is still true. An image with no matching platform shows a clear message immediately, never a failed pull.

### Storage: HDD by default, flash is a hard safety line, not just a preference

Two different risks, not one:

- **Flash write endurance**: NAND/eMMC has finite write cycles; a database-heavy container constantly writing to it wears it out far faster than to a spinning disk.
- **Flash filling up entirely can stop DreamOS itself from booting** — a categorically worse failure than any single feature breaking. This is the harder constraint of the two.

Default: every app's storage goes on the recording HDD (write-heavy apps *must*; genuinely low-write apps *may* go on flash instead, e.g. to let the disk reach standby overnight — but still only if there's comfortable headroom). Per app, the suggested location is pre-filled but **always changeable** — the general "sensible default, never hidden" rule this whole plan follows (see below). Flash gets its own threshold watch, reusing the poller's existing `disk.aboveThreshold` pattern (already built for the HDD, `internal/events/poller.go`) but at a **more conservative threshold** (around 70–75%, not 90%) and sharper wording ("the box may fail to restart if internal storage fills up") given the stakes. No app-related write is permitted onto flash once that margin is gone, regardless of what the app itself requested.

### One running principle across the whole feature: a sensible default, always visible, always changeable

Storage location, host port, resource limits (RAM/CPU cap), restart/autostart behavior, backup inclusion, the container's `TZ` (pre-filled from the box's own configured time zone) — every one of these gets a proposed default from DreamHub, shown plainly, and a one-click way to change it. Nothing is silently decided and hidden (`PRINCIPLES.md`'s existing "nothing happens silently", applied consistently across every one of these small decisions rather than ad hoc per field).

### Resource budget: computed live, not a hardcoded number

The reference box (2 GB RAM, a real quad-core SoC) can likely run meaningfully more than e.g. a Raspberry Pi — a fixed ceiling would either block the strong box pointlessly or under-protect a weaker one. Instead: before starting any app, compute headroom from what's actually free right now minus what enigma2/DreamHub/the OS baseline already use (the systemd `Unit.Memory` readings `internal/systemd` already exposes make this cheap) and refuse (with a clear reason) a container that wouldn't fit — never let the box become unstable. **Needs real-box measurement to calibrate thresholds properly** — the mechanism can be built now, the exact numbers can't be guessed from here.

A kernel **cgroups** capability check backs this (resource limits need cgroup support to actually be enforced) — cheap to add, likely moot in practice since the vendor's own Docker plugin is presumably already hardware-gated and wouldn't appear on a box too weak to support it properly, but worth keeping as a safety net rather than assumed away.

### Ports: known reserved list + a live bind check, suggested not forced

A port can be taken by something DreamHub doesn't know about, so a static "known ports" list (WebControl :80, DreamHub :8080, shellinaboxd :4200, Samba, oscam, ZeroTier — already in `ARCHITECTURE.md`) isn't enough on its own — combine it with an actual bind attempt before offering a port. Shown clearly either way: "in use by `<known service>`" when recognized, otherwise just "already in use." An app that exposes its own web UI gets a one-click "Open app" link using whatever port it ended up on — no DreamHub-side proxying of an app's own interface (unlike widget-based extensions, which are loopback-only on purpose; an app's own UI is allowed to be reachable directly, same LAN/reverse-proxy perimeter as DreamHub itself).

### Updates through the existing notification inbox, changelog when it's actually available

Same mechanism as AdminHub's package-update notifications (`events.PackageUpgradable`) — an `app.upgradable`-style event when a newer image digest exists, applied through the normal plan → apply flow like everything else. A changelog link is shown when Docker Hub's own metadata happens to carry the image's source GitHub repo (common for images with Docker Hub's automated-build/GitHub linking, not guaranteed) — when it's missing, the notification just says an update exists, which is fine, not a blocker.

### Backup is granular, built as a general mechanism from the start

Rather than an Apps-only special case, model this as a generic "backup sections" concept the existing Backup & Restore screen grows into, so future features (not just AppsHub) can offer the same per-item opt-in without redesigning backup again. Default per app: **configuration only**, never bulk volume data automatically (could be enormous) — a large volume is something the owner explicitly opts into backing up, with its size shown plainly first.

### Compose/multi-container apps: later, not v1

Single container per app covers the realistic first wave (Pi-hole, AdGuard-style tools). Multi-container (an app needing its own database alongside it) is real but adds real complexity — deferred, additive later, same versioning posture Extension Hub already uses for its own widget schema.

### Autostart and crash-loop protection

An app runs whenever the box itself is on — no separate "paused during TV-standby" state to design, since the box in question is fully powered off most of the day rather than kept in a lighter standby; there's nothing running to pause. Docker's own `--restart=on-failure:N` caps retries; on top of that, reuse `admin_watch.go`'s existing service-watch shape to notice a container stuck crash-looping, disable its autostart, and say so through the same notification inbox — never a silent infinite restart loop.

### Logs: two tiers, both reusing the existing viewer

Docker-the-daemon's own health (is the service running at all) reuses AdminHub's journald viewer, filtered to the docker service's unit — nothing new needed there. Per-container output (`docker logs`) is a second, separate view per app.

### A per-container console, with Terminal's exact safety posture

Same category of power as AdminHub's existing box terminal (A4, #36): off by default, needs DreamHub's own auth turned on and the password typed again, a clear standing warning, admin-only — not a lighter version of that bar just because it's "only" inside a container. Technically more involved than the terminal's plain reverse proxy to `shellinaboxd` (an always-running daemon): this needs a small WebSocket-to-PTY bridge driving `docker exec -it`, built fresh rather than reusing the terminal proxy as-is.

### Graceful shutdown ordering, not a workaround for abrupt power loss

The owner's own box is a controlled software shutdown "80–90%" of the time (uncontrolled power loss is the residual, unavoidable case any embedded device already has, nothing Docker-specific). The one thing worth actually building: make sure the Docker-facing helper's systemd unit is ordered so a normal shutdown sends it (and through it, running containers) a clean stop signal before power actually drops — standard systemd unit ordering, not a bespoke mechanism.

### Admin-only to manage; a later, separate idea for viewers

Installing, configuring, removing an app needs the `system` permission, same bar as AdminHub's package management — no new permission axis. A **later** idea, not v1: once real apps exist, a viewer-facing dashboard tile (the same tile mechanism every hub already has, `CONCEPT.md`) could offer a plain "open this app's link" button to non-admin profiles, read-only, no management capability at all.

### Not onboarding material

Like Extension Hub itself, AppsHub stays a power-user feature you find by navigating to it — not mentioned in the setup wizard.

## Open questions (resolve before building)

- Whether the reference box's Docker is cgroup v2-capable and whether rootless mode is realistic on it — needs a real box check, not assumed.
- Whether the box's standby/power-down is always a clean systemd shutdown or can be abrupt enough to skip ordering entirely — the owner's own experience says mostly clean, worth a real test anyway before relying on it for data-safety claims.
- Exact resource-budget thresholds (how much headroom to reserve for the core) and the flash-fill safety margin — both need real measurement on the box, not a guess from here.

## Packages

| # | Package | Contents |
|---|---|---|
| App1 | Rename + shell | **built 2026-10-12.** Extension Hub → AppsHub (nav label, route, hub ID), existing Extensions content becomes one tab, an "Apps" tab placeholder — see below |
| App2 | Docker isolation | the loopback-only helper process, the whitelisted Docker API subset, hardening (same template shape as `packaging/extension-template/`) |
| App3 | Curated list + search | the lean, AI-free app catalog (linuxserver.io first), free search with on-click multi-arch manifest checks |
| App4 | Install/run as a plan | pull + run through preview/confirm, the live resource-budget and flash-safety checks, port suggestion + bind check + "open app" link, `TZ` passthrough |
| App5 | Updates, logs, console | the notification-inbox update flow, the two-tier log viewer, the Terminal-equivalent per-container console |
| App6 | Backup | the general "backup sections" mechanism, config-only default for apps |
| later | Compose, viewer dashboard tile | multi-container apps; a read-only "open app" tile for non-admin profiles |

### App1, built 2026-10-12

`hub.IDExtensions` → `hub.IDApps` (`"apps"`), `Name: "AppsHub"`, `Label: "hub.apps"`, `Path: "/apps"` — the API routes stay under `/api/v1/extensions` unchanged (E1's contract itself isn't touched, only the hub's nav identity). `AppsHubView.vue` replaces `ExtensionsHubView.vue`, same one-entry-many-tabs pattern `DeviceHubView.vue` already uses: an "Extensions" tab (E1's cards, moved into `components/apps/ExtensionsTab.vue` verbatim) and an "Apps" tab (`components/apps/AppsTab.vue`) that's honestly just a placeholder — no fake connector-style state machine for a Docker backend that doesn't exist yet (that's App2+). Old `/extensions` links redirect to `/apps`. `go build`/`go vet`/`go test ./...`/`npm run build` all green.

Everything else (App2 onward) hangs off this shell now existing.
