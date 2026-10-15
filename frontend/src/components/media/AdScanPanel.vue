<!--
  Ad breaks of one recording, in its detail dialog (docs/ADSCAN_PLAN.md,
  #44). Kept small on purpose: one action, a timeline, one line of text.
  - not analysed: "Detect ads"
  - queued/running/waiting: progress and Stop (the work runs in the
    background queue; leaving the dialog doesn't stop it)
  - done: the timeline — breaks dark, uncertain ones hatched and labelled
    (never colour alone) — and "3 breaks · 14 min ads · 46 min without".
    A break is a button: it plays the recording in the browser from just
    before it, to check the boundary.
  Analysing changes nothing. "Cut out the ads" (only where file operations
  work) ticks the certain breaks, leaves the uncertain ones to the user,
  and shows the usual preview; the cut runs in the background, the
  original goes to the trash.
-->
<template>
  <!-- Someone who can't start it sees the section only once there's
       something to see. -->
  <section v-if="result || canStart || active || trimActive" class="mt-4 rounded-sm border border-line-soft bg-bg p-3" :aria-busy="active">
    <div class="flex items-center gap-3">
      <h3 class="text-[12.5px] text-faint">{{ t('media.ads.heading') }}</h3>
      <button
        v-if="result && !active && canStart"
        type="button"
        class="ms-auto rounded-sm px-2 py-0.5 text-[12.5px] text-accent hover:bg-tile-2"
        @click="start"
      >
        {{ t('media.ads.again') }}
      </button>
    </div>

    <!-- Cutting -->
    <div v-if="trimActive && trimJob" class="mt-2">
      <div class="flex items-center gap-3 text-[13.5px]">
        <span class="text-muted" aria-live="polite">{{ trimProgressText }}</span>
        <button type="button" class="ms-auto rounded-sm px-2 py-0.5 text-[12.5px] text-accent hover:bg-tile-2" @click="jobs.cancel(trimJob.id)">
          {{ t('media.ads.stop') }}
        </button>
      </div>
      <div
        class="mt-2 h-1.5 overflow-hidden rounded-full bg-field"
        role="progressbar"
        :aria-valuenow="trimPercent"
        aria-valuemin="0"
        aria-valuemax="100"
        :aria-label="t('media.ads.cut.heading')"
      >
        <div class="h-full bg-accent transition-[width]" :style="{ width: `${trimPercent}%` }" />
      </div>
    </div>

    <!-- Already cut -->
    <p v-else-if="result?.trimmed" class="mt-2 text-[13.5px] text-text">
      {{ t('media.ads.cut.done', { length: formatMinutes(result.duration, locale) }) }}
    </p>

    <!-- Running, waiting or queued -->
    <div v-else-if="active && job" class="mt-2">
      <div class="flex items-center gap-3 text-[13.5px]">
        <span class="text-muted" aria-live="polite">{{ progressText }}</span>
        <button type="button" class="ms-auto rounded-sm px-2 py-0.5 text-[12.5px] text-accent hover:bg-tile-2" @click="jobs.cancel(job.id)">
          {{ t('media.ads.stop') }}
        </button>
      </div>
      <div
        class="mt-2 h-1.5 overflow-hidden rounded-full bg-field"
        role="progressbar"
        :aria-valuenow="percent"
        aria-valuemin="0"
        aria-valuemax="100"
        :aria-label="t('media.ads.heading')"
      >
        <div class="h-full bg-accent transition-[width]" :style="{ width: `${percent}%` }" />
      </div>
    </div>

    <!-- The result -->
    <template v-else-if="result">
      <div class="relative mt-2 h-6 overflow-hidden rounded-xs bg-field">
        <component
          :is="canPlay ? 'button' : 'span'"
          v-for="(b, i) in result.blocks"
          :key="i"
          :type="canPlay ? 'button' : undefined"
          :role="canPlay ? undefined : 'img'"
          class="absolute inset-y-0 border-x border-bg"
          :class="[b.uncertain ? 'ad-uncertain' : 'bg-muted', canPlay ? 'hover:opacity-80 focus-visible:outline-solid focus-visible:outline-2 focus-visible:outline-accent' : '']"
          :style="{ insetInlineStart: pct(b.start), width: pct(b.end - b.start) }"
          :aria-label="blockLabel(b)"
          :title="blockLabel(b)"
          @click="canPlay && play(b.start)"
        />
      </div>
      <p class="mt-2 text-[13.5px] text-text">
        <template v-if="result.blocks.length === 0">{{ t('media.ads.none') }}</template>
        <template v-else>
          {{ summary }}
          <span v-if="uncertain > 0" class="text-warn"> · {{ t('media.ads.uncertainCount', { count: uncertain }, uncertain) }}</span>
        </template>
      </p>
      <p v-if="result.weakLogo" class="mt-1 text-[12.5px] text-muted">{{ t('media.ads.weakLogo') }}</p>
      <p v-if="result.marked && !choosing && !markChoosing" class="mt-1 text-[12.5px] text-muted">{{ t('media.ads.mark.done') }}</p>

      <!-- Cutting out, or marking in the cut list: choose, preview, confirm -->
      <div v-if="canTrim && result.blocks.length > 0 && !choosing && !markChoosing" class="mt-3 flex flex-wrap gap-2">
        <button type="button" class="rounded-sm border border-line px-3 py-1 text-[13.5px] text-text hover:bg-field" @click="startChoosing">
          {{ t('media.ads.cut.open') }}
        </button>
        <button type="button" class="rounded-sm border border-line px-3 py-1 text-[13.5px] text-text hover:bg-field" @click="startMarkChoosing">
          {{ t('media.ads.mark.open') }}
        </button>
      </div>
      <div v-if="choosing" class="mt-3 flex flex-col gap-2">
        <fieldset :disabled="!!trimPlan">
          <legend class="text-[12.5px] text-faint">{{ t('media.ads.cut.which') }}</legend>
          <label v-for="(b, i) in result.blocks" :key="i" class="mt-1 flex items-center gap-2 text-[13.5px] text-text">
            <input v-model="ticked[i]" type="checkbox" class="accent-(--dh-accent)" />
            <span>{{ t('media.ads.block', { from: formatPosition(b.start), to: formatPosition(b.end) }) }}</span>
            <span v-if="b.uncertain" class="text-[12.5px] text-warn">{{ t('media.ads.uncertain') }}</span>
          </label>
        </fieldset>
        <div v-if="!trimPlan" class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="rounded-sm bg-accent px-3 py-1 text-[13.5px] font-semibold text-bg disabled:opacity-60"
            :disabled="tickedCount === 0 || trimBusy"
            @click="previewTrim"
          >
            {{ t('media.ads.cut.preview') }}
          </button>
          <button type="button" class="rounded-sm px-3 py-1 text-[13.5px] text-text hover:bg-field" @click="stopChoosing">{{ t('plan.cancel') }}</button>
        </div>
        <PlanPreview
          v-else
          :plan="trimPlan"
          :confirm-label="t('media.ads.cut.confirm')"
          :busy="trimBusy"
          :error="trimError"
          @confirm="applyTrim"
          @cancel="trimPlan = null"
        >
          {{ trimSummary }}
        </PlanPreview>
        <p v-if="trimError && !trimPlan" class="text-[12.5px] text-warn" role="alert">{{ trimError }}</p>
      </div>
      <div v-if="markChoosing" class="mt-3 flex flex-col gap-2">
        <fieldset :disabled="!!markPlan">
          <legend class="text-[12.5px] text-faint">{{ t('media.ads.cut.which') }}</legend>
          <label v-for="(b, i) in result.blocks" :key="i" class="mt-1 flex items-center gap-2 text-[13.5px] text-text">
            <input v-model="markTicked[i]" type="checkbox" class="accent-(--dh-accent)" />
            <span>{{ t('media.ads.block', { from: formatPosition(b.start), to: formatPosition(b.end) }) }}</span>
            <span v-if="b.uncertain" class="text-[12.5px] text-warn">{{ t('media.ads.uncertain') }}</span>
          </label>
        </fieldset>
        <div v-if="!markPlan" class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="rounded-sm bg-accent px-3 py-1 text-[13.5px] font-semibold text-bg disabled:opacity-60"
            :disabled="markTickedCount === 0 || markBusy"
            @click="previewMark"
          >
            {{ t('media.ads.mark.preview') }}
          </button>
          <button type="button" class="rounded-sm px-3 py-1 text-[13.5px] text-text hover:bg-field" @click="stopMarkChoosing">{{ t('plan.cancel') }}</button>
        </div>
        <PlanPreview
          v-else
          :plan="markPlan"
          :confirm-label="t('media.ads.mark.confirm')"
          :busy="markBusy"
          :error="markError"
          @confirm="applyMark"
          @cancel="markPlan = null"
        >
          {{ markSummary }}
        </PlanPreview>
        <p v-if="markError && !markPlan" class="text-[12.5px] text-warn" role="alert">{{ markError }}</p>
      </div>
    </template>

    <!-- Not analysed yet -->
    <div v-else-if="loaded" class="mt-2 flex flex-wrap items-center gap-3">
      <button
        v-if="canStart"
        type="button"
        class="rounded-sm border border-line px-3 py-1 text-[13.5px] text-text hover:bg-field disabled:opacity-60"
        :disabled="starting"
        @click="start"
      >
        {{ t('media.ads.detect') }}
      </button>
    </div>

    <p v-if="errorText" class="mt-2 text-[12.5px] text-warn" role="alert">{{ errorText }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { api, ApiError } from '@/api/client'
import PlanPreview from '@/components/PlanPreview.vue'
import { usePlanError } from '@/composables/usePlanError'
import type { AdScanResult, AdSummary, Plan } from '@/api/types'
import { formatMinutes, formatPosition } from '@/lib/format'
import { ACTIVE, cleanPath, useJobsStore } from '@/stores/jobs'

const props = defineProps<{
  path: string
  // For the browser player's heading and time labels.
  title: string
  channel: string
  length: string
  // The profile may start an analysis (recordings.manage) and it can run here.
  canStart: boolean
  // StreamHub is on: a break plays from there.
  canPlay: boolean
  // File operations work here and the profile may change recordings.
  canTrim: boolean
}>()
const emit = defineEmits<{
  // A new result: the recording list shows its summary.
  analysed: [summary: AdSummary]
  // The recording was cut: its length and size changed.
  trimmed: []
}>()

const { t, te, locale } = useI18n()
const router = useRouter()
const jobs = useJobsStore()

const result = ref<AdScanResult | null>(null)
// A failed job's message shows until the next action; older failures
// (from before this dialog's last start) don't.
const errorsSince = ref(0)
function freshErrors() {
  errorsSince.value = Math.max(0, ...Object.keys(jobs.byId).map(Number))
}
const loaded = ref(false)
const starting = ref(false)
const startError = ref('')

const job = computed(() => jobs.latest('media.adscan', props.path))
const active = computed(() => !!job.value && ACTIVE.includes(job.value.state))
const percent = computed(() => {
  const j = job.value
  return j && j.total > 0 ? Math.min(100, Math.round((j.done / j.total) * 100)) : 0
})
const progressText = computed(() => {
  const j = job.value
  if (!j) return ''
  if (j.state === 'queued') return t('media.ads.queued')
  if (j.state === 'waiting') return t(`media.ads.waiting.${jobs.blockedBy || 'recording'}`)
  return t('media.ads.running', { percent: percent.value })
})

const errorText = computed(() => {
  if (startError.value) return startError.value
  if (trimErrorFromJob.value) return trimErrorFromJob.value
  const j = job.value
  if (j?.state !== 'failed' || j.id <= errorsSince.value) return ''
  const key = `media.ads.error.${j.error}`
  return te(key) ? t(key) : t('media.ads.error.generic')
})

const adSeconds = computed(() => (result.value?.blocks ?? []).reduce((s, b) => s + (b.end - b.start), 0))
const uncertain = computed(() => (result.value?.blocks ?? []).filter((b) => b.uncertain).length)
const summary = computed(() => {
  const r = result.value
  if (!r) return ''
  return t('media.ads.summary', {
    breaks: t('media.ads.breaks', { count: r.blocks.length }, r.blocks.length),
    ads: formatMinutes(adSeconds.value, locale.value),
    without: formatMinutes(r.duration - adSeconds.value, locale.value),
  })
})

function pct(seconds: number) {
  const d = result.value?.duration || 1
  return `${Math.max(0, Math.min(100, (seconds / d) * 100))}%`
}
function blockLabel(b: { start: number; end: number; uncertain?: boolean }) {
  const range = t('media.ads.block', { from: formatPosition(b.start), to: formatPosition(b.end) })
  const parts = [range]
  if (b.uncertain) parts.push(t('media.ads.uncertain'))
  if (props.canPlay) parts.push(t('media.ads.playFromHere'))
  return parts.join(' · ')
}

// Plays the recording in the browser from shortly before the break, so
// the boundary itself is on screen.
function play(start: number) {
  const d = result.value?.duration
  if (!d) return
  const at = Math.max(0, (start - 5) / d)
  void router.push({
    path: '/stream',
    query: { recording: props.path, title: props.title, length: props.length, channel: props.channel, at: at.toFixed(4) },
  })
}

async function loadResult() {
  try {
    const res = await api.media.adScanResult(props.path)
    result.value = res.result
    const sum: AdSummary = {
      blocks: res.result.blocks.length,
      seconds: adSeconds.value,
      uncertain: uncertain.value || undefined,
      trimmed: res.result.trimmed || undefined,
      marked: res.result.marked || undefined,
    }
    return sum
  } catch {
    // 404: not analysed yet.
    return null
  } finally {
    loaded.value = true
  }
}

async function start() {
  freshErrors()
  starting.value = true
  startError.value = ''
  try {
    const res = await api.media.adScanStart([props.path])
    const item = res.items[0]
    if (item?.reason) {
      startError.value = t(`media.ads.reason.${item.reason}`)
    } else if (item?.jobId && !jobs.byId[item.jobId]) {
      // Shown at once; the event stream brings the real state.
      jobs.apply({ id: item.jobId, kind: 'media.adscan', key: cleanPath(props.path), ownerId: 0, state: 'queued', done: 0, total: 0, createdAt: new Date().toISOString() })
    }
  } catch (e) {
    startError.value = e instanceof ApiError && te(`media.ads.error.${e.code}`) ? t(`media.ads.error.${e.code}`) : t('media.ads.error.generic')
  } finally {
    starting.value = false
  }
}

// --- cutting out -------------------------------------------------------------

const { planErrorMessage } = usePlanError()
const choosing = ref(false)
const ticked = ref<boolean[]>([])
const trimPlan = ref<Plan | null>(null)
const trimBusy = ref(false)
const trimError = ref('')
const trimKeptSeconds = ref(0)

const trimJob = computed(() => jobs.latest('media.trim', props.path))
const trimActive = computed(() => !!trimJob.value && ACTIVE.includes(trimJob.value.state))
const trimPercent = computed(() => {
  const j = trimJob.value
  return j && j.total > 0 ? Math.min(100, Math.round((j.done / j.total) * 100)) : 0
})
const trimProgressText = computed(() => {
  const j = trimJob.value
  if (!j) return ''
  if (j.state === 'queued') return t('media.ads.queued')
  if (j.state === 'waiting') return t(`media.ads.waiting.${jobs.blockedBy || 'recording'}`)
  return t('media.ads.cut.running', { percent: trimPercent.value })
})
const tickedCount = computed(() => ticked.value.filter(Boolean).length)
const trimSummary = computed(() =>
  t('media.ads.cut.summary', {
    breaks: t('media.ads.breaks', { count: tickedCount.value }, tickedCount.value),
    length: formatMinutes(trimKeptSeconds.value, locale.value),
  }),
)

// Certain breaks ticked, uncertain ones left to the user.
function startChoosing() {
  freshErrors()
  ticked.value = (result.value?.blocks ?? []).map((b) => !b.uncertain)
  trimPlan.value = null
  trimError.value = ''
  choosing.value = true
}
function stopChoosing() {
  choosing.value = false
  trimPlan.value = null
}

async function previewTrim() {
  const blocks = (result.value?.blocks ?? []).filter((_, i) => ticked.value[i]).map((b) => ({ start: b.start, end: b.end }))
  trimBusy.value = true
  trimError.value = ''
  try {
    const res = await api.media.trimPlan(props.path, blocks)
    trimKeptSeconds.value = res.keep.reduce((s, k) => s + (k.end - k.start), 0)
    trimPlan.value = res.plan
  } catch (e) {
    trimError.value = planErrorMessage(e)
  } finally {
    trimBusy.value = false
  }
}

async function applyTrim() {
  if (!trimPlan.value) return
  trimBusy.value = true
  trimError.value = ''
  try {
    await api.plans.apply(trimPlan.value.id)
    choosing.value = false
    trimPlan.value = null
    // The job arrives on the event stream; until then, load the list.
    await jobs.load()
  } catch (e) {
    trimError.value = planErrorMessage(e)
  } finally {
    trimBusy.value = false
  }
}

watch(
  () => trimJob.value?.state,
  async (state, before) => {
    if (state === 'done' && before && before !== 'done') {
      await loadResult()
      emit('trimmed')
    }
  },
)

const trimErrorFromJob = computed(() => {
  const j = trimJob.value
  if (j?.state !== 'failed' || j.id <= errorsSince.value) return ''
  const key = `media.ads.cut.error.${j.error}`
  return te(key) ? t(key) : t('media.ads.cut.error.generic')
})

// --- marking in the cut list -------------------------------------------------

const markChoosing = ref(false)
const markTicked = ref<boolean[]>([])
const markPlan = ref<Plan | null>(null)
const markBusy = ref(false)
const markError = ref('')

const markTickedCount = computed(() => markTicked.value.filter(Boolean).length)
const markSummary = computed(() =>
  t('media.ads.mark.summary', {
    breaks: t('media.ads.breaks', { count: markTickedCount.value }, markTickedCount.value),
    name: props.title,
  }),
)

function startMarkChoosing() {
  freshErrors()
  markTicked.value = (result.value?.blocks ?? []).map((b) => !b.uncertain)
  markPlan.value = null
  markError.value = ''
  markChoosing.value = true
}
function stopMarkChoosing() {
  markChoosing.value = false
  markPlan.value = null
}

async function previewMark() {
  const blocks = (result.value?.blocks ?? []).filter((_, i) => markTicked.value[i]).map((b) => ({ start: b.start, end: b.end }))
  markBusy.value = true
  markError.value = ''
  try {
    const res = await api.media.markPlan(props.path, blocks)
    markPlan.value = res.plan
  } catch (e) {
    markError.value = planErrorMessage(e)
  } finally {
    markBusy.value = false
  }
}

async function applyMark() {
  if (!markPlan.value) return
  markBusy.value = true
  markError.value = ''
  try {
    await api.plans.apply(markPlan.value.id)
    markChoosing.value = false
    markPlan.value = null
    const sum = await loadResult()
    if (sum) emit('analysed', sum)
  } catch (e) {
    markError.value = planErrorMessage(e)
  } finally {
    markBusy.value = false
  }
}

// A finished analysis: fetch its result.
watch(
  () => job.value?.state,
  async (state, before) => {
    if (state === 'done' && before && before !== 'done') {
      const sum = await loadResult()
      if (sum) emit('analysed', sum)
    }
  },
)

onMounted(() => {
  if (!jobs.loaded) void jobs.load()
  void loadResult()
})
</script>

<style scoped>
/* Uncertain breaks: hatched, and labelled — not told apart by colour alone. */
.ad-uncertain {
  background-image: repeating-linear-gradient(135deg, var(--dh-warn) 0 3px, transparent 3px 7px);
}
</style>
