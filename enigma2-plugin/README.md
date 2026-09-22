# enigma2-plugin/

Minimal Enigma2 plugin (Python 2.7 — the only interpreter present on-device, see `ARCHITECTURE.md`) for the TV side. Display-only,
calls `127.0.0.1:8080` (the local Go backend). No state, no business logic of its own.
Must follow DreamOS's configured system language (`config.osd.language.value`) via gettext —
see `PROJECT_PLAN.md`, multilingual requirement.
Target distribution: `.deb` via its own apt feed (see [../ROADMAP.md](../ROADMAP.md), Phase 8).
