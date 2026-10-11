# Roadmap

What's built and what's open, by release and hub ([`CONCEPT.md`](CONCEPT.md)). This is the authoritative status; every open item links its GitHub issue. How each part was built, what was tried and why, is in the archived [build log](docs/archive/BUILD_LOG.md) (up to 2026-10-09), in [`CHANGELOG.md`](CHANGELOG.md) and in the commits. Device facts live in [`ARCHITECTURE.md`](ARCHITECTURE.md).

Releases: **Foundation** → **v1** → **v1.1** → **Later**; GitHub milestones mirror them. Released: **0.2.0** (2026-10-07). On `main` for the next release: ChannelHub, ad detection and cutting, the background queue, change log by profile, radio in the background, and the bug-hunt fixes of 2026-10-09 — see `CHANGELOG.md`, "Unreleased".

"Mock" below means built and tested against the mock box only; "box" means confirmed on the reference Dreambox One.

## Foundation — core shell (done)

Go backend (`/api/v1`, SQLite store with migrations, schema-driven settings in tabs with backup/restore), auth (Basic Auth with bcrypt, API tokens), profiles and roles (admin, viewer; PINs; device sessions; permission sets in `internal/perm`), preview and confirm (`internal/plan`: single-use, expiring, owner-bound, stale-checked plans; change log), CSRF and Host checks, request IDs, runtime log level, the hub registry and contract (status, search, views for custom tiles, events), the event bus with the poller (recordings, timer conflicts, disk) and push through ntfy, SSE live updates, the background job queue (`internal/jobs`), connectors with health checks, i18n (de/it/en; fr/es/nl/pl/tr prepared in backend and TV plugin), per-profile display time zone, the dashboard with custom tiles, global search, the setup assistant, diagnostics, an installable web app, accessibility (WCAG 2.1 AA checked with axe-core). All on the box.

- [ ] Web UI texts for fr/es/nl/pl/tr; runtime drop-in locales (`/data/dreamhub/locales/`); RTL (#40)

## v1 — hubs (done)

All on the box unless noted.

- **MediaHub** — folders, disk space, rename (single, batch by naming scheme, into folders) with every sidecar file, `.ts.meta` title edits, covers (box's own, TMDB, "Save covers"), trash compatible with AdvancedMovieSelection, name suggestions (metadata, TMDB, AI), a recording index for search across all folders.
- **TimerHub** — list, create, delete, disable/enable, all through plans and verified against the box afterwards; tuner- and transponder-aware conflict estimate; duplicate warning; AutoTimer read-only with simulation; viewers own the timers they make.
- **GuideHub** — grid, now & next, tonight, one channel; channel logos; record or switch from any programme; EPG search.
- **DiscoveryHub** — AI recommendations from the real EPG, optional taste profile, daily cost limit, one tap to a timer.
- **SeriesHub** — SerienRecorder markers, episode guide with missing episodes (TVmaze, absolute numbering), what's coming; notification when a missing episode airs.
- **SystemHub** — device, disks, diagnostics with a box self-test, change log (filter by profile).
- **ControlHub** — remote with the live picture on every page, power, zap, message to the TV.
- **AdminHub** — the box's services with restart through a plan, journal logs live, crash logs.
- **TV plugin** — launcher with QR code and backend check; plays Plex films in the box's movie player (`/dreamhub/play`); follows the box's language.

Open:
- [ ] Exact conflict dry run through the TV plugin (decision D7) (#33)

Confirmed on the box 2026-10-09: SeriesHub against the box's real SerienRecorder data (#25, no bugs found — info/markers/timer/seriesinfo all match the Go structs exactly); DiscoveryHub's taste-based picks with the real OpenAI key already in Settings (#24, generated real German recommendations from real taste data).

## v1 — release & packaging

Done: `.deb` for arm64 (box) and armhf (built, not tried on a DM900/DM920) with systemd unit, avahi service and the TV plugin; CI builds both on every push; install and in-place upgrade confirmed on the box; `postinst` restarts enigma2 only for a changed plugin and never while the box records; `build-deb.sh` refuses a mismatched web UI or plugin version; user guide ([`docs/USER_GUIDE.md`](docs/USER_GUIDE.md)); first security pass (symlinks, CSRF coverage, image proxy). Making a release: [`docs/RELEASE.md`](docs/RELEASE.md).

- [ ] apt feed (decision D4) (#21)
- [ ] Uninstall and purge on the box (#22)
- [ ] WP10 before 1.0.0: DST days on the box, a run without the optional plugins, dogfooding (#23)

## v1.1

- **RadioHub** — stations with what's on, play on the box or in the browser (also with the screen locked, with a visualization), radio timers. Box.
- **PlexHub** — films and series from a Plex server, cached for when it's off, watched state synced, films in the box's movie player with resume, address failover. Box.
- **AdminHub service status**, **ControlHub message to the TV** — box.
- **ChannelHub** ([`docs/CHANNELHUB_PLAN.md`](docs/CHANNELHUB_PLAN.md)) — bouquets, all channels with "new since", channel check, picon sets; editing as one plan with a backup before and an automatic restore on any difference; bouquets created, renamed, deleted; backups restorable. Box (read-only checks and the first real writes — create/delete a bouquet, add and reorder channels — confirmed 2026-10-09; three real bugs found and fixed in the process, see `docs/CHANNELHUB_PLAN.md`).
- **Ad detection and cutting** ([`docs/ADSCAN_PLAN.md`](docs/ADSCAN_PLAN.md)) — logo learned per channel from keyframes, coarse sampling with bisection, a background job, results that follow renames; a timeline in the recording's details (breaks play in the browser), batch from the selection; cutting ticked breaks out without re-encoding, the original kept in the trash. Box (phase 0 and a real cut played on the TV confirmed 2026-10-09).
  - [ ] Writing the breaks into `.ts.cuts` (the TV skips them) once the format is confirmed (#44) — built and mock-tested (`media.adscan.mark`, "Mark in cut list" in the detail dialog); still needs a box check that the player actually skips the marked OUT→IN regions before release
  - [ ] Phases 2–3: audio fingerprints, learned series patterns, learned timer margins, suspicious recordings (#44)
  - [ ] Cutting by hand (own in/out points) (#27)
- [ ] Radio text / now playing (#28)

## Later

- **StreamHub** ("Live TV", built early) — live TV and recordings decoded in the browser (FFmpeg in WebAssembly, WebCodecs), audio tracks, full screen. Mock with real recordings.
  - [ ] On the box and on phones: live through port 8001, a recording from disk, a mid-range phone (#34)
  - [ ] The threaded build if phones need it; HEVC/UHD (WebCodecs only)
- [ ] **AutomationHub** — the only place anything happens automatically, always opt-in; ideas collected in #35 (nightly ad detection and cutting, cleaning up finished timers) (#35)
- [ ] **AdminHub terminal** — `shellinaboxd` behind DreamHub's auth, off by default (#36)
- [ ] Jellyfin / Emby connectors (#37)
- [ ] More AI providers (#38)
- [ ] Profiles: a kids role with allowed bouquets (prepared: `users.bouquets`) and a Plex age limit (#45)

## Decided against or parked

See `PROJECT_PLAN.md`, "Out of scope", for what was evaluated and turned down. Parked until profiles were done, to be discussed again: a native/Capacitor app (only background playback still argues for it now that HTTPS works through a reverse proxy).
