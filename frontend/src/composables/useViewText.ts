// Text for custom dashboard tiles' views (internal/hub/view.go): the
// one-line summary of a view's criteria ("Recordings · older than 90 days ·
// at least 3 GB · largest first"), shared by the tile and its builder, and
// the time zone a view's "from 20:15" is meant in.
import { useI18n } from 'vue-i18n'
import { useDisplayTimeZone } from './useDisplayTimeZone'
import type { ViewQuery } from '@/api/types'

// Source ID → i18n key, matching the labels the backend's sources report
// (backend/internal/hubs/views.go) — kept here too so a saved tile can be
// described even while its hub is unavailable and the source isn't listed.
export const SOURCE_LABELS: Record<string, string> = {
  'media.recordings': 'dashboard.source.recordings',
  'timers.upcoming': 'dashboard.source.timers',
  'guide.broadcasts': 'dashboard.source.guide',
  'discovery.picks': 'dashboard.source.discoveryPicks',
  'radio.now': 'dashboard.source.radioNow',
  'series.upcoming': 'dashboard.source.seriesUpcoming',
}

export function useViewText() {
  const { t, locale } = useI18n()
  const { timeZone } = useDisplayTimeZone()

  function zoneName(): string {
    return timeZone.value ?? Intl.DateTimeFormat().resolvedOptions().timeZone
  }

  function num(n: number) {
    return new Intl.NumberFormat(locale.value).format(n)
  }

  function describe(source: string, q: ViewQuery): string {
    const label = SOURCE_LABELS[source] ?? (source.startsWith('plex.library.') ? 'dashboard.source.plexLibrary' : source)
    const parts = [t(label)]
    if (q.text) parts.push(t('dashboard.criteria.text', { text: q.text }))
    if (q.olderThanDays) parts.push(t('dashboard.criteria.olderThanDays', { n: num(q.olderThanDays) }, q.olderThanDays))
    if (q.newerThanDays) parts.push(t('dashboard.criteria.newerThanDays', { n: num(q.newerThanDays) }, q.newerThanDays))
    if (q.minSizeGB) parts.push(t('dashboard.criteria.minSizeGB', { n: num(q.minSizeGB) }))
    if (q.withinDays) parts.push(t('dashboard.criteria.withinDays', { n: num(q.withinDays) }, q.withinDays))
    if (q.fromTime) parts.push(t('dashboard.criteria.fromTime', { time: q.fromTime }))
    if (q.minDuration) parts.push(t('dashboard.criteria.minDuration', { n: num(q.minDuration) }))
    if (q.warnOnly) parts.push(t(source === 'discovery.picks' ? 'dashboard.criteria.withoutTimer' : 'dashboard.criteria.warnOnly'))
    if (q.unwatched) parts.push(t('dashboard.criteria.unwatched'))
    if (q.sort) parts.push(t(`dashboard.sort.${q.sort}`))
    return parts.join(' · ')
  }

  return { describe, zoneName }
}
