# CLAUDE.md

Instructions for any Claude Code (or other AI coding agent) instance working in this repository.

## Start here

Read [`PROJECT_PLAN.md`](PROJECT_PLAN.md) first — it has the full context: why this project exists, what was already considered and rejected, and what's technically confirmed. Then [`ROADMAP.md`](ROADMAP.md) for the phase breakdown. `README.md` is just the one-paragraph pitch.

**Do not re-propose ideas listed as rejected in `PROJECT_PLAN.md`** without a genuinely new argument — they were already brought up and turned down for stated reasons during planning.

The person you're working with (GitHub: `antonbier`) planned this project across a long conversation with another Claude instance that no longer has access to this repo. This file and `PROJECT_PLAN.md` exist so that context isn't lost. Treat them as ground truth for prior decisions, not as suggestions to second-guess from scratch.

## Language rule

- **All repository content is English**: documentation, commit messages, issue/PR titles and bodies, code comments. No exceptions, even though the project was originally planned in German.
- This is a rule about the *repository*, not the *product* — see multilingual requirement below, which is a separate, runtime concern.

## Multilingual requirement (product feature)

DreamHub itself must support multiple languages at runtime:

- **Web UI (PWA)**: a user-switchable language selector, independent of the OS. Use an i18n library (e.g. vue-i18n) with per-locale JSON translation strings.
- **Enigma2 plugin**: must follow DreamOS's own configured system language automatically — no separate setting for the user to maintain. Read `config.osd.language.value` from Enigma2's config system and load matching gettext `.po`/`.mo` translations, the same mechanism other Enigma2 plugins use for their own localization. See the open spike issue for the exact loading convention to verify on-device.

## Architecture constraints (do not violate)

- **Never modify the native WebControl** on port 80. It's Dreambox's own closed-source interface, not meant to be extended for license reasons. DreamHub runs alongside it (planned port 8080), it does not replace or patch it.
- **No live dependency on the Unraid server.** It is not on 24/7. Any feature touching it must be an on-demand/batch action the user triggers when the server happens to be up — never something the core UI depends on being reachable.
- **No software transcoding of live TV.** The Dreambox One's Amlogic S922X has no hardware video encoder (decode-only chip). CPU-side transcoding is too heavy for this box and is explicitly out of scope. See `PROJECT_PLAN.md`, Streaming Bridge section, for the chosen workaround.
- **Don't re-implement what Enigma2/WebControl already exposes** (EPG cache, OSD screenshot grab, power/reboot, on-screen message popup). Call the existing local API instead of re-parsing internal files or binary caches — those formats are undocumented and can change between firmware builds.

## Repo conventions

- Phases are tracked as GitHub Milestones (Phase 0–8).
- Open technical questions that must be resolved before their dependent phase starts are tracked as Issues labeled `spike`, all currently in the "Phase 0 — Spikes" milestone.
- Project board (Kanban): https://github.com/users/antonbier/projects/1
