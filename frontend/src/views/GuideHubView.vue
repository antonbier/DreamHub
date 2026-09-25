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
  without fighting CSS grid auto-placement. Positions use inset-inline-start,
  not left, so the time axis flips with the text direction (PRINCIPLES.md).

  Clicking a programme opens a small panel with its details and "Record",
  which goes through TimerHub's own plan/apply (CONCEPT.md: hubs talk
  through the core API) with the EPG event id, so the box links the timer to
  the programme. ?at=<unix seconds> (global search results) opens the grid
  at that time.
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

      <div v-if="selected" class="mt-4 rounded border border-line bg-tile p-4">
        <div class="flex items-start gap-3">
          <div class="min-w-0 flex-1">
            <span class="block text-[15.5px] font-semibold text-text">{{ selected.title }}</span>
            <span class="block text-[13px] text-muted">{{ selected.serviceName }} · {{ eventTimeRange(selected) }}</span>
            <span v-if="selected.description" class="mt-1 block text-[13.5px] text-text">{{ selected.description }}</span>
          </div>
          <button type="button" class="shrink-0 rounded px-2 py-1 text-[13px] text-muted hover:bg-tile-2" @click="closeSelected">
            {{ t('plan.close') }}
          </button>
        </div>
        <div class="mt-3">
          <p v-if="record.status === 'created'" class="text-[13.5px] text-ok" role="status">{{ t('guide.timerCreated') }}</p>
          <PlanPreview
            v-else-if="record.plan"
            :plan="record.plan"
            :confirm-label="t('guide.confirmRecord')"
            :busy="record.status === 'applying'"
            :error="record.error"
            @confirm="confirmRecord"
            @cancel="record = { status: 'idle' }"
          >
            {{ t('guide.recordPreview', { name: selected.title, channel: selected.serviceName, when: recordWindow }) }}
          </PlanPreview>
          <template v-else>
            <div class="flex flex-wrap items-center gap-2">
              <button
                v-if="!selectedEnded"
                type="button"
                class="rounded bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60"
                :disabled="record.status === 'loading' || !canRecord"
                :title="canRecord ? undefined : t('guide.timerHubUnavailable')"
                @click="previewRecord"
              >
                {{ t('guide.record') }}
              </button>
              <button
                v-if="selectedLive"
                type="button"
                class="rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field disabled:opacity-60"
                :disabled="zapping"
                @click="zapToSelected"
              >
                {{ zapped ? t('guide.switchedNow') : t('guide.switchNow') }}
              </button>
            </div>
            <p v-if="record.error" class="mt-2 text-[12.5px] text-warn">{{ record.error }}</p>
            <p v-if="zapError" class="mt-2 text-[12.5px] text-warn">{{ zapError }}</p>
          </template>
        </div>
      </div>

      <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted">{{ t('common.backendUnreachable') }}</p>
      <p v-else-if="channels.length === 0" class="mt-5 text-[14px] text-muted">{{ t('guide.empty') }}</p>
      <div v-else class="relative mt-5">
        <div v-if="nowPercent !== null" class="absolute top-0 bottom-0 w-px bg-warn" :style="{ insetInlineStart: nowLeftStyle }" />

        <div class="flex">
          <div class="w-28 shrink-0" />
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
          <div class="w-28 shrink-0 truncate py-1 pe-2 text-[13px] font-semibold text-text">{{ ch.serviceName }}</div>
          <div class="relative h-11 flex-1 border-b border-line-soft">
            <button
              v-for="(e, i) in ch.events"
              :key="i"
              type="button"
              class="absolute top-1 bottom-1 overflow-hidden text-ellipsis whitespace-nowrap rounded px-1.5 py-1 text-start text-[12px] text-text hover:bg-tile-2"
              :class="isSelected(e) ? 'bg-tile-2 ring-1 ring-accent' : 'bg-tile'"
              :style="eventStyle(e)"
              :title="e.title"
              @click="select(e)"
            >
              {{ e.title }}
            </button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import { usePlanError } from '@/composables/usePlanError'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { Bouquet, ChannelSchedule, EPGEvent, Plan } from '@/api/types'

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
  return { insetInlineStart: left + '%', width: width + '%' }
}

// --- selected programme → record (TimerHub's plan/apply) ------------------

const route = useRoute()
const { planErrorMessage } = usePlanError()

// Recording needs TimerHub; GuideHub itself works without it.
const canRecord = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.id === 'timer')?.status !== 'unavailable')

const selected = ref<EPGEvent | null>(null)
type RecordState = { status: 'idle' | 'loading' | 'previewing' | 'applying' | 'created'; plan?: Plan; error?: string }
const record = ref<RecordState>({ status: 'idle' })

const selectedEnded = computed(() => (selected.value ? selected.value.start + selected.value.duration <= Date.now() / 1000 : false))
// "Switch now" only makes sense for a programme actually airing right now
// (ControlHub's zap, pulled forward from v1.1 — docs/V1_PLAN.md).
const selectedLive = computed(() => {
  const e = selected.value
  if (!e) return false
  const now = Date.now() / 1000
  return e.start <= now && now < e.start + e.duration
})
const zapping = ref(false)
const zapError = ref('')
const zapped = ref(false)
async function zapToSelected() {
  const e = selected.value
  if (!e) return
  zapping.value = true
  zapError.value = ''
  try {
    await api.control.zap(e.serviceReference)
    zapped.value = true
  } catch (err) {
    zapError.value = planErrorMessage(err)
  } finally {
    zapping.value = false
  }
}

function isSelected(e: EPGEvent): boolean {
  return selected.value !== null && selected.value.id === e.id && selected.value.serviceReference === e.serviceReference
}

function eventTimeRange(e: EPGEvent): string {
  return `${formatHour(e.start)} – ${formatHour(e.start + e.duration)}`
}

// The recording window as planned (margins included), from the plan itself.
const recordWindow = computed(() => {
  const after = record.value.plan?.changes[0]?.after as { recordBegin?: number; recordEnd?: number } | undefined
  return after?.recordBegin && after?.recordEnd ? `${formatHour(after.recordBegin)} – ${formatHour(after.recordEnd)}` : ''
})

function select(e: EPGEvent) {
  selected.value = e
  record.value = { status: 'idle' }
  zapped.value = false
  zapError.value = ''
}
function closeSelected() {
  selected.value = null
  record.value = { status: 'idle' }
}

async function previewRecord() {
  const e = selected.value
  if (!e) return
  record.value = { status: 'loading' }
  try {
    const res = await api.timerPlan({
      serviceReference: e.serviceReference,
      serviceName: e.serviceName,
      name: e.title,
      description: e.description,
      begin: e.start,
      end: e.start + e.duration,
      eventId: e.id,
    })
    if (selected.value === e) record.value = { status: 'previewing', plan: res.plan }
  } catch (err) {
    if (selected.value === e) record.value = { status: 'idle', error: planErrorMessage(err) }
  }
}

async function confirmRecord() {
  const plan = record.value.plan
  if (!plan) return
  record.value = { status: 'applying', plan }
  try {
    await api.plans.apply(plan.id)
    record.value = { status: 'created' }
  } catch (err) {
    // The plan is used up either way; "Record" starts a fresh preview.
    record.value = { status: 'idle', error: planErrorMessage(err) }
  }
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

// A search result while already on /guide only changes the query.
watch(
  () => route.query.at,
  (value) => {
    const at = Number(value)
    if (Number.isFinite(at) && at > 0) windowStart.value = Math.floor(at / 3600) * 3600
  },
)

onMounted(async () => {
  const at = Number(route.query.at)
  if (Number.isFinite(at) && at > 0) windowStart.value = Math.floor(at / 3600) * 3600
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
