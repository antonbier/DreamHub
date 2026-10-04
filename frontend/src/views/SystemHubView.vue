<!--
  SystemHub's real screen (docs/V1_PLAN.md, WP5/WP6): device info
  (GET /api/v1/deviceinfo), backup/restore of DreamHub's own state
  (preview-and-confirm, PRINCIPLES.md), the change log
  (GET /api/v1/changes) with a client-side filter, and — as a second tab —
  diagnostics (GET /api/v1/hubs), which used to be its own core route.
  CONCEPT.md already describes diagnostics as "a diagnostics page in
  SystemHub"; this folds the two together instead of keeping near-duplicate
  hub-status UI in two places. Same status-banner-plus-content pattern as
  SeriesHubView.vue.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="system" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ t(item ? item.label : 'hub.system') }}</h1>
    </div>

    <div v-if="item && item.status && item.status !== 'available'" class="mt-4 rounded border border-line bg-tile p-4">
      <div class="mb-1.5 flex items-center gap-2 text-[13.5px] font-semibold">
        <span class="h-1.5 w-1.5 rounded-full" :class="item.status === 'degraded' ? 'bg-warn' : 'bg-faint'" />
        {{ t(`status.${item.status}`) }}
      </div>
      <ul class="flex flex-col gap-1 text-[13px] text-muted">
        <li v-for="(p, i) in item.problems ?? []" :key="i">
          {{ describe(p) }}
          <span v-if="describeFix(p)" class="text-faint">{{ ' — ' + describeFix(p) }}</span>
        </li>
      </ul>
    </div>

    <div class="mt-4 flex gap-1 border-b border-line-soft" role="tablist">
      <button
        type="button"
        role="tab"
        :aria-selected="tab === 'overview'"
        class="border-b-2 px-3 py-2 text-[13.5px] font-semibold"
        :class="tab === 'overview' ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
        @click="tab = 'overview'"
      >
        {{ t('system.tabs.overview') }}
      </button>
      <button
        type="button"
        role="tab"
        :aria-selected="tab === 'diagnostics'"
        class="border-b-2 px-3 py-2 text-[13.5px] font-semibold"
        :class="tab === 'diagnostics' ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
        @click="tab = 'diagnostics'"
      >
        {{ t('diagnostics.title') }}
      </button>
    </div>

    <template v-if="tab === 'overview'">
      <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted">{{ t('common.backendUnreachable') }}</p>
      <template v-else>
        <!-- Device info -->
        <section class="mt-5 rounded border border-line bg-tile p-4">
          <h2 class="text-[14px] font-semibold text-text">{{ t('system.device.title') }}</h2>
          <!-- Label and value side by side; one under the other on phones. -->
          <dl class="mt-2 grid grid-cols-[max-content_1fr] gap-x-6 gap-y-1.5 text-[13px] below-sm:grid-cols-1 below-sm:gap-y-0">
            <template v-for="row in deviceRows" :key="row.label">
              <dt class="text-muted below-sm:mt-1.5">{{ row.label }}</dt>
              <dd class="min-w-0 break-words text-text">{{ row.value }}</dd>
            </template>
          </dl>
        </section>

        <!-- Backup & restore -->
        <section class="mt-4 rounded border border-line bg-tile p-4">
          <h2 class="text-[14px] font-semibold text-text">{{ t('system.backup.title') }}</h2>
          <p class="mt-0.5 text-[12.5px] text-muted">{{ t('system.backup.body') }}</p>
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

        <!-- Change log -->
        <section class="mt-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <h2 class="text-[15px] font-semibold text-text">{{ t('system.changes.title') }}</h2>
            <input
              v-model="filter"
              type="search"
              :placeholder="t('system.changes.filterPlaceholder')"
              class="w-64 max-w-full rounded border border-line bg-field px-2.5 py-1.5 text-[13px] text-text"
            />
          </div>
          <p v-if="filteredChanges.length === 0" class="mt-2 text-[13px] text-faint">{{ t('system.changes.empty') }}</p>
          <ul v-else class="mt-2 flex flex-col">
            <li v-for="c in filteredChanges" :key="c.id" class="flex items-start gap-3 border-b border-line-soft py-2 last:border-none">
              <span class="w-36 shrink-0 whitespace-nowrap text-[12px] font-mono text-faint">{{ formatDate(c.at) }}</span>
              <span class="min-w-0 flex-1">
                <span class="block truncate text-[13.5px] text-text">{{ c.summary }}</span>
                <span class="block truncate text-[12px] text-faint">{{ c.action }}</span>
              </span>
            </li>
          </ul>
          <button
            v-if="changes.length >= changesLimit"
            type="button"
            class="mt-3 rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field"
            @click="loadMoreChanges"
          >
            {{ t('system.changes.loadMore') }}
          </button>
        </section>
      </template>
    </template>

    <template v-else>
      <SelfTestPanel />
      <p v-if="diagLoading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="diagLoadFailed" class="mt-5 text-[14px] text-muted">{{ t('common.backendUnreachable') }}</p>
      <template v-else>
        <section v-for="group in diagGroups" :key="group.key" class="mt-5 first:mt-5">
          <h2 class="text-[15px] font-semibold text-text">{{ t(`diagnostics.group.${group.key}`) }} ({{ group.hubs.length }})</h2>
          <ul v-if="group.hubs.length > 0" class="mt-2 flex flex-col">
            <li v-for="hub in group.hubs" :key="hub.id" class="flex items-start gap-3 border-b border-line-soft py-2.5 last:border-none">
              <Icon :name="hub.icon" :size="18" class="mt-0.5 shrink-0 text-muted" />
              <span class="min-w-0 flex-1">
                <span class="flex items-center gap-2">
                  <span class="truncate text-[14px] text-text">{{ t(hub.label) }}</span>
                  <span v-if="!hub.enabled" class="shrink-0 text-[12px] text-faint">{{ t('diagnostics.disabled') }}</span>
                </span>
                <ul v-if="hub.problems.length > 0" class="mt-0.5 flex flex-col gap-0.5 text-[12.5px] text-muted">
                  <li v-for="(p, i) in hub.problems" :key="i">
                    {{ describe(p) }}
                    <span v-if="describeFix(p)" class="text-faint">{{ ' — ' + describeFix(p) }}</span>
                  </li>
                </ul>
              </span>
            </li>
          </ul>
          <p v-else class="mt-2 text-[13px] text-faint">{{ t('diagnostics.groupEmpty') }}</p>
        </section>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import SelfTestPanel from '@/components/system/SelfTestPanel.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { usePlanError } from '@/composables/usePlanError'
import type { BackupFile, ChangeEntry, DeviceInfo, HubStatus, Plan } from '@/api/types'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()
const { timeZone } = useDisplayTimeZone()
const { planErrorMessage } = usePlanError()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/system'))

type Tab = 'overview' | 'diagnostics'
const tab = ref<Tab>(route.query.tab === 'diagnostics' ? 'diagnostics' : 'overview')
watch(tab, (t) => router.replace({ query: { ...route.query, tab: t === 'overview' ? undefined : t } }))

const device = ref<DeviceInfo | null>(null)
const deviceRows = computed(() => {
  const d = device.value
  if (!d) return []
  const tuners = d.frontends.map((f) => f.model.trim()).filter(Boolean)
  return [
    { label: t('system.device.model'), value: `Dreambox ${d.deviceName}` },
    { label: t('system.device.imageLabel'), value: d.imageVersion },
    { label: 'Enigma2', value: d.enigmaVersion },
    { label: 'WebControl', value: d.webifVersion },
    { label: t('system.device.tuners', { count: d.frontends.length }, d.frontends.length), value: tuners.join(', ') || '—' },
  ]
})
const changes = ref<ChangeEntry[]>([])
const changesLimit = ref(50)
const filter = ref('')
const loading = ref(true)
const loadFailed = ref(false)

const filteredChanges = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q) return changes.value
  return changes.value.filter((c) => c.summary.toLowerCase().includes(q) || c.action.toLowerCase().includes(q))
})

function formatDate(iso: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'short', timeZone: timeZone.value }).format(new Date(iso))
}

async function loadChanges() {
  const res = await api.changes(changesLimit.value)
  changes.value = res.changes
}

async function loadMoreChanges() {
  changesLimit.value += 50
  await loadChanges()
}

onMounted(async () => {
  try {
    const [deviceRes] = await Promise.all([api.deviceInfo(), loadChanges()])
    device.value = deviceRes
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
})

// Diagnostics tab: GET /api/v1/hubs, the same data and grouping the old
// standalone /diagnostics route used. Loaded once, on first visit to the
// tab, not eagerly — most visits to SystemHub are for the overview.
const diagHubs = ref<HubStatus[]>([])
const diagLoading = ref(true)
const diagLoadFailed = ref(false)
const diagLoaded = ref(false)

const diagGroups = computed(() => [
  { key: 'unavailable', hubs: diagHubs.value.filter((h) => h.enabled && h.status === 'unavailable') },
  { key: 'degraded', hubs: diagHubs.value.filter((h) => h.enabled && h.status === 'degraded') },
  { key: 'disabled', hubs: diagHubs.value.filter((h) => !h.enabled) },
  { key: 'available', hubs: diagHubs.value.filter((h) => h.enabled && h.status === 'available') },
])

async function loadDiagnostics() {
  if (diagLoaded.value) return
  diagLoaded.value = true
  try {
    const res = await api.hubs()
    diagHubs.value = res.hubs
  } catch {
    diagLoadFailed.value = true
  } finally {
    diagLoading.value = false
  }
}

watch(tab, (t) => { if (t === 'diagnostics') loadDiagnostics() }, { immediate: true })

// Backup: downloaded client-side as a plain JSON file — the browser never
// needs the endpoint's own Content-Disposition header since fetch() already
// parsed the body.
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
    const text = await file.text()
    const parsed = JSON.parse(text) as BackupFile
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
    await loadChanges()
  } catch (err) {
    applyError.value = planErrorMessage(err)
  } finally {
    applying.value = false
  }
}
</script>
