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

// backend/internal/webcontrol.Timer, as returned by GET /api/v1/timers.
export interface Timer {
  serviceReference: string
  serviceName: string
  name: string
  description: string
  begin: number // Unix seconds, UTC, includes margin
  end: number
  disabled: boolean
  state: number
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
  type: string
  label: string
  width: number
  hidden: boolean
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
