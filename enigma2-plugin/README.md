# enigma2-plugin/

Minimal Enigma2 plugin for the TV side. Python 2.7 — the only interpreter present on-device (see `ARCHITECTURE.md`). Not yet started.

**v1 scope — a launcher, not a second UI** (same division of labor as the WebControl plugin: basic settings on the TV, everything else in the web UI):
- Shows a **QR code** that opens DreamHub on a phone — no IP address to type.
- Shows whether the backend is running and its version (via the public `/api/v1/health`), with a clear message if plugin and backend versions don't match.
- A few basic settings: auth on/off, port.

**Later**: AI recommendations and notifications shown directly on the TV.

Calls `127.0.0.1:8080` (the local Go backend) only. No business logic of its own. Follows DreamOS's configured system language (`config.osd.language.value`, format `de_DE`) via gettext — see `PROJECT_PLAN.md`.

Distribution: part of DreamHub's `.deb`, via its own apt feed (see [../ROADMAP.md](../ROADMAP.md), release & packaging).
