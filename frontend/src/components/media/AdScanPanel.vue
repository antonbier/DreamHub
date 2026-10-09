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
  Analysing changes nothing; taking the breaks over into the recording
  comes later, as its own preview.
-->
<template>
  <section class="mt-4 rounded border border-line-soft bg-bg p-3" :aria-busy="active">
    <div class="flex items-center gap-3">
      <h3 class="text-[12.5px] text-faint">{{ t('media.ads.heading') }}</h3>
      <button
        v-if="result && !active && canStart"
        type="button"
        class="ms-auto rounded px-2 py-0.5 text-[12.5px] text-accent hover:bg-tile-2"
        @click="start"
      >
        {{ t('media.ads.again') }}
      </button>
    </div>

    <!-- Running, waiting or queued -->
    <div v-if="active && job" class="mt-2">
      <div class="flex items-center gap-3 text-[13.5px]">
        <span class="text-muted" aria-live="polite">{{ progressText }}</span>
        <button type="button" class="ms-auto rounded px-2 py-0.5 text-[12.5px] text-accent hover:bg-tile-2" @click="jobs.cancel(job.id)">
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
      <div class="relative mt-2 h-6 overflow-hidden rounded-sm bg-field">
        <component
          :is="canPlay ? 'button' : 'span'"
          v-for="(b, i) in result.blocks"
          :key="i"
          :type="canPlay ? 'button' : undefined"
          class="absolute inset-y-0 border-x border-bg"
          :class="[b.uncertain ? 'ad-uncertain' : 'bg-muted', canPlay ? 'hover:opacity-80 focus-visible:outline focus-visible:outline-2 focus-visible:outline-accent' : '']"
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
    </template>

    <!-- Not analysed yet -->
    <div v-else-if="loaded" class="mt-2 flex flex-wrap items-center gap-3">
      <button
        v-if="canStart"
        type="button"
        class="rounded border border-line px-3 py-1 text-[13.5px] text-text hover:bg-field disabled:opacity-60"
        :disabled="starting"
        @click="start"
      >
        {{ t('media.ads.detect') }}
      </button>
      <span v-else class="text-[13px] text-muted">{{ t('media.ads.notAnalysed') }}</span>
    </div>

    <p v-if="errorText" class="mt-2 text-[12.5px] text-warn" role="alert">{{ errorText }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { api, ApiError } from '@/api/client'
import type { AdScanResult, AdSummary } from '@/api/types'
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
}>()
const emit = defineEmits<{
  // A new result: the recording list shows its summary.
  analysed: [summary: AdSummary]
}>()

const { t, te, locale } = useI18n()
const router = useRouter()
const jobs = useJobsStore()

const result = ref<AdScanResult | null>(null)
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
  const j = job.value
  if (j?.state !== 'failed') return ''
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
    const sum: AdSummary = { blocks: res.result.blocks.length, seconds: adSeconds.value, uncertain: uncertain.value || undefined }
    return sum
  } catch {
    // 404: not analysed yet.
    return null
  } finally {
    loaded.value = true
  }
}

async function start() {
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
