// Helpers for the receiver's setup menus (components/boxsettings): the key
// of a setting's own explanation and how a value is shown.
import type { BoxSetting } from '@/api/types'

type T = (key: string, named?: Record<string, unknown>) => string

// The i18n key of a setting's own explanation under `boxsettings.help`:
// the config path without "config." and with every "." or "-" an
// underscore (config.recording.margin_before → recording_margin_before).
export function helpKey(path: string): string {
  return path.replace(/^config\./, '').replace(/[.-]/g, '_')
}

// What a setting's current value looks like on screen. A password or PIN
// is only ever "set" or "not set": its value never reaches the browser.
export function formatValue(item: BoxSetting, t: T, locale: string): string {
  if (item.secret) return item.isSet ? t('boxsettings.value.set') : t('boxsettings.value.notSet')
  switch (item.kind) {
    case 'readonly':
      return t('boxsettings.value.notShown')
    case 'bool':
      return item.value ? t('boxsettings.value.on') : t('boxsettings.value.off')
    case 'number':
      return typeof item.value === 'number' ? new Intl.NumberFormat(locale).format(item.value) : String(item.value ?? '')
    case 'choice': {
      const v = String(item.value ?? '')
      return item.choices?.find((c) => c.value === v)?.label ?? v
    }
    case 'multichoice': {
      const list = Array.isArray(item.value) ? item.value : []
      if (list.length === 0) return t('boxsettings.value.none')
      return list.map((v) => item.choices?.find((c) => c.value === String(v))?.label ?? String(v)).join(', ')
    }
    case 'text': {
      const v = String(item.value ?? '')
      return v === '' ? t('boxsettings.value.empty') : v
    }
    default:
      return ''
  }
}
