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
