# backend/

Go server (ARM64, statically compiled). REST API on port 8080. Will embed the web frontend via `go:embed` once it exists (Foundation, see [../ROADMAP.md](../ROADMAP.md)).

## Running locally

No device needed — defaults to the mock WebControl client (see [`internal/webcontrol/mock`](internal/webcontrol/mock), fixtures taken directly from real responses recorded in [../ARCHITECTURE.md](../ARCHITECTURE.md)):

```sh
go run ./cmd/dreamhub
curl http://127.0.0.1:8080/api/v1/health
```

Against a real device instead:

```sh
DREAMHUB_MODE=live DREAMHUB_WEBCONTROL_URL=http://192.168.1.x go run ./cmd/dreamhub
```

## Testing

```sh
go test ./...
```

All tests run against the mock client — no physical Dreambox required, per [../PRINCIPLES.md](../PRINCIPLES.md)'s "Mock-first development" and "Testable without the physical device".

## Cross-compiling for the device

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o dreamhub-arm64 ./cmd/dreamhub
```

Produces a static ARM64 Linux binary (confirmed ~10 MB) — no libc/runtime dependencies on the target device.

## Layout

- `cmd/dreamhub/` — entrypoint, wires config → webcontrol client (mock or live) → API router.
- `internal/api/` — the `/api/v1` REST router and the shared JSON error envelope.
- `internal/auth/` — Basic Auth + API token verification, toggleable (see `PRINCIPLES.md`).
- `internal/config/` — environment-based configuration.
- `internal/webcontrol/` — the WebControl `/web/*` API client (real HTTP + XML parsing) and its `Client` interface.
- `internal/webcontrol/mock/` — the mock implementation of that interface, used by default and by all tests.
- `internal/store/` — reserved for the SQLite settings store (not yet implemented — see `PRINCIPLES.md`, "Data storage").
