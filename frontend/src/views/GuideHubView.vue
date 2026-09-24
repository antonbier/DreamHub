<!--
  GuideHub's real screen (ROADMAP.md, "v1 — hubs"): a TV-magazine-style
  timeline grid — several events per channel across a switchable time
  window, not just now/next (GET /api/v1/guide/bouquets,
  GET /api/v1/guide/grid). Same status-banner-plus-content pattern as
  MediaHubView.vue; see its comment for why they're independent.

  Layout: each row is a flex box (a fixed-width channel-name column plus a
  flex-1 timeline area); event blocks are positioned inside that timeline
  area with percentage left/width, computed from the event's start/duration
  relative to the visible window. The "now" line is a single absolutely
  positioned element on the outer wrapper, offset past the name column via
  calc() — see NAME_COL_REM — so it lines up with the timeline areas
  without fighting CSS grid auto-placement.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="guide" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ t(item ? item.label : 'hub.guide') }}</h1>
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
      <div class="mt-5 flex flex-wrap items-center gap-3">
        <select
          v-if="bouquets.length > 1"
          v-model="selectedBouquet"
          class="rounded border border-line bg-field px-3 py-1.5 text-[13.5px] text-text"
        >
          <option v-for="b in bouquets" :key="b.serviceReference" :value="b.serviceReference">{{ b.serviceName }}</option>
        </select>
        <div class="flex items-center gap-1.5">
          <button type="button" class="rounded border border-line px-2.5 py-1 text-[13px] text-text hover:bg-tile" @click="shiftWindow(-WINDOW_HOURS)">
            {{ '←' }}
          </button>
          <button type="button" class="rounded border border-line px-2.5 py-1 text-[13px] text-text hover:bg-tile" @click="jumpToNow">
            {{ t('guide.now') }}
          </button>
          <button type="button" class="rounded border border-line px-2.5 py-1 text-[13px] text-text hover:bg-tile" @click="shiftWindow(WINDOW_HOURS)">
            {{ '→' }}
          </button>
        </div>
        <span class="text-[13px] text-muted">{{ windowLabel }}</span>
      </div>

      <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted">{{ t('common.backendUnreachable') }}</p>
      <p v-else-if="channels.length === 0" class="mt-5 text-[14px] text-muted">{{ t('guide.empty') }}</p>
      <div v-else class="relative mt-5">
        <div v-if="nowPercent !== null" class="absolute top-0 bottom-0 w-px bg-warn" :style="{ left: nowLeftStyle }" />

        <div class="flex">
          <div class="w-28 shrink-0" />
          <div class="relative h-6 flex-1 border-b border-line-soft">
            <span
              v-for="m in hourMarks"
              :key="m.label"
              class="absolute -translate-x-1/2 whitespace-nowrap text-[11px] text-faint"
              :style="{ left: m.leftPercent + '%' }"
            >
              {{ m.label }}
            </span>
          </div>
        </div>

        <div v-for="ch in channels" :key="ch.serviceReference" class="flex">
          <div class="w-28 shrink-0 truncate py-1 pe-2 text-[13px] font-semibold text-text">{{ ch.serviceName }}</div>
          <div class="relative h-11 flex-1 border-b border-line-soft">
            <div
              v-for="(e, i) in ch.events"
              :key="i"
              class="absolute top-1 bottom-1 overflow-hidden text-ellipsis whitespace-nowrap rounded bg-tile px-1.5 py-1 text-[12px] text-text"
              :style="eventStyle(e)"
              :title="e.title"
            >
              {{ e.title }}
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { Bouquet, ChannelSchedule, EPGEvent } from '@/api/types'

// Matches the w-28 (7rem) name-column class on every row and header below —
// keep both in sync if either changes.
const NAME_COL_REM = 7
const WINDOW_HOURS = 4

const { t, locale } = useI18n()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()
const { timeZone } = useDisplayTimeZone()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/guide'))

const bouquets = ref<Bouquet[]>([])
const selectedBouquet = ref('')
const channels = ref<ChannelSchedule[]>([])
const windowStart = ref(Math.floor(Date.now() / 1000 / 3600) * 3600)
const loading = ref(true)
const loadFailed = ref(false)

const windowEnd = computed(() => windowStart.value + WINDOW_HOURS * 3600)

function formatHour(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23', timeZone: timeZone.value }).format(
    unixSeconds * 1000,
  )
}

const windowLabel = computed(() => `${formatHour(windowStart.value)} – ${formatHour(windowEnd.value)}`)

const hourMarks = computed(() => {
  const total = windowEnd.value - windowStart.value
  const marks: { label: string; leftPercent: number }[] = []
  for (let t = Math.ceil(windowStart.value / 3600) * 3600; t < windowEnd.value; t += 3600) {
    marks.push({ label: formatHour(t), leftPercent: ((t - windowStart.value) / total) * 100 })
  }
  return marks
})

const nowPercent = computed(() => {
  const now = Date.now() / 1000
  if (now < windowStart.value || now > windowEnd.value) return null
  return ((now - windowStart.value) / (windowEnd.value - windowStart.value)) * 100
})
const nowLeftStyle = computed(() => `calc(${NAME_COL_REM}rem + (100% - ${NAME_COL_REM}rem) * ${(nowPercent.value ?? 0) / 100})`)

function eventStyle(e: EPGEvent) {
  const total = windowEnd.value - windowStart.value
  const clampedStart = Math.max(e.start, windowStart.value)
  const clampedEnd = Math.min(e.start + e.duration, windowEnd.value)
  const left = ((clampedStart - windowStart.value) / total) * 100
  const width = Math.max(((clampedEnd - clampedStart) / total) * 100, 1)
  return { left: left + '%', width: width + '%' }
}

function shiftWindow(hours: number) {
  windowStart.value += hours * 3600
}
function jumpToNow() {
  windowStart.value = Math.floor(Date.now() / 1000 / 3600) * 3600
}

// Guards against out-of-order responses: rapidly switching bouquet/window
// (e.g. double-clicking the arrows) can make an older request's response
// arrive after a newer one's — only the response matching the latest call
// may update state.
let loadGridToken = 0

async function loadGrid() {
  if (!selectedBouquet.value) return
  const token = ++loadGridToken
  loading.value = true
  try {
    const res = await api.guide.grid(selectedBouquet.value, windowStart.value, windowEnd.value)
    if (token !== loadGridToken) return
    channels.value = res.channels
    loadFailed.value = false
  } catch {
    if (token !== loadGridToken) return
    loadFailed.value = true
  } finally {
    if (token === loadGridToken) loading.value = false
  }
}

watch([selectedBouquet, windowStart], loadGrid)

onMounted(async () => {
  try {
    const res = await api.guide.bouquets()
    bouquets.value = res.bouquets
    if (res.bouquets.length > 0) {
      selectedBouquet.value = res.bouquets[0].serviceReference
    } else {
      loading.value = false
    }
  } catch {
    loadFailed.value = true
    loading.value = false
  }
})
</script>
