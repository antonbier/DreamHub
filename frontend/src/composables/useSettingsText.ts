// Translates the schema-driven settings menu's i18n keys
// (backend/internal/settings, /internal/connector, /internal/hub).
//
// A *section* label from the backend (e.g. "settings.general") and a
// *field* label one level under it (e.g. "settings.general.language") would
// collide as vue-i18n keys — a JSON node can't be both a leaf string and an
// object with children. So locale files nest field labels directly under
// the section's own object and add a sibling ".title" leaf for the section
// heading; sectionTitle() appends ".title", fieldLabel() uses the backend
// key as-is. See src/locales/en.json, "settings".
import { useI18n } from 'vue-i18n'
import { isSupportedLocale, LOCALE_NAMES } from '@/i18n'
import type { FieldError, SettingField, SettingSection } from '@/api/types'

function translateOrRaw(
  t: (key: string, params: Record<string, unknown>) => string,
  key: string,
  raw: string,
  params: Record<string, unknown>,
) {
  const result = t(key, params)
  return result === key ? raw : result
}

export function useSettingsText() {
  const { t } = useI18n()

  function sectionTitle(section: SettingSection): string {
    return translateOrRaw(t, `${section.label}.title`, section.id, {})
  }

  function fieldLabel(field: SettingField): string {
    return translateOrRaw(t, field.label, field.key, {})
  }

  // general.language's options are language codes — shown as the language's
  // own name (LOCALE_NAMES, same as the top-bar switcher), not translated
  // into the current UI language. Every other enum falls back to the
  // shared settings.option.<value> pool, or the raw value if untranslated.
  function optionLabel(field: SettingField, value: string): string {
    if (field.key === 'general.language' && isSupportedLocale(value)) {
      return LOCALE_NAMES[value]
    }
    return translateOrRaw(t, `settings.option.${value}`, value, {})
  }

  function fieldError(err: FieldError): string {
    return translateOrRaw(t, `settings.error.${err.code}`, err.code, err.params ?? {})
  }

  return { sectionTitle, fieldLabel, optionLabel, fieldError }
}
