// Mirrors backend/internal/hub, /internal/status and /internal/settings JSON
// shapes. Kept hand-written and small rather than generated — see
// ../../README.md for why (no build-time dependency on the Go backend).

export type StatusLevel = 'available' | 'degraded' | 'unavailable'

export interface Problem {
  code: string
  params?: Record<string, unknown>
  fix?: string
  fixParams?: Record<string, unknown>
}

export interface NavItem {
  id: string
  kind: 'core' | 'hub'
  name?: string
  label: string
  icon: string
  path: string
  status?: StatusLevel
  problems?: Problem[]
}

export interface NavSection {
  id: string
  label?: string
  items: NavItem[]
}

export interface NavBox {
  id: string
  // DreamHub runs on that box; otherwise only its web interface is used.
  full: boolean
  capabilities: string[]
}

export interface Navigation {
  labels: 'descriptive' | 'hub'
  sections: NavSection[]
  box?: NavBox
}

export type SettingType = 'string' | 'bool' | 'int' | 'enum' | 'timezone' | 'secret' | 'password'
export type SettingScope = 'global' | 'user'

export interface SettingField {
  key: string
  type: SettingType
  scope: SettingScope
  label: string
  default: unknown
  options?: string[]
  min?: number
  max?: number
  minLength?: number
  // Edited by a panel of its own (or by nobody), not the generic form.
  hidden?: boolean
  value: unknown
  hasValue: boolean
  isDefault: boolean
}

export interface SettingSection {
  id: string
  label: string
  // The settings tab it shows on (src/lib/settingsTabs.ts).
  tab: string
  fields: SettingField[]
}

export interface SettingsView {
  sections: SettingSection[]
}

export interface HealthInfo {
  status: string
  version: string
  time: string // RFC3339, UTC
  // The box's own IANA zone (e.g. "Europe/Berlin"), derived from
  // /etc/localtime; empty if it couldn't be resolved. Backs the per-user
  // "box" display-zone mode (ROADMAP.md, "Time zones").
  serverTimeZone: string
}

// What a profile may do (backend/internal/perm).
export type Permission =
  | 'watch'
  | 'control'
  | 'plex.use'
  | 'discovery'
  | 'timers.plan'
  | 'timers.manage'
  | 'recordings.manage'
  | 'settings.global'
  | 'system'
  | 'profiles'
  | 'channels.manage'

// 'user' is the viewer role ("Zuschauer").
export type Role = 'admin' | 'user'

// GET /api/v1/whoami.
export interface WhoAmI {
  id: number
  username: string
  role: Role
  color: string
  hasPin: boolean
  permissions: Permission[]
  // Whether this household picks profiles at all.
  profiles: boolean
}

// backend/internal/profile.Profile.
export interface Profile {
  id: number
  name: string
  role: Role
  color: string
  hasPin: boolean
  // The admin's per-profile switches (perm.Switchable): on/off.
  overrides: Partial<Record<Permission, boolean>>
  permissions: Permission[]
  // The boxes the profile may use; empty means all.
  boxes: string[]
}

// GET /api/v1/profiles — the picker.
export interface ProfileList {
  profiles: Profile[]
  pickerNeeded: boolean
  current: number
  colors: string[]
}

// The fields to create or change; pin "" removes the PIN.
export interface ProfileInput {
  name?: string
  role?: Role
  color?: string
  pin?: string
  overrides?: Partial<Record<Permission, boolean>>
  boxes?: string[]
}

// GET /api/v1/boxes (docs/MULTIDEVICE_PLAN.md, M3). The address and login
// name come for an administrator only; the password never.
export interface BoxInfo {
  id: string
  name: string
  color: string
  // DreamHub runs on this box ("Full"); every other box is Basic.
  full: boolean
  online: boolean
  // The caller's profile may use it (an administrator also sees the rest).
  allowed: boolean
  url?: string
  username?: string
  hasPassword?: boolean
}

export interface BoxInput {
  name?: string
  color?: string
  url?: string
  username?: string
  password?: string
}

// GET /api/v1/boxes/overview — one line per box for the dashboard's strip.
export interface BoxStatus {
  id: string
  online: boolean
  standby?: boolean
  service?: string
  recording?: boolean
}

// backend/internal/webcontrol.Frontend / .DeviceInfo.
export interface Frontend {
  name: string
  model: string
}

export interface DeviceInfo {
  enigmaVersion: string
  imageVersion: string
  webifVersion: string
  deviceName: string
  frontends: Frontend[]
}

// A field-level validation error from a rejected PATCH /api/v1/settings —
// backend/internal/settings.FieldError.
export interface FieldError {
  code: string
  params?: Record<string, unknown>
}

// backend/internal/hub.Requirement.
export interface Requirement {
  connector?: string
  capability?: string
  hub?: string
  optional?: boolean
}

// backend/internal/hub.Status — the diagnostics view from GET /api/v1/hubs
// (every hub the user may see, including disabled ones).
export interface HubStatus {
  id: string
  name: string
  label: string
  icon: string
  path: string
  group: string
  role: 'admin' | 'user'
  enabled: boolean
  status: StatusLevel
  problems: Problem[]
  requires: Requirement[]
}

// backend/internal/hubs.TimerView (webcontrol.Timer plus what TimerHub
// derives), as returned by GET /api/v1/timers.
export interface Timer {
  serviceReference: string
  serviceName: string
  name: string
  description: string
  begin: number // Unix seconds, UTC, includes margin
  end: number
  disabled: boolean
  // Enigma2's timer state: 0 waiting, 1 about to start, 2 recording, 3 ended
  // (a disabled timer is reported as 3).
  state: number
  justPlay: boolean
  eventId: number
  tags: string
  repeated: number
  // Who created it, read from its tags; absent when unknown.
  source?: 'serienrecorder' | 'autotimer'
  // Likely clashes with another timer on this box's tuners (an estimate —
  // the box has the final word when a timer is saved).
  conflict: boolean
  // This profile may delete or switch off the timer: admins every one,
  // viewers those they made through DreamHub.
  mayChange: boolean
}

// backend/internal/autotimer — the optional AutoTimer plugin, read-only.
export interface AutoTimerService {
  serviceReference: string
  serviceName: string
}
export interface AutoTimer {
  id: number
  name: string
  match: string
  enabled: boolean
  services: AutoTimerService[]
}
export type AutoTimers = { installed: false } | { installed: true; autoTimers: AutoTimer[]; editorPath: string }
export interface SimulatedTimer {
  serviceReference: string
  serviceName: string
  name: string
  begin: number
  end: number
  autoTimerName: string
}

// backend/internal/hubs.MovieView, as returned by GET /api/v1/media/movies.
export interface Movie {
  serviceReference: string
  title: string
  description: string
  descriptionExtended: string
  serviceName: string
  time: number // Unix seconds, UTC
  length: string // MM:SS, as WebControl reports it
  filename: string
  fileSize: number
  isRecording: boolean
  // Cover art already next to the recording on the box (another plugin's),
  // served by GET /api/v1/media/cover; only known when DreamHub runs on the box.
  cover?: boolean
  backdrop?: boolean
  // The box is writing ("recording") or playing it right now; no file
  // actions until it's done.
  inUse?: 'recording' | 'playing'
  // The last ad detection's summary, when the recording was analysed.
  ads?: AdSummary
}

// backend/internal/hubs.adSummary — what the list shows of an analysis.
export interface AdSummary {
  blocks: number
  seconds: number
  uncertain?: number
  // The breaks were cut out of the recording.
  trimmed?: boolean
  // The breaks were written into the recording's own cut list.
  marked?: boolean
}

// backend/internal/adscan.Result, from GET /api/v1/media/adscan?path=.
// Times are seconds from the recording's start.
export interface AdScanResult {
  duration: number
  blocks: { start: number; end: number; uncertain?: boolean }[]
  probes: number
  weakLogo?: boolean
  learned?: boolean
  // The breaks were cut out; duration is the new length.
  trimmed?: boolean
  // The breaks were written into the recording's own cut list — nothing
  // removed, but the box's own remote can jump past them.
  marked?: boolean
}

// backend/internal/api.jobView — a background job (GET /api/v1/jobs, and
// "event: job" on the event stream). message/error are i18n keys.
export type JobState = 'queued' | 'running' | 'waiting' | 'done' | 'failed' | 'canceled'
export interface Job {
  id: number
  kind: string
  key?: string
  ownerId: number
  state: JobState
  done: number
  total: number
  message?: string
  messageParams?: Record<string, unknown>
  error?: string
  createdAt: string
  startedAt?: string
  finishedAt?: string
}

// backend/internal/hubs.list — one folder of recordings, GET /api/v1/media/movies?dir=.
export interface MediaFolder {
  dir: string // absolute, trailing slash
  root: string
  parent?: string // absent at the root
  subfolders: { path: string; name: string }[]
  movies: Movie[]
  // File operations (rename, trash) only work when DreamHub runs on the box.
  fileOps: { available: boolean; reason?: string }
  // Whether "Detect ads" can run here (FFmpeg on the box, or mock mode).
  adScan?: boolean
}

// backend/internal/hubs.Suggestion — a proposed name; nothing changes
// until it goes through the normal rename preview.
export type SuggestionSource = 'metadata' | 'tmdb' | 'ai'
export interface NameSuggestion {
  name: string
  source: SuggestionSource
  kind?: 'movie' | 'tv'
}

// backend/internal/connector.Report (subset), GET /api/v1/connectors.
export interface ConnectorState {
  id: string
  state: 'ok' | 'not_configured' | 'unreachable' | 'not_implemented' | 'not_installed'
}

// backend/internal/recfs.TrashItem.
export interface TrashItem {
  path: string // the ".ts.trash" file
  originalPath: string
  title: string
  size: number // bytes, all of its files
  modified: string // RFC 3339, UTC
}

// backend/internal/hubs.Disk, as returned by GET /api/v1/media/disk.
export interface Disk {
  model: string
  capacityText: string
  freeText: string
  usedPercent?: number
}

// backend/internal/webcontrol.Service, as returned by
// GET /api/v1/guide/bouquets and used as ChannelSchedule.serviceReference.
export interface Bouquet {
  serviceReference: string
  serviceName: string
}

// backend/internal/webcontrol.EPGEvent, embedded in ChannelSchedule.
export interface EPGEvent {
  id: number
  start: number // Unix seconds, UTC
  duration: number // seconds
  title: string
  description: string
  serviceReference: string
  serviceName: string
  // Set by GET /api/v1/guide/grid only: the title matches a film already
  // in the Plex library (#32).
  inPlex?: boolean
}

// backend/internal/hubs.ChannelSchedule, as returned by GET /api/v1/guide/grid:
// every event in the requested window, in chronological order.
// One channel's current and next programme (GET /api/v1/guide/nownext);
// null where the box has no EPG.
export interface NowNext {
  serviceReference: string
  serviceName: string
  now: EPGEvent | null
  next: EPGEvent | null
}

export interface ChannelSchedule {
  serviceReference: string
  serviceName: string
  events: EPGEvent[]
}

// backend/internal/serienrecorder.Marker, as returned by
// GET /api/v1/series/markers.
export interface SeriesMarker {
  id: number
  name: string
  fsid: string
  wlid: string
  active: boolean
  info: string
  channels: string
  numberOfRecords: number
  seasons: string
  recordfolder: string
}

// GET /api/v1/series/markers/{id} (backend/internal/hubs/series_detail.go).
export interface SeriesRecording {
  path: string
  title: string
  description: string
  serviceName: string
  time: number // Unix seconds, UTC
  fileSize: number
  season: number // 0 when the title/description has no S01E02 code
  episode: number
}

export interface SeriesBroadcast {
  id: number
  start: number // Unix seconds, UTC
  duration: number
  title: string
  description: string
  serviceReference: string
  serviceName: string
  hasTimer: boolean
  // The other channels showing the same broadcast (regional variants).
  alsoOn?: string[]
  // Which episode this is, when known: from the EPG or DreamMediaInfo, or —
  // for an absolutely numbered series — matched by its episode title.
  season?: number
  episode?: number
  codeSource?: 'epg' | 'dreammediainfo' | 'tvmaze' | 'recording' | 'serienrecorder'
  episodeStatus?: 'recorded' | 'planned' | 'missing' | 'unmatched'
}

// backend/internal/hubs/series_episodes.go: every episode TVmaze knows.
export type EpisodeStatus = 'recorded' | 'planned' | 'upcoming' | 'missing' | 'aired'
export interface GuideEpisode {
  season: number
  number: number
  name: string
  airdate?: string // YYYY-MM-DD
  airstamp?: number // Unix seconds, UTC
  status: EpisodeStatus
}
export interface GuideSeason {
  season: number
  episodes: GuideEpisode[]
  recorded: number
  planned: number
  missing: number
  upcoming: number
}
export interface EpisodeGuide {
  state: 'ok' | 'off' | 'noMatch' | 'failed'
  source?: string
  showName?: string
  matchable: boolean // recordings carry episode codes, so "missing" means something
  absolute?: boolean // matched by absolute episode number (S00E8610 = TVmaze's 8610)
  seasons: GuideSeason[]
}

// SerienRecorder's own information about a series (its seriesinfo endpoint).
export interface SeriesInfo {
  title: string
  year: string // "2018-"
  info: string // "Drama, Crime, Action (42 min)"
  description: string
  cast?: string[]
}

export interface SeriesDetail {
  marker: SeriesMarker
  info?: SeriesInfo
  cover: boolean // a folder.jpg in the series' folder on the box
  upcoming: UpcomingEpisode[]
  recordings: SeriesRecording[]
  broadcasts: SeriesBroadcast[]
  episodes: EpisodeGuide
  unavailable: Array<'upcoming' | 'recordings' | 'broadcasts'>
}

// backend/internal/serienrecorder.UpcomingEpisode, as returned by
// GET /api/v1/series/upcoming.
export interface UpcomingEpisode {
  series: string
  title: string
  channel: string
  serviceReference: string
  season: string
  episode: string
  startTime: number // Unix seconds, UTC
  fsID: string
}

// backend/internal/ai.Recommendation, as returned by
// POST /api/v1/discovery/recommendations.
export interface Recommendation {
  title: string
  serviceReference: string
  serviceName: string
  start: number // Unix seconds, UTC
  duration: number // seconds
  reason: string
  hasTimer?: boolean // a timer covers it now
  inPlex?: boolean // the film's title matches one already in the Plex library (#32)
}

// The last generated set (GET /api/v1/discovery/recommendations), kept
// server-side per user.
export interface SavedPicks {
  bouquet: string
  generatedAt: number // Unix seconds
  taste?: TasteCounts
  recommendations: Recommendation[]
}

// GET /api/v1/discovery/status and the same fields on the POST response.
export interface DiscoveryStatus {
  usedToday: number
  dailyLimit: number
}

// backend/internal/plan.Change / .Plan — a previewed change, returned by
// POST /api/v1/timers/plan and applied by POST /api/v1/plans/{id}/apply
// (PRINCIPLES.md, "Preview and confirm").
export interface PlanChange {
  type: string
  before?: unknown
  after?: unknown
}
export interface Plan {
  id: string
  action: string
  createdAt: string
  expiresAt: string
  changes: PlanChange[]
  warnings: Problem[]
  // Set only by a caller with no i18n key of its own to render instead —
  // today just Extension Hub's own actions (internal/extension). Every
  // built-in hub's PlanPreview slot uses its own local i18n text and
  // ignores this.
  summary?: string
}

// backend/internal/dashboard.Tile / .Layout.
export interface DashboardTile {
  type: string // a built-in tile type, or 'custom'
  id?: string // custom tiles only; assigned by the backend on first save
  label: string // i18n key; empty for custom tiles
  width: number
  hidden: boolean
  custom?: CustomTileView
}

// A custom tile's saved view (backend/internal/dashboard.Custom).
export interface CustomTileView {
  title: string
  source: string
  query: ViewQuery
  display: 'list' | 'count' | 'posters'
}

// backend/internal/hub.ViewQuery — zero/absent means "no filter".
export interface ViewQuery {
  text?: string
  olderThanDays?: number
  newerThanDays?: number
  minSizeGB?: number
  withinDays?: number
  fromTime?: string // "HH:MM"
  minDuration?: number // minutes
  warnOnly?: boolean
  unwatched?: boolean
  bouquet?: string
  sort?: string
  limit?: number
}

// backend/internal/hub.ViewSource, from GET /api/v1/dashboard/sources.
export interface ViewSource {
  id: string
  label: string // i18n key
  // Shown instead of label when set: named by the user's data (a Plex
  // library's title).
  name?: string
  hubId: string
  filters: string[]
  sorts: string[] // the first is the default
}

export interface ViewItem {
  title: string
  subtitle?: string
  time?: number // Unix seconds, UTC
  end?: number
  size?: number // bytes
  warn?: boolean
  path?: string
  // A picture for the posters display (a Plex picture path).
  image?: string
  unwatched?: boolean
  // A recording's file, to watch it in the browser.
  recording?: string
}

export interface ViewResult {
  items: ViewItem[]
  total: number
  totalSize?: number
}

export interface DashboardLayout {
  tiles: DashboardTile[]
  // Not customized yet: the role's template (docs/archive/PROFILES_PLAN.md, P4).
  fromTemplate?: boolean
}

// backend/internal/hub.SearchResult, from GET /api/v1/search.
export interface SearchResult {
  hubId: string
  hubLabel: string
  title: string
  subtitle?: string
  path: string
}

// backend/internal/apitoken.Token / .Created.
export interface ApiToken {
  id: number
  name: string
  createdAt: string // RFC3339, UTC
}

export interface CreatedApiToken extends ApiToken {
  token: string // the plaintext secret — only ever present in the creation response
}

// backend/internal/hubs's changeView, from GET /api/v1/changes. userId is
// absent for a system-detected entry (no user made it — e.g. hub.statusChanged).
export interface ChangeEntry {
  id: number
  at: string // RFC3339, UTC
  userId?: number
  // The profile's name now; absent for the system's own entries and
  // deleted profiles.
  userName?: string
  action: string
  summary: string
  detail: Record<string, unknown>
}

// backend/internal/hubs's backupFile, from GET /api/v1/system/backup and the
// body POST /api/v1/system/backup/restore expects. Never carries secrets or
// passwords (docs/archive/V1_PLAN.md, WP5).
export interface BackupFile {
  version: number
  exportedAt: string // RFC3339, UTC
  settings: Record<string, unknown>
  dashboard: DashboardLayout
  // sections is App6's own addition — opt-in, omitted unless asked for
  // (GET /api/v1/system/backup?sections=apps). Opaque per-section
  // payloads; the UI never inspects their content, only whether a
  // known section id is present.
  sections?: Record<string, unknown>
}

// GET /api/v1/system/backup/sections (App6, docs/APPSHUB_PLAN.md).
export interface BackupSectionsList {
  sections: {
    id: string
    items: { id: string; name: string; sizeBytes: number; defaultIncluded: boolean }[]
  }[]
}

// GET /api/v1/system/backup/files (App15): backups kept on the box.
export interface StoredBackup {
  name: string
  sizeBytes: number
  createdAt: string // RFC3339, UTC
}

// GET /api/v1/apps/restorable (App15): apps whose choices are stored on the
// box but which are not installed.
export interface RestorableApp {
  id: string
  name: string
  root: string
  custom?: boolean
}

// backend/internal/hubs's logEntryView, from GET /api/v1/admin/logs.
export interface LogEntry {
  time: string // RFC3339, UTC
  unit?: string
  level: 'emerg' | 'alert' | 'crit' | 'err' | 'warning' | 'notice' | 'info' | 'debug'
  message: string
}

// backend/internal/hubs's crashLogView, from GET /api/v1/admin/crashlogs.
export interface CrashLog {
  name: string
  at?: string // RFC3339, UTC — parsed from the file name, when it carries one
  size: number
}

// backend/internal/hubs.scriptView — AdminHub's script library (A3).
export interface AdminScript {
  id: number
  name: string
  uploadedAt: string // RFC3339, UTC
}

// backend/internal/hubs.scriptRunView — one past run of a script.
export interface ScriptRun {
  startedAt: string // RFC3339, UTC
  finishedAt: string // RFC3339, UTC
  exitCode: number
  output: string
}

// apt.Package — AdminHub's package browser (A2), read-only so far.
export interface AdminPackage {
  name: string
  category: string
  version: string
  availableVersion?: string
  upgradable: boolean
  description: string
  // The DreamHub hubs that use this package (hub.Descriptor.Packages).
  neededBy?: string[]
  // A package a hub needs that isn't installed ("needed" view only).
  missing?: boolean
  // What the tab shows (backend internal/pkginfo): the curated text, else
  // the package's own description, else nothing.
  info: PackageInfo
}

export interface PackageInfo {
  description: string
  category: string
  categoryLabel: string
  neededBy?: string[]
  source: 'curated' | 'package' | 'none'
}

export interface PackageCategory {
  id: string
  label: string
}

// apt.Step — one package a package plan installs, upgrades or removes.
export interface PackageStep {
  kind: 'install' | 'upgrade' | 'remove' | 'downgrade'
  name: string
  oldVersion?: string
  newVersion?: string
}

// dockersetup.State — whether Docker is set up on the box (AppsHub).
export interface DockerSetupState {
  supported: boolean
  version?: string
  overlay: boolean
  configured: boolean
  dockerRunning: boolean
  helperRunning: boolean
  ready: boolean
  freeBytes: number
}

// dockersetup.Step — one thing "Set up Docker" (or "Uninstall Docker") would do.
export interface DockerSetupStep {
  kind: 'overlay' | 'download' | 'config' | 'services' | 'containers' | 'modules' | 'files' | 'data'
  detail?: Record<string, string>
}

// The most recent package apply and apt-get's output.
export interface PackageRun {
  at: string
  action: string
  steps: PackageStep[]
  ok: boolean
  output: string
}

// apt.Feed — AdminHub's feed list, with a trust signal (A2).
// extension.Widget — Extension Hub's own fixed widget vocabulary
// (docs/EXTENSIONHUB_PLAN.md, E1): never frontend code of its own, just
// these shapes, rendered with DreamHub's own components. Fields not used
// by `type` are simply absent.
export interface ExtensionListRow {
  label: string
  value: string
  icon?: string
}
export interface ExtensionWidget {
  type: 'status' | 'meter' | 'list' | 'action'
  label: string
  value?: string
  tone?: 'good' | 'warn' | 'bad'
  current?: number
  max?: number
  unit?: string
  rows?: ExtensionListRow[]
  actionId?: string
  buttonLabel?: string
  // Schema 3: also shown on the start page.
  tile?: boolean
}

// hubs.cardView — one registered extension's card.
export interface ExtensionCard {
  id: string
  name: string
  icon: string
  admin: boolean
  status: 'available' | 'degraded' | 'unavailable'
  problems: Problem[]
  widgets: ExtensionWidget[]
  settings?: boolean
}

// extension.SettingField — one input of an extension's settings form.
export interface ExtensionSetting {
  key: string
  label: string
  type: 'text' | 'secret' | 'bool' | 'select' | 'info' | 'link'
  help?: string
  value?: string
  set?: boolean
  options?: { value: string; label: string }[]
}

// hubs.StorageChoice — one place an app can be stored.
export interface AppStorageChoice {
  path: string
  label: string
  freeBytes: number
  default?: boolean
}

// hubs.catalogEntry — one curated AppsHub app (docs/APPSHUB_PLAN.md, App3).
export interface CatalogApp {
  portEditable?: boolean
  // A bundled UI icon name; empty: the category's.
  icon?: string
  // The install form's inputs (a compose file's variables), labels in the
  // requested language.
  fields: AppField[]
  id: string
  name: string
  category: string
  description: string
  image: string
  // The catalog's plain-English hint for this box.
  note?: string
  // Helper containers installed along with it (e.g. "db").
  services?: string[]
  // Memory limit of all its containers together; its web port.
  memoryMB: number
  port?: number
  compatible: boolean
  installed: boolean
}

// compose.Field — one input of the install form (docs/APPSHUB_PLAN.md, App12).
export interface AppField {
  name: string
  label: string
  description?: string
  type: 'port' | 'path' | 'number' | 'text'
  default?: string
  required?: boolean
  secret?: boolean
  // Only shown in the advanced view (the web port and required fields never are).
  advanced?: boolean
  webPort?: boolean
}

// compose.Issue — one reason a compose file is refused, or one note about it.
// `code` is stable and translated under apps.compose.issue.<code>.
export interface ComposeIssue {
  code: string
  service?: string
  key?: string
  detail?: string
  message: string
}

// hubs.elevatedItem — something beyond a plain container that needs the
// person's explicit confirmation.
export interface ElevatedItem {
  service?: string
  kind: 'privileged' | 'capability' | 'device' | 'pid' | 'ipc'
  value?: string
}

// hubs.parsedApp — the form for an admin's own container (POST
// /api/v1/apps/custom/parse).
export interface ParsedApp {
  // The normalized compose file, sent back with the install request.
  compose: string
  id: string
  name: string
  description?: string
  note?: string
  fields: AppField[]
  services: { name: string; image: string }[]
  elevated: ElevatedItem[]
  notes: ComposeIssue[]
  webPort?: number
  portEditable?: boolean
  memoryMB: number
}

// POST /api/v1/apps/install/plan — a curated app by id, or the admin's own
// compose file; the same values, extra rows and storage choice for both.
export interface InstallRequest {
  appId?: string
  custom?: { compose: string; name: string }
  storage?: string
  memoryLimitMB?: number
  values?: Record<string, string>
  extra?: {
    env: { key: string; value: string }[]
    ports: { port: number; udp: boolean }[]
    paths: { host: string; container: string; readOnly: boolean }[]
  }
  confirmElevated?: boolean
}

// GET /api/v1/apps/installed/{id}/edit — what the install form is
// pre-filled with (App13). Secret values never come back: secretSet names the
// fields that have one, extra rows with keep are secrets that stay.
export interface AppEditView {
  kind: 'catalog' | 'custom'
  app: CatalogApp | ParsedApp
  known: boolean
  values: Record<string, string>
  secretSet: string[]
  extra: {
    env: { key: string; value: string; keep?: boolean }[]
    ports: { port: number; udp: boolean }[]
    paths: { host: string; container: string; readOnly: boolean }[]
  }
  memoryLimitMB: number
  storage: { path: string; label: string; freeBytes: number }
}

// GET /api/v1/apps/docker/images — images no container uses.
export interface UnusedImages {
  images: { id: string; tags: string[]; sizeBytes: number }[]
  reclaimableBytes: number
  // The previous versions kept for "Roll back"; the clean-up never removes them.
  kept: { appId: string; service?: string; imageId: string; sizeBytes: number }[]
  keptBytes: number
}

// GET /api/v1/apps/updates — one count over packages and containers (App14).
export interface UpdatesView {
  packages: { name: string; from: string; to: string; touchesEnigma2?: boolean }[]
  containers: { id: string; appId: string; name: string }[]
  count: number
  // DreamHub's own package has an update; it is not part of "Update all".
  selfUpdate: boolean
  // Whether an update could start now; why not and until when (Unix seconds).
  window: { closed: boolean; reason?: 'recording' | 'starting'; name?: string; until?: number }
  auto: { containers: boolean; packages: boolean }
}

// One step of the combined update plan, in the order it runs.
export interface UpdateStep {
  kind: 'container' | 'package'
  name: string
  appId?: string
  change?: 'upgrade' | 'install' | 'remove' | 'downgrade'
  from?: string
  to?: string
  touchesEnigma2?: boolean
}

export interface UpdatePlan {
  plan: Plan
  steps: UpdateStep[]
  // Apps that can't be updated (their settings no longer validate), with why.
  skipped: { name: string; reason: string }[]
  order: string[]
}

// What a failed "Update all" says (the error's details).
export interface UpdateFailureDetails {
  done: string[]
  failed: { kind: 'container' | 'package'; name: string; appId?: string; code: string; reason: string; rolledBack?: boolean; output?: string }[]
  skipped: string[]
}

// What an install plan will create (Plan.changes[0].after for apps.install).
export interface InstallPreviewContainer {
  service?: string
  image: string
  memoryMB: number
  ports?: { port: number; udp?: boolean; local?: boolean }[]
  mounts?: { host: string; container: string; readOnly?: boolean }[]
  env?: { key: string; value: string; masked?: boolean; generated?: boolean }[]
  command?: string[]
  elevated?: { privileged?: boolean; capAdd?: string[]; devices?: string[]; pidHost?: boolean; ipcHost?: boolean }
}

// hubs.searchResult — one free-search hit, not yet arch-checked.
export interface AppSearchResult {
  image: string
  description: string
  stars: number
}

// GET /api/v1/apps/manifest.
export interface AppManifest {
  architectures: { architecture: string; variant?: string }[]
  compatible: boolean
}

// hubs.installedApp — one running or stopped app (docs/APPSHUB_PLAN.md, App4).
export interface InstalledApp {
  id: string
  appId: string
  name: string
  image: string
  state: string
  hostPort?: number
  upgradable?: boolean
  // Starts when the box starts; the edit form can be opened; the row's picture.
  autostart: boolean
  editable: boolean
  // The version before the last update is kept: "Roll back" is offered.
  rollBack?: boolean
  icon?: string
  category?: string
  // The app's helper containers (e.g. its database), each with own logs.
  services?: { id: string; service: string; image: string; state: string }[]
}

// GET /api/v1/apps/console/status (docs/APPSHUB_PLAN.md, App5) — the
// exact same shape TerminalStatus above uses, Terminal's own posture
// reused for a per-container console.
export interface AppConsoleStatus {
  available: boolean
  authOn: boolean
  enabled: boolean
}

export interface AdminFeed {
  name: string
  url?: string
  enabled: boolean
  known: boolean
}

// GET /api/v1/admin/terminal/status.
export interface TerminalStatus {
  // false on a system without shellinaboxd configured (not running on the box).
  available: boolean
  // DreamHub's own password protection must be on before the terminal can be enabled.
  authOn: boolean
  enabled: boolean
}

export interface LogLevel {
  level: 'info' | 'debug'
  until?: string // RFC3339, UTC — only while level is "debug"
}

// backend/internal/api's notificationView — the core event bus's feed
// (docs/archive/V1_PLAN.md, WP7), from GET /api/v1/notifications and pushed live
// over GET /api/v1/events/stream. key is an i18n key, params its
// arguments — never finished text, translated client-side like
// status.Problem everywhere else.
export interface Notification {
  id: number
  at: string // RFC3339, UTC
  key: string
  params: Record<string, unknown>
  read: boolean
  // The box the event is about; only with more than one box.
  box?: { id: string; name: string; color?: string }
}

// backend/internal/ai.Taste, from GET /api/v1/discovery/taste: exactly what
// "use my taste" sends to the AI provider.
export interface TasteProfile {
  recordings: string[]
  series: string[]
  timers: string[]
  autoTimers: string[]
}

export interface TasteCounts {
  recordings: number
  series: number
  timers: number
  autoTimers: number
}

// backend/internal/enrich.Info, from POST /api/v1/enrich/lookup.
export interface EnrichInfo {
  source: string // "dreammediainfo" (the plugin's own data, pictures via TMDB) or "tmdb"
  fsk?: string // German age rating, from DreamMediaInfo
  kind: 'movie' | 'tv'
  id: number
  title: string
  originalTitle?: string
  year?: number
  overview?: string
  rating?: number // 0-10
  votes?: number
  genres?: string[]
  poster?: string // image path for /api/v1/enrich/image
  backdrop?: string
}

// What further databases add to one title (GET /api/v1/enrich/extras).
export interface EnrichExtras {
  imdbId?: string
  imdb?: number // 0-10
  imdbVotes?: number
  rottenTomatoes?: number // Tomatometer, percent
  metascore?: number // 0-100
  logo?: string // fanart.tv image path for /api/v1/enrich/logo
}

export interface EnrichQuery {
  title: string
  kind?: 'movie' | 'tv'
}

// backend/internal/hubs/media_batch.go: one recording of a batch rename.
// One recording in "Save covers" (POST /api/v1/media/covers/plan).
export interface CoverItem {
  path: string
  title: string
  status: 'write' | 'has_cover' | 'skipped'
  reason?: string // an error.media code, for skipped
  files?: string[] // file names to be written
  poster?: string // TMDB image path, for the thumbnail
}

export interface BatchRenameItem {
  path: string
  before: string // base name, without .ts
  after?: string
  folder?: string // new folder relative to the recording folder, when it moves
  files?: number
  status: 'rename' | 'unchanged' | 'skipped'
  reason?: string // an error.media code, for skipped
}

// GET /api/v1/control/current — what the box shows right now.
export interface CurrentService {
  serviceReference: string
  serviceName: string
  providerName: string
  kind: 'tv' | 'radio' | 'recording'
  now: EPGEvent | null
  next: EPGEvent | null
  volume: number
  muted: boolean
  inStandby: boolean
}

// GET /api/v1/admin/services — one systemd unit on the box.
export interface ServiceUnit {
  name: string // "enigma2.service"
  description: string
  active: string // systemd's ActiveState: active, inactive, failed, activating, …
  sub: string
  since?: string // RFC 3339, while active
  mainPid?: number
  memory?: number // bytes
  restarts: number // automatic restarts by systemd
}

// GET/POST /api/v1/system/selftest — the box self-test.
export interface SelfTestCheck {
  id: string
  status: 'ok' | 'warn' | 'fail' | 'skip'
  code: string // detail line, under system.selftest.result
  params?: Record<string, string | number | boolean>
  ms: number
}
export interface SelfTestReport {
  startedAt: string
  checks: SelfTestCheck[]
  captured: string[] // files kept next to the report (none in mock mode)
}

// backend/internal/plex and internal/hubs/plex.go — PlexHub, "Plex Mediathek".
export interface PlexPart {
  id: number
  key: string
  size?: number
  container?: string
}
export interface PlexMedia {
  container?: string
  videoCodec?: string
  audioCodec?: string
  audioChannels?: number
  videoResolution?: string
  bitrate?: number
  parts?: PlexPart[]
}
export interface PlexItem {
  ratingKey: string
  type: 'movie' | 'show' | 'season' | 'episode'
  title: string
  showKey?: string
  seasonKey?: string
  showTitle?: string
  seasonTitle?: string
  index?: number
  parentIndex?: number
  year?: number
  summary?: string
  thumb?: string
  art?: string
  logo?: string // transparent title logo (Plex's clearLogo)
  genres?: string[]
  rating?: number
  contentRating?: string
  duration?: number // ms
  viewCount?: number
  viewOffset?: number // ms
  lastViewedAt?: number
  leafCount?: number
  viewedLeafCount?: number
  childCount?: number
  addedAt?: number // Unix seconds
  aired?: string
  media?: PlexMedia[]
  watched: boolean
}
// Where an answer's data came from: the server just now (online) or the
// cache from syncedAt.
export interface PlexMeta {
  online: boolean
  syncedAt?: number
}
export interface PlexLibrary {
  id: string
  title: string
  type: 'movie' | 'show'
  count: number
  selected: boolean
}
export interface PlexStatus {
  configured: boolean
  canPlay: boolean
  online?: boolean
  error?: '' | 'unreachable' | 'unauthorized'
  server?: { name: string; version: string }
  syncedAt?: number
  syncing?: boolean
  url?: string
  machineId?: string
}
export interface PlexServerChoice {
  name: string
  id: string
  owned: boolean
  connections: { uri: string; local: boolean }[]
}
export interface PlexPin {
  id: number
  code: string
  expiresAt: string
  linkUrl: string
}

// ChannelHub (backend/internal/hubs/channels.go, docs/CHANNELHUB_PLAN.md).
export interface ChannelBouquet {
  ref: string
  name: string
  kind: 'tv' | 'radio'
  count: number
}

export interface BouquetEntry {
  ref: string
  name: string
  marker?: boolean
  quality?: 'sd' | 'hd' | 'uhd'
  picon: boolean
  now?: EPGEvent
}

export interface ChannelInfo {
  ref: string
  name: string
  provider?: string
  kind: 'tv' | 'radio'
  quality?: 'sd' | 'hd' | 'uhd'
  firstSeen: number // Unix seconds
  // Appeared after DreamHub's first look at the box's list.
  new?: boolean
  // The box no longer lists it.
  gone?: boolean
  bouquets: string[]
}

export interface ChannelIssue {
  kind: 'duplicate' | 'gone' | 'noEpg' | 'noPicon'
  bouquetRef: string
  bouquet: string
  ref: string
  name: string
}

export interface PiconSet {
  name: string
  path: string
  covered: number
}

// A line of how a bouquet should look (POST /api/v1/channels/bouquet/plan);
// a new heading has no ref yet.
export interface DraftEntry {
  ref: string
  name: string
  marker?: boolean
}

export interface ChannelBackup {
  filename: string
  createdAt: string
  summary: string
}

// backend/internal/hubs.ChannelChange — before/after of a channel plan's
// changes ("channel.move", "channel.add", "bouquet.rename", …).
export interface ChannelChange {
  name?: string
  newName?: string
  from?: number
  to?: number
}

// logosearch.Entry — one logo of the public TV-logo collection.
export interface LogoResult {
  path: string
  name: string
  country: string
}
