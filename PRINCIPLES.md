# Design Principles

These are DreamHub's foundational engineering principles — decisions about *how* the system is built, independent of any single phase or module. Where a principle conflicts with something written in `ROADMAP.md` for a specific phase, this document wins; update the roadmap instead of quietly ignoring a principle.

## Multilingual by construction

Both surfaces (web UI and Enigma2 plugin) support multiple languages from the start, not as a bolt-on. See `PROJECT_PLAN.md` and `CLAUDE.md` for the concrete mechanism (i18n library for the PWA, gettext + `config.osd.language` for the plugin).

## Authentication: simple by default, pluggable for later

- **Web UI**: HTTP Basic Auth, off by default, toggled on in Settings. Simple, universally supported, adequate for a LAN appliance.
- **API/app clients**: a separate, user-generatable **API token** (Settings → generate/revoke), not tied to the Basic Auth password. This is what a future Android/iOS app authenticates with, and it's what lets a token be revoked independently without changing the web login.
- **OIDC is explicitly not a v1 requirement.** Running a full identity provider for a single-household device is disproportionate complexity. But the auth layer itself must be built as a swappable interface (not hardcoded Basic-Auth-everywhere), so OIDC — or anything else — can be added later without a rewrite, if DreamHub ever needs to federate identity across a wider homelab.
- **Don't rely on WebControl's own auth.** This project's own device inspection found `config.plugins.Webinterface.http.auth=true` configured but not actually being enforced in practice (see `ARCHITECTURE.md`, "Open question: WebControl's own auth doesn't seem to be enforced"). DreamHub needs its own auth that it controls and can verify independently, not an assumption borrowed from a neighboring service.

## API-first

The REST API is the product; the PWA is just its first client. Concretely:

- Version the API from day one (`/api/v1/...`) so a future mobile app doesn't break when the API evolves.
- No backend logic that only the web frontend can reach — if the web UI can do it, the API can do it, and the web UI calls the API like any other client would.
- WebSockets for real-time push (e.g. "recording finished", "timer conflict") — useful for the web UI today and for a future app's push notifications tomorrow.
- Generate/maintain an OpenAPI spec from the Go code rather than hand-written docs that drift out of sync.

## Interoperability over hardcoded integrations

Prefer generic mechanisms that let DreamHub talk to *anything*, over building bespoke integrations for specific tools:

- **Outbound webhooks** as the general integration point (e.g. "on recording finished, POST to a configured URL") — this is what "out of scope" services (Home Assistant, Telegram, ntfy, whatever the user picks later) hook into, without DreamHub needing to know any of them exist.
- Where a standard format already fits, use it rather than inventing one (e.g. iCal for timers/schedules, M3U for channel/stream lists) — free interoperability with tools that already speak those formats.
- Where DreamHub's API covers the same concepts WebControl's `/web/*` API does, there's no requirement to mirror its exact shape, but avoid gratuitously diverging in naming for no reason — familiarity helps anyone who already knows the Enigma2/OpenWebif ecosystem.

## Fully modular — core vs. feature modules

Mirrors a pattern this project already found and validated on the platform itself: Enigma2 core vs. Gemini's own closed `GP4/` layer vs. open community `Extensions/` (see `ARCHITECTURE.md`) — a clean separation between "the engine" and "swappable feature modules" is a proven pattern here, not a novel idea.

- **Core**: auth, the API framework/router, settings storage, device-capability detection. Nothing feature-specific lives here.
- **Modules**: FilmHub, AIO Search, TV Control, Streaming Bridge, AI Discovery, Plugin/Script Hub (the Phase 2–7 roadmap items) are each independent modules that register their own routes/settings with core at startup.
- Each module must be **independently disableable** without breaking any other module. No module should directly import another module's internals — only core's shared interfaces.
- The PWA frontend mirrors this: each module owns its own routed UI section, hidden entirely when its module is disabled.
- The Settings UI is organized per module, not one undifferentiated form (see below).

## Settings & secrets, handled properly

A single settings area in the web UI, organized per module, covering things like the LLM API key (Phase 6), TMDB API key (Phase 2/4), webhook URLs, the API token(s) from the auth section above.

- Secrets are never echoed back in full after saving — show a masked form (`sk-...ab12`) like every other product that stores API keys.
- Stored with restrictive file permissions, readable only by the service's own user (root, realistically, since `enigma2` itself runs as root — but at least not world-readable).
- Never logged, not even at debug level.

## Runtime capability detection, not hardcoded assumptions

This project's own `ARCHITECTURE.md` is full of facts specific to *one* reference Dreambox One. A different device (different model, different image build, a plugin not installed) will differ. DreamHub should detect what's actually available at runtime — does `ffmpeg` exist, is `/data` mounted, is a given plugin installed — and degrade a feature gracefully (hide it, show a clear "unavailable" state) rather than assume the reference device's configuration and break on anything else.

## Backup/restore, matching the platform's own convention

Every existing plugin surveyed for this project (Bouqueteditor, WebAdmin's package list, etc.) has "Backup Settings" / "Restore Settings" as a standard pattern. DreamHub should follow the same convention: export its own settings + module state as a single file, importable the same way — not a bespoke mechanism users have to learn separately.

## Resilience when external services are unavailable

Phase 6 depends on an external LLM API; the box's internet connection isn't guaranteed. Any feature with an external dependency must fail visibly and locally — cache the last good result, show a clear "unavailable" state — rather than taking down unrelated parts of the UI when one external call fails.

## Ship updates the platform's own way

The platform already has apt/dpkg with its own feed convention and a friendly "Update"/"Upgrade" UI inside WebAdmin (see `ARCHITECTURE.md`). DreamHub's own updates should go through the same `.deb`/apt feed mechanism — no bespoke self-updater to build or maintain.

## Structured logging, no bespoke log management

The platform already runs full systemd with journald (see `ARCHITECTURE.md`). Log to stdout/stderr in a structured (not free-text-spam) format and let `journalctl -u dreamhub` do the rest — don't build custom log rotation or file-based logging.

## Testable without the physical device

This exact session is the reason this principle exists: architecture work happened live against a reference device, and now that device is offline and unreachable. Ongoing development shouldn't have the same constraint. The WebControl API surface documented in `ARCHITECTURE.md` should be mockable, so core logic (FilmHub naming, search, etc.) has tests that run without a live Dreambox connected — device-dependent behavior gets verified separately, opportunistically, whenever access is available.

## License: CC BY-NC-SA 3.0, matching the ecosystem

See [`LICENSE`](LICENSE). This isn't a generic pick — it's the license actually observed in wider use across this specific community (`AdvancedMovieSelection`, `StartUpService`, and others found during the `ARCHITECTURE.md` inspection all use this exact template, including the "may be distributed and executed on hardware licensed by Dream Property GmbH" rider). Fitting the ecosystem's own convention matters more here than picking a more common general-purpose OSS license.

## Data storage: one SQLite file, on `/data`

All of DreamHub's own persistent state (settings, API tokens, cached module state) lives in a single SQLite database, not scattered flat config files. Two reasons: it's the same pattern SeriesRecorder already uses on this platform (see `ARCHITECTURE.md`), and it keeps things in one place that's trivial to include in the backup/restore export described above.

Location: under `/data/dreamhub/`, not the root partition. Root is a 2 GB slot shared by every installed package (see `ARCHITECTURE.md`'s triple-rootfs finding) — keep DreamHub's footprint there limited to the `.deb`'s own binary/assets, and put anything that grows (database, logs if ever file-based, cached data) on `/data`.

## Consistent API error shape

Every endpoint returns errors in the same JSON envelope (an error code plus a human-readable message, at minimum) rather than each module inventing its own shape as it's built. Decide the exact envelope once, in Phase 1, before any other module ships an endpoint.

## Resource budget, stated up front

Confirmed on the reference device: ~1.2 GB RAM available under normal load, **no swap at all** (see `ARCHITECTURE.md`). Target: the Go backend plus its embedded PWA assets should stay well under ~150–200 MB resident. This isn't a nice-to-have — it should actively shape implementation choices from the start: stream/parse XML responses rather than buffering them whole, paginate large lists (recordings, EPG) instead of loading everything into memory, avoid caching more than necessary given there's no swap to fall back on.

## Mock-first development

A config flag/environment variable that points the Go backend at a mocked WebControl API instead of a real device. Makes the "testable without the physical device" principle above concrete and actually usable — including for situations exactly like this one, where the device is temporarily unreachable.

## Plugin/backend version compatibility check

The Enigma2 plugin and the Go backend are separate deployable pieces (even if shipped in the same `.deb`) and can end up out of sync — e.g. the backend gets updated and restarted, the plugin doesn't. The plugin should check the backend's version (a simple `/api/v1/health` call with a version field) on connect and show a clear, friendly error on mismatch rather than failing in some confusing indirect way.

## Semantic versioning + a changelog

DreamHub releases follow SemVer (`MAJOR.MINOR.PATCH`), tracked in [`CHANGELOG.md`](CHANGELOG.md) (Keep a Changelog format). This is what makes the "Update available" flow in a WebAdmin-style UI (see Phase 8) mean anything concrete to the user, and gives a future contributor a fast way to see what changed between two versions without reading commit history.

## Explicit hardware/support scope

DreamHub officially targets the **Dreambox One / DreamOS AIO** — the actual reference device this whole `ARCHITECTURE.md` is built from. Other Enigma2 images or hardware (OpenATV, OpenPLi, non-DreamOS Dreamboxes, other DreamOS-Amlogic models) are best-effort at most, never a support obligation. Stating this explicitly now heads off scope-creep pressure later ("why doesn't this work on my Vu+"), consistent with the capability-detection principle above — DreamHub should degrade gracefully elsewhere, but isn't required to be *correct* elsewhere.

## Code style: idiomatic, not bikeshed

Go code is `gofmt`-formatted, full stop, no style debates. The frontend follows whatever the chosen framework's standard linter/formatter defaults to (e.g. Prettier/ESLint defaults for Vue). Pick the tool once in Phase 1 and never revisit the question.

## Definition of done

A module isn't finished when the feature works. It's finished when: it's documented in the relevant `.md` file, it follows the error-envelope and auth conventions above, it has at least one test that runs without the physical device, and it doesn't push the resource budget over its stated target.

## Keep the documentation current

This isn't aspirational — it's how this project has been run since planning started. Any new confirmed device fact goes into `ARCHITECTURE.md`; any new scope or architecture decision goes into `PROJECT_PLAN.md` or here. A future contributor (human or AI) with no access to this conversation should be able to reconstruct every non-obvious decision from these files alone.
