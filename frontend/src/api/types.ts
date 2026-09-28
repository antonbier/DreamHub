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

export interface Navigation {
  labels: 'descriptive' | 'hub'
  sections: NavSection[]
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
  value: unknown
  hasValue: boolean
  isDefault: boolean
}

export interface SettingSection {
  id: string
  label: string
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

// GET /api/v1/whoami.
export interface WhoAmI {
  username: string
  role: 'admin' | 'user'
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
}

// backend/internal/hubs.ChannelSchedule, as returned by GET /api/v1/guide/grid:
// every event in the requested window, in chronological order.
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
  seasons: GuideSeason[]
}

export interface SeriesDetail {
  marker: SeriesMarker
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
  display: 'list' | 'count'
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
  bouquet?: string
  sort?: string
  limit?: number
}

// backend/internal/hub.ViewSource, from GET /api/v1/dashboard/sources.
export interface ViewSource {
  id: string
  label: string // i18n key
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
}

export interface ViewResult {
  items: ViewItem[]
  total: number
  totalSize?: number
}

export interface DashboardLayout {
  tiles: DashboardTile[]
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
  action: string
  summary: string
  detail: Record<string, unknown>
}

// backend/internal/hubs's backupFile, from GET /api/v1/system/backup and the
// body POST /api/v1/system/backup/restore expects. Never carries secrets or
// passwords (docs/V1_PLAN.md, WP5).
export interface BackupFile {
  version: number
  exportedAt: string // RFC3339, UTC
  settings: Record<string, unknown>
  dashboard: DashboardLayout
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

export interface LogLevel {
  level: 'info' | 'debug'
  until?: string // RFC3339, UTC — only while level is "debug"
}

// backend/internal/api's notificationView — the core event bus's feed
// (docs/V1_PLAN.md, WP7), from GET /api/v1/notifications and pushed live
// over GET /api/v1/events/stream. key is an i18n key, params its
// arguments — never finished text, translated client-side like
// status.Problem everywhere else.
export interface Notification {
  id: number
  at: string // RFC3339, UTC
  key: string
  params: Record<string, unknown>
  read: boolean
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
  source: string // "tmdb"
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

export interface EnrichQuery {
  title: string
  kind?: 'movie' | 'tv'
}
