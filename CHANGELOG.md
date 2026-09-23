# Changelog

All notable changes to DreamHub are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows [Semantic Versioning](https://semver.org/) (see `PRINCIPLES.md`).

## [Unreleased]

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
