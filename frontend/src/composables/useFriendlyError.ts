// Errors in everyday words (docs/archive/PROFILES_PLAN.md, P3): what went wrong as
// the person in front of the screen understands it — "the box doesn't
// answer", not "502 webcontrol_unavailable". The technical part (code,
// request ID) stays available behind "Details" for whoever fixes it.
import { useI18n } from 'vue-i18n'
import { ApiError, lastError } from '@/api/client'

// Error codes with their own everyday sentence (error.friendly.<key>).
const BY_CODE: Record<string, string> = {
  network_error: 'dreamhub',
  webcontrol_unavailable: 'box',
  webcontrol_unauthorized: 'boxLogin',
  forbidden: 'forbidden',
  plex_offline: 'plex',
  plex_tv_unavailable: 'internet',
  stream_no_tuner: 'tuner',
  stream_busy: 'streams',
  hub_disabled: 'hubOff',
  not_found: 'notFound',
}

export interface FriendlyError {
  text: string
  // "code · request ID", empty when there's nothing technical to add.
  details: string
}

export function useFriendlyError() {
  const { t } = useI18n()

  function friendly(err: unknown): FriendlyError {
    if (!(err instanceof ApiError)) return { text: t('error.friendly.generic'), details: '' }
    let key = BY_CODE[err.code]
    if (!key) key = err.status >= 500 ? 'server' : 'generic'
    const details = [err.code, err.requestId].filter(Boolean).join(' · ')
    return { text: t(`error.friendly.${key}`), details }
  }

  // For the views that only remember "it failed": the last failed request
  // is almost always the one they made.
  function lastFriendly(): FriendlyError {
    return friendly(lastError.value)
  }

  return { friendly, lastFriendly }
}
