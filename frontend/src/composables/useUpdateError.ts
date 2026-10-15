// Plain sentences for the update codes (App14): the refusal while a recording
// runs (with the time it will be possible again), a step of "Update all" that
// failed, an update that did not run and was rolled back. Anything else goes
// through the ordinary plan error text.
import { useI18n } from 'vue-i18n'
import { ApiError } from '@/api/client'
import type { UpdateFailureDetails } from '@/api/types'
import { useDisplayTimeZone } from './useDisplayTimeZone'
import { usePlanError } from './usePlanError'

interface HeldDetails {
  reason?: string
  name?: string
  until?: number
}

export function useUpdateError() {
  const { t, locale } = useI18n()
  const { timeZone } = useDisplayTimeZone()
  const { planErrorMessage } = usePlanError()

  // "21:15", or with the weekday when it is not today.
  function whenText(until: number): string {
    const d = new Date(until * 1000)
    const sameDay = new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeZone: timeZone.value }).format(d) ===
      new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeZone: timeZone.value }).format(new Date())
    return new Intl.DateTimeFormat(locale.value, {
      ...(sameDay ? {} : { weekday: 'short' }),
      hour: '2-digit',
      minute: '2-digit',
      timeZone: timeZone.value,
    }).format(d)
  }

  // The sentence for a closed window: shown on the summary and, the same, in
  // the preview when an update is refused.
  function heldSentence(h: HeldDetails): string {
    const key = h.reason === 'starting' ? 'updates.held.starting' : 'updates.held.recording'
    return t(key, { name: h.name ?? '', time: h.until ? whenText(h.until) : '' })
  }

  // The report of an "Update all" that stopped: what was done, what failed and
  // why, what was not started.
  function failureLines(d: UpdateFailureDetails): string[] {
    const lines: string[] = []
    if (d.done?.length) lines.push(t('updates.result.done', { names: d.done.join(', ') }))
    for (const f of d.failed ?? []) {
      lines.push(t('updates.result.failed', { name: f.name, reason: f.reason }))
      if (f.kind === 'container') lines.push(t(f.rolledBack ? 'updates.result.restored' : 'updates.result.notRestored'))
    }
    if (d.skipped?.length) lines.push(t('updates.result.skipped', { names: d.skipped.join(', ') }))
    return lines
  }

  function updateErrorMessage(err: unknown): string {
    if (err instanceof ApiError) {
      const d = err.details as unknown as Record<string, unknown> | undefined
      if (err.code === 'updates_held' && d) return heldSentence(d as HeldDetails)
      if (err.code === 'updates_failed' && d) return failureLines(d as unknown as UpdateFailureDetails).join(' ')
      const reason = typeof d?.reason === 'string' && d.reason ? d.reason : err.message
      if (err.code === 'update_rolled_back') return t('updates.rolledBack', { reason })
      if (err.code === 'rollback_failed') return t('updates.rollbackFailed', { reason })
      if (err.code === 'update_failed') return t('updates.updateFailed', { reason })
    }
    return planErrorMessage(err)
  }

  return { heldSentence, failureLines, updateErrorMessage }
}
