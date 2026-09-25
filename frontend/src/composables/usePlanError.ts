// Turns a failed plan/apply call into a user-facing message. Codes are
// translated under error.plan; a refusal by the box itself (box_rejected)
// also carries the box's own explanation — already in the box's OSD
// language — which is shown as-is after the translated sentence, since it
// usually names the clashing timer.
import { useI18n } from 'vue-i18n'
import { ApiError } from '@/api/client'

export function usePlanError() {
  const { t } = useI18n()

  function planErrorMessage(err: unknown): string {
    const code = err instanceof ApiError ? err.code : 'network_error'
    const details = err instanceof ApiError ? (err.details as Record<string, unknown> | undefined) : undefined
    // MediaHub's file errors live under error.media; "in use" says why.
    const keys = [`error.plan.${code}`, `error.media.${code}`]
    if (code === 'media_in_use' && typeof details?.reason === 'string') keys.unshift(`error.media.media_in_use_${details.reason}`)
    let message = code
    for (const key of keys) {
      const translated = t(key)
      if (translated !== key) {
        message = translated
        break
      }
    }
    const boxText = details?.boxText
    if (typeof boxText === 'string' && boxText !== '') {
      message += ' ' + t('error.plan.boxSays', { text: boxText })
    }
    return message
  }

  return { planErrorMessage }
}
