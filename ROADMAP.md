# Roadmap

Organized by the hub structure described in [`CONCEPT.md`](CONCEPT.md). (Earlier versions of this file were organized as Phase 0–8; the technical notes from those phases have been moved into the matching sections below.)

Releases: **Foundation** → **v1** → **v1.1** → **Later**. GitHub milestones mirror these.

## Phase 0 — Spikes (done)

- [x] **Bouquet editing** — don't reimplement. The existing BouquetEditor webif plugin already provides a complete UI and its own API (`/bouqueteditor/*`, see `ARCHITECTURE.md`).
- [x] **Hardware transcoding** — confirmed unavailable on this hardware.
- [x] **ffmpeg availability** — available via the standard feed (package `ffmpeg`; the box uses `apt`/`dpkg`, not opkg). Declare it as a `.deb` dependency rather than assuming it's preinstalled.
- [x] **Enigma2 language mechanism** — `PluginLanguageDomain`/`PluginLanguagePath` + `gettext.bindtextdomain()` + `language.addCallback()`, `.po`→`.mo` via `msgfmt`. Reference: [`opendreambox/enigma2-plugins`](https://github.com/opendreambox/enigma2-plugins), [`opendreambox/enigma2-po`](https://github.com/opendreambox/enigma2-po).
- [x] **Recording sidecar files** — confirmed on-device: `.ts`, `.ts.meta`, `.ts.ap`, `.ts.sc`, `.ts.cuts`, `.eit`.

## Foundation — backend & core shell

**Done** (built and tested against the mock WebControl client, no device access — see `backend/README.md`):
- [x] Go module, `/api/v1` router, JSON error envelope, structured logging (`internal/api`)
- [x] Basic Auth + API token middleware, toggleable (`internal/auth`); `/api/v1/health` stays public so the TV plugin's version check works with auth enabled
- [x] Environment-based config, mock/live mode switch (`internal/config`)
- [x] WebControl client: real HTTP+XML implementation and a mock (fixtures from `ARCHITECTURE.md`) behind one interface
- [x] `/api/v1/health`, `/api/v1/deviceinfo`, `/api/v1/movies`
- [x] Tests run entirely against the mock
- [x] Cross-compiles to a static ARM64 Linux binary (`CGO_ENABLED=0 GOOS=linux GOARCH=arm64`), ~11 MB stripped incl. SQLite
- [x] **3a** SQLite store (`internal/store`, pure-Go driver, migrations via `PRAGMA user_version`, file mode 0600): users with **roles**, per-user and global settings, **multiple dashboards per user** (v1 creates one), change log. Data dir `/data/dreamhub` (live) or `./.dreamhub-data` (mock)
- [x] **3a** Schema-driven settings backend (`internal/settings`): typed fields (string, bool, int, enum, timezone, secret, password), per-field validation codes, cross-field checks (e.g. auth can't be enabled without credentials), all-or-nothing updates recorded in the change log without secret values, secrets masked, passwords stored as bcrypt and never returned. `GET`/`PATCH /api/v1/settings`
- [x] **3a** Auth configured through settings instead of env vars; Basic Auth verified with bcrypt, successful checks cached 10 min (bcrypt is slow on the box's ARM cores), cache invalidated on any auth change
- [x] **3a** `/api/v1/health` reports server time in UTC (for the clock check)

- [x] **3b** Connector registry (`internal/connector`): WebControl (real check via `deviceinfo`), SerienRecorder (OK in mock mode, honestly "not implemented" on a real device until #10), OpenAI and TMDB (own settings sections for their keys; "not configured" until a key is set). Reports cached 30 s, recomputed immediately when settings change; checks time out after 2 s
- [x] **3b** Hub registry (`internal/hub`) with the v1 hub descriptors: name, descriptive label, icon, path, group + order, minimum role, requirements (connector / capability / other hub, required or optional) → status *available / degraded / unavailable* with problem and fix codes; one enable switch per hub in settings. `GET /api/v1/navigation` (sidebar per user: role filter, disabled hubs hidden, empty groups omitted, labels mode), `GET /api/v1/hubs` (diagnostics incl. disabled hubs), `GET /api/v1/connectors`
- [x] **3b** Shared wiring in `internal/app`, used by `cmd/dreamhub` and the API tests alike
- [x] **3c** Preview-and-confirm (`internal/plan`): plans are single use, expire after 10 min, belong to their creator, and are rejected as stale when the underlying state changed since the preview; applied plans are written to the change log. `POST /api/v1/plans/{id}/apply`
- [x] **3c** First real flow: TimerHub `GET /api/v1/timers` and `POST /api/v1/timers/plan` with margins, single-tuner-aware conflict detection (peak concurrency vs. tuner count) and duplicate detection as non-blocking warnings
- [x] **3c** Hubs mount their own routes (`api.Mount`); the core guards them with auth and the hub's enable switch (`404 hub_disabled`)
- [x] **3c** Security: CSRF (`X-DreamHub-Request` header required on mutating requests, foreign `Origin` rejected, no CORS headers), `Host` header check against DNS rebinding (IPs, `localhost`, `*.local`, plus `network.allowedHosts`)
- [x] **3c** Request IDs in every response header, error body and log line; runtime-switchable log level (AdminHub, admin only, debug reverts automatically)
- [x] **3c** SystemHub change log endpoint `GET /api/v1/changes`
- [x] **3c** Real-box writes (`/web/timeradd`) — verified live 2026-09-24 and **on by default** since (`DREAMHUB_ENABLE_DEVICE_WRITES=0` turns them off); see the v1 WP1 entry under TimerHub

- [x] **4** Frontend scaffold (Vue 3 + TypeScript + Tailwind, `frontend/`), built straight into `backend/internal/webui/dist` and served via `go:embed` with an SPA fallback for client-side routes (`internal/webui`); a tracked placeholder page keeps the backend buildable before the frontend is ever built
- [x] **4** Core shell per `DESIGN.md`: design tokens as CSS custom properties (dark/light), Source Sans 3 + Source Code Pro bundled (no CDN), a self-drawn icon set, sidebar (collapsible on desktop, a drawer below 900px per the documented breakpoints), top bar (search and notifications shown as explained-not-built placeholders — both still open below)
- [x] **4** Navigation: sidebar rendered entirely from `GET /api/v1/navigation` — hub status dots with translated problem/fix explanations on hover, unavailable hubs greyed out, descriptive/hub label switch
- [x] **4** Dashboard route with the tokened 4/2/1-column responsive grid from `DESIGN.md` and an honest empty state for the (then-hypothetical) case of no tiles; one placeholder route per hub showing its live status/problems
- [x] **4** i18n: bundled `de`/`it`/`en` JSON locales (vue-i18n), English fallback, language switcher (names, not flags) wired to `PATCH /api/v1/settings`; runtime drop-in locales from `/data/dreamhub/locales/` is still open
- [x] **4** Appearance (dark/light/system, system follows `prefers-color-scheme` live) and sidebar label mode wired to real settings, not local-only state

- [x] **5** Dashboards API (`internal/dashboard`, `GET`/`PUT /api/v1/dashboard`): a fixed v1 catalog of built-in tile types (each backed by an existing endpoint, e.g. TimerHub — CONCEPT.md's *hub-contributed* tile types still need per-hub search providers, see "Open" below), reconciled on read/write, saved directly and logged like settings
- [x] **5** Dashboard tiles (decided in #16): show/hide, drag-and-drop reorder (native HTML5 DnD), width 1–4 via the tile's own ←/→ buttons, per-user persisted layout; two real built-in tiles — "Upcoming timers" (`GET /api/v1/timers`) and "Needs attention" (`GET /api/v1/hubs`, degraded/unavailable hubs with the same translated problem/fix as the sidebar)
- [x] **5** Settings UI rendered from the schema (`SettingsView.vue`): every section and field `GET /api/v1/settings` returns, with no frontend-side list to keep in sync — bool toggle, enum select, int, string, timezone (text + hint), secret/password (masked/write-only, focus-to-clear-for-typing, explicit Clear), per-field validation errors from a rejected `PATCH`, reset-to-default, one Save/Cancel per section matching the backend's per-section cross-field checks. Saving `general.*` here also updates the live top-bar/user-menu state (`preferences.ts`) instead of only taking effect after a reload

- [x] **6** Login screen (`LoginScreen.vue`, `internal/api`'s `GET /api/v1/whoami`): the app manages Basic Auth itself instead of relying on the browser's native prompt, since only `/api/v1/*` sits behind auth, not the SPA shell — closes the gap found while testing step 5. Credentials live in `sessionStorage` for the tab's lifetime; a 401 from any endpoint drops them and falls back to the login screen; "Sign out" is in the user menu whenever a credential is actually held
- [x] **6** Global search (`SearchPalette.vue`, `Cmd/Ctrl+K` or `/`): a command palette over the sidebar's pages (Dashboard, Settings, every enabled hub) — real, substring-matched, keyboard-navigable. Per-hub *content* search (recordings, timers, EPG entries) still needs the hub search-provider contract below; this is the page-level layer everything else will plug into
- [x] **6** Notifications (`NotificationsBell.vue`): the top bar's bell now shows *current* issues — degraded/unavailable hubs (same data and translated problem/fix as the sidebar) plus a box-clock-skew check against `GET /api/v1/health`. This is a live snapshot, not the persisted, hub-emitted event history CONCEPT.md describes; that needs the core event bus below and is intentionally not simulated here
- [x] **6** Onboarding wizard (`OnboardingView.vue`, `/onboarding`, CONCEPT.md's 7 steps): language, sign-in, device detection (`GET /api/v1/deviceinfo`) + clock check, hub enable switches, AI provider/key (skippable), channels (an honest "not built yet" — needs ChannelHub, v1.1), and a finish screen with a QR code linking to the current origin. Runs once automatically (a local "seen it" flag, not server state) and is re-runnable any time from a link on the Settings page

- [x] **7** API tokens (`internal/apitoken`, `GET`/`POST /api/v1/tokens`, `DELETE /api/v1/tokens/{id}`): generate (secret shown once), list, revoke — SHA-256 lookup, not bcrypt (PRINCIPLES.md: a random token doesn't need deliberately-slow hashing the way a user password does). `SettingsVerifier` now accepts a bearer token as an alternative to Basic Auth on any endpoint. Managed from `ApiTokensPanel.vue`, the one hand-built exception on the otherwise fully generic settings page, right after Authentication
- [x] **7** Hub contract: search providers (`internal/hub/search.go`) — a hub registers a `SearchProvider`; TimerHub is the first real one (matches timer/channel name). `GET /api/v1/search?q=` skips disabled, role-invisible or unavailable hubs and degrades gracefully if a provider errors. `SearchPalette.vue` now merges these with its page matches, debounced
- [x] **7** Hub contract: status-change events (`internal/hub/watcher.go`) — the closest thing to a hub-emitted event v1 has: a hub's level actually changing is noticed and written to the change log (no `userId` — system-detected, not a user action) the next time navigation/hub status happens to be computed. No background poller; an honest limitation, not a hidden one. `NotificationsBell.vue` shows the last 5 as "Recent activity" alongside its live current-issues view
- [x] **7** Status strip (`StatusStrip.vue`, DESIGN.md): connected, clock skew, hub issue count as small dotted items, plus the browser's own time zone right-aligned in faint (the per-user display-zone *preference* is still open, below). Replaces the earlier ad hoc "backend unreachable" banner

**Open**:
- [ ] Remaining parts of the hub contract: search providers now cover TimerHub, MediaHub, SeriesHub and GuideHub, events still just TimerHub-triggered status changes — extend both to the rest of the hubs as they're built
- [x] Custom tiles (2026-09-28): a new part of the hub contract, **views** (`internal/hub/view.go`) — a hub offers view sources and returns its candidate items (`title`, `subtitle`, `time`, `end`, `size`, `warn`, `path`); filters, sort and limit are applied centrally, the same for every source, and a source lists only the filters and sorts that make sense for it. Not built on the search providers as CONCEPT.md first sketched: those return title/subtitle/path only, too little to filter "older than 90 days, larger than 3 GB" on. Sources: `media.recordings` (top folder only, like MediaHub's search — the full-tree walk blocks enigma2), `timers.upcoming` (with the conflict estimate as `warn`), `guide.broadcasts` (a bouquet, up to 3 days ahead; "movies" = a minimum length, since the box's EPG has no genre), `series.upcoming`. A custom tile (`type: "custom"`, own `id`, `custom: {title, source, query, display}`, up to 12) is saved with the layout; `CustomTileBuilder.vue` (the mockup's "view with criteria" dialog, with a live match count) creates and edits them, `CustomTile.vue` shows list or count. Cover-row display is still open
- [x] Core event bus (`internal/events`, docs/V1_PLAN.md WP7): "detection, not automation" — a `Poller` diffs successive `/web/timerlist`/`/web/deviceinfo` snapshots (the same data TimerHub/MediaHub already read) into `recording.started`/`recording.finished`/`disk.aboveThreshold` events (disk uses hysteresis: fires at 90%, only re-arms below 85%, so it can't spam one line every 30s while hovering at the line). Every event is persisted (`store`'s `notifications` table) and fanned out live to subscribers (`events.Bus`) — a slow subscriber just misses the live push, never blocks a publisher, and can always catch up from the persisted feed. `hub.statusChanged` still goes through the change log only, as before; unifying the two is a later cleanup, not required for this. Timer conflicts aren't detected here yet (still open below) — TimerHub's own per-timer marker already covers the interactive case
- [x] Live updates over SSE rather than a real WebSocket (`GET /api/v1/events/stream`, `GET /api/v1/admin/logs/stream`): every event here only ever flows server → client, so Server-Sent Events gets the identical "no polling" result over plain HTTP, with reconnection built into the format and no new dependency for a hand-rolled WebSocket handshake — see `api.handleEventsStream`'s doc comment. The frontend still can't use the browser's native `EventSource`, though: DreamHub's own Basic Auth header (`api/authState.ts`) isn't something `EventSource` can send at all, so `useEventStream.ts` reads the stream itself over `fetch()` instead, with the same auth header every other request uses. `NotificationsBell.vue`'s "Recent activity" is now the real, persisted inbox (`stores/notifications.ts`), not just `hub.statusChanged` entries filtered out of the change log; AdminHub's log viewer has a "Live" toggle (polls `journald.Reader` every 3s server-side and pushes only new lines — journald has no `tail -f` a Go client can hold open without a second, separately-managed process, and this gets the same result far more simply)
- [x] `docs/V1_PLAN.md` WP7, first bullet: `webcontrol.HTTPClient` now coalesces concurrent identical `GET`s (`coalescedGet`) — several callers asking for the same path at once (dashboard tiles loading together, the debounced search palette re-firing) make exactly one real request to the box instead of one each, addressing `ARCHITECTURE.md`'s "enigma2 serves WebControl single-threaded" finding. Deliberately *not* a time-based cache on top of that: a short TTL cache would also apply to a plan's own apply-time freshness re-check (`internal/plan`'s Fingerprint/Current), silently weakening preview-and-confirm's staleness guarantee — coalescing alone can't, since it only ever merges requests already happening at the same instant
- [x] Timer-conflict detection in the event bus (2026-09-28): the poller publishes `timer.conflict` when a timer still to come newly clashes with another (typically SerienRecorder or AutoTimer adding one in the background). The per-timer estimate moved into `tuner.TimerConflicts`, which TimerHub's list and the poller now share. The first tick only takes a baseline, so a restart doesn't re-announce existing conflicts; a conflict is announced once, not every 30 s
- [x] Push notifications to the phone (2026-10-04, `internal/push`): events from the bus are forwarded through **ntfy** (ntfy.sh or a self-hosted server; the free ntfy app subscribes to a topic — no account, nothing to open on the router). Settings → Push: server, topic (checked against ntfy's syntax), optional token, and one switch per kind — recording finished (on), recording started (off), missing episode airing (on), timer conflict (on), disk almost full (on); "Send a test" (`POST /api/v1/push/test`). Texts leave the box finished, so `push/messages.go` carries them in de/en/it; the bell keeps translating client-side. Best effort: a push that fails is logged, the event stays in the bell. What a push contains (titles, channel names) goes to the ntfy server — hence a hard-to-guess topic, suggested on the page
- [x] Missing episodes airing soon (2026-10-04, `internal/hubs/series_watch.go`): every 3 hours SeriesHub runs its detail page's check over all active followed series and publishes `series.missingAiring` for each broadcast of a missing episode in the next 24 hours that no timer covers — once per broadcast, across restarts (the bus's own feed is the memory). Detection only: it never sets a timer
- [x] UX pass over every hub (2026-10-04), at desktop and phone width, dark and light: the top bar's search shrinks to an icon on phones, rows and log lines stack instead of squeezing, native controls follow the theme, a sticky jump row on Settings (`/settings#<section>`), weekday/date on dashboard times that aren't today (`formatWhen`) — see DESIGN.md
- [x] Installable app (2026-10-04, pulled forward from Later): `manifest.webmanifest`, icons (`frontend/public/icons`, rendered from SVG by `frontend/art/render-icons.mjs`), `public/sw.js` caching only the shell — never `/api/*` — and registered only in a secure context (`src/lib/pwa.ts`); Settings → General explains per browser how to install (Install button where offered, iOS share-sheet steps, why plain http blocks the prompt); `sw.js` and the shell are served `no-cache`. HTTPS in front stays the user's choice (USER_GUIDE.md §4)
- [x] "Can't reach DreamHub" instead of the login form when the first `whoami` gets no answer at all (box off, app opened away from home) — it used to look like a password problem; retries on `online` and every 15 s (`UnreachableScreen.vue`)
- [ ] Runtime drop-in locales (`/data/dreamhub/locales/`) and a full accessibility pass are still open (new screens since 2026-09-28 are built accessible from the start — real buttons with labels, keyboard operation, focus handling in dialogs and panels — so the pass mostly concerns older screens)
- [x] Italian strings complete (checked 2026-09-28): every key of the web UI's `en.json` exists in `it.json` (and `de.json`), the only identical values being names and formats (SerienRecorder, `S{season}E{episode}`, "Enigma2 {version}"); the Enigma2 plugin's `it` catalog translates all 14 messages. A native speaker's read-through is still worth doing
- [x] Time zones: the per-user display zone setting (`general.timezoneMode`: box/browser/fixed, `general.timezone`) already existed but nothing consumed it — resolved via a new `useDisplayTimeZone.ts` composable (mode 'fixed' → the configured IANA zone; mode 'box' → the device's own zone; mode 'browser', the previous behavior, or an unresolved 'box' → Intl's runtime default). "Box" needed a new data source: WebControl's API has no timezone field (confirmed, ARCHITECTURE.md), so `GET /api/v1/health` now also reports `serverTimeZone`, read from the `/etc/localtime` symlink target (the one place this is derivable without SSH — works on Linux and macOS, useful for local dev too). Wired into every place that formats a time: the timers/series dashboard tiles, MediaHub/SeriesHub/GuideHub/DiscoveryHub screens, API tokens, and the status strip (which now shows the *effective* zone, not just the browser's)
- [x] Keyboard shortcuts beyond search: `g` + letter global navigation (`useGlobalShortcuts.ts`, mounted once in `App.vue`, same "not while typing" guard as the search palette's own Cmd/Ctrl+K) — `g d` dashboard, `g m` recordings, `g s` series, `g g` guide, `g t` timers, `g r` recommendations, `g y` system, `g c` control, `g a` admin, `g ,` settings. Accessible contrast and labels audit still open
- [x] Diagnostics: grouping every hub the user may see — including disabled ones — into Unavailable/Degraded/Disabled/Available, each with the same translated problem/fix text as everywhere else. No new backend needed: `GET /api/v1/hubs`'s own doc comment already called it "the diagnostics view". Originally its own core nav entry (`/diagnostics`, `DiagnosticsView.vue`); folded into SystemHub as a tab in WP6 (see SystemHub, below) since `CONCEPT.md` already described it as "a diagnostics page in SystemHub" — `/diagnostics` now redirects to `/system?tab=diagnostics`
- [ ] mDNS advertisement via a service file in `/etc/avahi/services/` (`avahi-daemon` already runs and is used this way by an existing companion app — see `ARCHITECTURE.md`)
- [x] Capture real fixtures (#14): done 2026-09-24, see `backend/testdata/fixtures/` and `ARCHITECTURE.md`

## v1 — hubs

### MediaHub

**Done**:
- [x] Listing via `/web/movielist` (already-parsed title/description/length/filesize — see `ARCHITECTURE.md`), `GET /api/v1/media/movies` (`internal/hubs/media.go`)
- [x] The scanner distinguishes real recordings from other media in the same folder (`isRecording`, by file extension — confirmed live against a real `.mkv` file manually copied into `/media/hdd/movie/` on the reference device, see `ARCHITECTURE.md`): the frontend (`MediaHubView.vue`) shows non-recordings dimmed and labeled, doesn't touch them
- [x] Disk space overview, `GET /api/v1/media/disk` (the reference device's disk is close to full — a real, recurring pain point): WebControl's HDD fields are human-formatted strings (`"2 TB"`), parsed into a usage percentage by `webcontrol.ParseSize`
- [x] MediaHub is the hub contract's second search provider (after TimerHub) — recording titles are searchable from `SearchPalette.vue`
- [x] Dashboard tile: disk usage (`media.diskSpace`, `MediaDiskSpaceTile.vue`)
- [x] Confirmed 2026-09-24 (#11): Enigma2's movie list picks up files renamed outside Enigma2 correctly, no rescan needed — see `ARCHITECTURE.md`

**Done** (v1 plan WP4):
- [x] Folder browsing: every recording folder, one folder per request (`GET /api/v1/media/movies?dir=`). Previously only the top folder was visible (36 of 773 recordings on the reference box, see `ARCHITECTURE.md`)
- [x] Tolerant listing: `None` sizes no longer drop entries and subfolders
- [x] `internal/recfs`: which files belong to a recording, rename with rollback, trash in AdvancedMovieSelection's `.ts.trash` convention (works without AMS), restore, empty. Paths are confined to the recording folders. In-memory FS for the mock
- [x] Rename, trash (single and bulk), restore and delete-for-good, each as preview-and-confirm. Refused while the file is being recorded or played, checked again at apply. Stale previews rejected
- [x] File operations only when DreamHub runs on the box, and switched off together with device writes (`DREAMHUB_ENABLE_DEVICE_WRITES=0`)
- [x] Verified live 2026-09-25 on the reference box: rename there and back, trash and restore of a real recording; file list byte-identical afterwards (`ARCHITECTURE.md`)
- [x] MediaHub screen: folder breadcrumbs, recordings with inline rename (every file before → after), selection for bulk trash, trash tab with total size. Sizes are now decimal (GB) per `CONCEPT.md`

**Open**:
- [x] Name suggestions (decision D2), all ending in the same rename preview (`POST /api/v1/media/rename/suggest`):
  - three sources: the recording's own metadata (`.ts.meta` plus folder names, always available), TMDB (optional, v3 key or v4 token, results in the user's language, episode titles for series) and the AI provider (optional, counts against `ai.dailyLimit`)
  - every source returns only fields; `internal/naming` builds the name: `Title - S01E07 - Episode` / `Title (Year)`, the Plex/Jellyfin/Kodi convention, made SMB-safe
  - verified on the reference box's real recordings (metadata source)
- [x] Change the title in `.ts.meta` along with a rename, or on its own (2026-09-28): an option in the rename form; the preview lists the title change after the files; only line 2 of the file changes, written atomically (temporary file + rename, permissions kept); the title is part of the plan's staleness check; if the renames fail after the title was written, the old content goes back
- [x] Confirmed live: the box's movie list shows a changed `.ts.meta` title immediately. The description (line 3) isn't editable yet
- [x] Global search across all folders (2026-09-28, `internal/hubs/media_index.go`): a recording index read straight from the file system when DreamHub runs on the box (the same condition as file operations) — never through WebControl, whose full walk takes ~40 s and blocks enigma2. Titles and times from `.ts.meta` (never from file names), other video files by name, up to 6 folder levels, hidden folders and symlinks skipped. Built on first use, refreshed in the background after 10 minutes and right after every rename/trash/restore/empty. Search results and the recordings view source (custom tiles) use it and open the recording's folder (`/media?dir=`); off the box both fall back to the top folder
- [x] Rename engine covering **all** sidecar files of a `.ts` recording: `.ts`, `.ts.meta`, `.ts.ap`, `.ts.sc`, `.ts.cuts`, `.eit`. Plus existing cover art (`.jpg`, `.backdrop.jpg`) — move it along, don't re-fetch from TMDB. WebControl's API has no file-operation endpoints (`ARCHITECTURE.md`), so this needs its own filesystem-backed component (real: direct `os.Rename` — the backend runs on-device with local access to `/media/hdd`, per `PROJECT_PLAN.md`'s packaging plan, no SSH needed in production; mock: in-memory, per `PRINCIPLES.md`'s "Mock-first development")
- [x] Every rename/delete goes through preview-and-confirm (`internal/plan`, same pattern as TimerHub's `planCreate`); delete means move to trash
- [x] Never touch a file that is currently being recorded or played — cross-reference `/web/timerlist` against the current time (already resolved conceptually, #12; not yet wired into a mutating MediaHub endpoint since none exist yet)
- [x] Naming schemes and batch rename (2026-09-28): Settings → Recordings holds one scheme for films and one for episodes (`media.naming.movie`/`.episode`, `internal/naming/template.go`): placeholders `{title} {year} {season} {episode} {episodeTitle} {channel} {date}`, zero padding `{episode:02}`, optional parts in `[…]` dropped when a placeholder in them is empty; checked on save (must contain `{title}`, episodes `{episode}`, no slashes). The defaults reproduce the previous fixed names exactly. Single-rename suggestions now follow the scheme; `POST /api/v1/media/rename/batch/plan` renames a whole selection ("Select all" + "Rename by scheme" in the selection bar): every recording before → after, and what's left alone and why (already named so, name taken, two aiming at the same name, in use); optionally completed by TMDB (a film's year, an episode's title — exact matches only). One confirm, all or nothing: a failure renames every recording back. `{date}` is the recording's day from `.ts.meta`, in the box's own zone
- [x] Sorting into folders by scheme (2026-10-04): two folder schemes in Settings → Recordings (`media.naming.folderMovie`/`.folderEpisode`, e.g. `{title}/Staffel {season:02}` or `Filme`), same placeholders as the name schemes, always relative to the recording folder (no leading `/`, no empty or `..` segment, checked on save; empty = don't move). The batch rename preview has "Sort into folders": each recording shows its target folder, and the new folders are listed. On apply the missing folders are created (mode 0755) and every file of a recording moves along, all or nothing — a failure moves everything back and removes the folders it made. A path through a link is refused. Moves stay within the recording folder, i.e. one file system
- [ ] Bulk actions via a background queue (only needed once something takes longer than one request — the batch rename is a series of renames within one file system, well under a second)
- [ ] `.ts.meta` format: line 1 service reference, line 2 title, line 3 title+description+genre, line 4 unix timestamp — parse only where the API doesn't already provide the field (not needed yet — `/web/movielist` already covers everything the current read-only screen shows)
- [ ] Reference implementation: `AdvancedMovieSelection` (`Rename.py`, `SearchTMDb*.py`) — native on-screen plugin, not web-based, CC BY-NC-SA licensed: read for approach, don't copy code

### TimerHub

**Done** (v1 plan WP1, `docs/V1_PLAN.md`):
- [x] `/web/timeradd` and `/web/timerdelete` verified live on the reference box (create + delete, reverted, `timers.xml` byte-identical afterwards — `ARCHITECTURE.md`)
- [x] WebControl client: every write a form-encoded `POST` with every parameter, `DeleteTimer`, `sessionid` retry on `412` (boxes with `extended_security`), optional Basic Auth from Settings → WebControl, `ErrUnauthorized` → connector problem `connector.authRequired`
- [x] Apply verifies against `/web/timerlist` (the box reports a silently dropped duplicate as success); the box's refusal surfaces as `box_rejected` with its own text
- [x] Duplicate warning follows the box's rule (same channel + same begin)
- [x] Conflict estimate is transponder- and delivery-system-aware (`internal/tuner`: one tuner records every channel of a transponder; satellite/cable/terrestrial matched to each frontend's capabilities), with test boxes for a triple-tuner, an FBC and a mixed sat/cable setup — layers 1–2 of the tuner proposal in `docs/V1_PLAN.md`
- [x] Delete via preview-and-confirm: `POST /api/v1/timers/delete/plan`, warning when the timer is recording right now

**Done** (v1 plan WP3):
- [x] TimerHub screen (`TimerHubView.vue`): upcoming timers grouped by day in the display time zone, finished ones folded away; state (scheduled / starting / recording / done / disabled), source read from the box's tags (SerienRecorder; AutoTimer when it tags), likely-conflict marker per timer (`GET /api/v1/timers` now returns `source` and `conflict`), tuner count
- [x] Delete from the list via preview-and-confirm (`PlanPreview.vue`, the shared inline preview), incl. the "stops the recording in progress" warning
- [x] Record from GuideHub: click a programme → details → preview → create, passing the EPG event id so the box links the timer to it
- [x] AutoTimer (optional plugin, decision D3): read-only list, the plugin's own dry run (`GET /api/v1/timers/autotimers[/simulate]`), a link to its editor (`/autotimereditor/`). A box without the plugin gets one quiet line, and TimerHub stays *available* (the AutoTimer connector reports `not_installed` under Diagnostics only)
- [x] Dashboard tile "Conflicts" (`timers.conflicts`)
- [x] Verified live against the reference box (read-only test instance): all 7 real timers recognized as SerienRecorder's, no false conflicts, AutoTimer detected with no rules configured

**Open**:
- [x] Disable/enable a timer (2026-10-04, decision D5, `internal/hubs/timer_toggle.go`): not through `/web/timerchange` (its conflict path is buggy) but by taking the timer out (`timerdelete`) and putting it back (`timeradd`) with only `disabled` changed — folder, tags, after-event, repeat and EPG link carried over (`Timer.AsNewTimer`). All or nothing: if the box refuses the changed timer (a conflict on enabling), the original is put back and the plan reports "unchanged". A recording timer is refused (taking it out would abort the recording); enabling previews the same conflict estimate as creating. Not yet run against the box
- [ ] Exact conflict dry run through DreamHub's own Enigma2 plugin (D7) — WP8

Notes:
- Timers via `/web/timerlist`; AutoTimer.
- **Conflict detection is central**: the reference device has a single tuner, so overlapping timers are common.
- Creating/deleting timers goes through preview-and-confirm, including when triggered from DiscoveryHub.

### GuideHub

**Done**:
- [x] A real TV-magazine-style timeline grid ("TV-Zeitschrift"), not just now/next: `GET /api/v1/guide/bouquets` (`/web/getservices`) and `GET /api/v1/guide/grid?bouquet=&start=&end=` (`/web/epgmulti?bRef=&time=&endTime=`: `endTime` is a **duration in minutes**, corrected 2026-09-24 after the grid was found shipping the whole EPG, about 2 MB per page — see `ARCHITECTURE.md`), grouped server-side (`internal/hubs/guide.go`'s `groupByChannel`) into one schedule per channel
- [x] `GuideHubView.vue`: a proportional timeline (event blocks positioned by percentage of the visible window, a "now" line, an hour ruler), switchable bouquet (selector hidden when there's only one, as on the reference device) and switchable time window (◀ / Now / ▶, `WINDOW_HOURS = 4` per page). Verified live against the reference device across two real bouquets ("Free TV", "neu (TV)") with real, correctly-positioned programming
- [x] Corrected an earlier assumption: `/web/epgnownext` (now/next only) has been fully replaced by `/web/epgmulti` (a genuine time window) — one real endpoint, not two overlapping ones

**Open**:
- [ ] EPG via `/web/getcurrent` for the currently-tuned service specifically (the grid already covers the general case)
- [x] Online enrichment (2026-09-28, `internal/enrich`, a core service every hub uses): poster, rating, genres and overview from TMDB for the selected programme, recordings (row thumbnail, genres, rating), the series list and series page, and recommendation cards. The EPG has no genre to restrict this to films/series as first planned; instead **only an exact title match counts** (title or original title, normalized — `internal/titles`), an episode title falls back to its series ("Tatort: …" → "Tatort"), and a near-miss shows nothing — a wrong poster is worse than none. Answers (misses too) are cached in SQLite (hits 30 days, misses 3), images on disk under the data dir (100 MB cap, least recently used dropped first) and served by DreamHub itself — the browser never contacts TMDB (PRINCIPLES.md), and cached pictures stay visible without internet. Lookups are batched (≤ 40 titles, 4 at a time, 8 s budget; what isn't found in time is asked again next time). Switchable: Settings → TMDB → "Load pictures, ratings and overviews" (`tmdb.enrich`, on by default once a key is set)
- [x] Sources in order, step 1 (2026-09-28): cover art **already on the box** comes first — `<base>.jpg`/`.png` and `<base>.backdrop.jpg` next to a recording (the convention confirmed on the reference box, ARCHITECTURE.md #11), whichever plugin wrote it. The folder listing flags `cover`/`backdrop` per recording (only when DreamHub runs on the box), `GET /api/v1/media/cover` serves them read-only (regular files only, checked by content to be JPEG/PNG, paths through `recfs.CleanIn`), and the recording row shows that picture instead of TMDB's. So data another plugin keeps isn't kept a second time; TMDB is the fallback
- [x] Step 2 (2026-09-28): DreamMediaInfo's database as the first source (`internal/dmi`, read-only; ARCHITECTURE.md, "DreamMediaInfo"): overview, genres, rating, **FSK** and the exact TMDB ID, so TMDB is only asked for the poster, by ID. Optional: without the plugin, TMDB directly. Covers EPG titles well (what DreamMediaInfo caches), recordings rarely
- [x] Which episode a broadcast is, and whether it's missing (2026-10-04, `internal/hubs/series_broadcast_episodes.go`): each broadcast under "In the EPG" on a series page gets its `S01E02` code — from the EPG itself when a channel writes it, else from DreamMediaInfo's `match_cache` (`dmi.Reader.EpisodeCode`: key `<tmdb id>|<episode title>`, tried with the short description's first line and the title after the series name, case-insensitive) — and a state: recorded, planned (SerienRecorder or a covering timer), missing, or unmatched. "Missing" is only claimed when the series' recordings carry season codes (season ≥ 1): SerienRecorder's absolute numbering (S00E8615) doesn't line up with TMDB's. The page says how many missing episodes air soon, next to the existing Record action. Not yet checked against the real `match_cache` on the box (offline): how its episode-title keys are spelled is inferred from the inspection, so the candidates may need tuning
- [x] Saving covers to the box (2026-10-04; decided: yes, by explicit action, never overwriting): "Save covers" in MediaHub's selection bar (`POST /api/v1/media/covers/plan`, `internal/hubs/media_cover_save.go`) writes the poster (TMDB `w342`) as `<base>.jpg`/`.png` and the backdrop (`w780`, JPEG only) as `<base>.backdrop.jpg` next to recordings — the names Enigma2 and other plugins already use. A recording that already has a poster keeps it, only a missing backdrop is added; checked at preview, again by the plan's fingerprint, and once more at write time (`recfs.Create` uses `O_EXCL`, so it neither replaces a file nor writes through a link). All pictures are fetched first, then written; a failure removes what this apply created. Files get mode 0644
- [x] Further databases (2026-10-04, `internal/enrich/extras.go`): OMDb for Rotten Tomatoes' Tomatometer, IMDb's rating and Metacritic's Metascore (by the IMDb ID from TMDB's `external_ids`), fanart.tv for title logos (films by TMDB ID, series by TheTVDB ID, also from TMDB) — two new optional connectors, each with its own key in Settings. Asked only for one title's detail view (`GET /api/v1/enrich/extras`), never for a list, cached a week, so OMDb's free 1,000 requests a day are never in reach; logos go through DreamHub's image cache like TMDB's pictures. Shown as chips after the TMDB rating and the logo above the details, credited in the source line
- [x] Details everywhere (2026-10-04): a recording's title opens the shared details dialog (`ProgrammeDialog`, its own cover on the box preferred over TMDB's poster, the recording's description, ratings, logo) with "Play on the TV" — a zap to the recording's own reference, like the remote (not yet tried on the box); the series page shows the logo and the OMDb ratings too
- [x] Checked on the box: 34 of 35 top-folder recordings found (after reading titles out of Enigma2 default file names), no wrong match seen
- [x] GuideHub search provider over the box's own EPG title search (`/web/epgsearch`): upcoming and running programmes only, soonest first, unnamed services (`-1:…`, `<n/a>`) dropped; a result opens the grid at that programme (`/guide?at=`)
- [x] Record a programme from the grid (via TimerHub's plan/apply, see TimerHub)
- [x] "Switch now" on a programme actually airing right now — a direct call to ControlHub's zap (`/web/zap?sRef=`, confirmed live 2026-09-25), not preview-and-confirm (switching channels isn't a durable mutation the way creating a timer is)
- [x] Four switchable views (2026-10-04, `frontend/src/components/guide/`), the choice remembered per browser, phones starting on Now & next: **Grid** (the timeline, as before); **Now & next** — per channel the current programme with a progress bar and the next one, plus "Switch", from one call to the box (`GET /api/v1/guide/nownext` → `/web/epgnownext?bRef=`, whose "None" fields for channels without EPG are handled, fixture-tested); **Tonight** — what starts between 19:55 and 21:30 (display zone) as poster cards, a week ahead, short fillers under 15 minutes left out; **Channel** — one channel's TV day (05:00–05:00) as a list, now marked, past dimmed (`grid?service=` narrows the answer to that channel)
- [x] Filters across all views: films only, rated 7+ (from enrichment; a title that isn't known doesn't pass), remembered per browser; the grid dims instead of hiding
- [x] Channel logos (picons) from the box (`GET /api/v1/guide/picon`): `/data/piconpool` first, then the usual Enigma2 picon folders, by service reference (also as service type 1) or channel name — still to check against the reference box's real picon names
- [x] Every view opens the same details dialog as DiscoveryHub (`ProgrammeDialog`): backdrop, poster, logo, ratings, overview, the EPG text, Record and Switch now
- [ ] `GraphMultiEPG` and `TvSpielfilm` are native on-screen references, not replacements

### DiscoveryHub

**Done**:
- [x] AI recommendations from a chosen bouquet's EPG (`GET /api/v1/discovery/status`, `POST /api/v1/discovery/recommendations`), filtered by picking a bouquet (reuses GuideHub's `/api/v1/guide/bouquets`) rather than an individual-channel multi-select — a bouquet is already the platform's own "channels I care about" grouping, so this needed no new settings-framework field type
- [x] New `internal/ai` package (real `OpenAIClient` + mock, mirroring `internal/webcontrol`'s pattern, per `PRINCIPLES.md`'s "Mock-first development"). **Grounding, not generation**: the model is given a numbered candidate list and asked to return picks by *index* plus a reason; DiscoveryHub builds each `Recommendation` from its own `EPGItem` data at that index, never from fields the model could hallucinate (a wrong service reference would create a real timer for the wrong program) — out-of-range indices are silently dropped, covered by a unit test
- [x] Daily AI call budget actually enforced (`ai.dailyLimit`, previously an unenforced settings field): `internal/store`'s new `ai_usage` table, one row per UTC date, incremented *before* the real call (a slow/failed call still counts, so a retry-happy client can't exceed the cap); `0/day` works as a full off switch
- [x] "Create timer" is a plain frontend call to TimerHub's own `POST /api/v1/timers/plan` + `POST /api/v1/plans/{id}/apply` (CONCEPT.md: "When DiscoveryHub creates a timer, it calls TimerHub's API" — not a new backend endpoint); `DiscoveryHubView.vue` shows the plan's warnings (conflict/duplicate) before confirming, DreamHub's very first real preview-and-confirm UI
- [x] Nothing calls the AI automatically — every real call costs money, so "Generate recommendations" is always an explicit click, never on page load or a background refresh
- [x] The OpenAI connector's `Check` was already settings-only (API-key presence); left as-is deliberately — a network round-trip to a paid API just to show a status dot isn't worth it, unlike WebControl/SerienRecorder's real reachability pings to a local, free LAN device

- [x] Taste profile (2026-09-28): with "Take my taste into account" on, the request also carries what the user records (recent titles, top folder), follows in SerienRecorder (active markers), has planned (upcoming timers) and searches for with AutoTimer (enabled search terms) — titles only, deduplicated per series, at most 40 each (`internal/hubs/discovery_taste.go`, `ai.Taste`). The prompt asks for picks that fit the taste without re-recommending what's already followed or planned, with the reason naming the related show. Every source is optional: a missing plugin or unreachable box just leaves that list empty. **Opt-in, per browser**: it's the user's own viewing data going to an external provider, so it's off until switched on, and "What gets sent?" (`GET /api/v1/discovery/taste`, which sends nothing) shows the exact lists first; the response reports what was taken into account
- [x] Broadcasts a timer already covers are no longer offered to the AI as candidates — the user has them already
- [x] The day's picks are kept (2026-10-04): every generation is stored per user (`discovery_picks`, `internal/hubs/discovery_saved.go`) and `GET /api/v1/discovery/recommendations` returns it — leaving the page, reloading or opening DreamHub on another device shows the same set without a new AI call; ended broadcasts drop out, covered ones say "Timer set"
- [x] Cards and details (2026-10-04): picks grouped by day as poster cards (film/series, year, rating, "on now"); a card opens a detail dialog with the backdrop, everything the data sources know (poster, logo, ratings incl. Rotten Tomatoes/IMDb, FSK, full overview), why the AI picked it, and Create timer

**Open**:
- [ ] Check the taste-based picks against the real box's recordings and a real OpenAI key (only the mock AI was exercised, box offline)
- [ ] Swapping to a different AI provider (the interface is provider-agnostic already, only `OpenAIClient` exists)
- [ ] Per-user channel-level filtering finer than a whole bouquet, if a bouquet ever proves too coarse in practice

### SeriesHub

**Done**:
- [x] SerienRecorder's own web interface API confirmed reachable and wired up (`internal/serienrecorder`, real HTTP+JSON client and mock, per `PRINCIPLES.md`'s "Mock-first development") rather than reading `SerienRecorder.db` directly while SerienRecorder may be writing to it — resolved #10, see `ARCHITECTURE.md`
- [x] Tracked series (`GET /api/v1/series/markers` → `/serienrecorderapi/markers`) and upcoming episodes (`GET /api/v1/series/upcoming` → `/serienrecorderapi/timer?remaining=1`), `SeriesHubView.vue`
- [x] SeriesHub is a search provider (tracked series' names); a dashboard tile shows upcoming episodes (`series.upcoming`, `SeriesUpcomingTile.vue`)
- [x] The SerienRecorder connector does a real reachability check (`/serienrecorderapi/info`) instead of unconditionally reporting "not implemented" on a real device — the honest placeholder from before #10 was resolved
- [x] A link to SerienRecorder's own web GUI (`/serienrecorderui/`, confirmed no `X-Frame-Options`/CSP blocking it) rather than rebuilding marker/timer/settings editing DreamHub doesn't need to duplicate (`PRINCIPLES.md`): "Open in new tab" always works; "Show here" embeds it in an iframe, lazy-mounted only when toggled on. Verified live against the reference device — the real SerienRecorder app loads and is fully usable. Only resolves correctly when DreamHub is deployed on the box itself (same host as WebControl, just port 80 vs. 8080), not in local dev against a different device

**Open**:
- [x] SerienRecorder not installed → connector state `not_installed` with an install hint instead of "unreachable" (v1 plan rule 1: optional plugins never break DreamHub)
- [x] Episode guide with missing episodes (2026-09-28): a new optional connector, **TVmaze** (`internal/tvmaze`, public API, no key; Settings → TVmaze, on by default, since it sends followed series' names), supplies every episode; the series page lists them per season (newest first, open) as recorded (a recording carries the S01E02 code), planned (SerienRecorder has timered it), still to come, or missing. **"Missing" is only claimed when the series' recordings carry episode codes at all** — otherwise every aired episode would look missing, so it just says "aired" and explains why. The show must match the series name exactly (normalized), so "Tatort-Reiniger" is never taken for "Tatort". Show matches are cached 30 days (misses 3), episode lists 12 h, and a stale list is still used while TVmaze is down
- [ ] Check the guide on the box: how many followed series TVmaze finds by name (German titles), and whether SerienRecorder's recordings carry their S01E02 codes where `/web/movielist` reports them (title or description). SerienRecorder's own `transmissions` endpoint remains an alternative source once its format is captured
- [ ] Check the detail view's recording match against real SerienRecorder recordings on the box (how SR names its timers/files there, whether its `S01E02` codes land in the title or description)

**Done** — series detail view (2026-09-28, mock only, box was offline):
- [x] `GET /api/v1/series/markers/{id}` (`internal/hubs/series_detail.go`): the marker (now with `seasons` and `recordfolder`, both in the live fixture), its timered episodes (filtered by `fsid`), its recordings on disk and further EPG broadcasts. Each part from a different source, failing on its own (`unavailable: [...]`) instead of failing the view; WebControl is optional for SeriesHub
- [x] Recordings: the marker's record folder (or every recording folder) plus a direct subfolder named after the series, one level only — a few `/web/movielist` calls, not a disk walk. A recording counts when its title *is* the series ("Tatort", "Tatort: …", "Bones - …"; case, accents and punctuation ignored, "Tatort-Reiniger" excluded) or it sits in the series folder. Season/episode from an `S01E02` code in title or description, never from the file name
- [x] Broadcasts: the box's own EPG title search (`/web/epgsearch`), filtered to real matches that haven't ended, flagged when a timer already covers them; "Record" goes through TimerHub's own plan → apply, same inline preview as DiscoveryHub
- [x] `SeriesDetailView.vue` at `/series/{id}`; the series list and upcoming episodes link to it, search results for a series open it directly, and the sidebar keeps a hub highlighted on its sub-pages

### SystemHub

**Done** (v1 plan WP5, `docs/V1_PLAN.md`):
- [x] `SystemHubView.vue`: device info (`GET /api/v1/deviceinfo` — image/Enigma2/WebControl versions, tuner count and models) and a diagnostics tab. Diagnostics used to be its own core route (`/diagnostics`); it's now folded into SystemHub as a second tab instead of a link to a near-duplicate hub-status page (`docs/V1_PLAN.md`, WP6), matching `CONCEPT.md`'s own "a diagnostics page in SystemHub". Old `/diagnostics` links redirect to `/system?tab=diagnostics`
- [x] Backup and restore of DreamHub's own state as one JSON file, through preview-and-confirm like any other change: `GET /api/v1/system/backup` (settings plus the dashboard layout, decision-consistent with `PRINCIPLES.md`), `POST /api/v1/system/backup/restore` previews exactly which settings and whether the dashboard would change (`plan.Change` per field), applied via the normal `POST /api/v1/plans/{id}/apply`
- [x] API keys and passwords are never in a backup — excluded at export (`settings.TypeSecret`/`TypePassword` fields are skipped, not just masked), so the file itself can't unlock the box; restoring re-validates every field through the same `settings.Service.Update` a manual edit uses
- [x] Change log screen (`GET /api/v1/changes`) with a client-side filter over action/summary and a "load more" step

**Open**:
- [ ] Filtering the change log by user (v1 has exactly one user; revisit once profiles exist)

### ControlHub

Pulled forward from v1.1 (`CONCEPT.md`, `docs/V1_PLAN.md`): verifying the Enigma2 plugin (WP8) needed a real screenshot/remote-control loop anyway, and GuideHub's "switch to this channel now" needs it too.

**Done** — confirmed live 2026-09-25 while verifying WP8 (`ARCHITECTURE.md`, "Real screenshot and remote-control endpoints"):
- [x] `webcontrol.Client` gains `Screenshot` (`GET /screenshot?format=png&osd=1&video=1` — a top-level path, not under `/web/`, native `ePixmap` capture, no external binary), `SendKey` (`GET /web/remotecontrol?command=`, standard Linux input-event codes), `Zap` (`GET /web/zap?sRef=`) and `InStandby` (`GET /web/powerstate`)
- [x] `GET /api/v1/control/screenshot`, `GET /api/v1/control/power`, `POST /api/v1/control/key`, `POST /api/v1/control/zap` (`internal/hubs/control.go`) — every action direct and immediate, not preview-and-confirm: PRINCIPLES.md's rule is about durable data mutations with real consequences, and a virtual remote is inherently a live, instantly-reversible input device the same way a physical one (or WebControl's own native remote page, which DreamHub deliberately doesn't reimplement) is
- [x] `Zap`/`SendKey` gated by `AllowWrites` like every other change to the box, for consistency with the read-only-trial switch even though channel-switching is trivially reversible; `Screenshot`/`InStandby` are read-only and always available
- [x] `ControlHubView.vue`: a live screenshot (manual refresh + optional auto-refresh every 4s), a standby banner with a wake button, and a virtual remote pad (arrows/OK, exit, menu, four color keys, power)
- [x] GuideHub's "switch now" button on a programme airing right now, calling ControlHub's zap directly (CONCEPT.md: a hub calls another hub's own API, not new backend logic)

**Done** — remote rework (2026-09-28, mock only, box was offline):
- [x] The remote laid out like a physical one (`components/control/RemotePad.vue`): power and mute, a number pad, Menu/EPG/Info, volume and channel rockers around a round direction pad with OK in the middle, Back, the four color keys, playback. Before this, every key showed only the first two letters of its name ("Ob", "Li") and the page's `lg:` grid never applied (the project defines no `lg` breakpoint, only `below-*`), so the remote always sat under the screenshot
- [x] The picture and the remote side by side on ControlHub's page (stacked below 900px, the picture pinned on top while scrolling), and the same two components as a side panel any page can open from the top bar (`RemotePanel.vue`, open/closed remembered per browser; an overlay drawer below 900px)
- [x] The picture refreshes itself ~0.7s after every key the box accepted (`stores/remote.ts`'s `keysSent`), so a key's result shows without a manual reload
- [x] The whole pad takes the physical keyboard while focused (`lib/remoteKeys.ts`: arrows, Enter = OK, Backspace = back, 0–9, PageUp/PageDown = channel, +/- = volume). Escape stays reserved for closing the panel
- [x] The screenshot is fetched with the app's own `Authorization` header (`api.control.screenshot()`) instead of a plain `<img src>`, which couldn't carry it and would have 401'd whenever DreamHub's auth is on
- [x] The mock box simulates a screen (`webcontrol/mock/screen.go`): a test card, a menu overlay MENU opens and UP/DOWN moves through, standby on POWER. ControlHub now visibly works in mock mode

**Open**:
- [x] Keys checked against the box's keymap and live (ARCHITECTURE.md): no EPG key exists on a Dreambox — replaced by TV, Radio, Audio, Text
- [x] "Message to TV" (2026-09-28, pulled forward from v1.1): `POST /api/v1/control/message` (`{text, kind: info|warning|error, timeout}`, 1–300 characters, 0–600 s) → `/web/message` as a form-encoded POST with the classic WebInterface's `text`/`type`/`timeout`, gated by `AllowWrites`; a small form under the picture on ControlHub's page, which refreshes the picture afterwards
- [x] `/web/message` confirmed from the plugin source and live

### AdminHub

**Done** (v1 plan WP6, `docs/V1_PLAN.md`):
- [x] Admin role only (`adminOnly` on every route); bottom of the navigation next to Settings
- [x] `internal/journald` (real `Exec{}` shelling out to `journalctl -o json`, mirroring `internal/webcontrol`'s real/mock pattern) reads DreamHub's, enigma2's and other units' log lines, bounded (`-n`, default 200, max 1000) and filtered server-side (`-u` unit, `-p` minimum severity) rather than ever fetching the journal in full — it's the box's whole RAM buffer since boot anyway (`ARCHITECTURE.md`, "journald is volatile"), which `AdminHubView.vue` states explicitly rather than implying real history
- [x] `AdminHubView.vue`: the log level control (already had an endpoint, now a screen — debug for 60 minutes, back to info), a filterable log viewer (unit, minimum level, text), enigma2's crash logs (`GET /api/v1/admin/crashlogs`, listed from `/media/hdd/enigma2_crash_<unix-seconds>.log` via the same file system MediaHub's `internal/recfs` uses) with view-in-browser, secrets redacted (`password`/`token`/`secret`/`apikey`/`authorization` key-value lines)
- [x] Request IDs already linked UI errors to log entries (Foundation, step 3c) — nothing new needed here

**Done** (v1 plan WP7's live-tail piece):
- [x] Live tail: a "Live" toggle on the log viewer subscribes to `GET /api/v1/admin/logs/stream` (SSE — see step 7, "Live updates over SSE"), appending new lines as `journald.Reader` picks them up server-side every 3s. Filters (unit/level/text) restart the subscription; switching it off leaves the last snapshot on screen

### Global search
- Every hub contributes a search provider; results grouped by hub. `EPGSearch` (native on-screen) is a reference for search/filter logic.

### Enigma2 TV plugin (launcher)

**Done** (v1 plan WP8, `docs/V1_PLAN.md`; `enigma2-plugin/`):
- [x] `GET /api/v1/qr.png`: the backend renders the QR code itself (new dependency `github.com/skip2/go-qrcode`, MIT-licensed pure Go, no cgo — hand-rolling a QR encoder correctly, with its Reed–Solomon error correction and mask-pattern selection, was judged not worth the correctness risk for a well-understood, already-solved problem), encoding `r.Host` so it always points wherever the caller actually reached the box, not a guessed interface address on a multi-homed box (LAN + ZeroTier). Deliberately public/unauthenticated, same reasoning as `/api/v1/health`: the plugin shows it before any credentials exist, and it encodes nothing secret
- [x] `enigma2-plugin/src/`: `plugin.py` (registration), `DreamHubScreen.py` (the launcher: QR code, backend status via `/api/v1/health`, a version-mismatch message), `DreamHubSetup.py` (the plugin's own port/auth-expected settings, reachable via the green button), `backend.py` (the two HTTP calls, over Twisted's `getPage` so a slow/unreachable backend never freezes the GUI thread), `__init__.py` (gettext wiring plus `config.plugins.dreamhub`)
- [x] Python 2.7 targeted specifically (the only interpreter on the device, confirmed `PROJECT_PLAN.md`); every module's syntax confirmed via `py_compile` against the reference box's own `/usr/bin/python2` (2.7.13) — genuinely on-device, not just locally
- [x] Language follows `config.osd.language` (format `de_DE`) via gettext, the confirmed on-device convention (`PROJECT_PLAN.md`): `PluginLanguageDomain`/`PluginLanguagePath`, `bindtextdomain()`, re-run on every `language.addCallback()`. `.po` sources for `de` and `it` committed (`en` is gettext's own source text); both validated with `msgfmt -c` — no missing/malformed entries
- [x] **Installed on the reference box 2026-09-25** (with approval) into `/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/`, then `systemctl restart enigma2`: the service came back `active (running)` within 15s, `journalctl -u enigma2 -b` has no traceback or error mentioning DreamHub anywhere in the full boot log, the pre-existing crash log (`enigma2_crash_1774122941.log`) is untouched (no new crash), and Python left behind compiled `.pyo` files for all five plugin modules next to the sources — evidence Enigma2's own boot-time plugin scan actually imported and compiled them for real, not just that they parse. This is real confirmation that `plugin.py`'s `Plugins()` registration and `__init__.py`'s gettext/config setup — the code that runs unconditionally at boot for every installed Extensions plugin — work correctly on-device
- [x] **Visually confirmed live, 2026-09-25** (with approval — the box was in standby; woke it, drove it via `/web/remotecontrol` key simulation and confirmed every step with a real screenshot via the WebInterface plugin's own `/screenshot` endpoint, discovered mid-session: `WebChilds/Screenshot.py`, native `ePixmap` capture, not the unrelated external-binary `/grab`, which really is unimplemented on this box). `DreamHub` shows correctly in Enigma2's standard Plugin Browser (`Screens.Screen` reports its own registered description, "DreamHub auf dem Handy öffnen", correctly gettext-translated to German) — it does *not* appear in this image's custom "Gemini BluePanel" quick-launcher, which turned out to use its own separate, manually-curated category list rather than the standard plugin registry; not a DreamHub bug, and not worth chasing further for a launcher plugin. Opening it renders the actual screen — title, QR code, Schließen/Einstellungen buttons — and the backend's own request log shows the real `GET /api/v1/health` and `GET /api/v1/qr.png` calls landing at the exact moment the screen opened
- [x] **Found, root-caused and fixed live**: the status text showed "backend not reachable" even though the backend log proved the request succeeded (200). The bare `except Exception: self.healthFailed(None)` in `gotHealth` was silently swallowing the real error; adding logging (`/tmp/dreamhub-plugin.log`) instead surfaced it on the very next attempt: `TypeError: in method 'eLabel_setText', argument 2 of type 'std::string const &'`. Root cause: `json.loads()` always returns Python 2 `unicode` for JSON strings, and `"%s" % a_unicode_value` promotes the whole formatted string to `unicode` too — Enigma2's SWIG-wrapped `eLabel.setText()` needs a C++ `std::string`, i.e. a plain `str`, and raises exactly this on `unicode`. gettext's own `dgettext()` returns plain `str` already (why the *static* messages, like `healthFailed`'s own text, never hit this — only text built from the backend's JSON did). Fixed with a small `_toStr()` helper (`.encode("utf-8")` for a `unicode` value) wrapping every `setText()` call that embeds backend data. **Re-verified live**: the screen now correctly shows "Backend läuft (v0.1.0-dev)" ("Backend running") on the reference box

**Open**:
- [x] A plugin icon (2026-10-04): `enigma2-plugin/src/plugin.png`, 100×40 with transparency, drawn from `enigma2-plugin/art/plugin-icon.svg` (DreamHub's dark tokens and Source Sans 3), set via `PluginDescriptor(icon=…)` and packaged. Still to check in the box's Plugin Browser (offline today)
- [ ] The `/dreamhub/timercheck` dry-run adapter (decision D7, if it goes ahead)

## v1 — release & packaging

**Done** (v1 plan WP9, `docs/V1_PLAN.md`; `packaging/`):
- [x] `packaging/build-deb.sh` produces `dreamhub_<version>_arm64.deb`: cross-compiles the backend (`CGO_ENABLED=0 GOOS=linux GOARCH=arm64`), compiles the Enigma2 plugin's `.po` files to `.mo`, assembles the file tree and runs `dpkg-deb --build`. Verified locally: `dpkg-deb --info`/`--contents` confirm correct metadata (`Depends: ffmpeg`, `Installed-Size`) and file layout; not yet installed on the reference box itself (open, below)
- [x] The backend ships as `dreamhub.service` (`packaging/dreamhub.service`: `Restart=on-failure`, `WantedBy=multi-user.target`, `After=`/`Wants=enigma2.service` since live mode talks to it), matching `/lib/systemd/system/enigma2.service`'s own shape (`ARCHITECTURE.md`)
- [x] mDNS advertisement (`packaging/dreamhub.avahi.service` → `/etc/avahi/services/dreamhub.service`), resolving the item previously open under step 7 — a phone can reach `<box>.local:8080` without knowing the IP
- [x] `postinst` creates `/data/dreamhub` (umask 700), enables/starts the service, and restarts `enigma2` so the launcher plugin shows up immediately — the same thing installing any other Extensions plugin through the Addon Manager already does. `postrm` only deletes `/data/dreamhub` on `purge`, never on a plain removal, so a reinstall picks up where the user left off
- [x] `Depends: ffmpeg` declared in the control file, per the research in `PROJECT_PLAN.md` (present via the standard feed, confirmed live)
- [x] CI (`.github/workflows/ci.yml`): backend `go vet`/`go test -race`, frontend type-check/build, then the `.deb` itself, on every push — the `.deb` is uploaded as a workflow artifact, not yet published anywhere a device could `apt` from (that's the open feed-hosting item below, decision D4)

**Open**:
- [ ] An actual apt feed to publish to (decision D4, `docs/V1_PLAN.md`) — the `.deb` itself doesn't depend on this being decided
- [x] Live install and in-place upgrade verified on the reference box (2026-09-28, ARCHITECTURE.md); upgrades only restart enigma2 when the launcher plugin changed
- [ ] Uninstall/purge on the box (not run: the box keeps DreamHub installed)
- [x] User docs (2026-10-04): [`docs/USER_GUIDE.md`](docs/USER_GUIDE.md) — installing the `.deb` by hand today (CI artifact → box → `dpkg -i` in WebAdmin's browser terminal, no SSH client needed), the feed route via WebAdmin's "Upload new `*-feed.list`" once D4 is decided, first steps (TV plugin QR code, `.local` name, setup assistant), recommended DreamHub password and `localauth=true`, the optional keys, every hub, push notifications, troubleshooting
- [x] WP10 security pass, first round (2026-09-28): **fixed** — file operations only checked paths lexically, so a symbolic link to a folder inside the recording folders ("movie/x -> /etc") would have let rename/trash/empty reach outside them; `recfs.CleanIn` now refuses any path whose folders below the recording folder aren't real folders. Writing a `.ts.meta` title refuses a meta file that isn't a regular file (a link would have copied the target's content into the recording folder), and the recording index doesn't read linked meta files. A written `.ts.meta` keeps its mode despite DreamHub's umask 077. **Confirmed by a test** — every mutating route (including the read-only POSTs added today) refuses a missing CSRF header and a foreign `Origin`; plan ownership, expiry and staleness were already covered (`internal/plan`); the TMDB image proxy accepts only TMDB's own image names and fixed sizes, so it can't be pointed elsewhere
- [ ] WP10 remaining: resource measurement on the box under real use, DST test days, a run with none of the optional plugins installed, dogfooding (D6) → 1.0.0
- [ ] If GPL codecs are ever needed: bundle a self-contained ffmpeg like `ffmpeg-mp` does under `/data/mediaportal/extra/ffmpeg/`, never replace the system one — not needed yet, nothing in v1 transcodes

## v1.1

### RadioHub
- Radio services, now playing, radio timers. Browser playback belongs in StreamHub (radio often uses MP2 audio, which browsers don't play reliably).

### ChannelHub
- Link to / embed the BouquetEditor plugin (`/bouqueteditor/*`); its write endpoints still need a write-permitted check on the device. Picons from `/data/piconpool`.

### ControlHub: "message to TV"
- `/web/message` — not yet researched or built. The rest of ControlHub (remote, screenshot, power) was pulled forward into v1; see the v1 section above.

### MediaHub trimming
- Lossless trimming via `ffmpeg -c copy` (cuts land on keyframe boundaries). `CutListEditor` is the on-screen reference for the `.ts.cuts` mechanism.

### AdminHub service status
- Status of enigma2, dreamhub, Samba and other units; restarting is a change and goes through preview-and-confirm.

## Later

### StreamHub
- No software transcoding (no hardware encoder; CPU transcoding is too heavy).
- Browsers don't play raw MPEG-TS natively — needs a library (e.g. mpegts.js) or a remux to HLS/fMP4. On top of that: AC3 audio isn't playable in HTML5 `<video>`, and some channels use HEVC, which browser support covers only partially.
- Fallback for anything the browser can't play: "open in VLC" (`.m3u` link to the native app).
- No browser-plugin embedding (NPAPI VLC and Chrome's "VXG" are dead).

### AutomationHub
- The only place where anything happens automatically — always opt-in.
- Scripts, webhooks, external notification channels (push, Telegram, ntfy, message on the TV).
- Detect state changes by polling `/web/timerlist` every 30–60 s instead of a deep Enigma2 hook.
- Separation: simple `scripts/` hooks vs. deeply integrated `plugins/` with their own settings.
- WebAdmin already has a "User Scripts" feature (`/webadmin/web/scriptlist`) — decide whether to complement or supersede it.

### AdminHub terminal
- Proxy the existing `shellinaboxd` (`127.0.0.1:4200`, used by WebAdmin at `/webadmin/terminal/`) behind DreamHub's auth and CSRF protection — no terminal of our own.
- Off by default; can only be enabled while auth is on; admin only; password re-entered before opening; warning; session start/end in the change log.

### Other
- Plex / Jellyfin / Emby connectors (optional, cached last-known state, graceful degradation when the server is off)
- More AI providers behind the same interface
- Profile/role UI (multi-user), multiple dashboards UI
- Additional languages, incl. RTL (the CSS is prepared for it)
- ~~Installable PWA~~ — done 2026-10-04, see the Foundation section

## Open items that need device access

Tracked as GitHub issues with the `device-access` label:
- [x] #10 Verify the SerienRecorder web interface API — resolved 2026-09-24: reachable, JSON, `/serienrecorderapi/*` (`ARCHITECTURE.md`)
- [x] #11 Verify Enigma2 picks up recordings renamed outside Enigma2 — resolved 2026-09-24: tested live against a real recording (all 8 sidecar files), reverted immediately after; picked up correctly with no rescan needed (`ARCHITECTURE.md`)
- [x] #12 Detect in-progress recording/playback before any file operation — resolved 2026-09-24: no dedicated "is recording" endpoint exists; cross-reference `/web/timerlist` (`e2timebegin`/`e2timeend`/`e2disabled`) against the current time, already sufficient with data DreamHub already fetches
- [x] #13 Resolve the WebControl auth discrepancy — resolved 2026-09-24: `config.plugins.Webinterface.localauth=false` bypasses auth for any LAN/private-range client regardless of `http.auth` (`ARCHITECTURE.md`)
- [x] #14 Capture real API responses as test fixtures — done 2026-09-24 directly on the device (`backend/testdata/fixtures/`)
- [x] #15 Investigate the remaining GP4 plugins (`geminiserver`, `geminicomm`, `geministreamripper`) — investigated 2026-09-24: all closed `.pyo`/`.so`, same as other GP4 plugins; file/folder names suggest `geminiserver` is the on-screen config UI for the web server ports, `geminicomm` a native media-center suite (movie/sound/pic player, file browser), `geministreamripper` an internet stream recorder with its own web UI — inferred from naming only, no readable source
- [x] #17 Check where Enigma2 stores its time zone and whether NTP is running — resolved 2026-09-24: `config.timezone.zone` (IANA name, e.g. `Europe/Berlin`); no NTP daemon found, clock is accurate regardless, likely synced from DVB broadcast time (`ARCHITECTURE.md`)
- [x] #18 Check whether WebAdmin's existing terminal (`/webadmin/terminal/`) requires a login — resolved 2026-09-24, and it's a real finding: the page itself is reachable from any LAN client with zero credentials (same `localauth=false` bypass as #13); the shell prompt inside still requires valid OS credentials, so it's exposure of a login prompt, not an open root shell (`ARCHITECTURE.md`)

Design decision #16 (home page): **tiles**, as a customizable dashboard — see `CONCEPT.md`.
