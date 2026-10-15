// What a hub is called on screen: its plain page name ("Recordings",
// "Live TV", translated) — never the internal hub name. Core pages
// (dashboard, settings) show their own label. Every place that names a hub
// goes through here — sidebar, search, page headings, status lists — and
// hubName turns the hub id the backend puts in a problem or a package note
// into the same name.
import { useI18n } from 'vue-i18n'
import { useNavigationStore } from '@/stores/navigation'

export interface Labelled {
  label: string
}

export function useHubLabel() {
  const { t, te } = useI18n()
  const navigation = useNavigationStore()

  function hubLabel(x: Labelled): string {
    return t(x.label)
  }

  // A hub page's heading: its sidebar entry when there is one, else the
  // fallback key.
  function hubTitle(id: string, fallbackKey: string): string {
    const item = navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.id === id)
    return item ? hubLabel(item) : t(fallbackKey)
  }

  // A hub id ("timer") as the page's name ("Timers").
  function hubName(id: string): string {
    return te(`hub.${id}`) ? t(`hub.${id}`) : id
  }

  return { hubLabel, hubTitle, hubName }
}
