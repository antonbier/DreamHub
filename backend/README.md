# backend/

Go server (ARM64, statically compiled). REST API on port 8080, plus the web frontend embedded via `go:embed` (see [`internal/webui`](internal/webui) below).

## Running locally

No device needed — defaults to the mock WebControl client (see [`internal/webcontrol/mock`](internal/webcontrol/mock), fixtures taken directly from real responses recorded in [../ARCHITECTURE.md](../ARCHITECTURE.md)). In mock mode the SQLite database goes to `./.dreamhub-data/` (git-ignored):

```sh
go run ./cmd/dreamhub
curl http://127.0.0.1:8080/api/v1/health
curl http://127.0.0.1:8080/api/v1/settings
```

Against a real device instead (data goes to `/data/dreamhub` by default):

```sh
DREAMHUB_MODE=live DREAMHUB_WEBCONTROL_URL=http://192.168.1.x go run ./cmd/dreamhub
```

Environment variables: `DREAMHUB_MODE` (`mock`/`live`), `DREAMHUB_LISTEN_ADDR` (default `:8080`), `DREAMHUB_WEBCONTROL_URL`, `DREAMHUB_DATA_DIR`, `DREAMHUB_ENABLE_DEVICE_WRITES` (see below). Auth is **not** configured via environment — it's a setting:

```sh
curl -X PATCH http://127.0.0.1:8080/api/v1/settings -H 'X-DreamHub-Request: 1' \
  -d '{"values":{"auth.enabled":true,"auth.username":"admin","auth.password":"at-least-8-chars"}}'
```

**Every mutating request (POST, PUT, PATCH, DELETE) needs the header `X-DreamHub-Request: 1`** and must not come from a foreign `Origin` — CSRF protection, since browsers send Basic Auth credentials automatically. DreamHub sends no CORS headers, so other web pages can't add it.

**Boxes**: every request is about one box. The optional query parameter `box=<id>` names it; without it the request is about the first box (ID `main`), so clients written before multi-box support are unaffected. An unknown ID is `404 box_not_found`. A plan is applied to the box it was previewed for. See [`docs/MULTIDEVICE_PLAN.md`](../docs/MULTIDEVICE_PLAN.md).

**Host names**: requests addressed by IP, `localhost` or `*.local` are accepted; any other host name (DNS-rebinding protection) only if listed in the setting `network.allowedHosts` (comma-separated). Otherwise: `421 host_not_allowed`.

**Writes to a real box are on by default** since the timer write endpoints (`/web/timeradd`, `/web/timerdelete`) were verified live on the reference device (`ARCHITECTURE.md`, "Timer write endpoints"). Set `DREAMHUB_ENABLE_DEVICE_WRITES=0` for a read-only run; applying a plan then answers `409 writes_disabled`. Every write still goes through preview-and-confirm, is sent as a form-encoded `POST` with every parameter, and is **verified against `/web/timerlist` afterwards**, because the box reports success for a timer it silently dropped as a duplicate.

**Box login**: if the box enforces auth for DreamHub's requests (`localauth=true` together with `streamauth=true`, see `ARCHITECTURE.md`), set Settings → Services → WebControl (`webcontrol.username`, `webcontrol.password`). The same login is used for SerienRecorder's API, which sits behind WebControl. Without it the WebControl connector reports `connector.authRequired`.

Enabling auth without a username and password is rejected (it would lock everyone out). `/api/v1/health` always stays public.

## Trying a build on the box

Without packaging (until WP9), a dev build can be tried on a real box by hand. Build with the frontend embedded, copy it to `/tmp` (RAM-backed, gone after a reboot), and run it next to WebControl:

```sh
(cd ../frontend && npm run build)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o dreamhub-arm64 ./cmd/dreamhub
git checkout internal/webui/dist/index.html   # don't commit the built index
ssh root@<box> 'mkdir -p /tmp/dreamhub-test && cat > /tmp/dreamhub-test/dreamhub && chmod 755 /tmp/dreamhub-test/dreamhub' < dreamhub-arm64
ssh root@<box> 'cd /tmp/dreamhub-test && DREAMHUB_MODE=live DREAMHUB_LISTEN_ADDR=:8090 DREAMHUB_DATA_DIR=/tmp/dreamhub-test/data DREAMHUB_ENABLE_DEVICE_WRITES=0 nohup ./dreamhub > log.jsonl 2>&1 &'
```

Then open `http://<box>:8090`. Stop with `kill $(pidof dreamhub)` and remove `/tmp/dreamhub-test`. Leave out `DREAMHUB_ENABLE_DEVICE_WRITES=0` only when you mean to change things on the box.

## API so far

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/health` | public; `status`, `version`, `time` (UTC) |
| GET | `/api/v1/whoami` | protected; `{username, role}` — doubles as a credentials check, since the web UI's login screen has no browser-native prompt to rely on (only `/api/v1/*` sits behind auth, not the SPA shell) |
| GET | `/api/v1/deviceinfo` | from WebControl |
| GET | `/api/v1/media/movies?dir=` | MediaHub: one recording folder, default the box's first recording folder: `{dir, root, parent?, subfolders: [{path, name}], movies: [...], fileOps: {available, reason?}}`. `fileOps.available` is false (`reason: "notOnBox"`) when DreamHub doesn't run on the box. `400 outside_folders` for anything outside the recording folders |
| POST | `/api/v1/media/rename/plan` | `{path, newName, title?}`: preview renaming every file of a recording (`changes`: one `file.rename` per file, before → after, main file last), plus a `meta.title` change when `title` is given (the `.ts.meta` title the box's own list shows; with `newName` unchanged, only the title changes — `400 invalid_title`, `409 no_meta`). Errors: `invalid_name`, `same_name`, `target_exists`, `not_a_recording`, `outside_folders`, `409 media_in_use` (`details.reason`: `recording`/`playing`), `409 file_ops_unavailable` |
| POST | `/api/v1/media/rename/batch/plan` | `{paths: [...] (1–200), source: "metadata"\|"tmdb", folders?: bool}`: preview renaming many recordings by the naming scheme (`media.naming.movie`/`.episode`); with `folders`, also sorting them into folders by the folder schemes (`media.naming.folderMovie`/`.folderEpisode`). `{plan?, items: [{path, before, after, folder?, files, fields, status: rename\|unchanged\|skipped, reason}], folders?: [new folders]}` — no `plan` when nothing would change. Apply is all or nothing |
| POST | `/api/v1/media/covers/plan` | `{paths: [...] (1–200)}`: preview saving covers from enrichment next to recordings (`<base>.jpg`/`.png`, `<base>.backdrop.jpg`). `{plan?, items: [{path, title, status: write\|has_cover\|skipped, reason?, files?, poster?}]}`. Never replaces an existing file; needs a TMDB key (409 `tmdb_not_configured`). Apply is all or nothing |
| POST | `/api/v1/media/adscan` | `{paths: [...] (1–200)}`: queue ad detection (`internal/adscan`, #44) for recordings, one background job each (`media.adscan`, `GET /api/v1/jobs`). `202 {items: [{path, jobId?, existing?, reason?}]}` — `existing` when that recording is already queued or running, `reason` (`not_a_recording`, `media_in_use`) when it wasn't queued. Changes nothing: the result is a suggestion. `409 adscan_unavailable` without FFmpeg or off the box; synthetic frames in mock mode |
| GET | `/api/v1/media/adscan?path=` | a recording's last analysis: `{path, serviceRef, analyzedAt, result: {duration, blocks: [{start, end, uncertain?}], probes, weakLogo?, learned?}}` (seconds from the recording's start). `404 adscan_not_analysed` |
| POST | `/api/v1/media/trim/plan` | `{path, blocks: [{start, end}]}` (seconds, from the recording's last analysis): preview cutting those breaks out losslessly (`internal/trim`, #27). `{plan, keep}`; the plan's changes are `recording.trim` (duration and size before/after) and `recording.trash` (the original, renamed `<name> - original`). Applying queues a `media.trim` job: FFmpeg's concat demuxer writes the kept parts to a hidden file next to the recording, then the original's files are renamed, `.ts.meta`/`.eit`/cover copied to the cut version, the cut file takes the original name and the original goes to the trash (its `.ap`/`.sc`/`.cuts` stay with it). `409 adscan_not_analysed`, `trim_unavailable`, `trim_not_enough_space` (free space checked on the box), `trim_original_exists`; `400 trim_invalid_blocks` |
| POST | `/api/v1/media/rename/suggest` | `{path, source}` with `source` = `metadata` \| `tmdb` \| `ai`: name suggestions `{suggestions: [{name, source, fields, kind?}]}`. Changes nothing. Every source yields only fields (`internal/naming` builds the name). `409 tmdb_not_configured` / `ai_not_configured` without a key, `429 ai_budget_exceeded` (the AI counts against `ai.dailyLimit`), `502 tmdb_unauthorized` / `tmdb_unavailable` / `ai_unavailable` |
| GET | `/api/v1/media/cover?path=&kind=poster\|backdrop` | a recording's own cover art already next to it on the box (`<base>.jpg`/`.png`, `<base>.backdrop.jpg`, written by another plugin) — read-only, regular JPEG/PNG files only; the folder listing flags `cover`/`backdrop` per recording when DreamHub runs on the box. `404` otherwise |
| POST | `/api/v1/media/trash/plan` | `{paths: [...]}` (1–200): preview moving recordings to the trash (`<base>.ts` → `<base>.ts.trash`, AdvancedMovieSelection's convention) |
| GET | `/api/v1/media/trash` | trashed recordings below the recording folders: `{items: [{path, originalPath, title, size, modified}]}` |
| POST | `/api/v1/media/trash/restore/plan` | `{paths}`: preview restoring (`409 restore_blocked` if the original name is taken again) |
| POST | `/api/v1/media/trash/empty/plan` | `{paths}`: preview deleting trashed recordings for good. Lists every file and returns `bytes`. MediaHub's only real delete |

MediaHub file changes are applied only when DreamHub runs on the box and device writes are on. With `DREAMHUB_ENABLE_DEVICE_WRITES=0`, previews still work and applying answers `409 writes_disabled`. Every apply re-checks that the file isn't being recorded or played.
| GET | `/api/v1/settings` | all sections with effective values; secrets masked, passwords never returned (`hasValue` only). Each section names its `tab` in the settings menu (`general`, `recordings`, `services`, `notifications`, `hubs`, `access`; `settings.Tab*`) |
| PATCH | `/api/v1/settings` | `{"values": {"<key>": <value or null>}}` — `null` resets to default. All-or-nothing; on failure `400 validation_failed` with a code per field in `error.details` |
| GET | `/api/v1/navigation` | the sidebar for the current user: sections (`main`, `media`, `program`, `device`, `bottom`), each hub with `name` (MediaHub…), `label` (i18n key), `status` and `problems`; disabled hubs and hubs the role can't see are omitted; `labels` is the user's `descriptive`/`hub` choice |
| GET | `/api/v1/hubs` | diagnostics: every hub the user may see, including disabled ones, with status, problems, requirements and `emits` — the event keys from its area (`internal/events/keys.go`) |
| GET | `/api/v1/jobs` | background jobs (`internal/jobs`, #41): the profile's own, everyone's with `system`; active first, then the last finished ones. `{jobs: [{id, kind, key?, ownerId, state: queued\|running\|waiting\|done\|failed\|canceled, done, total, message?, messageParams?, error?, createdAt, startedAt?, finishedAt?}], blockedBy}` — `message`/`error` are i18n keys, `blockedBy` why the queue is held (`recording`, `boxUnreachable`) or `""`. Changes also arrive live on `/api/v1/events/stream` as `event: job` |
| GET | `/api/v1/jobs/{id}` | one job; `404 job_not_found` for unknown jobs and other profiles' alike |
| POST | `/api/v1/jobs/{id}/cancel` | stops a job (owner or `system`): a queued one at once, a running one at its next check. `409 job_finished` when it already ended. Only stops work — what a job changed went through its own plan |
| GET | `/api/v1/connectors` | state of each connector (`ok`, `not_configured`, `unreachable`, `not_implemented`) with problem and fix; never contains secrets |
| GET | `/api/v1/dashboard` | the current user's dashboard layout: `{"tiles": [{"type", "label", "width", "hidden"}, ...]}` plus custom tiles (`{"type": "custom", "id", "width", "hidden", "custom": {"title", "source", "query", "display"}}`, up to 12; `PUT` assigns a missing `id`), reconciled against the built-in tile catalog (`internal/dashboard`) — a type missing from the saved layout is appended with its default width, an unknown/removed type is dropped |
| PUT | `/api/v1/dashboard` | saves the layout directly (no preview/apply — DreamHub's own state, not a device change; still recorded in the change log). Array order is display order. `400 invalid_tile` for an unknown type, a duplicate, or a width outside 1–4 |
| POST | `/api/v1/enrich/lookup` | `{items: [{title, kind?}]}` (≤ 40) → `{enabled, results: [Info \| null]}` in the same order: poster/backdrop path, rating, votes, genres, overview, year from TMDB (`internal/enrich`). Only exact title matches; cached. `enabled: false` (all null, not an error) without a TMDB key or with `tmdb.enrich` off. Read-only despite the POST |
| GET | `/api/v1/enrich/image/{size}/{file}` | a TMDB poster/backdrop (`size` w92/w185/w342/w780), served from DreamHub's disk cache or fetched once — the browser never talks to TMDB. `404` for anything that isn't a TMDB image name |
| POST | `/api/v1/push/test` | sends a test notification through ntfy with the saved settings — `{sent: true}`, `409 push_not_configured` without a topic, `502 push_failed` when the server can't be reached |
| GET | `/api/v1/enrich/extras?kind=movie\|tv&id=<TMDB ID>` | what further databases add to one title, for a detail view: `{extras: {imdbId, imdb, imdbVotes, rottenTomatoes, metascore, logo} \| null}` — ratings from OMDb (by the IMDb ID TMDB supplies), the logo from fanart.tv (films by TMDB ID, series by TheTVDB ID). `null` without an OMDb or fanart.tv key. Cached a week (a miss a day) |
| GET | `/api/v1/enrich/logo?path=` | one fanart.tv logo (a path from `extras.logo`, fanart.tv's own paths only), from the disk cache or fetched once |
| GET | `/api/v1/dashboard/sources` | view sources for custom tiles the user can use right now (`internal/hub/view.go`): `{sources: [{id, label, hubId, filters, sorts}]}` |
| POST | `/api/v1/dashboard/view` | evaluates a view — `{source, query, timeZone}` → `{items: [{title, subtitle, time, end, size, warn, path}], total, totalSize}`. Read-only despite the POST. `404 unknown_source`, `409 source_unavailable` (the hub is disabled/unavailable), `502 source_failed` |
| GET | `/api/v1/search?q=` | queries every hub's search provider (`internal/hub`, the hub contract) the current user can currently see — disabled, invisible-to-role or unavailable hubs are skipped, a provider error is skipped rather than failing the request. `{"results": [{"hubId", "hubLabel", "title", "subtitle", "path"}, ...]}`, grouped by hub in navigation order. Empty or missing `q` returns no results, not an error |
| GET | `/api/v1/tokens` | the current user's API tokens (never the secret): `{"tokens": [{"id", "name", "createdAt"}, ...]}` |
| POST | `/api/v1/tokens` | `{"name": "..."}` → creates a token and returns its secret **once**: `{"id", "name", "createdAt", "token"}`. `400 validation_failed` for an empty name |
| DELETE | `/api/v1/tokens/{id}` | revokes a token; `404 token_not_found` if it doesn't exist or belongs to another user (not distinguished, so one user can't probe another's IDs) |
| POST | `/api/v1/plans/{id}/apply` | executes a previewed plan (second step of every change). `404 plan_not_found` (unknown, expired after 10 min, or already used), `403 plan_forbidden` (other user), `409 plan_stale` (state changed since the preview — nothing done), `409 writes_disabled` |
| GET | `/api/v1/timers` | TimerHub: timers and the number of tuners. Each timer also carries `source` (`serienrecorder`/`autotimer`, from its tags; absent when unknown) and `conflict` (likely clashes with another timer, same tuner estimate as the plan warning) |
| POST | `/api/v1/timers/plan` | TimerHub: preview a new timer — `{serviceReference, serviceName, name, description?, begin, end, marginBefore?, marginAfter?, eventId?}` (Unix seconds; margins in minutes, default 5/10). Returns a plan with the recording window and warnings: `timer.conflict` (an **estimate**, transponder- and delivery-system-aware via `internal/tuner`; the box checks exactly on apply) and `timer.duplicate` (the box's own rule: same channel, same begin). Changes nothing. Apply can fail with `409 box_rejected` (the box refused, `details.boxText` is its own translated message) or `409 timer_not_created` (the box said yes but the timer isn't there) |
| GET | `/api/v1/timers/autotimers` | TimerHub: the optional AutoTimer plugin's configuration, read-only — `{"installed": false}` on a box without it (a normal answer, not an error), else `{"installed": true, "autoTimers": [...], "editorPath": "/autotimereditor/"}` |
| GET | `/api/v1/timers/autotimers/simulate` | TimerHub: the plugin's own dry run — the timers the AutoTimers would create now. Changes nothing on the box. `404 autotimer_not_installed` without the plugin |
| POST | `/api/v1/timers/delete/plan` | TimerHub: preview deleting a timer — `{serviceReference, begin, end}` exactly as `GET /api/v1/timers` lists it (`404 timer_not_found` otherwise). Warns `timer.deleteRunning` when the timer is recording right now: the box aborts the recording on delete |
| POST | `/api/v1/timers/toggle/plan` | `{serviceReference, begin, end, enabled}` as listed: preview disabling (`enabled: false`) or enabling a timer — done by taking it out and putting it back with only `disabled` changed (not `/web/timerchange`, decision D5); enabling carries the conflict estimate. `409 timer_toggle_running` while it records; on apply, `timer_toggle_restored` when the box refused and the original was put back |
| GET | `/api/v1/changes?limit=` | SystemHub: change log, newest first. Includes system-detected entries (no `userId`) alongside user actions — currently just `hub.statusChanged` when a hub's status actually transitions (`internal/hub/watcher.go`), noticed the next time navigation/hub status happens to be computed, not via a background poller |
| GET | `/api/v1/discovery/recommendations` | DiscoveryHub: the user's last generated set, kept server-side — `{saved: {bouquet, generatedAt, taste?, recommendations: [{…, hasTimer}]} \| null}`; broadcasts that ended are left out, `hasTimer` says whether a timer covers one now. No AI call. `POST` replaces it |
| GET | `/api/v1/discovery/taste` | DiscoveryHub: the taste profile `POST /api/v1/discovery/recommendations` sends with `useTaste: true` — `{recordings, series, timers, autoTimers}` (titles only, each list always present). Sends nothing itself; for showing the user what would be sent |
| GET | `/api/v1/guide/bouquets` | GuideHub: the box's bouquets |
| GET | `/api/v1/guide/grid?bouquet=&start=&end=[&service=]` | GuideHub: every programme starting in `[start, end)` (Unix seconds), grouped per channel — `{channels: [{serviceReference, serviceName, events}]}`; `service` narrows it to one channel (the Channel view) |
| GET | `/api/v1/guide/nownext?bouquet=` | GuideHub: now and next for every channel of the bouquet in one call (`/web/epgnownext`) — `{channels: [{serviceReference, serviceName, now, next}]}`, `null` where the box has no EPG |
| GET | `/api/v1/guide/picon?ref=&name=` | GuideHub: a channel's logo from the box's picon folders (`/data/piconpool`, `/usr/share/enigma2/picon`, …), by service reference or channel name; real PNGs only, `404` when there's none |
| GET | `/api/v1/radio/bouquets` | RadioHub: the box's radio bouquets (`Services(webcontrol.RadioRoot)`), headings dropped |
| GET | `/api/v1/radio/stations?bouquet=` | RadioHub: every station of the bouquet in its order with now/next (`null` without EPG) and `playing` — the reference the box plays — `{stations: [{serviceReference, serviceName, now, next}], playing}` |
| GET | `/api/v1/radio/timers` | RadioHub: timers on radio services that haven't ended, soonest first — `{timers: [...]}` |
| GET | `/api/v1/radio/picon?ref=&name=` | RadioHub: a station's logo, same lookup as GuideHub's |
| GET | `/api/v1/stream/bouquets` | StreamHub: the TV bouquets, headings dropped |
| GET | `/api/v1/stream/channels?bouquet=` | StreamHub: the bouquet's channels with now/next and `playing`, as RadioHub's stations |
| GET | `/api/v1/stream/picon?ref=&name=` | StreamHub: a channel's logo |
| GET | `/api/v1/stream/live?ref=` | StreamHub: the box's own stream of a channel (port 8001), passed through unchanged as `video/mp2t`. Only broadcast references (`1:0:…`); waits for the first data and answers `503 stream_no_tuner` when none comes (no tuner free); at most two at once (`429 stream_busy`) |
| GET/HEAD | `/api/v1/stream/recording?path=` | StreamHub: a recording (`.ts` below a recording folder) with `Range` for seeking — read from disk on the box (`recfs.Opener`), else through WebControl's `/file` |
| GET | `/api/v1/series/markers/{id}` | SeriesHub: one tracked series — `{marker, upcoming, recordings, broadcasts, episodes, unavailable}`. `episodes` is the TVmaze episode guide: `{state: ok\|off\|noMatch\|failed, matchable, seasons: [{season, episodes: [{number, name, airdate, status: recorded\|planned\|upcoming\|missing\|aired}], recorded, planned, missing, upcoming}]}`. `recordings` are found in the marker's record folder and a series-named subfolder (title match or folder match, `season`/`episode` from an `S01E02` code, 0 if none); `broadcasts` are the box's EPG search results that really are this series and haven't ended, with `hasTimer`. A part whose source failed is listed in `unavailable` and left empty. `404 series_not_found` |
| POST | `/api/v1/control/message` | ControlHub: show a text on the TV — `{text, kind: "info"\|"warning"\|"error", timeout}` (seconds, 0 = until closed). Direct like the remote keys, gated by device writes. `400 invalid_message` |
| GET | `/api/v1/control/current` | ControlHub: what the box shows right now (`/web/getcurrent`) — `{serviceReference, serviceName, providerName, kind: "tv"\|"radio"\|"recording", now, next, volume, muted, inStandby}`; `now`/`next` `null` without EPG. Backs the dashboard's "Now on the box" tile |
| GET/PUT | `/api/v1/admin/log-level` | AdminHub (admin only): `{"level": "debug", "minutes": 60}` — debug reverts to info automatically |
| GET | `/api/v1/admin/services` | AdminHub (admin only): the box's services (`hubs.DefaultServices`: enigma2, dreamhub, Samba, NFS, avahi, ZeroTier — ones the box lacks left out) via `systemctl show` — `{services: [{name, description, active, sub, since, mainPid, memory, restarts}]}`; `503 systemd_unavailable` without systemd |
| GET/POST | `/api/v1/system/selftest` | SystemHub (admin only): the box self-test (`internal/hubs/selftest.go`). `POST` asks the box once, read-only and one question after another — WebControl (device, TV bouquets and EPG, what it shows, the radio root and the first radio bouquet, timers, recording folders), SerienRecorder, AutoTimer, picon folders and names, `systemctl show` for the service list, DreamMediaInfo — and rates each `ok`/`warn`/`fail`/`skip` with a detail code; `GET` returns the last report (`null` before the first run). The raw answers are kept in `<data>/selftest/` as fixtures (the real client only); `409 selftest_running` |
| GET | `/api/v1/system/selftest/download` | SystemHub (admin only): the last run's `report.json` and captured answers as one zip |
| POST | `/api/v1/admin/services/plan` | AdminHub (admin only): preview restarting (`service.restart`) or, when it's down, starting (`service.start`) one of those units — `{name}`. Warnings: `service.restartEnigma2`, `service.recordingsInterrupted` (`names` of recordings running now), `service.restartSelf`. Applied through `/api/v1/plans/{id}/apply` (`systemctl restart --no-block`), stale if the unit changed in between, refused with `writes_disabled` under `DREAMHUB_ENABLE_DEVICE_WRITES=0` |
| GET | `/api/v1/plex/status` | PlexHub: `{configured, canPlay, online, error: ""\|unreachable\|unauthorized, server: {name, version}, syncedAt, syncing, url}` — never the token |
| POST | `/api/v1/plex/sync` | reads libraries, the shown libraries' items and "continue watching" into the cache now; answers like status. `502 plex_offline` / `plex_unauthorized` |
| GET | `/api/v1/plex/libraries` | film and series libraries `{libraries: [{id, title, type, count, selected}], meta: {online, syncedAt}}`; from the server when it answers, else the cache. `PUT {selected: [ids]}` chooses which are shown (stored as `plex.libraries`, empty = all) |
| GET | `/api/v1/plex/libraries/{id}/items` | a library's films or series `{items: [Item + watched], meta}` |
| GET | `/api/v1/plex/items/{key}` | one item with its media versions; a series lists its seasons, a season its episodes (`children`) |
| GET | `/api/v1/plex/ondeck` | Plex's "continue watching" |
| GET | `/api/v1/plex/image?path=&w=` | an item's own picture (`/library/metadata/<id>/thumb\|art…` only; `w` 120/240/360/720/1280), scaled by Plex, kept in DreamHub's disk cache |
| POST | `/api/v1/plex/items/{key}/watched` | `{watched}`: one film or episode, directly (logged); `400 plex_needs_preview` for a series/season, `409 plex_offline` |
| POST | `/api/v1/plex/items/{key}/watched/plan` | `{watched}`: preview for a series or season — one `plex.watched` change per episode that changes; `409 plex_nothing_to_change` |
| POST | `/api/v1/plex/items/{key}/play` | plays a film/episode on the box with its own player through `GET /api/v1/plex/stream/{ticket}/{name}` — public, loopback only, ticket valid 12 h. `409 plex_play_unavailable` when DreamHub doesn't run on the box or it's switched off |
| POST | `/api/v1/plex/login` | starts the plex.tv sign-in: `{id, code, expiresAt, linkUrl}`; `GET /api/v1/plex/login/{id}` → `{state: waiting\|linked\|expired, servers?}`; `POST …/{id}/use {serverId, url}` checks the server answers there and saves `plex.url`/`plex.token` |

Authentication accepts **either** Basic Auth or a bearer API token (`Authorization: Bearer dh_...`, from `POST /api/v1/tokens`) on any endpoint, whenever `auth.enabled` is on.

Hub routes answer `404 hub_disabled` when the hub is switched off in settings.

Every response carries an `X-Request-ID` header; error bodies repeat it as `error.requestId`, and every log line includes it.

Problems are `{"code", "params", "fix", "fixParams"}` — all i18n keys plus parameters for the frontend to translate (e.g. `connector.notConfigured` with fix `fix.configureInSettings` and `{"section": "ai"}`).

Connector states are `ok`, `not_configured`, `unreachable`, `not_implemented` and `not_installed`. The last one is for optional plugins: SerienRecorder's API answering `404` means the plugin isn't installed, which is a normal state (`connector.notInstalled`, fix `fix.installPlugin`), not a connection problem. SeriesHub then shows as unavailable with that explanation.

Errors always use `{"error": {"code": ..., "message": ..., "details": ...}}`. Codes are for clients to translate; `message` is an English developer hint.

## Frontend

`npm run build` in [`../frontend`](../frontend) builds straight into `internal/webui/dist` (see `frontend/vite.config.ts`). A minimal placeholder `index.html` is tracked there so `go build`/`go run`/`go test` all work without Node installed; building the real frontend overwrites that file locally (don't commit the overwrite — see `.gitignore`). `go run ./cmd/dreamhub` then serves it at `/` with an SPA fallback (`internal/webui/spa.go`): any path that isn't a real static file and doesn't look like an asset request (no dot in the last path segment) gets `index.html` instead, so vue-router's client-side routes work on a direct load or a refresh; a genuinely missing asset still 404s.

For frontend development against a running backend, run `go run ./cmd/dreamhub` (port 8080) and `npm run dev` in `frontend/` (port 5173) separately — the dev server proxies `/api` to 8080 (see `frontend/vite.config.ts`) and gives you hot reload.

## Testing

```sh
go test ./...
```

All tests run against the mock client and a real SQLite database in a temp directory — no physical Dreambox required, per [../PRINCIPLES.md](../PRINCIPLES.md)'s "Mock-first development" and "Testable without the physical device".

When testing a running server by hand, prefer a built binary over `go run`: `go run` starts a child process that isn't stopped when you kill `go run` itself.

## Cross-compiling for the device

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o dreamhub-arm64 ./cmd/dreamhub
```

Produces a static ARM64 Linux binary (~12–13 MB stripped, including the pure-Go SQLite driver and the embedded frontend) — no libc/runtime dependencies on the target device.

## Layout

- `cmd/dreamhub/` — entrypoint: reads config, picks the WebControl client (mock or live), starts the app.
- `internal/app/` — the wiring: store → connectors → hubs → settings schema (core + connector sections + hub switches) → auth → API router. Used by `cmd/dreamhub` and the API tests, so tests run the production wiring.
- `internal/connector/` — connector registry with cached, time-limited checks, and the built-in connectors (WebControl, SerienRecorder, AutoTimer, OpenAI, TMDB, TVmaze, OMDb, fanart.tv, ntfy).
- `internal/hub/` — hub registry: descriptors, requirements, status evaluation, navigation, and the hub contract's search providers (`search.go`) and status-change detection (`watcher.go`, feeds `GET /api/v1/changes`). `v1.go` declares the v1 hubs and device capabilities.
- `internal/status/` — the shared "available / degraded / unavailable" levels and problem/fix codes.
- `internal/api/` — the core `/api/v1` routes and the assembly of hub routes behind auth and each hub's enable switch.
- `internal/httpx/` — shared HTTP conventions: error envelope, request IDs + request logging, user in context, body decoding, CSRF and Host-header middleware.
- `internal/plan/` — preview-and-confirm: single-use, expiring, owner-bound plans with stale detection; applied plans go to the change log.
- `internal/dashboard/` — the dashboard layout (`GET`/`PUT /api/v1/dashboard`): custom tiles (saved views over a hub's view source, `internal/hub/view.go`) alongside a fixed catalog of built-in tile types (each backed by data an existing endpoint already provides, e.g. TimerHub for "upcoming timers"), reconciled on read/write so a removed type is dropped and a newly added one always shows up. Saved directly, no plan/apply — same exception as settings. Tiles can need a permission ("Needs attention" is admin-only); the viewers' start page is a template an admin designs (`GET`/`PUT /api/v1/dashboard/template`), shown to a viewer until they save their own; `DELETE /api/v1/dashboard` goes back to it.
- `internal/hubs/` — hub behavior (routes): TimerHub (timers, create via plan with conflict check), SystemHub (change log), AdminHub (log level).
- `internal/enrich/` — enrichment: poster, rating, overview, genres for a title from TMDB (first source), exact-title matches only, cached in SQLite (`enrichment` table) and images on disk (`<data dir>/cache/tmdb`, capped). `internal/titles/` — the shared title normalization and "is this an episode of that series" matching.
- `internal/plex/` — the Plex connector's client (+ `mock/`): a Plex Media Server's libraries, items, watched state, pictures and files, and plex.tv's PIN sign-in and server list. PlexHub (`internal/hubs/plex*.go`) caches what it reads in the store (`plex_cache`).
- `internal/push/` — push notifications through ntfy: the client (+ `mock/`), the texts per event in de/en/it (`messages.go`), and the `Notifier` that forwards bus events according to Settings → Notifications.
- `internal/jobs/` — the background queue (#41): hubs register job kinds (`Kind`, `Run`) and enqueue jobs (`Enqueue`, one per kind and key at a time); jobs are persisted (`jobs` table), run one at a time by default, report progress (stored and sent at most once a second), save checkpoints, and wait at safe points (`Run.Wait`) while a blocker holds the queue — in `internal/app`, any running recording, or a box whose timer list can't be read. After a restart, resumable kinds continue from their checkpoint, others are marked `interrupted`. Finished jobs are pruned (200, 30 days).
- `internal/compose/` — the compose subset (docs/APPSHUB_PLAN.md, App12): `Parse` reads a `docker-compose.yml` with the `x-dreamhub` extension into services, form `Vars`/`Fields`, notes and refusals (`Issues` with stable codes); `File.Resolve(values)` applies `${VAR:-default}` values and types everything (ports, volumes, memory, elevated settings). Pure — no network, files or Docker.
- `internal/appcatalog/` — the curated list as embedded `apps/*.yml` compose files, and `Resolve`, the one place a compose file plus form values becomes an `App` (also for an admin's own container). `legacy_test.go` freezes the former Go data; `equivalence_test.go` proves every resolved entry equals it.
- `internal/unraid/` — pure conversion of an Unraid `<Container version="2">` template into a compose file (Port/Path/Variable entries become fields; Device, Label and ExtraParams are left out with a note).
- `internal/dockerd/validate.go` — the helper's mount, device and elevated-confirmation rules (`ValidateSpec`, `MountPolicy`), enforced in the helper's HTTP surface and at the socket.
- `internal/pkginfo/` — package information without an AI (docs/APPSHUB_PLAN.md, App11): `entries.json` (embedded, de/en/it descriptions, optional category and `neededBy` hub ids; deliberately incomplete until the box feed is read), category ids and labels with the name-convention default (`category.go`), `Resolve` (curated → the package's own apt Description → none). `cmd/pkginfo-missing` lists the packages of a `Packages` index without an entry.
- `internal/adscan/` — ad detection by the channel logo (docs/ADSCAN_PLAN.md, #44): logo masks learned from keyframes (`mask.go`, edge direction per pixel), coarse sampling with bisection (`sampler.go`), the analysis with reuse/relearning of a channel's mask and "uncertain" blocks (`analyze.go`), frames from the system FFmpeg fed 1.5 MB slices of the `.ts` (`ffmpeg.go`, seeking through `.ts.ap`) or synthetic ones in mock mode (`synthetic.go`), and the `.ts.ap`/`.ts.cuts` formats with a cut-list merge that keeps marks, the resume position and hand-made cuts (`sidecar.go`, not used for writing until the format is confirmed on the box). Results follow their recording through rename, trash and restore and are dropped when it is deleted for good. MediaHub runs it as the `media.adscan` job (`internal/hubs/media_adscan.go`); masks per service reference and results per path are in `adscan_logos` / `adscan_results`. `ADSCAN_SAMPLE=<copied recording.ts> go test -run RealSample -v ./internal/adscan` runs it on a real recording.
- `internal/trim/` — lossless cutting (#27): `Keep` turns breaks into the parts to keep (merging overlaps, dropping slivers under 2 s), `FFmpeg` cuts with the concat demuxer (in/out points on the stream's own clock, from `adscan.FFmpeg.Start`; the last part runs to the file's end; video, all audio, subtitles and teletext copied; progress from bytes written), `Mock` writes a stand-in in mock mode. `ADSCAN_SAMPLE=<recording.ts> go test -run RealSample ./internal/trim` cuts a real file.
- `internal/omdb/`, `internal/fanart/` — the OMDb (Rotten Tomatoes, IMDb, Metascore by IMDb ID) and fanart.tv (title logos) clients, each with a `mock/`; used only through `enrich.Service.Extras`.
- `internal/logging/` — runtime log level with automatic revert from debug.
- `internal/auth/` — auth middleware; `SettingsVerifier` checks Basic Auth against the credentials in settings (bcrypt, cached) or a bearer token against `internal/apitoken` (`TokenVerifier`); `Credentials` is a static verifier for tests.
- `internal/perm/` — permissions (docs/archive/PROFILES_PLAN.md): the roles' permission sets, the per-profile switches, `For(user)`. Code asks for a permission, never for a role.
- `internal/profile/` — profiles: create/change/delete with their invariants (an admin always remains; admins need a PIN once there's a second profile), PINs (bcrypt, lockout after wrong tries), device sessions (`X-DreamHub-Profile`). `GET /api/v1/profiles` and `POST /api/v1/profiles/{id}/session` sit behind the door but before a profile; managing is `POST`/`PATCH`/`DELETE /api/v1/profiles…`, the own PIN `PUT /api/v1/profiles/me/pin`.
- `internal/api/identity.go` — who a request is (API token owner, the device's profile session, or the only profile without PIN) and the permission check every route is registered through (`guardMux`): hubs declare their changing routes' permissions in `Perms()` next to `Mount`; an undeclared changing route is admin-only.
- `internal/hubs/channels.go` — ChannelHub (docs/CHANNELHUB_PLAN.md): bouquets and entries, the channel snapshot (`store/channels.go`), the channel check, picon sets; `GET /api/v1/channels/{bouquets,bouquet,all,check,picons,picon}`, `PUT /api/v1/channels/picons`. Admin only (`channels.manage`).
- `internal/apitoken/` — API tokens: generate (returns the secret once), list, revoke, verify (SHA-256 lookup — no per-request password hashing like Basic Auth's bcrypt, since a random token doesn't need that). `GET`/`POST /api/v1/tokens`, `DELETE /api/v1/tokens/{id}`.
- `internal/config/` — environment-based configuration.
- `internal/store/` — SQLite: users and roles, settings (global and per user), dashboards (multiple per user in the model), change log, API tokens (hash only). Migrations are appended to `migrations` in `store.go` and never edited once applied.
- `internal/settings/` — the settings schema (`schema.go`), the core sections (`core.go`: general, auth, network), and the service that validates, stores, masks and logs (`service.go`). Hubs and connectors register their own sections alongside these (see `internal/app`'s wiring).
- `internal/webcontrol/` — the WebControl `/web/*` API client (real HTTP + XML parsing) and its `Client` interface.
- `internal/webcontrol/mock/` — the mock implementation of that interface, used by default and by all tests. Stateful timers: two fixture timers that overlap (a real conflict on the single-tuner box), and `AddTimer` is visible in later `TimerList` calls.
- `internal/webui/` — embeds the built frontend (`go:embed dist`) and serves it with an SPA fallback; see "Frontend" above.

Run `go test -race ./...` before committing changes that touch concurrency (plans, connector cache, log level).
