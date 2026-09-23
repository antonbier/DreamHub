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
