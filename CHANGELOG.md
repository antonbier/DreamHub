# Changelog

All notable changes to DreamHub are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows [Semantic Versioning](https://semver.org/) (see `PRINCIPLES.md`).

## [Unreleased]

### Added (ControlHub, pulled forward from v1.1)
- While visually verifying WP8's Enigma2 plugin, found the WebInterface plugin's real screenshot (`/screenshot`, native `ePixmap` capture) and remote-control (`/web/remotecontrol`, `/web/zap`, `/web/powerstate`) endpoints — genuinely useful beyond that one test, so ControlHub (originally scoped for v1.1) was built now instead of later (decision D8, `docs/V1_PLAN.md`).
- `webcontrol.Client` gains `Screenshot`, `SendKey`, `Zap`, `InStandby`. `GET`/`POST /api/v1/control/*` (`internal/hubs/control.go`) exposes them as direct, immediate actions — not preview-and-confirm, since a virtual remote is inherently a live input device, not a durable data mutation.
- `ControlHubView.vue`: a live screenshot with manual/auto refresh, a standby banner with a wake button, and a virtual remote pad. GuideHub gets a "switch now" button on whatever's airing right now, calling ControlHub's zap directly.

### Added (v1 plan, WP9: packaging and CI)
- `packaging/build-deb.sh` builds `dreamhub_<version>_arm64.deb`: the cross-compiled backend, `dreamhub.service`, an avahi mDNS service file, and the Enigma2 plugin with its `.po` files pre-compiled to `.mo`. Verified locally via `dpkg-deb --info`/`--contents`; not yet installed live on the reference box.
- `.github/workflows/ci.yml`: backend `go vet`/`go test -race`, frontend type-check/build, then the `.deb` itself, on every push — uploaded as a workflow artifact. Publishing it somewhere a device could actually `apt` from is still open (decision D4).
- `postinst` restarts `enigma2` so the launcher plugin takes effect immediately (same as installing any other Extensions plugin); `postrm` only deletes `/data/dreamhub` on `purge`, never on a plain removal.

### Confirmed live (v1 plan, WP8)
- The Enigma2 launcher plugin installed on the reference box and enigma2 restarted cleanly: no traceback anywhere in the boot journal, the service came back within 15s, and Python's own bytecode cache (`.pyo`) shows the boot-time plugin scan really did import and run the plugin's registration and gettext/config setup.
- **Visually confirmed afterwards**: discovered the WebInterface plugin's real screenshot endpoint (`/screenshot`, not the unrelated `/grab`) and used it together with `/web/remotecontrol` key simulation to actually navigate to and open the plugin. It shows correctly in Enigma2's standard Plugin Browser with its gettext-translated German description; opening it renders the real screen (QR code, buttons) and the backend's own log shows the exact `GET /api/v1/health`/`GET /api/v1/qr.png` calls landing at that moment. It doesn't appear in this image's custom "Gemini BluePanel" quick-launcher — that turned out to use its own separate category list, not a DreamHub issue.
- Found, root-caused and fixed a real bug this surfaced: the status text showed "backend not reachable" despite the backend log proving the call succeeded, because `gotHealth`'s bare `except Exception` swallowed the actual error. Logging it instead of swallowing it surfaced the real cause on the next attempt: `json.loads()` returns Python 2 `unicode`, and Enigma2's SWIG-wrapped `eLabel.setText()` needs a plain `str` — raises `TypeError` on `unicode`. Fixed with a small `_toStr()` helper; **re-verified live**, the screen now correctly shows "Backend läuft (v0.1.0-dev)".

### Added (v1 plan, WP8: Enigma2 launcher plugin; WP7: event bus, SSE live updates)
- The core event bus (`internal/events`): a poller diffs `/web/timerlist`/`/web/deviceinfo` snapshots into `recording.started`/`recording.finished`/`disk.aboveThreshold` events (disk with hysteresis so it can't spam one line every poll while hovering right at 90%), persisted and fanned out live. `GET /api/v1/notifications` (+ mark-read) and `GET /api/v1/events/stream` (SSE). `NotificationsBell.vue`'s "Recent activity" is now this real, persisted inbox instead of change-log entries filtered down to one type.
- Live updates over Server-Sent Events rather than a literal WebSocket — see `api.handleEventsStream`'s doc comment for why (server→client only, no new dependency, and the frontend needs its own stream reader either way since DreamHub's auth header isn't something the browser's native `EventSource` can send). AdminHub's log viewer gets a "Live" toggle on top of this (`GET /api/v1/admin/logs/stream`).
- Found and fixed while building the SSE stream: `httpx`'s request-logging middleware wrapped every response in a type that silently dropped `http.Flusher`, which would have made any streaming response hang indefinitely in production despite passing in Go's own `httptest` environment used by earlier tests.
- The Enigma2 launcher plugin (`enigma2-plugin/`): a QR code (`GET /api/v1/qr.png`, rendered server-side via `github.com/skip2/go-qrcode` so the Python 2.7 side needs no QR library), backend status/version with a mismatch warning, and basic settings (port, whether the backend expects a password). Written and syntax-verified against the reference box's own Python 2.7.13, but not yet installed into a live Enigma2 GUI session — see the module's README for the exact gap.

### Added (v1 plan, WP6: AdminHub logs; WP7: request coalescing)
- AdminHub screen: DreamHub's own runtime log level (debug for 60 minutes, back to info), a journald log viewer filtered and bounded server-side (unit, minimum severity, text — `internal/journald`, real `journalctl -o json` client + mock), and enigma2's crash logs with view-in-browser and secrets redacted on read. The journal is stated as "since last boot only" — it's RAM-backed and cleared on every reboot (`ARCHITECTURE.md`).
- Live tail over WebSocket is still open (needs WP7's event bus); the screen reads a bounded snapshot with its own refresh button for now.
- Diagnostics folded into SystemHub as a second tab instead of its own top-level page — `CONCEPT.md` already described it that way. Old `/diagnostics` links redirect to `/system?tab=diagnostics`.
- `webcontrol.HTTPClient` now coalesces concurrent identical `GET`s: several callers asking for the same path at once (e.g. dashboard tiles loading together) make one real request to the box instead of one each, addressing enigma2's single-threaded request handling. Deliberately not a time-based cache on top of that — it would also apply to a plan's own apply-time freshness check and could silently weaken preview-and-confirm's staleness guarantee.

### Added (v1 plan, WP5: SystemHub)
- SystemHub screen: device info (`GET /api/v1/deviceinfo`), a link into the existing Diagnostics page rather than a second copy of it, and the change log with a client-side filter by action/summary.
- Backup and restore of DreamHub's own state (settings and the dashboard layout) as one JSON file, through preview-and-confirm like any other change: `GET /api/v1/system/backup` downloads it, `POST /api/v1/system/backup/restore` previews exactly which settings and whether the dashboard would change, `POST /api/v1/plans/{id}/apply` applies it. API keys and passwords are never included in a backup — they're excluded at export, not just masked, so the file itself can't unlock the box.

### Added / changed (v1 plan, phase A — WP1: correct box writes)
- Timer writes verified live on the reference box and **enabled by default** in live mode (`DREAMHUB_ENABLE_DEVICE_WRITES=0` switches them off). Writes are form-encoded `POST`s with every parameter. Boxes with the WebInterface's default `anti_hijack` refuse `GET`, and a missing parameter crashes the plugin.
- Every apply is verified against `/web/timerlist`, because the box reports a silently dropped duplicate as success (`409 timer_not_created`). A refusal by the box, e.g. its own tuner conflict check, comes back as `409 box_rejected` with the box's text.
- Delete timers through preview-and-confirm (`POST /api/v1/timers/delete/plan`). The preview warns when the timer is recording right now, since deleting it stops the recording.
- Conflict warnings are now a transponder- and delivery-system-aware estimate (`internal/tuner`), and multi-tuner, FBC and mixed sat/cable boxes are covered by tests. On a single tuner, two channels on the same transponder are no longer flagged. The duplicate warning follows the box's own rule (same channel, same start).
- Optional box login (Settings → WebControl) for boxes that enforce auth for DreamHub. It's used for SerienRecorder's API too. A `401` shows as "asks for a password" instead of "unreachable".
- A SerienRecorder that isn't installed now shows as `not_installed` with an install hint instead of a connection problem. Optional plugins never break DreamHub (`docs/V1_PLAN.md`).
- `docs/V1_PLAN.md`: the plan for v1 (work packages, open decisions, the tuner proposal).

### Added / fixed (v1 plan, WP4: MediaHub files)
- MediaHub browses **every** recording folder. Before, only the top folder was visible (36 of 773 recordings on the reference box), because the box crashes on the subfolder part of `/web/movielist` without an explicit `dirname`.
- Fixed: a `None` file size (a manually copied `.mkv`) made the listing lose that entry and everything after it.
- Rename a recording: every file shown before → after, all or nothing. Move to trash, single or bulk, in AdvancedMovieSelection's `.ts.trash` convention, so it's the same trash the TV shows; it works without AMS too. Restore, and delete for good with the total size named.
- Nothing is touched while it's being recorded or played; this is checked at preview and again at apply. File operations only run when DreamHub is on the box, and `DREAMHUB_ENABLE_DEVICE_WRITES=0` switches them off too.
- Sizes are shown in decimal units (GB), as Enigma2 reports them.
- Name suggestions for renaming, from three sources: the recording's own data (always), TMDB (with a key, in the UI language, episode titles for series) and the AI (with a key, counts against the daily AI limit). DreamHub always builds the name itself (`Title - S01E07 - Episode` / `Title (Year)`, SMB-safe); the AI never writes a file name.
- Verified live on the reference box (approved, reverted): rename and back, trash and restore of a real recording; file list byte-identical afterwards.

### Added (v1 plan, WP3: TimerHub)
- TimerHub screen: timers by day, state, source (SerienRecorder/AutoTimer, from the box's tags), a likely-conflict marker, and deleting through preview-and-confirm.
- Record a programme straight from the TV guide grid. The preview shows the recording window including margins.
- GuideHub search: global search finds programmes through the box's own EPG search, and a result opens the grid at that time.
- AutoTimer (optional plugin), read-only: its rules, its own dry run, and a link to its editor. Without the plugin TimerHub stays fully available.
- Dashboard tile "Conflicts". A shared inline preview component (`PlanPreview.vue`). A refusal by the box now shows the box's own reason everywhere, DiscoveryHub included.
- GuideHub's timeline positions use logical properties (RTL-ready).

### Fixed (v1 plan, phase A — WP2: first run on the box)
- GuideHub's grid shipped the whole EPG (~25 days, about 2 MB per page): `/web/epgmulti`'s `endTime` is a duration in minutes, not an end timestamp. Now 52 KB and 0.1 s for a 4-hour page.
- DreamHub's data directory and SQLite's `-wal`/`-shm` files were world-readable. It now sets umask `077` at startup.

### Added (DiscoveryHub, Diagnostics, time zones, keyboard shortcuts, step 10)
- DiscoveryHub: AI recommendations from a chosen bouquet's EPG (`GET /api/v1/discovery/status`, `POST /api/v1/discovery/recommendations`). New `internal/ai` package (real OpenAI client + mock) — grounded in real EPG data by index, not free-text, so the model can't hallucinate a service reference that would create a timer for the wrong program. Daily budget (`ai.dailyLimit`) now actually enforced via a new `ai_usage` store table (previously an unenforced setting). "Create timer" is a plain call to TimerHub's own plan/apply endpoints — DreamHub's first real preview-and-confirm UI, showing conflict/duplicate warnings before confirming. Nothing calls the AI automatically; generating is always an explicit, budget-visible click.
- Diagnostics page (`/diagnostics`): every hub the user may see, including disabled ones, grouped by Unavailable/Degraded/Disabled/Available with the same translated problem/fix text as everywhere else — no new backend, `GET /api/v1/hubs` already existed for exactly this.
- Time zones: the long-existing per-user display-zone setting (box/browser/fixed) is now actually used everywhere a time is shown, via a new `useDisplayTimeZone` composable; `GET /api/v1/health` gained `serverTimeZone` (read from `/etc/localtime`) so "box" mode has something to resolve to.
- Keyboard shortcuts: `g` + letter global navigation to every hub and Settings.

### Added (GuideHub and SeriesHub, step 9)
- GuideHub: a real TV-magazine-style timeline grid, not just now/next — `GET /api/v1/guide/bouquets`, `GET /api/v1/guide/grid?bouquet=&start=&end=` (`/web/epgmulti`, a genuine time window per channel), grouped server-side. `GuideHubView.vue`: a proportional timeline with an hour ruler and a "now" line, switchable bouquet and switchable time window (◀ / Now / ▶). Verified live against the reference device across two real bouquets.
- SeriesHub: a new `internal/serienrecorder` package (real HTTP+JSON client and mock, mirroring `internal/webcontrol`'s pattern) talks to SerienRecorder's own API, confirmed reachable in the previous device-access session (#10). Tracked series and upcoming episodes (`GET /api/v1/series/markers`, `GET /api/v1/series/upcoming`), `SeriesHubView.vue`. The SerienRecorder connector now does a real reachability check instead of unconditionally reporting "not implemented" on a real device. A link to SerienRecorder's own web GUI ("open in new tab" or an inline, lazy-mounted iframe) rather than rebuilding its marker/timer editing — verified live, the real app loads and works.
- Both hubs are search providers; a new dashboard tile shows upcoming episodes (`series.upcoming`).

### Added (MediaHub read-only screen, device-access investigation, step 8)
- Device access session 2026-09-24: the reference Dreambox came back online after an extended gap. Captured real `/web/*` and SerienRecorder API responses as test fixtures (`backend/testdata/fixtures/`); resolved all remaining `device-access`-labeled roadmap items (#10–#18) — see `ARCHITECTURE.md` and GitHub issue #19 for details.
- MediaHub, first real screens (`GET /api/v1/media/movies`, `GET /api/v1/media/disk`, `MediaHubView.vue`): recordings list with real vs. manually-copied media distinguished (`isRecording`), disk usage overview (WebControl's human-formatted HDD sizes parsed into a percentage via `webcontrol.ParseSize`). Read-only — rename/delete need their own preview-and-confirm design (`ROADMAP.md`).
- MediaHub is the hub contract's second search provider (after TimerHub); a new dashboard tile shows disk usage.
- Confirmed live against the reference device (#11): Enigma2 picks up recordings renamed outside Enigma2 correctly.

### Added (API tokens, hub contract, status strip, step 7)
- API tokens: generate (secret shown once), list, revoke (`internal/apitoken`, `GET`/`POST /api/v1/tokens`, `DELETE /api/v1/tokens/{id}`). `Authorization: Bearer dh_...` now works as an alternative to Basic Auth on any endpoint. Managed from Settings → Authentication.
- Hub contract, first real implementations: search providers (`internal/hub/search.go` — TimerHub is the first) power real hub-content results in the search palette; status-change events (`internal/hub/watcher.go`) write a `hub.statusChanged` entry to the change log whenever a hub's level actually transitions, surfaced as "Recent activity" in the notifications bell.
- Status strip under the top bar: connected, clock skew, hub issue count, and the browser's time zone — replaces the earlier ad hoc "backend unreachable" banner.

### Added (login, search, notifications, onboarding, step 6)
- Login screen: the app manages Basic Auth itself (`GET /api/v1/whoami`), since only `/api/v1/*` sits behind auth, not the SPA shell, so the browser never had a native prompt to react to. Fixes the gap found while testing step 5's settings menu.
- Global search: a `Cmd/Ctrl+K` / `/` command palette over the sidebar's pages. Per-hub content search is still open, pending hub search providers.
- Notifications: the top bar bell shows current issues (degraded/unavailable hubs, box clock skew) — a live snapshot, not yet the persisted event history CONCEPT.md describes.
- Onboarding wizard: language, sign-in, device detection + clock check, hub enable switches, AI key (skippable), channels (honest "not built yet"), and a finish screen with a QR code. Runs once automatically, re-runnable from Settings.

### Added (settings menu & dashboard tiles, step 5)
- Dashboards API (`internal/dashboard`, `GET`/`PUT /api/v1/dashboard`): a fixed catalog of built-in tile types backed by existing endpoints, reconciled on read/write, saved directly and logged like settings.
- Dashboard tiles: show/hide, drag-and-drop reorder, width 1–4, persisted per user; two real tiles — upcoming timers and hubs needing attention.
- Settings menu rendered generically from `GET /api/v1/settings` — every section and field, per-type controls, per-field validation errors, reset-to-default, per-section save; saving language/appearance/sidebar labels here updates the running UI immediately, not only after a reload.
- Found while testing: enabling Basic Auth from the new settings menu has no way to actually sign back in from the web UI yet (tracked in `ROADMAP.md`).

### Added (frontend, step 4)
- Frontend scaffold (Vue 3 + TypeScript + Tailwind + vue-i18n + Pinia, `frontend/`), built into and embedded by the Go backend via `go:embed` with an SPA fallback for client-side routes (`backend/internal/webui`).
- Core shell per `DESIGN.md`: design tokens, bundled Source Sans 3 / Source Code Pro, a self-drawn icon set, collapsible sidebar with a drawer below 900px, top bar with search/notifications shown as explained-not-built placeholders.
- Sidebar driven entirely by `GET /api/v1/navigation`, including translated hub status/problem/fix explanations and the descriptive/hub label switch.
- Dashboard route with the tokened responsive tile grid and an honest empty state; a placeholder route per hub showing its live status.
- i18n: bundled German/Italian/English locales, language switcher, appearance (dark/light/system) and sidebar label mode wired to real settings via `PATCH /api/v1/settings`.

### Added (backend foundation, step 3c)
- Preview-and-confirm: single-use, expiring, owner-bound plans with stale detection; `POST /api/v1/plans/{id}/apply`; applied plans recorded in the change log.
- TimerHub: list timers and preview a new one with margins, single-tuner-aware conflict warnings and duplicate detection (`/api/v1/timers`, `/api/v1/timers/plan`).
- SystemHub change log (`/api/v1/changes`) and AdminHub runtime log level (`/api/v1/admin/log-level`, admin only, debug reverts automatically).
- CSRF protection (`X-DreamHub-Request` header, foreign origins rejected), Host-header check against DNS rebinding (`network.allowedHosts`), request IDs in headers, error bodies and logs.
- Writing to a real box is implemented but off by default until the endpoint is verified on the device (`DREAMHUB_ENABLE_DEVICE_WRITES=1`).

### Added (backend foundation, step 3b)
- Connector registry with cached, time-limited checks: WebControl, SerienRecorder (mock only for now), OpenAI and TMDB with their own settings sections.
- Hub registry with the v1 hubs, requirements and status evaluation (available / degraded / unavailable, with problem and fix codes), plus an enable switch per hub.
- `GET /api/v1/navigation`, `GET /api/v1/hubs`, `GET /api/v1/connectors`.

### Added (backend foundation, step 3a)
- SQLite store (pure Go, statically cross-compilable): users with roles, global and per-user settings, multiple dashboards per user in the data model, change log; database file restricted to the owner.
- Schema-driven settings with typed fields, validation codes per field, cross-field checks, masked secrets, bcrypt-hashed passwords; `GET`/`PATCH /api/v1/settings`.
- Auth is now configured through settings (Basic Auth with bcrypt, cached successful checks) instead of an environment variable.
- `/api/v1/health` reports the server time in UTC.

### Changed
- Concept extended: customizable tile dashboard (decided over a timeline), sidebar navigation, AdminHub (logs in v1, terminal later), roles and multiple dashboards in the data model, internationalization (DE/IT/EN, drop-in locales, RTL-ready), time zone handling (UTC inside), UX direction ("good-looking work tool", desktop-first, Source Sans 3 bundled, dark by default), LAN security (CSRF, DNS rebinding).
- Product restructured into a core (shell), hubs and connectors — see `CONCEPT.md`. `ROADMAP.md` is now organized by hub and release (Foundation / v1 / v1.1 / Later) instead of Phase 0–8.
- New principles: preview and confirm for every change (two-step `plan` → `apply` API, change log, trash instead of delete), schema-driven central settings, hub status with fix instructions, AI provider as a swappable connector, profiles-ready data model.

### Fixed
- `/api/v1/health` is now public when auth is enabled, so the TV plugin's version check works without credentials. Added tests for Basic Auth and API tokens.

### Added
- Backend scaffold (Go): `/api/v1` router with health/deviceinfo/movies endpoints, JSON error envelope, structured logging, Basic Auth + API token middleware, environment-based config, and a WebControl client with both a real HTTP/XML implementation and a mock backed by fixtures from `ARCHITECTURE.md`. Tests run entirely against the mock — no device required. Confirmed cross-compiling to a static ARM64 Linux binary.

Still pre-release — see `PROJECT_PLAN.md`, `ARCHITECTURE.md`, `PRINCIPLES.md`, `ROADMAP.md` for full context.
