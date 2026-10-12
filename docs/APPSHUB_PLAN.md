# AppsHub — plan

Status: **decided 2026-10-12 (brainstorm with the owner); App1 through App6 all built the same day — a complete, working vertical slice for curated apps (browse → install → manage → update → logs/console → backup → remove).** No issue opened yet — ask before creating one. Widens and renames Extension Hub (`docs/EXTENSIONHUB_PLAN.md`, #46, E1 already built 2026-10-12); this now shows in `ROADMAP.md` too. **App7 (Docker on the box, set up from the UI) built and confirmed on the reference box 2026-10-12** — see below. Still open: free-search install, multi-container/compose apps, a read-only viewer dashboard tile, the curated list's rework (the owner wants it changed; on hold), and a reboot check that Docker comes back by itself.

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
| App2 | Docker isolation | **built 2026-10-12** (the helper binary, the loopback protocol, the whitelisted Docker API subset; systemd hardening/packaging still open — see below) |
| App3 | Curated list + search | **built 2026-10-12** (the curated catalog, free search, on-click manifest checks, and a first "Apps" tab UI — see below) |
| App4 | Install/run as a plan | **built 2026-10-12** (pull + run through preview/confirm, a resource-budget check, a port bind check, `TZ` passthrough, start/stop, remove — curated apps only, see below) |
| App5 | Updates, logs, console | **built 2026-10-12** (the notification-inbox update flow, the two-tier log viewer, the Terminal-equivalent per-container console — see below) |
| App6 | Backup | **built 2026-10-12** (the general "backup sections" mechanism, config-only default for apps — see below) |
| later | Compose, viewer dashboard tile | multi-container apps; a read-only "open app" tile for non-admin profiles |

### App1, built 2026-10-12

`hub.IDExtensions` → `hub.IDApps` (`"apps"`), `Name: "AppsHub"`, `Label: "hub.apps"`, `Path: "/apps"` — the API routes stay under `/api/v1/extensions` unchanged (E1's contract itself isn't touched, only the hub's nav identity). `AppsHubView.vue` replaces `ExtensionsHubView.vue`, same one-entry-many-tabs pattern `DeviceHubView.vue` already uses: an "Extensions" tab (E1's cards, moved into `components/apps/ExtensionsTab.vue` verbatim) and an "Apps" tab (`components/apps/AppsTab.vue`) that's honestly just a placeholder — no fake connector-style state machine for a Docker backend that doesn't exist yet (that's App2+). Old `/extensions` links redirect to `/apps`. `go build`/`go vet`/`go test ./...`/`npm run build` all green.

### App2, built 2026-10-12

`internal/dockerd`: `Client` (`List`/`Start`/`Stop`/`Logs`/`Exec`, scoped to containers carrying the `dreamhub.app` label — `AppLabel`), two implementations of it:

- `Exec` — hand-rolled HTTP against Docker's own Engine API over its unix socket (no SDK dependency, the same choice this repo already made for WebControl/apt), used only inside the new `cmd/dreamhub-dockerd-helper` binary. Every call (`Start`/`Stop`/`Logs`/`Exec`) checks the target container actually carries `AppLabel` before doing anything (`requireOwned`) — a container outside DreamHub's own apps is invisible, not just filtered from a listing. `List` filters server-side via Docker's own `filters` query param, and again client-side as a belt-and-suspenders check. Docker's multiplexed stdout/stderr log framing is demultiplexed (`demux`) rather than passed through raw.
- `HTTPClient` — the loopback client DreamHub's main backend will use (once App3/4 wire a hub to it); talks only to the helper, never the socket.

`dockerd.Server` is the helper's own HTTP surface: exactly five routes (`GET /containers`, `POST .../start`, `POST .../stop`, `GET .../logs`, `POST .../exec`) — the whitelisted subset holds because nothing else is registered on the mux, not because of a runtime check a bug could skip. There is still no create/run/pull anywhere in this package (that's App4) and so no path to `--privileged`, an arbitrary mount, or re-exposing the socket — the hard denials the plan calls for.

`cmd/dreamhub-dockerd-helper/main.go`: the actual process, binds `127.0.0.1:8090` by default (`DREAMHUB_DOCKERD_HELPER_ADDR`), reads `/var/run/docker.sock` by default (`DREAMHUB_DOCKER_SOCKET`), with the same `DREAMHUB_MOCK=1` switch (`internal/dockerd/mock`, two demo containers) the main binary already uses for mock/live.

Tests: `internal/dockerd`'s socket client is exercised against a real fake daemon listening on an actual unix socket (`httptest.Server{Listener: ...}`, not just assumed from the HTTP-shape docs), proving label filtering, the demux framing, and the not-whitelisted-container denial; the loopback `Server`+`HTTPClient` pair is tested as a real round trip over `httptest.NewServer`, including that `POST .../create` 404s (no such route). All green under `-race`.

**Not done in this pass, open before App2 can actually ship:**
- No systemd unit / hardening template yet (`packaging/extension-template/`-equivalent) — the helper isn't wired into `build-deb.sh` or `postinst`, so it doesn't run on the box today. Needs a dedicated, Docker-group-only system user, loopback-restricted sandboxing, and shutdown ordering ahead of Docker itself (the plan's "graceful shutdown ordering" decision).
- Not run against a real Docker daemon anywhere — built and tested against a faithful fake unix-socket server, which is as far as this could go without a box or a local Docker install. The box's own vendor Docker plugin's exact API version, and whether it matches the unversioned paths this client assumes, is unconfirmed.
- No hub/frontend wiring yet (AppsHub's "Apps" tab is still App1's placeholder) — deliberately so; that's App3 (curated list) and App4 (install/run as a plan), which is what will actually call `HTTPClient` for the first time.

### App3, built 2026-10-12

`internal/appcatalog`: `Apps`, a short, hand-maintained list (pihole, AdGuard Home, WireGuard, Syncthing, qBittorrent, Heimdall, Sonarr, Radarr, Prowlarr, Bazarr) — plain data, never AI-generated, each entry's `Arches` a point-in-time fact checked against Docker Hub's own `tags/latest` manifest on 2026-10-12 rather than guessed. One real, useful correction to the plan's own assumption while building this: **linuxserver.io's current images (heimdall, sonarr/radarr/prowlarr/bazarr, syncthing, qbittorrent, wireguard) only ship amd64+arm64 — they dropped armhf support**, so an armhf box (DM900/DM920) only gets the two non-linuxserver.io entries (Pi-hole, AdGuard Home, both still arm/v6+v7) from this list; everything else needs free search to find an armhf-capable alternative, if one exists.

`internal/dockerhub`: `Client` (`Search`/`Manifest`), `HTTPClient` (the real one, hand-rolled against `hub.docker.com`'s public v2 API, no SDK — same house style as every other outbound connector here), with `Manifest` results cached in-process for 10 minutes (`manifestTTL`) rather than fetched on every click. `BoxArch()` reads `runtime.GOARCH` directly — exactly the architecture `packaging/build-deb.sh` built this very binary for, so there's no separate box-arch detection to get wrong. `Supports` treats Docker's `arm`+`v6`/`v7` variants as armhf-equivalent (a documented approximation, not a guarantee).

`hubs.Apps` (`internal/hubs/apps_catalog.go`): `GET /api/v1/apps/catalog` (the curated list with `compatible` already resolved server-side), `GET /api/v1/apps/search` (free search, Docker Hub only, no per-result arch check), `GET /api/v1/apps/manifest` (the on-click check, `image` validated against a strict `namespace/repo` pattern before ever touching a request path). All three need the `system` permission, like every other AdminHub-style route — proven with a viewer-gets-403 test, since the hub's own nav entry is `perm.Watch` (everyone) and these routes need their own explicit override.

Frontend: `AppsTab.vue` is no longer a placeholder — the curated list (with a per-row compatibility badge), a search box, and a "Check compatibility" button per search result that calls the manifest endpoint lazily. Confirmed with a real headless-browser pass against the mock backend (not just the Go tests): the catalog renders, "pihole" search returns pihole/pihole from the mock Docker Hub fixture, clicking "Check compatibility" shows "Works on this box", and a nonsense query shows the empty-search state.

**What's still not here, on purpose:** no install/run (App4 — this tab is read-only, there's no button anywhere that would pull or start a container), so `internal/dockerd`'s `HTTPClient` (App2) still has no caller. Not run against the real Docker Hub from the box itself (only the real public API, reachable from this dev environment, and a mocked one in tests) — the box's own outbound internet reachability for this isn't confirmed either.

### App4, built 2026-10-12

Scoped narrower than the original plan text on purpose: **only a curated catalog app can be installed in this first slice**, not an arbitrary free-search image — App3's free search only ever offered a compatibility check, never a web port or a config path, and App4 needs both before it can build a real container spec. Extending install to a free-search result is a later, separate form (asking the admin for those two fields by hand), not built here.

- `appcatalog.App` gained `ContainerPort` (the image's own documented web UI port — 0 for wireguard, which has none) and `ConfigPath` (the in-container path its persistent config lives at), both confirmed the same way `Arches` was (App3), from each image's own Docker Hub description's `docker run -p`/`-v` example on 2026-10-12 — not guessed. All ten curated apps use their own real, documented values; the linuxserver.io convention (`/config`) covers eight of them, pihole and AdGuard Home each use their own vendor path.
- `internal/dockerd` grew `Create`/`Remove` and `Container.HostPort` (read back live from Docker's own port mapping, never stored). `Create`'s own request shape (`ContainerSpec`) is a handful of scalars, not a raw Docker container spec — the hard denials from App2 (no `--privileged`, no arbitrary mount, no socket re-exposure) still hold because the daemon-facing JSON is built entirely from those scalars inside `Exec.Create`, never passed through from a caller. One container per `AppID`, enforced by `Create` itself refusing a second one (`ErrAlreadyExists`) rather than a separate check that could race. `Exec.Create` pulls the image first (`/images/create`, draining the whole streamed response — Docker reports a failed pull as a 200 with an "error" line partway through the stream, not a non-200 status, so the stream itself has to be read to catch one).
- `internal/meminfo` (new): reads `/proc/meminfo`'s own `MemAvailable` directly for the resource-budget check — deliberately *not* also reading `enigma2`/`dreamhub`'s own systemd `Unit.Memory` the way the original plan text assumed it would need to, since `MemAvailable` already nets today's usage out; a 128 MB reserve is held back on top before an app may claim the rest. The reference box's own exact safe threshold is still unmeasured (the plan's own open question) — this is the mechanism, not a calibrated number.
- The disk-full check reuses `events.poller`'s own 90% `disk.aboveThreshold` line (`internal/webcontrol.DeviceInfo`+`ParseSize`) rather than inventing a separate number for Apps — same box, same risk.
- Port: an admin's own explicit choice is checked with a real `net.Listen` bind attempt and refused outright if taken (never silently moved to another port); with no explicit choice, the app's own `ContainerPort` is tried first and nearby ports upward from there. A handful of the box's other well-known ports (80, 8080, 4200, Samba) get a named reason in the refusal when recognized.
- `TZ` passthrough: `time.Local.String()` (what `/etc/localtime`'s own zoneinfo link resolves to), not the per-profile *display* timezone setting (`settings.KeyTimezone`) — that one's a UI preference, this needs the box's real system zone. Falls back to `UTC` if no zoneinfo link is found (true in this sandboxed dev environment, confirmed while testing — only a real box's own `/etc/localtime` will show a real zone name).
- `hubs.Apps` grew `GET /api/v1/apps/installed`, `POST /api/v1/apps/install/plan`, `POST /api/v1/apps/installed/{id}/remove/plan` (both go through the normal plan→apply flow every other change in DreamHub uses — `PlanPreview.vue`, nothing new), and direct (no preview) `POST .../start`/`.../stop` — the same bar ControlHub's own zap/power actions already use, since starting or stopping an already-installed app is reversible and low-risk compared to install (pulls new software) or remove (deletes a container).
- Frontend: `AppsTab.vue` grew an "Installed" section (state, an "Open" link when the app has a web UI, Start/Stop, Remove-with-confirm) above the catalog, each catalog row now showing "Install", "Already installed" or the existing not-compatible state. Confirmed for real in a full headless-browser pass against the mock backend: installed WireGuard from the catalog (preview correctly showed no port, since it has none), stopped and restarted an existing app, previewed and confirmed removing one (the danger-styled confirm button, the real "config is kept" wording) — not just the Go API tests.
- Tests: the whole backend round trip (`internal/api/apps_install_test.go`) — preview-then-apply for both install and remove, a 409 on an already-installed app, a 409 naming the conflicting port when an admin's own explicit port choice is actually bound (the test binds it itself first, rather than assuming something else on the box already is), wireguard getting no host port at all, start/stop flipping state, and the whole surface refusing a viewer with 403. `internal/dockerd`'s `Create`/`Remove` are tested against the same real fake-unix-socket daemon App2's tests use, including a failed pull. All green under `-race`.

**Not done here, on purpose:**
- Still curated-catalog-only — no way yet to install a free-search result, and no multi-container/compose support (that's the plan's own later tier).
- No systemd packaging for the helper (App2's own gap, unchanged) — nothing here runs on the box yet regardless of how complete the backend is.
- Resource and flash-fill thresholds are still the mechanism, not calibrated numbers — both still need real-box measurement, called out above and in "Open questions".

### App5, built 2026-10-12

Three independent slices, all on top of App2's `internal/dockerd` helper and App3's `internal/dockerhub` client, none needing the real box:

- **Updates**: `dockerhub.Client` grew `Digest` (the registry's current manifest digest for an `image:tag`, the same cached-lookup shape `Manifest` already uses) and `dockerd.Client` grew `ImageDigest` (what a running container was actually pulled at, read back from the real daemon's own `RepoDigests` via `/containers/{id}/json` then `/images/{imageID}/json` — not guessed from the tag string). `internal/hubs/apps_watch.go` is AppsHub's own update watch, the exact "detection, not automation" shape AdminHub's package watch (`admin_packages_watch.go`) already established: hourly, compares each installed curated app's running digest against the registry's current one, publishes a new `events.AppUpgradable` (wired through `events/keys.go`, `hub/v1.go`'s `Emits`, `push/messages.go` in all 8 languages, and the frontend locales, same as every other event key) only for an app that *just* became upgradable — never acts on it itself, since there's no "update in place," only remove-then-reinstall, which the admin chooses through the existing plan flow. `GET /api/v1/apps/installed` also surfaces the same check per row live, so the "Apps" tab shows an "Update available" badge without waiting for a watch cycle.
- **Logs, two tiers**: the daemon's own health reuses DeviceHub's existing journald log viewer completely unchanged — linking there with `?logUnit=docker` (`DeviceHubView.vue` reads the query param into its existing `logUnit` ref) was all that was needed, since the viewer's own backend already takes an arbitrary systemd unit name. Per-container output is new: `GET /api/v1/apps/installed/{id}/logs` calls `dockerd.Client.Logs` (App2) directly.
- **Console**: the exact same safety posture AdminHub's Terminal already uses (A4, #36) — off by default, needs DreamHub's own auth on globally *and* the password typed again right now (`apps_console.go`'s `consoleState`, byte-for-byte `admin_terminal.go`'s `terminalState`), a standing warning shown throughout, admin-only, nothing persisted across a restart. Technically the harder half the plan called out: `dockerd.Client` grew `Attach`, hand-rolling Docker's own `docker exec -it` wire protocol (create an exec instance, then a raw hijacked connection for its `start`, the same `net.Conn`-after-HTTP-response trick Docker's own CLI uses) rather than the one-shot non-interactive `Exec` App2 already had. A new, minimal `internal/wsutil` package (hand-rolled WebSocket server, RFC 6455, single unfragmented frames only — this one caller never needs more) bridges a browser WebSocket to that raw stream; `GET /api/v1/apps/installed/{id}/console` is three hops of the exact same hijack trick stacked (daemon → helper → DreamHub's backend → browser), not three different protocols. **Found and fixed a real, previously-latent bug while wiring this up**: `httpx.statusRecorder`, the logging middleware's `ResponseWriter` wrapper that sits in front of *every* request, didn't implement `http.Hijacker` — so any hijack-based endpoint behind it (this one, or any future one) would have silently 500'd regardless of its own correctness. Fixed the same way `Flush` already handles this passthrough for SSE.
- Confirmed in a full headless-browser pass against the mock backend (not just the Go tests, which also all pass under `-race`, including a hand-rolled raw-socket WebSocket test client dialing the real handler — the same "don't just trust the handler shape" standard `dockerd`'s own `fakeDaemon` tests hold to): the "Update available" badge on Pi-hole (the mock fixtures are seeded so it lags behind the registry on purpose, AdGuard Home matches), the logs modal, the Docker-service-logs link landing on the filtered viewer, and the console's complete round trip — warning shown, password prompt, enable, a real echo over the bridge, disable.
- **Not done here, on purpose**: still curated-catalog-only (App4's own scope, unchanged) — the update watch only ever checks curated apps; no systemd packaging for the helper yet (App2's gap, still open); no crash-loop detection (the plan's own decision text calls for it, but it isn't part of this package's table entry — a later pass, not forgotten).

### App6, built 2026-10-12

Modeled as a generic mechanism from the start, exactly as the plan's own "Backup is granular, built as a general mechanism" decision calls for, not an AppsHub-only special case:

- `hubs.System` grew a `Sections []BackupSection` field and `GET /api/v1/system/backup/sections` (every registered section's own items, each with a real size, so the admin sees what they're about to include before they do). `backupFile` grew an optional `Sections map[string]json.RawMessage` — a plain settings-only backup (no `?sections=` query, or an older file with no `"sections"` key) behaves exactly as it always did; this is additive, not a breaking change to the existing format.
- AppsHub is the first (and so far only) section: every installed curated app's own config directory (its `HostDir`, App4) is included by default — the plan's own "configuration only, never bulk volume data automatically" default. There's nothing else to make opt-in yet, since no curated app has a second, larger bind mount beyond the one `ConfigPath` App4 already mounts — `BackupItem`'s own per-item size and `DefaultIncluded` fields are already there for when one does, without needing another redesign.
- `internal/configfs` (new): tars and gzips a directory for export, and restores by extracting — deliberately never deleting a file already on disk that the archive doesn't mention (the simplest safe behavior without needing to reason about what else might have legitimately been written there since the backup, e.g. a database file growing). Restore rejects a path-escaping archive entry (`../…`) outright, since an uploaded backup file is untrusted input, not something only this package's own `Archive` could have produced.
- Frontend: `BackupPanel.vue` (Settings → System) shows a checkbox for the apps section with its own item count and total size, checked by default; the restore preview notes "This file also includes installed apps' configs" when the uploaded file carries one.
- Confirmed in a full headless-browser pass against the mock backend: downloaded a real backup with both mock apps' configs actually embedded (not just a stub), re-uploaded it, saw the correct note in the restore preview. `internal/api/system_test.go` covers the full preview→apply round trip (plus the section being entirely absent when not asked for); `internal/configfs`'s own tests cover the archive/restore round trip, "restore never deletes," and the path-escape rejection. All green under `-race`.
- **Not done here, on purpose**: no finer per-item selection in the UI yet (the backend's own `Items`/`BackupItem` already support it — App6 only exposes a whole-section toggle); no section beyond AppsHub's exists yet, so the mechanism itself is proven by exactly one real user, not several.

Everything past App6 is the plan's own explicitly-later tier (compose/multi-container apps, a read-only viewer dashboard tile) — see the table above.

### App7: Docker on the box, set up from the UI (built and confirmed on the box 2026-10-12)

Decided with the owner after the 0.4.0 box checks found **no Docker on the reference box** (the feed's `docker` 1.13 from 2017 was installed once and removed) and a kernel that limits what Docker can do there (ARCHITECTURE.md, "Box checks for 0.4.0").

- **Official static Docker, pinned, not the feed's**: `internal/dockersetup` downloads Docker 29.9.0's static release for the box's processor from download.docker.com, at a version and SHA-256 pinned in the code (`dockersetup.Releases`) — a newer Docker is a DreamHub release, never "whatever is newest at setup time". Unpacked to `/data/dreamhub-docker/bin`; images and containers under `/data/dreamhub-docker/root`. **Nothing on the root flash** (560 MB free there, and a full root can stop DreamOS from booting; `/data` is its own 7.6 GB partition).
- **Host network only, no kernel modules of our own**: the box's kernel 4.9 has no `veth`, `br_netfilter` or `xt_addrtype`, and they aren't in any feed, so Docker's bridge network can't work. Building those modules ourselves was considered and rejected: they must match the vendor's exact kernel build, break with any kernel update, have to be maintained per box model, and a bad module takes the whole box (and a running recording) down. Instead dockerd runs with `bridge: none`, `iptables: false`, and every app container uses `--network host`: an app listens on its own port, which can't be remapped (`resolvePort` refuses a different one and names who holds a taken one — Pi-hole's port 80 is WebControl's). The port is kept in a `dreamhub.port` label since there are no port mappings to read it back from. Home Assistant needs the host network anyway.
- **The overlay module, exactly matching**: Docker's storage needs the `overlay` module, packaged separately. The Gemini feed's `kernel-module-overlay` is built for 4.9.113, the box runs 4.9.269-dream — a different build, never used. Setup takes the module from the enabled feeds only if its version equals the installed `kernel-image`'s, otherwise from the **official Dreambox feed's index** (URL read from the box's own, disabled `dreambox-feed.list.off`) at exactly that version, SHA-256 from the index. Only that one file — the official feed is never switched on, it would offer the official image's packages over Gemini's. Loaded via `/etc/modules-load.d/dreamhub-docker.conf` at boot.
- **Two services, shipped disabled**: the `.deb` now carries `dreamhub-dockerd-helper` (App2's helper, finally packaged) and two units, `dreamhub-docker.service` (dockerd with `/data/dreamhub-docker/daemon.json`, its own socket `/run/dreamhub-docker.sock` so it never collides with a vendor Docker, bounded json-file logs) and `dreamhub-dockerd-helper.service` (bound to it, `ProtectSystem=strict`). Setup enables and starts both; `postinst` restarts only the helper on a DreamHub update (apps keep running); `prerm` on removal stops and disables both; purge deletes `/data/dreamhub-docker`.
- **From the UI, as a plan**: the Apps tab shows "Set up Docker …" while Docker isn't running; the preview lists exactly the missing steps (overlay module and where it comes from, the download, the config, the services), applying shows setup's own log. Refused with less than 1 GB free on `/data`.
- **Confirmed on the box**: setup took 10.7 s (overlay already installed by the box check). Then through the normal App4 plan: Syncthing installed (9 s, image pulled), reachable on `:8384` from the LAN, 28 MiB RAM; logs, stop/start, the backup section and the per-container console over the real daemon (WebSocket → helper → `docker exec`) all worked; removed through its plan. Pi-hole refused (port 80). Idle Docker after cleanup: 211 MB on `/data`.

