// The settings menu's tabs in display order — the ids match
// backend/internal/settings (Tab*), except "profiles", which holds only
// hand-built panels (docs/archive/PROFILES_PLAN.md). Shared by SettingsView.vue and
// the search palette, which links a matching setting to its tab. The
// backend's "hubs" tab (which features are on) is shown inside General.
export const SETTINGS_TABS = ['general', 'profiles', 'boxes', 'recordings', 'services', 'notifications', 'access', 'system'] as const

export type SettingsTab = (typeof SETTINGS_TABS)[number]
