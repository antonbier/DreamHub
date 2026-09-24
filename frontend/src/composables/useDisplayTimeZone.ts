// Resolves general.timezoneMode/general.timezone (backend/internal/settings
// /core.go) into the actual IANA zone name to pass as Intl's `timeZone`
// option (ROADMAP.md, "Time zones": "the display logic and hint are still
// open" — the setting itself already existed). `undefined` means "let Intl
// use the runtime's own zone", which is also the correct behavior for mode
// 'browser' and for 'box' before the health check has resolved a zone.
import { computed } from 'vue'
import { usePreferencesStore } from '@/stores/preferences'
import { useDeviceStatus } from './useDeviceStatus'

export function useDisplayTimeZone() {
  const preferences = usePreferencesStore()
  const status = useDeviceStatus()
  void status.checkClock() // idempotent; populates status.serverTimeZone for 'box' mode

  const timeZone = computed<string | undefined>(() => {
    if (preferences.timezoneMode === 'fixed' && preferences.timezone) return preferences.timezone
    if (preferences.timezoneMode === 'box' && status.serverTimeZone.value) return status.serverTimeZone.value
    return undefined
  })

  return { timeZone }
}
