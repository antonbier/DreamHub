# DreamHub

A modern all-in-one extension system for the **Dreambox One (DreamOS AIO, Amlogic S922X)**.

Combines media management, EPG search, TV control and optional AI recommendations in a fast web interface — as a companion to the native WebControl (port 80), not a replacement for it.

## Tech stack

- **Backend:** Go, statically compiled for ARM64. Runs on port 8080. REST API + WebSockets.
- **Frontend:** PWA (Vue.js + Tailwind), embedded into the Go binary via `go:embed`.
- **TV client:** a minimal Enigma2 plugin (Py2/Py3-compatible), display-only, calls `127.0.0.1:8080`.
- **Distribution (target):** an installable `.ipk` package via its own feed — not a manually placed binary.

## Why this exists

Generic "app-like" functionality (media browsing, dashboards) is already well covered by existing platforms and apps. The only thing worth building here is what **only the Dreambox can do**, because it's the only tuner in the setup: live TV, EPG, recordings, timers. The current native WebControl is functional but dated, both visually and functionally — DreamHub aims to replace that experience without touching the Enigma2 core itself.

## Status

Early planning phase. See [PROJECT_PLAN.md](PROJECT_PLAN.md) for full context (including out-of-scope items and confirmed technical facts) and [ROADMAP.md](ROADMAP.md) for the phase breakdown.

## Out of scope

See [PROJECT_PLAN.md](PROJECT_PLAN.md#2-out-of-scope) for the full list and reasoning. In short: no home automation, no generic infrastructure dashboards, no casting, no media browsing — anything not tied to the tuner itself.
