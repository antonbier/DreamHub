<!--
  Cutting a recording by hand (#27): the user names the parts to cut out
  (from … to …, as m:ss or h:mm:ss), can look at each boundary in the
  browser player, previews, confirms. The cut itself is ad cutting's:
  POST /api/v1/media/trim/plan, no re-encoding, the original kept in the
  trash. The recording's length comes from the file, so no ad detection is
  needed first.
-->
<template>
  <section class="mt-4 border-t border-line-soft pt-3" :aria-label="t('media.manual.heading')">
    <div class="flex items-center gap-3">
      <h3 class="text-[12.5px] text-faint">{{ t('media.manual.heading') }}</h3>
    </div>

    <div v-if="trimActive && trimJob" class="mt-2 text-[13.5px] text-muted" aria-live="polite">
      {{ t('media.ads.cut.running', { percent: trimPercent }) }}
    </div>

    <template v-else-if="!open">
      <p class="mt-1 text-[13px] text-muted">{{ t('media.manual.explain') }}</p>
      <p v-if="doneNote" class="mt-1 text-[13px] text-text" role="status">{{ doneNote }}</p>
      <button type="button" class="mt-2 rounded-sm border border-line px-3 py-1 text-[13.5px] text-text hover:bg-field" @click="start">
        {{ t('media.manual.open') }}
      </button>
    </template>

    <div v-else class="mt-2 flex flex-col gap-2">
      <fieldset :disabled="!!plan" class="flex flex-col gap-1.5">
        <legend class="text-[12.5px] text-faint">{{ t('media.manual.which') }}</legend>
        <div v-for="(r, i) in rows" :key="i" class="flex flex-wrap items-center gap-2">
          <label class="flex items-center gap-1 text-[13.5px] text-text">
            <span class="text-faint">{{ t('media.manual.from') }}</span>
            <input v-model="r.start" type="text" inputmode="numeric" placeholder="0:00" class="w-20 rounded-sm border border-line bg-field px-2 py-1 text-[14px] tabular-nums" :aria-invalid="!validRow(r)" />
          </label>
          <label class="flex items-center gap-1 text-[13.5px] text-text">
            <span class="text-faint">{{ t('media.manual.to') }}</span>
            <input v-model="r.end" type="text" inputmode="numeric" placeholder="0:00" class="w-20 rounded-sm border border-line bg-field px-2 py-1 text-[14px] tabular-nums" :aria-invalid="!validRow(r)" />
          </label>
          <button v-if="canPlay" type="button" class="rounded-sm px-2 py-1 text-[12.5px] text-accent hover:bg-tile-2" :disabled="seconds(r.start) === null" @click="look(r.start)">
            {{ t('media.manual.lookStart') }}
          </button>
          <button v-if="canPlay" type="button" class="rounded-sm px-2 py-1 text-[12.5px] text-accent hover:bg-tile-2" :disabled="seconds(r.end) === null" @click="look(r.end)">
            {{ t('media.manual.lookEnd') }}
          </button>
          <button v-if="rows.length > 1" type="button" class="rounded-sm px-2 py-1 text-[12.5px] text-onair hover:bg-tile-2" @click="rows.splice(i, 1)">
            {{ t('media.manual.remove') }}
          </button>
        </div>
        <div>
          <button type="button" class="rounded-sm px-2 py-1 text-[13px] text-accent hover:bg-tile-2" @click="rows.push({ start: '', end: '' })">{{ t('media.manual.add') }}</button>
        </div>
      </fieldset>

      <div v-if="!plan" class="flex flex-wrap items-center gap-2">
        <button type="button" class="rounded-sm bg-accent px-3 py-1 text-[13.5px] font-semibold text-bg disabled:opacity-60" :disabled="!allValid || busy" @click="review">
          {{ t('media.ads.cut.preview') }}
        </button>
        <button type="button" class="rounded-sm px-3 py-1 text-[13.5px] text-text hover:bg-field" @click="stop">{{ t('plan.cancel') }}</button>
      </div>
      <PlanPreview v-else :plan="plan" :confirm-label="t('media.ads.cut.confirm')" :busy="busy" :error="error" @confirm="apply" @cancel="plan = null">
        {{ summary }}
      </PlanPreview>
      <p v-if="error && !plan" class="text-[12.5px] text-warn" role="alert">{{ error }}</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { api } from '@/api/client'
import PlanPreview from '@/components/PlanPreview.vue'
import { usePlanError } from '@/composables/usePlanError'
import { formatMinutes } from '@/lib/format'
import { parseClock, parseLength } from '@/lib/timeInput'
import { ACTIVE, useJobsStore } from '@/stores/jobs'
import type { Plan } from '@/api/types'

const props = defineProps<{ path: string; title: string; channel: string; length: string; canPlay: boolean }>()
const emit = defineEmits<{ trimmed: [] }>()

const { t, locale } = useI18n()
const router = useRouter()
const jobs = useJobsStore()
const { planErrorMessage } = usePlanError()

interface Row {
  start: string
  end: string
}
const open = ref(false)
const rows = ref<Row[]>([{ start: '', end: '' }])
const plan = ref<Plan | null>(null)
const busy = ref(false)
const error = ref('')
const keptSeconds = ref(0)
const doneNote = ref('')

const seconds = parseClock
const total = computed(() => parseLength(props.length))
function validRow(r: Row) {
  const a = seconds(r.start)
  const b = seconds(r.end)
  return a !== null && b !== null && b > a
}
const allValid = computed(() => rows.value.length > 0 && rows.value.every(validRow))
const summary = computed(() =>
  t('media.manual.summary', { parts: t('media.manual.parts', { count: rows.value.length }, rows.value.length), length: formatMinutes(keptSeconds.value, locale.value) }),
)

const trimJob = computed(() => jobs.latest('media.trim', props.path))
const trimActive = computed(() => !!trimJob.value && ACTIVE.includes(trimJob.value.state))
const trimPercent = computed(() => {
  const j = trimJob.value
  return j && j.total > 0 ? Math.min(100, Math.round((j.done / j.total) * 100)) : 0
})

function start() {
  doneNote.value = ''
  error.value = ''
  plan.value = null
  rows.value = [{ start: '', end: '' }]
  open.value = true
}
function stop() {
  open.value = false
  plan.value = null
  error.value = ''
}

// The player takes a position as a share of the whole length.
function look(text: string) {
  const s = seconds(text)
  if (s === null) return
  const at = total.value ? Math.min(0.9999, Math.max(0, (s - 3) / total.value)) : 0
  void router.push({ path: '/stream', query: { recording: props.path, title: props.title, length: props.length, channel: props.channel, at: at.toFixed(4) } })
}

async function review() {
  busy.value = true
  error.value = ''
  try {
    const blocks = rows.value.map((r) => ({ start: seconds(r.start)!, end: seconds(r.end)! }))
    const res = await api.media.trimPlan(props.path, blocks)
    keptSeconds.value = res.keep.reduce((s, k) => s + (k.end - k.start), 0)
    plan.value = res.plan
  } catch (e) {
    error.value = planErrorMessage(e)
  } finally {
    busy.value = false
  }
}

async function apply() {
  if (!plan.value) return
  busy.value = true
  error.value = ''
  try {
    await api.plans.apply(plan.value.id)
    open.value = false
    plan.value = null
    await jobs.load()
  } catch (e) {
    error.value = planErrorMessage(e)
  } finally {
    busy.value = false
  }
}

watch(
  () => trimJob.value?.state,
  (state, before) => {
    if (state === 'done' && before && before !== 'done') {
      doneNote.value = t('media.manual.done')
      emit('trimmed')
    }
  },
)
</script>
