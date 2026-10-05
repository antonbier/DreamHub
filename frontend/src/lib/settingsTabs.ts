// The settings menu's tabs in display order — the ids match
// backend/internal/settings (Tab*). Shared by SettingsView.vue and the
// search palette, which links a matching setting to its tab.
export const SETTINGS_TABS = ['general', 'recordings', 'services', 'notifications', 'hubs', 'access', 'system'] as const

export type SettingsTab = (typeof SETTINGS_TABS)[number]
