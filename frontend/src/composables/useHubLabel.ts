// What a hub is called on screen: its descriptive label ("Recordings",
// translated) or its hub name ("MediaHub"), per the profile's choice in the
// user menu or Settings → General (general.navLabels). Core pages
// (dashboard, settings) have no hub name and always show their label.
// Every place that names a hub goes through here — sidebar, search, page
// headings, status lists — so the switch changes all of them at once.
import { useI18n } from 'vue-i18n'
import { useNavigationStore } from '@/stores/navigation'
import { usePreferencesStore } from '@/stores/preferences'

export interface Labelled {
  name?: string
  label: string
}

export function useHubLabel() {
  const { t } = useI18n()
  const preferences = usePreferencesStore()
  const navigation = useNavigationStore()

  function hubLabel(x: Labelled): string {
    return preferences.navLabels === 'hub' && x.name ? x.name : t(x.label)
  }

  // A hub page's heading: its sidebar entry when there is one, else the
  // descriptive fallback key.
  function hubTitle(id: string, fallbackKey: string): string {
    const item = navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.id === id)
    return item ? hubLabel(item) : t(fallbackKey)
  }

  return { hubLabel, hubTitle }
}
