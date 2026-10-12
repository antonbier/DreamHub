# packaging/extension-template/

A starting point for packaging a DreamHub extension (`docs/EXTENSIONHUB_PLAN.md`, E2 distribution + E3 isolation, #46) — not DreamHub's own package, see `../README.md` for that. Its first real user is the eWeLink/Sonoff extension (`extensions/ewelink/packaging/build-deb.sh`, E4); it's the packaging half of the contract E1 already implements on the core side (`backend/internal/extension`, `backend/internal/hubs/extensions.go`). See [`docs/BUILDING_EXTENSIONS.md`](../../docs/BUILDING_EXTENSIONS.md) for the full guide (registration, the three calls, a worked example) — this file covers packaging only.

## What an extension actually is

A separate OS process — its own `.deb`, its own systemd service, its own small HTTP server on a **localhost-only** port — that on startup:

1. Registers itself with DreamHub: `POST http://127.0.0.1:8080/api/v1/extensions/register` with `{"id", "name", "icon", "admin", "schemaVersion", "baseUrl"}` (`baseUrl` must itself be a loopback address — DreamHub rejects anything else, same as the request's own origin). No DreamHub login: this call is public but checked by remote address, exactly like `Plex.Stream` (`backend/internal/hubs/plex_play.go`).
2. From then on answers three calls DreamHub's backend makes of it, all under its own `baseUrl` (`backend/internal/extension/http_client.go` is the exact client that calls these):
   - `GET /widgets` → a JSON array of widgets (status/meter/list/action — `backend/internal/extension/extension.go` has the full shape and field names)
   - `POST /actions/{id}/plan` → `{"summary", "warnings", "fingerprint"}` (read-only preview)
   - `POST /actions/{id}/apply` with `{"fingerprint"}` → `200`, or `409` if the extension's own state moved on since the preview

An extension never ships any frontend code — DreamHub's `ExtensionsHubView.vue` is the one generic renderer for every extension's widgets.

## Files here

| File | Fill in, then | Installs as |
| --- | --- | --- |
| `control.template` | `@EXT_ID@`, `@VERSION@`, `@ARCH@`, `@MIN_DREAMHUB_VERSION@`, `@EXT_MAINTAINER@`, `@EXT_HOMEPAGE@`, `@EXT_DESCRIPTION@`/`@EXT_DESCRIPTION_LONG@` | `DEBIAN/control` |
| `extension.service` | `@EXT_DESCRIPTION@`, `@EXT_HOMEPAGE@`, `@EXT_USER@`, `@EXT_EXEC@`, `@EXT_ID@` | `/lib/systemd/system/@EXT_ID@.service` |
| `postinst.template` | `@EXT_USER@`, `@EXT_ID@` | `DEBIAN/postinst` |
| `prerm.template` | `@EXT_ID@` | `DEBIAN/prerm` |
| `postrm.template` | `@EXT_ID@`, `@EXT_USER@` | `DEBIAN/postrm` |

Same `sed` substitution DreamHub's own `../build-deb.sh` uses for `control.in`'s `@VERSION@`/`@ARCH@` works here for every placeholder.

## Isolation (E3)

`extension.service` runs the process as its own dedicated system user (created by `postinst.template`, removed only on purge by `postrm.template`, never on a plain removal — matching DreamHub's own data-keeping convention) with systemd's sandboxing directives: no new privileges, a read-only view of the system outside its own state directory, every capability dropped, and network restricted to loopback only (`RestrictAddressFamilies`/`IPAddressAllow=localhost`, on systemd ≥ 235 — see below for the reference box) — an extension reaches nothing beyond `127.0.0.1`, consistent with the core only ever calling *it*, never the other way around, and never proxying it onward to the LAN or internet itself.

**Checked on the reference box (systemd 232, kernel 4.9), 2026-10-12**, with a throwaway unit built from this template (`/run/systemd/system`, removed afterwards): the unit loads and runs — directives systemd 232 doesn't know are logged as `Unknown lvalue … ignoring` and skipped, never fatal. What actually holds there: `NoNewPrivileges`, `ProtectSystem=strict` (a write to `/usr` fails with "Read-only file system"), `ProtectHome`, `PrivateTmp`, `ProtectKernelTunables`/`Modules`, `ProtectControlGroups`, `MemoryDenyWriteExecute`, `RestrictRealtime`, the empty capability set (`CapBnd: 0`) and `RestrictAddressFamilies`. **Ignored on systemd 232**: `ProtectKernelLogs`, `ProtectClock`, `ProtectHostname`, `RestrictNamespaces`, `RestrictSUIDSGID`, `LockPersonality`, `RemoveIPC`, `IPAddressDeny`/`IPAddressAllow`.

**So on this box the loopback-only network restriction does not hold** — the test unit reached the LAN router. There is no per-process fallback either: the box's kernel is built without `CONFIG_NETFILTER_XT_MATCH_OWNER`, so an iptables `--uid-owner` rule can't replace it. On a box with systemd ≥ 235 the directives take effect unchanged; on the reference box an extension's process is sandboxed for files, privileges and capabilities, but not for the network. Treat an extension like any other package you install from a feed: only install ones you trust.

## Second feed (E2)

Extensions are meant to live on their own apt feed, separate from DreamHub's own (`docs/EXTENSIONHUB_PLAN.md`: "a second feed hosts them, matching the original idea that e.g. an Arrr-suite extension would live on its own feed"). `backend/internal/apt/feeds.go`'s `knownFeeds` already recognizes `dreamhub-extensions-feed` as a known, trusted feed name once one exists — standing up the actual feed server is an operational task, not a code one, and stays open until there's a real extension to publish.
