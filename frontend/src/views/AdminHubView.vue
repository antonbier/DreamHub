<!--
  AdminHub's real screen (docs/V1_PLAN.md, WP6): DreamHub's own runtime log
  level, a journald log viewer (bounded, filtered server-side — the journal
  itself is the box's whole RAM buffer since boot, ARCHITECTURE.md,
  "journald is volatile"), and enigma2's own crash logs. Admin role only —
  same status-banner-plus-content pattern as the other hub screens. Live
  tail over WebSocket (ROADMAP.md) isn't built yet; this always reads a
  snapshot, with its own refresh button.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="admin" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ t(item ? item.label : 'hub.admin') }}</h1>
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

    <template v-if="!item || item.status !== 'unavailable'">
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
      <section class="mt-4">
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
            class="w-56 max-w-full rounded border border-line bg-field px-2.5 py-1.5 text-[13px] text-text"
            @change="onFilterChange"
          />
          <select v-model="logLevelFilter" class="rounded border border-line bg-field px-2.5 py-1.5 text-[13px] text-text" @change="onFilterChange">
            <option value="">{{ t('admin.logs.levelAny') }}</option>
            <option v-for="lv in logLevels" :key="lv" :value="lv">{{ t(`admin.logs.level.${lv}`) }}</option>
          </select>
          <input
            v-model="logText"
            type="search"
            :placeholder="t('admin.logs.textPlaceholder')"
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
            class="flex items-start gap-3 border-b border-line-soft py-1.5 last:border-none"
            :class="{ 'text-warn': e.level === 'warning', 'text-onair': ['emerg', 'alert', 'crit', 'err'].includes(e.level) }"
          >
            <span class="w-36 shrink-0 whitespace-nowrap text-faint">{{ formatDate(e.time) }}</span>
            <span class="w-40 shrink-0 truncate text-faint">{{ e.unit }}</span>
            <span class="min-w-0 flex-1 break-words">{{ e.message }}</span>
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
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import Icon from '@/components/Icon.vue'
import { formatBytes } from '@/lib/format'
import { openEventStream } from '@/composables/useEventStream'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { CrashLog, LogEntry, LogLevel } from '@/api/types'

const { t, locale } = useI18n()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()
const { timeZone } = useDisplayTimeZone()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/admin'))

function formatDate(iso: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'medium', timeZone: timeZone.value }).format(new Date(iso))
}

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
    logsError.value = t(`error.admin.${code}`) !== `error.admin.${code}` ? t(`error.admin.${code}`) : t('common.backendUnreachable')
  } finally {
    logsLoading.value = false
  }
}

// Live tail (ROADMAP.md, AdminHub — SSE, see backend's handleEventsStream
// doc comment for why not a real WebSocket). Starts from the current
// snapshot so switching it on doesn't lose what's already on screen, then
// appends each new line as the stream pushes it.
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
  const [levelRes, crashRes] = await Promise.all([api.admin.logLevel.get(), api.admin.crashLogs()])
  level.value = levelRes
  crashLogs.value = crashRes.logs
  await loadLogs()
})
</script>
