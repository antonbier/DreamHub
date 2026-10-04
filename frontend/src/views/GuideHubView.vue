<!--
  GuideHub's real screen (ROADMAP.md, "v1 — hubs"): the box's EPG in four
  switchable views (components/guide) over one bouquet —

    - Grid: the TV-magazine timeline, several programmes per channel
      across a four-hour window — for planning on a large screen;
    - Now & next: per channel what runs now (with progress) and next, with
      "Switch" — for phones, where the grid is too wide;
    - Tonight: what starts in the evening's prime slot, as poster cards;
    - Channel: one channel's whole TV day as a list.

  The chosen view is remembered per browser; a phone starts on Now & next.
  Two filters apply to every view: films only, rated 7+ (enrichment —
  useGuideFilters). Any programme opens the shared details dialog
  (ProgrammeDialog): what the data sources know, the EPG's text, "Record"
  through TimerHub's own plan → apply with the EPG event id (so the box
  links the timer to the programme), and "Switch now" while it airs.
  ?at=<unix seconds> (global search results) opens the grid at that time.
  Same status-banner-plus-content pattern as the other hubs.
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
        <div class="flex items-center gap-1 rounded bg-field p-1 text-[13px]" role="radiogroup" :aria-label="t('guide.views.label')">
          <button
            v-for="v in VIEWS"
            :key="v"
            type="button"
            role="radio"
            :aria-checked="view === v"
            class="rounded px-2.5 py-1"
            :class="view === v ? 'bg-tile text-text' : 'text-muted hover:text-text'"
            @click="view = v"
          >
            {{ t(`guide.views.${v}`) }}
          </button>
        </div>
        <select
          v-if="bouquets.length > 1"
          v-model="selectedBouquet"
          class="rounded border border-line bg-field px-3 py-1.5 text-[13.5px] text-text"
          :aria-label="t('guide.bouquet')"
        >
          <option v-for="b in bouquets" :key="b.serviceReference" :value="b.serviceReference">{{ b.serviceName }}</option>
        </select>
        <div class="flex items-center gap-1.5 text-[13px]" role="group" :aria-label="t('guide.filters.label')">
          <button
            type="button"
            :aria-pressed="moviesOnly"
            class="rounded-full border px-2.5 py-0.5"
            :class="moviesOnly ? 'border-accent bg-accent/15 text-text' : 'border-line text-muted hover:text-text'"
            @click="moviesOnly = !moviesOnly"
          >
            {{ t('guide.filters.movies') }}
          </button>
          <button
            type="button"
            :aria-pressed="topRated"
            class="rounded-full border px-2.5 py-0.5"
            :class="topRated ? 'border-accent bg-accent/15 text-text' : 'border-line text-muted hover:text-text'"
            @click="topRated = !topRated"
          >
            {{ t('guide.filters.rated') }}
          </button>
        </div>
      </div>
      <p v-if="filterActive && !enrich.enabled" class="mt-2 text-[12.5px] text-faint">{{ t('guide.filters.needsEnrichment') }}</p>

      <p v-if="bouquetsFailed" class="mt-5 text-[14px] text-muted">{{ t('common.backendUnreachable') }}</p>
      <p v-else-if="!selectedBouquet && !bouquetsLoading" class="mt-5 text-[14px] text-muted">{{ t('guide.empty') }}</p>
      <div v-else-if="selectedBouquet" class="mt-4">
        <GuideGrid
          v-if="view === 'grid'"
          :bouquet="selectedBouquet"
          :at="at"
          :selected="selected"
          :filter-active="filterActive"
          :passes="passes"
          @select="select"
        />
        <GuideNowNext v-else-if="view === 'nowNext'" :bouquet="selectedBouquet" :filter-active="filterActive" :passes="passes" @select="select" />
        <GuideTonight v-else-if="view === 'tonight'" :bouquet="selectedBouquet" :filter-active="filterActive" :passes="passes" @select="select" />
        <GuideChannel v-else :bouquet="selectedBouquet" :filter-active="filterActive" :passes="passes" @select="select" />
      </div>
    </template>

    <ProgrammeDialog
      v-if="selected"
      :title="selected.title"
      :service-name="selected.serviceName"
      :start="selected.start"
      :duration="selected.duration"
      :when="formatWhen(selected.start, selected.duration)"
      :now="Date.now() / 1000"
      :info="enrich.info(selected.title)"
      @close="closeSelected"
    >
      <section v-if="selected.description" class="mt-4 rounded border border-line-soft bg-bg p-3">
        <h3 class="text-[12.5px] text-faint">{{ t('guide.epgText') }}</h3>
        <p class="mt-1 text-[14px] text-text">{{ selected.description }}</p>
      </section>

      <template #actions>
        <p v-if="record.status === 'created'" class="flex items-center gap-1.5 text-[13.5px] text-ok" role="status">
          <span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('guide.timerCreated') }}
        </p>
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
              class="rounded bg-accent px-3.5 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-60"
              :disabled="record.status === 'loading' || !canRecord"
              :title="canRecord ? undefined : t('guide.timerHubUnavailable')"
              @click="previewRecord"
            >
              {{ t('guide.record') }}
            </button>
            <button
              v-if="selectedLive"
              type="button"
              class="rounded border border-line px-3.5 py-1.5 text-[13.5px] text-text hover:bg-field disabled:opacity-60"
              :disabled="zapping"
              @click="zapToSelected"
            >
              {{ zapped ? t('guide.switchedNow') : t('guide.switchNow') }}
            </button>
          </div>
          <p v-if="record.error" class="mt-2 text-[12.5px] text-warn">{{ record.error }}</p>
          <p v-if="zapError" class="mt-2 text-[12.5px] text-warn">{{ zapError }}</p>
        </template>
      </template>
    </ProgrammeDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import ProgrammeDialog from '@/components/programme/ProgrammeDialog.vue'
import GuideGrid from '@/components/guide/GuideGrid.vue'
import GuideNowNext from '@/components/guide/GuideNowNext.vue'
import GuideTonight from '@/components/guide/GuideTonight.vue'
import GuideChannel from '@/components/guide/GuideChannel.vue'
import { useEnrichStore } from '@/stores/enrich'
import { usePlanError } from '@/composables/usePlanError'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { useGuideFilters } from '@/composables/useGuideFilters'
import type { Bouquet, EPGEvent, Plan } from '@/api/types'

const VIEWS = ['grid', 'nowNext', 'tonight', 'channel'] as const
type View = (typeof VIEWS)[number]
const VIEW_KEY = 'dreamhub.guideView'

const { t, locale } = useI18n()
const navigation = useNavigationStore()
const enrich = useEnrichStore()
const { describe, describeFix } = useProblem()
const { timeZone } = useDisplayTimeZone()
const { planErrorMessage } = usePlanError()
const { moviesOnly, topRated, active: filterActive, passes } = useGuideFilters()
const route = useRoute()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/guide'))

// --- view: remembered per browser; phones start on now & next -------------

function initialView(): View {
  try {
    const saved = localStorage.getItem(VIEW_KEY) as View | null
    if (saved && VIEWS.includes(saved)) return saved
  } catch {
    // no memory
  }
  return window.matchMedia?.('(max-width: 900px)').matches ? 'nowNext' : 'grid'
}
const view = ref<View>(initialView())
watch(view, (v) => {
  try {
    localStorage.setItem(VIEW_KEY, v)
  } catch {
    // Not remembered; still switched.
  }
})

// A search result (?at=) opens the grid at that time.
const at = ref<number | undefined>(undefined)
function applyAt(value: unknown) {
  const n = Number(value)
  if (Number.isFinite(n) && n > 0) {
    at.value = n
    view.value = 'grid'
  }
}
watch(() => route.query.at, applyAt)

// --- bouquets ---------------------------------------------------------------

const bouquets = ref<Bouquet[]>([])
const selectedBouquet = ref('')
const bouquetsLoading = ref(true)
const bouquetsFailed = ref(false)

onMounted(async () => {
  applyAt(route.query.at)
  try {
    const res = await api.guide.bouquets()
    bouquets.value = res.bouquets
    if (res.bouquets.length > 0) selectedBouquet.value = res.bouquets[0].serviceReference
  } catch {
    bouquetsFailed.value = true
  } finally {
    bouquetsLoading.value = false
  }
})

// --- the selected programme: details, record, switch ------------------------

// Recording needs TimerHub; GuideHub itself works without it.
const canRecord = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.id === 'timer')?.status !== 'unavailable')

const selected = ref<EPGEvent | null>(null)
type RecordState = { status: 'idle' | 'loading' | 'previewing' | 'applying' | 'created'; plan?: Plan; error?: string }
const record = ref<RecordState>({ status: 'idle' })

function formatHour(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23', timeZone: timeZone.value }).format(unixSeconds * 1000)
}
function formatWhen(start: number, duration: number): string {
  const day = new Intl.DateTimeFormat(locale.value, { weekday: 'short', day: 'numeric', month: 'numeric', timeZone: timeZone.value }).format(start * 1000)
  return `${day}, ${formatHour(start)} – ${formatHour(start + duration)}`
}

const selectedEnded = computed(() => (selected.value ? selected.value.start + selected.value.duration <= Date.now() / 1000 : false))
// "Switch now" only for a programme airing right now (ControlHub's zap).
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
</script>
