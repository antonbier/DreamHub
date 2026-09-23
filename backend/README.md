# backend/

Go server (ARM64, statically compiled). REST API on port 8080. Will embed the web frontend via `go:embed` once it exists (Foundation, see [../ROADMAP.md](../ROADMAP.md)).

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

Environment variables: `DREAMHUB_MODE` (`mock`/`live`), `DREAMHUB_LISTEN_ADDR` (default `:8080`), `DREAMHUB_WEBCONTROL_URL`, `DREAMHUB_DATA_DIR`. Auth is **not** configured via environment — it's a setting:

```sh
curl -X PATCH http://127.0.0.1:8080/api/v1/settings -H 'Content-Type: application/json' \
  -d '{"values":{"auth.enabled":true,"auth.username":"admin","auth.password":"at-least-8-chars"}}'
```

Enabling auth without a username and password is rejected (it would lock everyone out). `/api/v1/health` always stays public.

## API so far

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/health` | public; `status`, `version`, `time` (UTC) |
| GET | `/api/v1/deviceinfo` | from WebControl |
| GET | `/api/v1/movies` | from WebControl `/web/movielist` |
| GET | `/api/v1/settings` | all sections with effective values; secrets masked, passwords never returned (`hasValue` only) |
| PATCH | `/api/v1/settings` | `{"values": {"<key>": <value or null>}}` — `null` resets to default. All-or-nothing; on failure `400 validation_failed` with a code per field in `error.details` |

Errors always use `{"error": {"code": ..., "message": ..., "details": ...}}`. Codes are for clients to translate; `message` is an English developer hint.

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

Produces a static ARM64 Linux binary (~11 MB stripped, including the pure-Go SQLite driver) — no libc/runtime dependencies on the target device.

## Layout

- `cmd/dreamhub/` — entrypoint: config → store → settings → auth → webcontrol client (mock or live) → API router.
- `internal/api/` — the `/api/v1` REST router and the shared JSON error envelope.
- `internal/auth/` — auth middleware; `SettingsVerifier` checks Basic Auth against the credentials in settings (bcrypt, cached); `Credentials` is a static verifier for tests. API tokens are not implemented yet.
- `internal/config/` — environment-based configuration.
- `internal/store/` — SQLite: users and roles, settings (global and per user), dashboards (multiple per user in the model), change log. Migrations are appended to `migrations` in `store.go` and never edited once applied.
- `internal/settings/` — the settings schema (`schema.go`), the core sections (`core.go`: general, auth), and the service that validates, stores, masks and logs (`service.go`). Hubs and connectors will register their own sections.
- `internal/webcontrol/` — the WebControl `/web/*` API client (real HTTP + XML parsing) and its `Client` interface.
- `internal/webcontrol/mock/` — the mock implementation of that interface, used by default and by all tests.
