# DreamHub

A modern all-in-one extension system for the **Dreambox One (DreamOS AIO, Amlogic S922X)**.

Combines media management, EPG search, TV control and optional AI recommendations in a fast web interface — as a companion to the native WebControl (port 80), not a replacement for it.

## Tech stack

- **Backend:** Go, statically compiled for ARM64. Runs on port 8080. REST API + WebSockets.
- **Frontend:** PWA (Vue.js + Tailwind), embedded into the Go binary via `go:embed`.
- **TV client:** a minimal Enigma2 plugin (Py2/Py3-compatible), display-only, calls `127.0.0.1:8080`.
- **Distribution (target):** an installable `.ipk` package via its own feed — not a manually placed binary.

## Why this exists

Anything "app-like" (photos, music, dashboards) is already better handled by Android TV on the actual TV. The only thing worth building is what **only the Dreambox can do**, because it's the only device in the house with a tuner: live TV, EPG, recordings, timers. The current native WebControl is functional but dated, both visually and functionally — DreamHub aims to replace that experience without touching the Enigma2 core itself.

## Status

Early planning phase. See [PROJECT_PLAN.md](PROJECT_PLAN.md) for full context (including rejected alternatives and confirmed technical facts) and [ROADMAP.md](ROADMAP.md) for the phase breakdown.

## Out of scope

- **Space-Saver / transcoding via Unraid:** dropped — the Unraid server isn't on 24/7. Solved separately, as an independent script that re-downloads finished releases through the Arr stack — not a live feature, no dependency on DreamHub.
- **Home Assistant integration:** not used, not needed.
- **Casting to other devices:** already solved by DreamEPG Premium.
