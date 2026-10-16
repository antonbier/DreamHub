<!--
  Backup & restore of DreamHub's own state (settings, dashboard, tokens …),
  on Settings → System. The backup downloads as a JSON file; restoring
  reads one back and previews what would change before anything is written
  (preview-and-confirm, PRINCIPLES.md). Used to live in SystemHub.
-->
<template>
  <section class="rounded-sm border border-line bg-tile p-5">
    <h2 class="text-[15.5px] font-semibold">{{ t('system.backup.title') }}</h2>
    <p class="mt-1 text-[13px] text-muted">{{ t('system.backup.body') }}</p>

    <!-- App6: opt-in sections beyond settings, each with its own real
         size shown before the admin includes it. -->
    <div v-if="appsSection && appsSection.items?.length" class="mt-3">
      <div class="flex items-center gap-2">
        <input id="backup-include-apps" v-model="includeApps" type="checkbox" class="rounded-sm border-line" />
        <label for="backup-include-apps" class="text-[13px] text-text">
          {{ t('system.backup.includeApps', { count: pickedItems.length, size: formatSize(appsTotalSize) }, pickedItems.length) }}
        </label>
      </div>
      <!-- Each app folder on its own: a database is in by default, an
           app's bulk files (Nextcloud's, downloads) are not. -->
      <ul v-if="includeApps" class="ms-6 mt-1.5 flex flex-col gap-1">
        <li v-for="it in appsSection.items" :key="it.id" class="flex items-center gap-2">
          <input :id="'backup-item-' + it.id" v-model="picked[it.id]" type="checkbox" class="rounded-sm border-line" />
          <label :for="'backup-item-' + it.id" class="text-[12.5px] text-muted">{{ it.name }} · {{ formatSize(it.sizeBytes) }}</label>
        </li>
      </ul>
    </div>

    <p v-if="withApps" class="mt-2 text-[12.5px] text-faint">{{ t('system.backup.appsPasswords') }}</p>

    <div class="mt-3 flex flex-wrap gap-2">
      <button type="button" class="rounded-sm border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field" @click="download">
        {{ t('system.backup.download') }}
      </button>
      <button
        v-if="stored"
        type="button"
        class="rounded-sm border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field disabled:opacity-60"
        :disabled="saving"
        @click="saveOnBox"
      >
        {{ saving ? t('system.backup.saving') : t('system.backup.saveOnBox') }}
      </button>
      <button
        type="button"
        class="rounded-sm border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field disabled:opacity-60"
        :disabled="restoring"
        @click="fileInput?.click()"
      >
        {{ restoring ? t('system.backup.restoring') : t('system.backup.restore') }}
      </button>
      <input ref="fileInput" type="file" accept="application/json" class="hidden" @change="onFileChosen" />
    </div>
    <p v-if="restoreError" class="mt-2 text-[12.5px] text-warn">{{ restoreError }}</p>
    <p v-if="notice" class="mt-2 text-[12.5px] text-muted">{{ notice }}</p>
    <p v-if="restored" class="mt-2 text-[12.5px] text-muted">{{ t('system.backup.restored') }}</p>

    <!-- App15: backups kept on the box itself (an app's data can be far too
         big to carry through a phone). -->
    <div v-if="stored" class="mt-4 border-t border-line-soft pt-3">
      <h3 class="text-[13px] font-semibold text-muted">{{ t('system.backup.storedTitle') }}</h3>
      <p v-if="stored.files.length === 0" class="mt-1 text-[12.5px] text-faint">{{ t('system.backup.storedEmpty') }}</p>
      <ul v-else class="mt-1.5 flex flex-col gap-1.5">
        <li v-for="f in stored.files" :key="f.name" class="flex flex-wrap items-center gap-x-3 gap-y-1 text-[12.5px]">
          <span class="min-w-0 flex-1 basis-48 text-text">{{ formatDate(f.createdAt) }} <span class="text-faint">· {{ formatSize(f.sizeBytes) }}</span></span>
          <button type="button" class="text-accent hover:underline disabled:opacity-60" :disabled="restoring" @click="restoreStored(f.name)">{{ t('system.backup.storedRestore') }}</button>
          <button type="button" class="text-accent hover:underline" @click="downloadStored(f.name)">{{ t('system.backup.storedDownload') }}</button>
          <button v-if="deleting !== f.name" type="button" class="text-onair hover:underline" @click="deleting = f.name">{{ t('system.backup.storedDelete') }}</button>
          <button v-else type="button" class="font-semibold text-onair hover:underline" @click="deleteStored(f.name)">{{ t('system.backup.storedDeleteSure') }}</button>
        </li>
      </ul>
      <p class="mt-1.5 text-[12px] text-faint">{{ t('system.backup.storedWhere', { dir: stored.dir }) }}</p>
    </div>
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
        <p v-if="restoreIncludesApps" class="mt-1 text-[12.5px] text-faint">{{ t('system.backup.restoreIncludesApps') }}</p>
        <p v-if="restoreIncludesAppList" class="mt-1 text-[12.5px] text-faint">{{ t('system.backup.restoreIncludesAppList') }}</p>
      </PlanPreview>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import PlanPreview from '@/components/PlanPreview.vue'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { usePlanError } from '@/composables/usePlanError'
import { formatBytes } from '@/lib/format'
import type { BackupFile, BackupSectionsList, Plan, StoredBackup } from '@/api/types'

// A restore changes settings: the page reloads them afterwards.
const emit = defineEmits<{ restored: [] }>()

const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()
const { planErrorMessage } = usePlanError()

function formatDate(iso: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'short', timeZone: timeZone.value }).format(new Date(iso))
}

function formatSize(bytes: number): string {
  return formatBytes(bytes, locale.value)
}

// App6: the "apps" backup section, if AppsHub has any installed app to
// offer — included by default (docs/APPSHUB_PLAN.md's own default),
// but always visible and always changeable, this screen's own version
// of the plan's running rule.
const sections = ref<BackupSectionsList['sections']>([])
const includeApps = ref(true)
const appsSection = computed(() => sections.value.find((s) => s.id === 'apps'))
// Which app folders go in — starts from each one's own default.
const picked = ref<Record<string, boolean>>({})
const pickedItems = computed(() => appsSection.value?.items?.filter((i) => picked.value[i.id]) ?? [])
const appsTotalSize = computed(() => pickedItems.value.reduce((sum, i) => sum + i.sizeBytes, 0))

// Backups kept on the box; null when the box has no place for them.
const stored = ref<{ dir: string; files: StoredBackup[] } | null>(null)
const notice = ref('')

async function loadStored() {
  try {
    stored.value = await api.system.storedBackups()
  } catch {
    stored.value = null
  }
}

async function reloadSections() {
  try {
    sections.value = (await api.system.backupSections()).sections
    picked.value = Object.fromEntries((appsSection.value?.items ?? []).map((i) => [i.id, i.defaultIncluded]))
  } catch {
    sections.value = []
  }
}

onMounted(async () => {
  await Promise.all([reloadSections(), loadStored()])
})

// Apps travel as two sections that the person sees as one: the folders they
// ticked, and the list of those apps (what was chosen for each, so they can
// be installed again on a new box).
const withApps = computed(() => !!appsSection.value && includeApps.value && pickedItems.value.length > 0)
function backupArgs(): [string[] | undefined, Record<string, string[]> | undefined] {
  if (!withApps.value) return [undefined, undefined]
  const ids = pickedItems.value.map((i) => i.id)
  return [['apps', 'appList'], { apps: ids, appList: [...new Set(ids.map((id) => id.split('.')[0]))] }]
}

function saveJSON(backup: BackupFile, name: string) {
  const blob = new Blob([JSON.stringify(backup, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

// Downloaded client-side as a plain JSON file — the browser never needs the
// endpoint's own Content-Disposition header since fetch() already parsed
// the body.
async function download() {
  const backup = await api.system.backup(...backupArgs())
  saveJSON(backup, `dreamhub-backup-${backup.exportedAt.slice(0, 10)}.json`)
}

const saving = ref(false)
async function saveOnBox() {
  saving.value = true
  restoreError.value = ''
  notice.value = ''
  try {
    const res = await api.system.saveBackup(...backupArgs())
    notice.value = t('system.backup.saved', { name: res.file.name })
    await loadStored()
  } catch (err) {
    restoreError.value = planErrorMessage(err)
  } finally {
    saving.value = false
  }
}

async function downloadStored(name: string) {
  try {
    saveJSON(await api.system.storedBackup(name), name)
  } catch (err) {
    restoreError.value = planErrorMessage(err)
  }
}

const deleting = ref('')
async function deleteStored(name: string) {
  deleting.value = ''
  try {
    await api.system.deleteStoredBackup(name)
    notice.value = t('system.backup.storedDeleted')
    await loadStored()
  } catch (err) {
    restoreError.value = planErrorMessage(err)
  }
}

async function restoreStored(name: string) {
  restoreError.value = ''
  notice.value = ''
  restored.value = false
  restorePlan.value = null
  restoring.value = true
  try {
    const res = await api.system.restoreStoredPlan(name)
    const file = stored.value?.files.find((f) => f.name === name)
    showPlan(res.plan, file?.createdAt ?? '', undefined)
  } catch (err) {
    restoreError.value = planErrorMessage(err)
  } finally {
    restoring.value = false
  }
}

function showPlan(plan: Plan, exportedAt: string, parsed?: BackupFile) {
  if (plan.changes.length === 0) {
    restoreError.value = t('system.backup.nothingToRestore')
    return
  }
  restorePlan.value = plan
  restoreExportedAt.value = exportedAt
  restoreIncludesApps.value = !!parsed?.sections?.apps
  restoreIncludesAppList.value = !!parsed?.sections?.appList
}

const fileInput = ref<HTMLInputElement | null>(null)
const restoring = ref(false)
const restoreError = ref('')
const restored = ref(false)
const restorePlan = ref<Plan | null>(null)
const restoreExportedAt = ref('')
const restoreIncludesApps = ref(false)
const restoreIncludesAppList = ref(false)
const applying = ref(false)
const applyError = ref('')

async function onFileChosen(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (fileInput.value) fileInput.value.value = ''
  if (!file) return
  restoreError.value = ''
  notice.value = ''
  restored.value = false
  restorePlan.value = null
  restoring.value = true
  try {
    const parsed = JSON.parse(await file.text()) as BackupFile
    const res = await api.system.restorePlan(parsed)
    showPlan(res.plan, parsed.exportedAt, parsed)
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
    await Promise.all([loadStored(), reloadSections()])
  } catch (err) {
    applyError.value = planErrorMessage(err)
  } finally {
    applying.value = false
  }
}
</script>
