// Text for the install form (docs/APPSHUB_PLAN.md, App12): compose issues
// and notes, field labels, elevated items and plan errors, in the reader's
// language. The backend sends stable codes; the English message it also
// sends is only the fallback for a code this file doesn't know.
import { useI18n } from 'vue-i18n'
import { ApiError } from '@/api/client'
import { usePlanError } from '@/composables/usePlanError'
import type { AppField, ComposeIssue, ElevatedItem } from '@/api/types'

// Plan error codes whose server message names the specifics (which port,
// how much memory, which image); it is appended to the translated sentence.
const DETAIL_CODES = new Set(['port_in_use', 'insufficient_memory', 'disk_full', 'image_not_found', 'arch_incompatible', 'install_refused'])

export function useAppForm() {
  const { t, te } = useI18n()
  const { planErrorMessage } = usePlanError()

  function issueText(i: ComposeIssue): string {
    const key = `apps.compose.issue.${i.code}`
    if (!te(key)) return i.message
    const where = i.service ? t('apps.compose.inService', { service: i.service }) : t('apps.compose.inFile')
    let reason = ''
    if (i.code === 'mount_refused' && i.detail && te(`apps.compose.mountReason.${i.detail}`)) reason = t(`apps.compose.mountReason.${i.detail}`)
    return t(key, {
      key: i.key ?? '',
      detail: i.detail ?? '',
      where,
      reason,
      plain: '${NAME}',
      withDefault: '${NAME:-default}',
    })
  }

  function fieldLabel(f: AppField): string {
    if (f.label !== f.name) return f.label
    const key = `apps.form.field.${f.name}.label`
    return te(key) ? t(key) : f.label
  }

  function fieldDescription(f: AppField): string {
    if (f.description) return f.description
    const key = `apps.form.field.${f.name}.description`
    return te(key) ? t(key) : ''
  }

  function elevatedText(e: ElevatedItem): string {
    const text = t(`apps.form.elevated.${e.kind}`, { value: e.value ?? '' })
    return e.service ? `${text} (${t('apps.form.inService', { service: e.service })})` : text
  }

  // What a failed plan or parse call says, plus the compose issues it carries.
  function failure(err: unknown): { message: string; issues: ComposeIssue[]; elevated: ElevatedItem[] } {
    let message = planErrorMessage(err)
    let issues: ComposeIssue[] = []
    let elevated: ElevatedItem[] = []
    if (err instanceof ApiError) {
      const details = err.details as unknown as { issues?: ComposeIssue[]; elevated?: ElevatedItem[] } | undefined
      issues = details?.issues ?? []
      elevated = details?.elevated ?? []
      if (DETAIL_CODES.has(err.code) && err.message) message += ` (${err.message})`
    }
    return { message, issues, elevated }
  }

  return { issueText, fieldLabel, fieldDescription, elevatedText, failure }
}
