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
    const key = `error.plan.${code}`
    const translated = t(key)
    let message = translated === key ? code : translated
    const boxText = err instanceof ApiError ? (err.details as Record<string, unknown> | undefined)?.boxText : undefined
    if (typeof boxText === 'string' && boxText !== '') {
      message += ' ' + t('error.plan.boxSays', { text: boxText })
    }
    return message
  }

  return { planErrorMessage }
}
