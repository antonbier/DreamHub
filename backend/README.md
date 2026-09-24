# backend/

Go server (ARM64, statically compiled). REST API on port 8080, plus the web frontend embedded via `go:embed` (see [`internal/webui`](internal/webui) below).

## Running locally

No device needed — defaults to the mock WebControl client (see [`internal/webcontrol/mock`](internal/webcontrol/mock), fixtures taken directly from real responses recorded in [../ARCHITECTURE.md](../ARCHITECTURE.md)). In mock mode the SQLite database goes to `./.dreamhub-data/` (git-ignored):

```sh
go run ./cmd/dreamhub
curl http://127.0.0.1:8080/api/v1/health
curl http://127.0.0.1:8080/api/v1/settings
```

Against a real device instead (data goes to `/data/dreamhub` by default):

```sh
DREAMHUB_MODE=live DREAMHUB_WEBCONTROL_URL=http://192.168.1.x go run ./cmd/dreamhub
```

Environment variables: `DREAMHUB_MODE` (`mock`/`live`), `DREAMHUB_LISTEN_ADDR` (default `:8080`), `DREAMHUB_WEBCONTROL_URL`, `DREAMHUB_DATA_DIR`, `DREAMHUB_ENABLE_DEVICE_WRITES` (see below). Auth is **not** configured via environment — it's a setting:

```sh
curl -X PATCH http://127.0.0.1:8080/api/v1/settings -H 'X-DreamHub-Request: 1' \
  -d '{"values":{"auth.enabled":true,"auth.username":"admin","auth.password":"at-least-8-chars"}}'
```

**Every mutating request (POST, PUT, PATCH, DELETE) needs the header `X-DreamHub-Request: 1`** and must not come from a foreign `Origin` — CSRF protection, since browsers send Basic Auth credentials automatically. DreamHub sends no CORS headers, so other web pages can't add it.

**Host names**: requests addressed by IP, `localhost` or `*.local` are accepted; any other host name (DNS-rebinding protection) only if listed in the setting `network.allowedHosts` (comma-separated). Otherwise: `421 host_not_allowed`.

**Writes to a real box are off by default.** Creating a timer uses the OpenWebif-style `/web/timeradd`, which wasn't observed during the read-only inspection. In live mode it's refused (`409 writes_disabled`) unless `DREAMHUB_ENABLE_DEVICE_WRITES=1` is set — do that only after verifying the endpoint on the device (#14). In mock mode writes always work.

Enabling auth without a username and password is rejected (it would lock everyone out). `/api/v1/health` always stays public.

## API so far

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/health` | public; `status`, `version`, `time` (UTC) |
| GET | `/api/v1/whoami` | protected; `{username, role}` — doubles as a credentials check, since the web UI's login screen has no browser-native prompt to rely on (only `/api/v1/*` sits behind auth, not the SPA shell) |
| GET | `/api/v1/deviceinfo` | from WebControl |
| GET | `/api/v1/movies` | from WebControl `/web/movielist` |
| GET | `/api/v1/settings` | all sections with effective values; secrets masked, passwords never returned (`hasValue` only) |
| PATCH | `/api/v1/settings` | `{"values": {"<key>": <value or null>}}` — `null` resets to default. All-or-nothing; on failure `400 validation_failed` with a code per field in `error.details` |
| GET | `/api/v1/navigation` | the sidebar for the current user: sections (`main`, `media`, `program`, `device`, `bottom`), each hub with `name` (MediaHub…), `label` (i18n key), `status` and `problems`; disabled hubs and hubs the role can't see are omitted; `labels` is the user's `descriptive`/`hub` choice |
| GET | `/api/v1/hubs` | diagnostics: every hub the user may see, including disabled ones, with status, problems and requirements |
| GET | `/api/v1/connectors` | state of each connector (`ok`, `not_configured`, `unreachable`, `not_implemented`) with problem and fix; never contains secrets |
| GET | `/api/v1/dashboard` | the current user's dashboard layout: `{"tiles": [{"type", "label", "width", "hidden"}, ...]}`, reconciled against the built-in tile catalog (`internal/dashboard`) — a type missing from the saved layout is appended with its default width, an unknown/removed type is dropped |
| PUT | `/api/v1/dashboard` | saves the layout directly (no preview/apply — DreamHub's own state, not a device change; still recorded in the change log). Array order is display order. `400 invalid_tile` for an unknown type, a duplicate, or a width outside 1–4 |
| GET | `/api/v1/search?q=` | queries every hub's search provider (`internal/hub`, the hub contract) the current user can currently see — disabled, invisible-to-role or unavailable hubs are skipped, a provider error is skipped rather than failing the request. `{"results": [{"hubId", "hubLabel", "title", "subtitle", "path"}, ...]}`, grouped by hub in navigation order. Empty or missing `q` returns no results, not an error |
| GET | `/api/v1/tokens` | the current user's API tokens (never the secret): `{"tokens": [{"id", "name", "createdAt"}, ...]}` |
| POST | `/api/v1/tokens` | `{"name": "..."}` → creates a token and returns its secret **once**: `{"id", "name", "createdAt", "token"}`. `400 validation_failed` for an empty name |
| DELETE | `/api/v1/tokens/{id}` | revokes a token; `404 token_not_found` if it doesn't exist or belongs to another user (not distinguished, so one user can't probe another's IDs) |
| POST | `/api/v1/plans/{id}/apply` | executes a previewed plan (second step of every change). `404 plan_not_found` (unknown, expired after 10 min, or already used), `403 plan_forbidden` (other user), `409 plan_stale` (state changed since the preview — nothing done), `409 writes_disabled` |
| GET | `/api/v1/timers` | TimerHub: timers and the number of tuners |
| POST | `/api/v1/timers/plan` | TimerHub: preview a new timer — `{serviceReference, serviceName, name, description?, begin, end, marginBefore?, marginAfter?}` (Unix seconds; margins in minutes, default 5/10). Returns a plan with the recording window and warnings (`timer.conflict` with the clashing timers and tuner count, `timer.duplicate`). Changes nothing |
| GET | `/api/v1/changes?limit=` | SystemHub: change log, newest first. Includes system-detected entries (no `userId`) alongside user actions — currently just `hub.statusChanged` when a hub's status actually transitions (`internal/hub/watcher.go`), noticed the next time navigation/hub status happens to be computed, not via a background poller |
| GET/PUT | `/api/v1/admin/log-level` | AdminHub (admin only): `{"level": "debug", "minutes": 60}` — debug reverts to info automatically |

Authentication accepts **either** Basic Auth or a bearer API token (`Authorization: Bearer dh_...`, from `POST /api/v1/tokens`) on any endpoint, whenever `auth.enabled` is on.

Hub routes answer `404 hub_disabled` when the hub is switched off in settings.

Every response carries an `X-Request-ID` header; error bodies repeat it as `error.requestId`, and every log line includes it.

Problems are `{"code", "params", "fix", "fixParams"}` — all i18n keys plus parameters for the frontend to translate (e.g. `connector.notConfigured` with fix `fix.configureInSettings` and `{"section": "ai"}`).

On a real device, SeriesHub shows as unavailable until the SerienRecorder connector is implemented (#10); in mock mode it's available.

Errors always use `{"error": {"code": ..., "message": ..., "details": ...}}`. Codes are for clients to translate; `message` is an English developer hint.

## Frontend

`npm run build` in [`../frontend`](../frontend) builds straight into `internal/webui/dist` (see `frontend/vite.config.ts`). A minimal placeholder `index.html` is tracked there so `go build`/`go run`/`go test` all work without Node installed; building the real frontend overwrites that file locally (don't commit the overwrite — see `.gitignore`). `go run ./cmd/dreamhub` then serves it at `/` with an SPA fallback (`internal/webui/spa.go`): any path that isn't a real static file and doesn't look like an asset request (no dot in the last path segment) gets `index.html` instead, so vue-router's client-side routes work on a direct load or a refresh; a genuinely missing asset still 404s.

For frontend development against a running backend, run `go run ./cmd/dreamhub` (port 8080) and `npm run dev` in `frontend/` (port 5173) separately — the dev server proxies `/api` to 8080 (see `frontend/vite.config.ts`) and gives you hot reload.

## Testing

```sh
go test ./...
```

All tests run against the mock client and a real SQLite database in a temp directory — no physical Dreambox required, per [../PRINCIPLES.md](../PRINCIPLES.md)'s "Mock-first development" and "Testable without the physical device".

When testing a running server by hand, prefer a built binary over `go run`: `go run` starts a child process that isn't stopped when you kill `go run` itself.

## Cross-compiling for the device

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o dreamhub-arm64 ./cmd/dreamhub
```

Produces a static ARM64 Linux binary (~12–13 MB stripped, including the pure-Go SQLite driver and the embedded frontend) — no libc/runtime dependencies on the target device.

## Layout

- `cmd/dreamhub/` — entrypoint: reads config, picks the WebControl client (mock or live), starts the app.
- `internal/app/` — the wiring: store → connectors → hubs → settings schema (core + connector sections + hub switches) → auth → API router. Used by `cmd/dreamhub` and the API tests, so tests run the production wiring.
- `internal/connector/` — connector registry with cached, time-limited checks, and the built-in connectors (WebControl, SerienRecorder, OpenAI, TMDB).
- `internal/hub/` — hub registry: descriptors, requirements, status evaluation, navigation, and the hub contract's search providers (`search.go`) and status-change detection (`watcher.go`, feeds `GET /api/v1/changes`). `v1.go` declares the v1 hubs and device capabilities.
- `internal/status/` — the shared "available / degraded / unavailable" levels and problem/fix codes.
- `internal/api/` — the core `/api/v1` routes and the assembly of hub routes behind auth and each hub's enable switch.
- `internal/httpx/` — shared HTTP conventions: error envelope, request IDs + request logging, user in context, body decoding, CSRF and Host-header middleware.
- `internal/plan/` — preview-and-confirm: single-use, expiring, owner-bound plans with stale detection; applied plans go to the change log.
- `internal/dashboard/` — the dashboard layout (`GET`/`PUT /api/v1/dashboard`): a fixed v1 catalog of built-in tile types (each backed by data an existing endpoint already provides, e.g. TimerHub for "upcoming timers"), reconciled on read/write so a removed type is dropped and a newly added one always shows up. Saved directly, no plan/apply — same exception as settings.
- `internal/hubs/` — hub behavior (routes): TimerHub (timers, create via plan with conflict check), SystemHub (change log), AdminHub (log level).
- `internal/logging/` — runtime log level with automatic revert from debug.
- `internal/auth/` — auth middleware; `SettingsVerifier` checks Basic Auth against the credentials in settings (bcrypt, cached) or a bearer token against `internal/apitoken` (`TokenVerifier`); `Credentials` is a static verifier for tests.
- `internal/apitoken/` — API tokens: generate (returns the secret once), list, revoke, verify (SHA-256 lookup — no per-request password hashing like Basic Auth's bcrypt, since a random token doesn't need that). `GET`/`POST /api/v1/tokens`, `DELETE /api/v1/tokens/{id}`.
- `internal/config/` — environment-based configuration.
- `internal/store/` — SQLite: users and roles, settings (global and per user), dashboards (multiple per user in the model), change log, API tokens (hash only). Migrations are appended to `migrations` in `store.go` and never edited once applied.
- `internal/settings/` — the settings schema (`schema.go`), the core sections (`core.go`: general, auth, network), and the service that validates, stores, masks and logs (`service.go`). Hubs and connectors register their own sections alongside these (see `internal/app`'s wiring).
- `internal/webcontrol/` — the WebControl `/web/*` API client (real HTTP + XML parsing) and its `Client` interface.
- `internal/webcontrol/mock/` — the mock implementation of that interface, used by default and by all tests. Stateful timers: two fixture timers that overlap (a real conflict on the single-tuner box), and `AddTimer` is visible in later `TimerList` calls.
- `internal/webui/` — embeds the built frontend (`go:embed dist`) and serves it with an SPA fallback; see "Frontend" above.

Run `go test -race ./...` before committing changes that touch concurrency (plans, connector cache, log level).
