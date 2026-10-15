<!--
  GuideHub's timeline grid: a TV-magazine layout — several events per
  channel across a switchable four-hour window (GET /api/v1/guide/grid).

  Layout: each row is a flex box (a fixed-width channel column plus a
  flex-1 timeline area); event blocks are positioned inside that timeline
  area with percentage start/width, computed from the event's
  start/duration relative to the visible window. The "now" line is a
  single absolutely positioned element on the outer wrapper, offset past
  the channel column via calc() — see NAME_COL_REM — so it lines up with
  the timeline areas. Positions use inset-inline-start, not left, so the
  time axis flips with the text direction (PRINCIPLES.md). With a filter
  on, programmes that don't pass are dimmed rather than removed, so the
  grid keeps its shape.
-->
<template>
  <div>
    <div class="flex flex-wrap items-center gap-3">
      <div class="flex items-center gap-1.5">
        <button type="button" class="rounded-sm border border-line px-2.5 py-1 text-[13px] text-text hover:bg-tile" :aria-label="t('guide.earlier')" @click="shiftWindow(-WINDOW_HOURS)">
          {{ '←' }}
        </button>
        <button type="button" class="rounded-sm border border-line px-2.5 py-1 text-[13px] text-text hover:bg-tile" @click="jumpToNow">
          {{ t('guide.now') }}
        </button>
        <button type="button" class="rounded-sm border border-line px-2.5 py-1 text-[13px] text-text hover:bg-tile" :aria-label="t('guide.later')" @click="shiftWindow(WINDOW_HOURS)">
          {{ '→' }}
        </button>
      </div>
      <span class="text-[13px] text-muted">{{ windowLabel }}</span>
    </div>

    <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
    <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted"><ErrorText /></p>
    <p v-else-if="channels.length === 0" class="mt-5 text-[14px] text-muted">{{ t('guide.empty') }}</p>
    <div v-else class="relative mt-5">
      <div v-if="nowPercent !== null" class="absolute top-0 bottom-0 w-px bg-warn" :style="{ insetInlineStart: nowLeftStyle }" />

      <div class="flex">
        <div class="w-32 shrink-0" />
        <div class="relative h-6 flex-1 border-b border-line-soft">
          <span
            v-for="m in hourMarks"
            :key="m.label"
            class="absolute -translate-x-1/2 whitespace-nowrap text-[11px] text-faint rtl:translate-x-1/2"
            :style="{ insetInlineStart: m.leftPercent + '%' }"
          >
            {{ m.label }}
          </span>
        </div>
      </div>

      <div v-for="ch in channels" :key="ch.serviceReference" class="flex">
        <div class="flex w-32 shrink-0 items-center gap-1.5 py-1 pe-2">
          <ChannelLogo :service-reference="ch.serviceReference" :service-name="ch.serviceName" :height="16" />
          <span class="truncate text-[13px] font-semibold text-text">{{ ch.serviceName }}</span>
        </div>
        <div class="relative h-11 flex-1 border-b border-line-soft">
          <button
            v-for="(e, i) in ch.events"
            :key="i"
            type="button"
            class="absolute top-1 bottom-1 overflow-hidden text-ellipsis whitespace-nowrap rounded-sm px-1.5 py-1 text-start text-[12px] text-text hover:bg-tile-2"
            :class="[isSelected(e) ? 'bg-tile-2 ring-1 ring-accent' : 'bg-tile', { 'opacity-35': filterActive && !passes(e.title) }]"
            :style="eventStyle(e)"
            :title="e.inPlex ? `${e.title} · ${t('discovery.inPlex')}` : e.title"
            @click="$emit('select', e)"
          >
            <span v-if="e.inPlex" class="me-1 inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-muted align-middle" aria-hidden="true" />
            {{ e.title }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import ErrorText from '@/components/ErrorText.vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import ChannelLogo from './ChannelLogo.vue'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { ChannelSchedule, EPGEvent } from '@/api/types'

// Matches the w-32 (8rem) channel-column class on every row and the header.
const NAME_COL_REM = 8
const WINDOW_HOURS = 4

const props = defineProps<{
  bouquet: string
  // Unix seconds to open at (a search result's ?at=); else now.
  at?: number
  selected: EPGEvent | null
  filterActive: boolean
  passes: (title: string) => boolean
}>()
defineEmits<{ select: [e: EPGEvent] }>()

const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()

const hourOf = (s: number) => Math.floor(s / 3600) * 3600
const windowStart = ref(hourOf(props.at ?? Date.now() / 1000))
const windowEnd = computed(() => windowStart.value + WINDOW_HOURS * 3600)
const channels = ref<ChannelSchedule[]>([])
const loading = ref(true)
const loadFailed = ref(false)

watch(
  () => props.at,
  (at) => {
    if (at) windowStart.value = hourOf(at)
  },
)

function formatHour(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23', timeZone: timeZone.value }).format(unixSeconds * 1000)
}
const windowLabel = computed(() => `${formatHour(windowStart.value)} – ${formatHour(windowEnd.value)}`)

const hourMarks = computed(() => {
  const total = windowEnd.value - windowStart.value
  const marks: { label: string; leftPercent: number }[] = []
  for (let s = Math.ceil(windowStart.value / 3600) * 3600; s < windowEnd.value; s += 3600) {
    marks.push({ label: formatHour(s), leftPercent: ((s - windowStart.value) / total) * 100 })
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
  return { insetInlineStart: left + '%', width: width + '%' }
}

function isSelected(e: EPGEvent): boolean {
  return props.selected !== null && props.selected.id === e.id && props.selected.serviceReference === e.serviceReference
}

function shiftWindow(hours: number) {
  windowStart.value += hours * 3600
}
function jumpToNow() {
  windowStart.value = hourOf(Date.now() / 1000)
}

// Only the response to the latest request may update the grid: switching
// bouquet or window quickly can make an older answer arrive last.
let token = 0
async function load() {
  if (!props.bouquet) return
  const mine = ++token
  loading.value = true
  try {
    const res = await api.guide.grid(props.bouquet, windowStart.value, windowEnd.value)
    if (mine !== token) return
    channels.value = res.channels
    loadFailed.value = false
  } catch {
    if (mine === token) loadFailed.value = true
  } finally {
    if (mine === token) loading.value = false
  }
}
watch([() => props.bouquet, windowStart], load, { immediate: true })
</script>
