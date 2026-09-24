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
