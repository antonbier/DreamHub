# CLAUDE.md

Instructions for any Claude Code (or other AI coding agent) instance working in this repository.

## Start here

Read [`PROJECT_PLAN.md`](PROJECT_PLAN.md) first — it has the full technical context: why this project exists, what's out of scope, and what's technically confirmed. Then [`ARCHITECTURE.md`](ARCHITECTURE.md) — a read-only inspection of a real reference device: the WebControl API, already-installed plugins that overlap with planned modules (check these before writing new code that duplicates them), process/service layout, config conventions. Then [`ROADMAP.md`](ROADMAP.md) for the phase breakdown. `README.md` is just the one-paragraph pitch.

**Do not re-propose items listed as out of scope in `PROJECT_PLAN.md`** without a genuinely new technical argument — they were already evaluated and turned down for stated reasons. Treat `PROJECT_PLAN.md` and `ROADMAP.md` as ground truth for prior decisions, not as suggestions to second-guess from scratch.

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
- **Don't re-implement what Enigma2/WebControl already exposes.** Its `/web/*` API is confirmed OpenWebif-compatible (movies, timers, EPG, device info, services — see `ARCHITECTURE.md` for the confirmed endpoint list and response schemas) and the `/bouqueteditor/*` plugin already handles bouquet editing. Call these instead of re-parsing internal files or binary caches, which are undocumented and can change between firmware builds.
- **Check `ARCHITECTURE.md`'s installed-plugin table before building any module.** Several planned modules (grid EPG, EPG search, recordings browser, cut-list editing, even AI recommendations) may already overlap with a plugin that's already on the device.

## Repo conventions

- Phases are tracked as GitHub Milestones (Phase 0–8).
- Open technical questions that must be resolved before their dependent phase starts are tracked as Issues labeled `spike`, all currently in the "Phase 0 — Spikes" milestone.
- Project board (Kanban): https://github.com/users/antonbier/projects/1
