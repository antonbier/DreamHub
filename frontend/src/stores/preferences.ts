// User preferences backed by GET/PATCH /api/v1/settings (general section —
// backend/internal/settings/core.go). DreamHub's own settings save directly
// once changed (no preview/apply step — PRINCIPLES.md, "Preview and
// confirm" names this exception), but every change is still recorded in the
// change log by the backend.
//
// Until the backend answers, in-memory defaults apply (dark, English,
// descriptive labels) — the app is usable, just not yet personalized;
// `reachable` drives the "can't reach the backend" banner in AppShell.vue.
import { defineStore } from 'pinia'
import { api, ApiError } from '@/api/client'
import { i18n, isSupportedLocale, type Locale } from '@/i18n'

export type Appearance = 'dark' | 'light' | 'system'
export type NavLabels = 'descriptive' | 'hub'
export type TimezoneMode = 'box' | 'browser' | 'fixed'

const KEY_LANGUAGE = 'general.language'
const KEY_APPEARANCE = 'general.appearance'
const KEY_NAV_LABELS = 'general.navLabels'
const KEY_TIMEZONE_MODE = 'general.timezoneMode'
const KEY_TIMEZONE = 'general.timezone'

function applyTheme(appearance: Appearance) {
  const resolved =
    appearance === 'system'
      ? window.matchMedia('(prefers-color-scheme: dark)').matches
        ? 'dark'
        : 'light'
      : appearance
  document.documentElement.setAttribute('data-theme', resolved)
}

let systemThemeListenerAttached = false

export const usePreferencesStore = defineStore('preferences', {
  state: () => ({
    language: 'en' as Locale,
    appearance: 'dark' as Appearance,
    navLabels: 'descriptive' as NavLabels,
    timezoneMode: 'box' as TimezoneMode,
    timezone: '',
    loaded: false,
    reachable: true,
  }),
  actions: {
    /** Loads effective settings once at startup (App.vue, onMounted). */
    async load() {
      try {
        const view = await api.settings.get()
        const general = view.sections.find((s) => s.id === 'general')
        const value = (key: string) => general?.fields.find((f) => f.key === key)?.value

        const lang = value(KEY_LANGUAGE)
        if (typeof lang === 'string' && isSupportedLocale(lang)) {
          this.setLanguage(lang, { persist: false })
        }
        const appearance = value(KEY_APPEARANCE)
        if (appearance === 'dark' || appearance === 'light' || appearance === 'system') {
          this.setAppearance(appearance, { persist: false })
        }
        const navLabels = value(KEY_NAV_LABELS)
        if (navLabels === 'descriptive' || navLabels === 'hub') {
          this.navLabels = navLabels
        }
        const timezoneMode = value(KEY_TIMEZONE_MODE)
        if (timezoneMode === 'box' || timezoneMode === 'browser' || timezoneMode === 'fixed') {
          this.timezoneMode = timezoneMode
        }
        const timezone = value(KEY_TIMEZONE)
        if (typeof timezone === 'string') this.timezone = timezone
        this.reachable = true
      } catch (err) {
        // Network error (no HTTP response at all) means the backend isn't
        // reachable; an ApiError with a real status (e.g. a 5xx) still
        // means the app itself is up, so don't show the unreachable banner
        // for that case.
        this.reachable = !(err instanceof ApiError) || err.status !== 0
      } finally {
        this.loaded = true
        this.attachSystemThemeListener()
      }
    },

    attachSystemThemeListener() {
      if (systemThemeListenerAttached) return
      systemThemeListenerAttached = true
      window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
        if (this.appearance === 'system') applyTheme('system')
      })
    },

    setLanguage(locale: Locale, opts: { persist?: boolean } = {}) {
      this.language = locale
      i18n.global.locale.value = locale
      document.documentElement.lang = locale
      if (opts.persist !== false) void this.save(KEY_LANGUAGE, locale)
    },

    setAppearance(appearance: Appearance, opts: { persist?: boolean } = {}) {
      this.appearance = appearance
      applyTheme(appearance)
      if (opts.persist !== false) void this.save(KEY_APPEARANCE, appearance)
    },

    setNavLabels(navLabels: NavLabels) {
      this.navLabels = navLabels
      void this.save(KEY_NAV_LABELS, navLabels)
    },

    async save(key: string, value: unknown) {
      try {
        await api.settings.update({ [key]: value })
      } catch {
        // The setting still applies locally for this session; it just
        // won't survive a reload. Non-fatal — no toast for a background
        // preference save per DESIGN.md's "calm" direction.
      }
    },
  },
})
