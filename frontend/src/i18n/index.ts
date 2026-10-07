// i18n setup — CONCEPT.md, "Internationalization". v1 bundles German,
// Italian and English; missing strings fall back to English, never to an
// empty label. Runtime-added locales (dropped into
// /data/dreamhub/locales/) are a later Foundation item — see ROADMAP.md.
import { createI18n } from 'vue-i18n'
import en from '../locales/en.json'
import de from '../locales/de.json'
import it from '../locales/it.json'

export const SUPPORTED_LOCALES = ['de', 'it', 'en'] as const
export type Locale = (typeof SUPPORTED_LOCALES)[number]

// Language *names*, not flags (CONCEPT.md: flags represent countries, not
// languages) — always shown in the language itself, regardless of the
// current UI locale, so this is deliberately not translated.
export const LOCALE_NAMES: Record<Locale, string> = {
  de: 'Deutsch',
  it: 'Italiano',
  en: 'English',
}

export function isSupportedLocale(value: string): value is Locale {
  return (SUPPORTED_LOCALES as readonly string[]).includes(value)
}

// The device remembers the last language it showed, so screens before any
// profile's settings are known — login, the profile picker — already speak
// it; else the browser's language when supported.
const DEVICE_LANGUAGE_KEY = 'dreamhub.language'

function deviceLanguage(): Locale {
  try {
    const stored = localStorage.getItem(DEVICE_LANGUAGE_KEY)
    if (stored && isSupportedLocale(stored)) return stored
  } catch {
    // Private browsing or a blocked store: fall through.
  }
  const browser = (navigator.language || 'en').slice(0, 2)
  return isSupportedLocale(browser) ? browser : 'en'
}

export function rememberDeviceLanguage(locale: Locale) {
  try {
    localStorage.setItem(DEVICE_LANGUAGE_KEY, locale)
  } catch {
    // See deviceLanguage.
  }
}

export const i18n = createI18n({
  legacy: false,
  locale: deviceLanguage(),
  fallbackLocale: 'en',
  messages: { en, de, it },
})
