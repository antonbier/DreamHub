<!--
  Backup & restore of DreamHub's own state (settings, dashboard, tokens …),
  on Settings → System. The backup downloads as a JSON file; restoring
  reads one back and previews what would change before anything is written
  (preview-and-confirm, PRINCIPLES.md). Used to live in SystemHub.
-->
<template>
  <section class="rounded border border-line bg-tile p-5">
    <h2 class="text-[15.5px] font-semibold">{{ t('system.backup.title') }}</h2>
    <p class="mt-1 text-[13px] text-muted">{{ t('system.backup.body') }}</p>
    <div class="mt-3 flex flex-wrap gap-2">
      <button type="button" class="rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field" @click="download">
        {{ t('system.backup.download') }}
      </button>
      <button
        type="button"
        class="rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field disabled:opacity-60"
        :disabled="restoring"
        @click="fileInput?.click()"
      >
        {{ restoring ? t('system.backup.restoring') : t('system.backup.restore') }}
      </button>
      <input ref="fileInput" type="file" accept="application/json" class="hidden" @change="onFileChosen" />
    </div>
    <p v-if="restoreError" class="mt-2 text-[12.5px] text-warn">{{ restoreError }}</p>
    <p v-if="restored" class="mt-2 text-[12.5px] text-muted">{{ t('system.backup.restored') }}</p>
    <div v-if="restorePlan" class="mt-3">
      <PlanPreview
        :plan="restorePlan"
        :confirm-label="t('system.backup.confirmRestore')"
        :busy="applying"
        :error="applyError"
        @confirm="applyRestore"
        @cancel="restorePlan = null"
      >
        {{ t('system.backup.restorePreview', { count: restorePlan.changes.length, date: formatDate(restoreExportedAt) }, restorePlan.changes.length) }}
      </PlanPreview>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import PlanPreview from '@/components/PlanPreview.vue'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { usePlanError } from '@/composables/usePlanError'
import type { BackupFile, Plan } from '@/api/types'

// A restore changes settings: the page reloads them afterwards.
const emit = defineEmits<{ restored: [] }>()

const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()
const { planErrorMessage } = usePlanError()

function formatDate(iso: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'short', timeZone: timeZone.value }).format(new Date(iso))
}

// Downloaded client-side as a plain JSON file — the browser never needs the
// endpoint's own Content-Disposition header since fetch() already parsed
// the body.
async function download() {
  const backup = await api.system.backup()
  const blob = new Blob([JSON.stringify(backup, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `dreamhub-backup-${backup.exportedAt.slice(0, 10)}.json`
  a.click()
  URL.revokeObjectURL(url)
}

const fileInput = ref<HTMLInputElement | null>(null)
const restoring = ref(false)
const restoreError = ref('')
const restored = ref(false)
const restorePlan = ref<Plan | null>(null)
const restoreExportedAt = ref('')
const applying = ref(false)
const applyError = ref('')

async function onFileChosen(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (fileInput.value) fileInput.value.value = ''
  if (!file) return
  restoreError.value = ''
  restored.value = false
  restorePlan.value = null
  restoring.value = true
  try {
    const parsed = JSON.parse(await file.text()) as BackupFile
    const res = await api.system.restorePlan(parsed)
    if (res.plan.changes.length === 0) {
      restoreError.value = t('system.backup.nothingToRestore')
    } else {
      restorePlan.value = res.plan
      restoreExportedAt.value = parsed.exportedAt
    }
  } catch (err) {
    restoreError.value = err instanceof SyntaxError ? t('error.plan.invalid_backup') : planErrorMessage(err)
  } finally {
    restoring.value = false
  }
}

async function applyRestore() {
  if (!restorePlan.value) return
  applying.value = true
  applyError.value = ''
  try {
    await api.plans.apply(restorePlan.value.id)
    restorePlan.value = null
    restored.value = true
    emit('restored')
  } catch (err) {
    applyError.value = planErrorMessage(err)
  } finally {
    applying.value = false
  }
}
</script>
