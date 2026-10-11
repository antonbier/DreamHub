<!--
  DeviceHub ("Gerät", docs/ADMINHUB_PLAN.md, #47): SystemHub and AdminHub
  merged into one nav entry, admin-only (both already needed the same
  `system` permission — SystemHub was never actually visible to a plain
  user despite CONCEPT.md's older description, which is corrected now
  too). Same device, three tabs instead of two separate hub screens:
  - Übersicht: device info, the change log (ex-SystemHubView)
  - Diagnose: the self-test, every hub's status (ex-SystemHubView)
  - Administration: services, log level, journald log viewer, crash logs
    (ex-AdminHubView) — packages/feeds/scripts/terminal land here later
    (#47).
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="admin" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ hubTitle('device', 'hub.device') }}</h1>
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
      <button
        type="button"
        role="tab"
        :aria-selected="tab === 'admin'"
        class="border-b-2 px-3 py-2 text-[13.5px] font-semibold"
        :class="tab === 'admin' ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
        @click="tab = 'admin'"
      >
        {{ t('admin.tabs.admin') }}
      </button>
    </div>

    <template v-if="tab === 'overview'">
      <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted"><ErrorText /></p>
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

        <!-- Backup & restore, and About, moved to Settings → System: both
             are app-level, administrative information, not live device
             status. -->
        <p class="mt-4 text-[13px] text-muted">
          {{ t('system.backup.movedHint') }}
          <RouterLink to="/settings/system" class="text-accent hover:underline">{{ t('system.backup.openSettings') }}</RouterLink>
        </p>

        <!-- Change log -->
        <section class="mt-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <h2 class="text-[15px] font-semibold text-text">{{ t('system.changes.title') }}</h2>
            <div class="flex flex-wrap items-center gap-2">
              <label v-if="profiles.length > 1" class="flex items-center gap-1.5 text-[13px]">
                <span class="text-muted">{{ t('system.changes.by') }}</span>
                <select v-model="byUser" class="rounded border border-line bg-field px-2 py-1.5 text-text" @change="loadChanges">
                  <option :value="-1">{{ t('system.changes.everyone') }}</option>
                  <option v-for="p in profiles" :key="p.id" :value="p.id">{{ p.name }}</option>
                  <option :value="0">{{ t('system.changes.system') }}</option>
                </select>
              </label>
              <input
                v-model="filter"
                type="search"
                :placeholder="t('system.changes.filterPlaceholder')"
                class="w-64 max-w-full rounded border border-line bg-field px-2.5 py-1.5 text-[13px] text-text"
              />
            </div>
          </div>
          <p v-if="filteredChanges.length === 0" class="mt-2 text-[13px] text-faint">{{ t('system.changes.empty') }}</p>
          <ul v-else class="mt-2 flex flex-col">
            <li v-for="c in filteredChanges" :key="c.id" class="flex items-start gap-3 border-b border-line-soft py-2 last:border-none">
              <span class="w-36 shrink-0 whitespace-nowrap text-[12px] font-mono text-faint">{{ formatDate(c.at) }}</span>
              <span class="min-w-0 flex-1">
                <span class="block truncate text-[13.5px] text-text">{{ c.summary }}</span>
                <span class="block truncate text-[12px] text-faint">
                  <template v-if="profiles.length > 1">{{ c.userName || (c.userId ? t('system.changes.deletedProfile') : t('system.changes.system')) }} · </template>{{ c.action }}
                </span>
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

    <template v-else-if="tab === 'diagnostics'">
      <SelfTestPanel />
      <p v-if="diagLoading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="diagLoadFailed" class="mt-5 text-[14px] text-muted"><ErrorText /></p>
      <template v-else>
        <section v-for="group in diagGroups" :key="group.key" class="mt-5 first:mt-5">
          <h2 class="text-[15px] font-semibold text-text">{{ t(`diagnostics.group.${group.key}`) }} ({{ group.hubs.length }})</h2>
          <ul v-if="group.hubs.length > 0" class="mt-2 flex flex-col">
            <li v-for="hub in group.hubs" :key="hub.id" class="flex items-start gap-3 border-b border-line-soft py-2.5 last:border-none">
              <Icon :name="hub.icon" :size="18" class="mt-0.5 shrink-0 text-muted" />
              <span class="min-w-0 flex-1">
                <span class="flex items-center gap-2">
                  <span class="truncate text-[14px] text-text">{{ hubLabel(hub) }}</span>
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

    <template v-else-if="!item || item.status !== 'unavailable'">
      <ServicesPanel @logs="showLogsFor" />

      <!-- Log level -->
      <section class="mt-5 rounded border border-line bg-tile p-4">
        <h2 class="text-[14px] font-semibold text-text">{{ t('admin.logLevel.title') }}</h2>
        <p class="mt-0.5 text-[12.5px] text-muted">
          {{ level?.until ? t('admin.logLevel.currentUntil', { level: level.level, until: formatDate(level.until) }) : t('admin.logLevel.current', { level: level?.level }) }}
        </p>
        <div class="mt-3 flex flex-wrap gap-2">
          <button
            type="button"
            class="rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field disabled:opacity-60"
            :disabled="levelBusy || level?.level === 'debug'"
            @click="setLevel('debug')"
          >
            {{ t('admin.logLevel.debugFor', { minutes: 60 }) }}
          </button>
          <button
            type="button"
            class="rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field disabled:opacity-60"
            :disabled="levelBusy || level?.level === 'info'"
            @click="setLevel('info')"
          >
            {{ t('admin.logLevel.backToInfo') }}
          </button>
        </div>
      </section>

      <!-- Logs -->
      <section ref="logsSection" class="mt-4 scroll-mt-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <h2 class="text-[15px] font-semibold text-text">{{ t('admin.logs.title') }}</h2>
          <div class="flex items-center gap-2">
            <button
              type="button"
              class="flex items-center gap-1.5 rounded border px-3 py-1.5 text-[13px] hover:bg-field"
              :class="live ? 'border-accent text-accent' : 'border-line text-text'"
              @click="toggleLive"
            >
              <span class="h-1.5 w-1.5 rounded-full" :class="live ? 'bg-onair' : 'bg-faint'" />
              {{ t('admin.logs.live') }}
            </button>
            <button
              type="button"
              class="rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field disabled:opacity-60"
              :disabled="live"
              @click="loadLogs"
            >
              {{ t('admin.logs.refresh') }}
            </button>
          </div>
        </div>
        <p class="mt-1 text-[12.5px] text-faint">{{ t('admin.logs.sinceBoot') }}</p>
        <div class="mt-2 flex flex-wrap gap-2">
          <input
            v-model="logUnit"
            type="text"
            :placeholder="t('admin.logs.unitPlaceholder')"
            :aria-label="t('admin.logs.unitPlaceholder')"
            class="w-56 max-w-full rounded border border-line bg-field px-2.5 py-1.5 text-[13px] text-text"
            @change="onFilterChange"
          />
          <select v-model="logLevelFilter" :aria-label="t('admin.logs.levelFilter')" class="rounded border border-line bg-field px-2.5 py-1.5 text-[13px] text-text" @change="onFilterChange">
            <option value="">{{ t('admin.logs.levelAny') }}</option>
            <option v-for="lv in logLevels" :key="lv" :value="lv">{{ t(`admin.logs.level.${lv}`) }}</option>
          </select>
          <input
            v-model="logText"
            type="search"
            :placeholder="t('admin.logs.textPlaceholder')"
            :aria-label="t('admin.logs.textPlaceholder')"
            class="w-56 max-w-full rounded border border-line bg-field px-2.5 py-1.5 text-[13px] text-text"
            @change="onFilterChange"
          />
        </div>
        <p v-if="logsLoading" class="mt-3 text-[13px] text-faint">{{ t('common.loading') }}</p>
        <p v-else-if="logsError" class="mt-3 text-[13px] text-warn">{{ logsError }}</p>
        <p v-else-if="logs.length === 0" class="mt-3 text-[13px] text-faint">{{ t('admin.logs.empty') }}</p>
        <ul v-else class="mt-2 flex flex-col font-mono text-[12.5px]">
          <li
            v-for="(e, i) in logs"
            :key="i"
            class="flex items-start gap-x-3 border-b border-line-soft py-1.5 last:border-none below-sm:flex-wrap"
            :class="{ 'text-warn': e.level === 'warning', 'text-onair': ['emerg', 'alert', 'crit', 'err'].includes(e.level) }"
          >
            <span class="w-44 shrink-0 whitespace-nowrap text-faint below-sm:w-auto">{{ formatDate(e.time) }}</span>
            <span class="w-40 shrink-0 truncate text-faint below-sm:w-auto below-sm:min-w-0 below-sm:flex-1">{{ e.unit }}</span>
            <!-- On phones the message gets its own full-width line: squeezed
                 next to time and unit it would wrap after every letter. -->
            <span class="min-w-0 flex-1 break-words below-sm:basis-full">{{ e.message }}</span>
          </li>
        </ul>
      </section>

      <!-- Crash logs -->
      <section class="mt-4">
        <h2 class="text-[15px] font-semibold text-text">{{ t('admin.crashLogs.title') }}</h2>
        <p v-if="crashLogs.length === 0" class="mt-2 text-[13px] text-faint">{{ t('admin.crashLogs.empty') }}</p>
        <ul v-else class="mt-2 flex flex-col">
          <li v-for="c in crashLogs" :key="c.name" class="flex items-center gap-3 border-b border-line-soft py-2 last:border-none">
            <span class="min-w-0 flex-1">
              <span class="block truncate text-[13.5px] text-text">{{ c.name }}</span>
              <span class="block text-[12px] text-faint">{{ c.at ? formatDate(c.at) : '' }} · {{ formatBytes(c.size, locale) }}</span>
            </span>
            <button type="button" class="shrink-0 rounded border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field" @click="viewCrashLog(c.name)">
              {{ t('admin.crashLogs.view') }}
            </button>
          </li>
        </ul>
        <div v-if="openCrashLog" class="mt-3 rounded border border-line bg-tile p-3">
          <div class="mb-2 flex items-center justify-between gap-3">
            <span class="truncate text-[13px] font-semibold text-text">{{ openCrashLog.name }}</span>
            <button type="button" class="shrink-0 text-[12.5px] text-accent hover:underline" @click="openCrashLog = null">{{ t('admin.crashLogs.close') }}</button>
          </div>
          <pre class="max-h-96 overflow-auto whitespace-pre-wrap break-words font-mono text-[12px] text-text">{{ openCrashLog.content }}</pre>
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { useHubLabel } from '@/composables/useHubLabel'
import { useFriendlyError } from '@/composables/useFriendlyError'
import ErrorText from '@/components/ErrorText.vue'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { api, ApiError } from '@/api/client'
import Icon from '@/components/Icon.vue'
import ServicesPanel from '@/components/admin/ServicesPanel.vue'
import SelfTestPanel from '@/components/system/SelfTestPanel.vue'
import { formatBytes } from '@/lib/format'
import { openEventStream } from '@/composables/useEventStream'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { ChangeEntry, CrashLog, DeviceInfo, HubStatus, LogEntry, LogLevel, Profile } from '@/api/types'

const { t, locale } = useI18n()
const { hubLabel, hubTitle } = useHubLabel()
const { lastFriendly } = useFriendlyError()
const route = useRoute()
const router = useRouter()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()
const { timeZone } = useDisplayTimeZone()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/device'))

function formatDate(iso: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'medium', timeZone: timeZone.value }).format(new Date(iso))
}

type Tab = 'overview' | 'diagnostics' | 'admin'
const tab = ref<Tab>(route.query.tab === 'diagnostics' || route.query.tab === 'admin' ? route.query.tab : 'overview')
watch(tab, (t) => router.replace({ query: { ...route.query, tab: t === 'overview' ? undefined : t } }))

// --- overview (ex-SystemHubView) ---------------------------------------------

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
// A search result (?changes=) opens the change log already filtered.
const filter = ref(typeof route.query.changes === 'string' ? route.query.changes : '')
watch(
  () => route.query.changes,
  (q) => {
    if (typeof q === 'string') {
      filter.value = q
      tab.value = 'overview'
    }
  },
)
const loading = ref(true)
const loadFailed = ref(false)

const filteredChanges = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q) return changes.value
  return changes.value.filter((c) => c.summary.toLowerCase().includes(q) || c.action.toLowerCase().includes(q))
})

// Whose changes: everyone's (-1), a profile's, or the system's own (0) —
// the profile choice only shows once there's more than one profile.
const profiles = ref<Profile[]>([])
const byUser = ref(-1)
void api.profiles
  .list()
  .then((r) => (profiles.value = r.profiles))
  .catch(() => {})

async function loadChanges() {
  const res = await api.changes(changesLimit.value, byUser.value)
  changes.value = res.changes
}

async function loadMoreChanges() {
  changesLimit.value += 50
  await loadChanges()
}

// --- diagnostics (ex-SystemHubView) ------------------------------------------

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

// --- administration (ex-AdminHubView) ----------------------------------------

const level = ref<LogLevel | null>(null)
const levelBusy = ref(false)

async function setLevel(l: 'info' | 'debug') {
  levelBusy.value = true
  try {
    level.value = await api.admin.logLevel.set(l, l === 'debug' ? 60 : undefined)
  } finally {
    levelBusy.value = false
  }
}

const logLevels = ['emerg', 'alert', 'crit', 'err', 'warning', 'notice', 'info', 'debug'] as const
const logUnit = ref('')
const logLevelFilter = ref('')
const logText = ref('')
const logs = ref<LogEntry[]>([])
const logsLoading = ref(false)
const logsError = ref('')

async function loadLogs() {
  logsLoading.value = true
  logsError.value = ''
  try {
    const res = await api.admin.logs({ unit: logUnit.value, level: logLevelFilter.value, text: logText.value })
    logs.value = res.entries
  } catch (err) {
    const code = err instanceof ApiError ? err.code : 'network_error'
    logsError.value = t(`error.admin.${code}`) !== `error.admin.${code}` ? t(`error.admin.${code}`) : lastFriendly().text
  } finally {
    logsLoading.value = false
  }
}

// Live tail (SSE, see backend's handleEventsStream doc comment for why not
// a real WebSocket). Starts from the current snapshot so switching it on
// doesn't lose what's already on screen, then appends each new line as the
// stream pushes it.
const live = ref(false)
let stopStream: (() => void) | null = null

function toggleLive() {
  if (live.value) {
    stopLive()
  } else {
    startLive()
  }
}

function startLive() {
  stopStream?.()
  live.value = true
  logsError.value = ''
  const path = api.admin.logsStreamPath({ unit: logUnit.value, level: logLevelFilter.value, text: logText.value })
  stopStream = openEventStream(path, (e) => {
    if (e.event !== 'log') return
    try {
      logs.value = [...logs.value, JSON.parse(e.data) as LogEntry]
    } catch {
      // Malformed push — skip it, the rest of the tail keeps working.
    }
  })
}

function stopLive() {
  live.value = false
  stopStream?.()
  stopStream = null
}

// A service's "Logs": filter the viewer to that unit, switch to the
// Administration tab and bring the log section into view.
const logsSection = ref<HTMLElement | null>(null)
function showLogsFor(unit: string) {
  tab.value = 'admin'
  logUnit.value = unit
  onFilterChange()
  logsSection.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function onFilterChange() {
  if (live.value) {
    startLive() // re-subscribe with the new filters
  } else {
    void loadLogs()
  }
}

onUnmounted(stopLive)

const crashLogs = ref<CrashLog[]>([])
const openCrashLog = ref<{ name: string; content: string } | null>(null)

async function viewCrashLog(name: string) {
  openCrashLog.value = await api.admin.crashLog(name)
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
  const [levelRes, crashRes] = await Promise.all([api.admin.logLevel.get(), api.admin.crashLogs()])
  level.value = levelRes
  crashLogs.value = crashRes.logs
  await loadLogs()
})
</script>
