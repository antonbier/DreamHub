// One line of a channel plan's preview in words ("ZDF HD moves from 14 to
// 2"), from backend/internal/hubs.ChannelChange.
import { useI18n } from 'vue-i18n'
import type { ChannelChange, PlanChange } from '@/api/types'

export function useChannelChange() {
  const { t, te } = useI18n()

  function describeChange(c: PlanChange): string {
    const before = (c.before ?? {}) as ChannelChange
    const after = (c.after ?? {}) as ChannelChange & { filename?: string }
    const key = `channels.change.${c.type}`
    if (!te(key)) return c.type
    return t(key, {
      name: before.name ?? after.name ?? '',
      newName: after.newName ?? '',
      from: before.from ?? '',
      to: after.to ?? '',
      filename: after.filename ?? '',
    })
  }

  return { describeChange }
}
