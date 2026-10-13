<!--
  TimerHub's real screen (ROADMAP.md, "v1 — hubs"; docs/archive/V1_PLAN.md, WP3):
  every timer on the box, upcoming ones grouped by day in the display time
  zone, finished ones folded away below. Each row shows its state, where it
  came from (SerienRecorder/AutoTimer, read from the box's tags) and whether
  it likely clashes with another timer on this box's tuners — an estimate
  the box double-checks, so it's worded as "likely".

  Deleting goes through preview-and-confirm (POST /api/v1/timers/delete/plan
  → PlanPreview → POST /api/v1/plans/{id}/apply); the preview warns when the
  timer is recording right now, because the box stops the recording.

  The AutoTimer section is read-only (decision D3): the plugin's own
  configuration, a dry run of what it would create, and a link to its own
  editor. The plugin is optional — without it the section is one quiet
  line, and TimerHub itself stays fully available (docs/archive/V1_PLAN.md, rule 1).
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="timers" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ hubTitle('timer', 'hub.timer') }}</h1>
      <span v-if="tuners > 0" class="ms-auto text-[13px] text-faint">{{ t('timers.tuners', { count: tuners }, tuners) }}</span>
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
      <p v-if="notice" class="mt-4 text-[13.5px] text-ok" role="status">{{ notice }}</p>

      <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted"><ErrorText /></p>
      <p v-else-if="upcomingDays.length === 0" class="mt-5 text-[14px] text-muted">{{ t('timers.empty') }}</p>

      <section v-for="day in upcomingDays" :key="day.key" class="mt-6">
        <h2 class="mb-1 text-[15.5px] font-semibold text-text">{{ day.label }}</h2>
        <ul class="flex flex-col">
          <TimerRow
            v-for="timer in day.timers"
            :key="rowKey(timer)"
            :timer="timer"
            :time-zone="timeZone"
            :deleting="deleting?.key === rowKey(timer) ? deleting : null"
            @delete="previewDelete(timer)"
            @toggle="previewToggle(timer)"
            @confirm="confirmDelete"
            @cancel="deleting = null"
          />
        </ul>
      </section>

      <section v-if="finished.length > 0" class="mt-6">
        <button type="button" class="text-[13.5px] text-accent hover:underline" :aria-expanded="showFinished" @click="showFinished = !showFinished">
          {{ showFinished ? t('timers.hideFinished') : t('timers.showFinished', { count: finished.length }) }}
        </button>
        <button v-if="cleanable > 0 && !cleanup" type="button" class="ms-4 text-[13.5px] text-accent hover:underline" @click="previewCleanup">
          {{ t('timers.cleanup.open', { count: cleanable }, cleanable) }}
        </button>
        <PlanPreview
          v-if="cleanup"
          class="mt-3"
          :plan="cleanup.plan"
          :confirm-label="t('timers.cleanup.confirm')"
          :busy="cleanup.busy"
          :error="cleanup.error"
          @confirm="confirmCleanup"
          @cancel="cleanup = null"
        >
          {{ t('timers.cleanup.preview', { count: cleanup.plan?.changes.length ?? 0 }, cleanup.plan?.changes.length ?? 0) }}
        </PlanPreview>
        <ul v-if="showFinished" class="mt-2 flex flex-col opacity-80">
          <TimerRow
            v-for="timer in finished"
            :key="rowKey(timer)"
            :timer="timer"
            :time-zone="timeZone"
            show-date
            :deleting="deleting?.key === rowKey(timer) ? deleting : null"
            @delete="previewDelete(timer)"
            @toggle="previewToggle(timer)"
            @confirm="confirmDelete"
            @cancel="deleting = null"
          />
        </ul>
      </section>

      <section class="mt-8 border-t border-line-soft pt-5">
        <div class="flex flex-wrap items-center gap-3">
          <h2 class="text-[15.5px] font-semibold text-text">{{ t('timers.autoTimer.title') }}</h2>
          <template v-if="autoTimers?.installed">
            <button type="button" class="rounded px-2 py-1 text-[13px] text-accent hover:bg-tile-2" :disabled="simulating" @click="simulate">
              {{ simulating ? t('timers.autoTimer.simulating') : t('timers.autoTimer.simulate') }}
            </button>
            <a :href="autoTimerEditorUrl" target="_blank" rel="noopener" class="rounded px-2 py-1 text-[13px] text-accent hover:bg-tile-2">
              {{ t('timers.autoTimer.openEditor') }}
            </a>
          </template>
        </div>
        <p v-if="autoTimers && !autoTimers.installed" class="mt-2 text-[13px] text-faint">{{ t('timers.autoTimer.notInstalled') }}</p>
        <p v-else-if="autoTimerError" class="mt-2 text-[13px] text-muted">{{ autoTimerError }}</p>
        <template v-else-if="autoTimers?.installed">
          <p v-if="autoTimers.autoTimers.length === 0" class="mt-2 text-[13px] text-muted">{{ t('timers.autoTimer.none') }}</p>
          <ul v-else class="mt-2 flex flex-col">
            <li v-for="at in autoTimers.autoTimers" :key="at.id" class="flex items-center gap-3 border-b border-line-soft py-2 last:border-none">
              <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="at.enabled ? 'bg-ok' : 'bg-faint'" />
              <span class="min-w-0 flex-1">
                <span class="block truncate text-[14px] text-text">{{ at.name }}</span>
                <span class="block truncate text-[12.5px] text-muted">
                  {{ t('timers.autoTimer.match', { match: at.match }) }}
                  <template v-if="at.services.length > 0"> · {{ at.services.map((s) => s.serviceName).join(', ') }}</template>
                </span>
              </span>
              <span class="shrink-0 text-[12.5px] text-faint">{{ at.enabled ? t('timers.autoTimer.enabled') : t('timers.autoTimer.disabled') }}</span>
            </li>
          </ul>
          <div v-if="simulated !== null" class="mt-3 rounded border border-line-soft bg-bg p-3">
            <p class="mb-1.5 text-[12px] text-faint">{{ t('timers.autoTimer.simulateEyebrow') }}</p>
            <p v-if="simulated.length === 0" class="text-[13px] text-muted">{{ t('timers.autoTimer.simulateNone') }}</p>
            <ul v-else class="flex flex-col">
              <li v-for="(s, i) in simulated" :key="i" class="flex items-center gap-3 py-1.5">
                <span class="w-28 shrink-0 whitespace-nowrap font-mono text-[12.5px] text-faint">{{ formatDateTime(s.begin) }}</span>
                <span class="min-w-0 flex-1 truncate text-[13.5px] text-text">{{ s.name }}</span>
                <span class="shrink-0 truncate text-[12.5px] text-muted">{{ s.serviceName }}</span>
              </li>
            </ul>
          </div>
        </template>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { useHubLabel } from '@/composables/useHubLabel'
import ErrorText from '@/components/ErrorText.vue'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import Icon from '@/components/Icon.vue'
import TimerRow from '@/components/timers/TimerRow.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import type { DeleteState } from '@/components/timers/types'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { usePlanError } from '@/composables/usePlanError'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { AutoTimers, Plan, SimulatedTimer, Timer } from '@/api/types'

const { t, locale } = useI18n()
const { hubTitle } = useHubLabel()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()
const { planErrorMessage } = usePlanError()
const { timeZone } = useDisplayTimeZone()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/timers'))

const timers = ref<Timer[]>([])
const tuners = ref(0)
const loading = ref(true)
const loadFailed = ref(false)
const showFinished = ref(false)
const notice = ref('')

const rowKey = (timer: Timer) => `${timer.serviceReference}|${timer.begin}|${timer.end}`

// A timer is finished once its end has passed — not by state alone, since
// a disabled timer is reported as "ended" (3) even when it lies ahead.
const nowSec = () => Date.now() / 1000
const upcoming = computed(() => timers.value.filter((x) => x.end > nowSec()).sort((a, b) => a.begin - b.begin))
const finished = computed(() => timers.value.filter((x) => x.end <= nowSec()).sort((a, b) => b.begin - a.begin))

// Day key and heading in the display time zone (PRINCIPLES.md: UTC inside,
// local only on screen).
function dayKey(unixSeconds: number): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: timeZone.value, year: 'numeric', month: '2-digit', day: '2-digit' }).format(unixSeconds * 1000)
}
function dayLabel(unixSeconds: number): string {
  const key = dayKey(unixSeconds)
  if (key === dayKey(nowSec())) return t('timers.today')
  if (key === dayKey(nowSec() + 86400)) return t('timers.tomorrow')
  return new Intl.DateTimeFormat(locale.value, { weekday: 'long', day: 'numeric', month: 'long', timeZone: timeZone.value }).format(unixSeconds * 1000)
}
const upcomingDays = computed(() => {
  const days: { key: string; label: string; timers: Timer[] }[] = []
  for (const timer of upcoming.value) {
    const key = dayKey(timer.begin)
    let day = days.find((d) => d.key === key)
    if (!day) {
      day = { key, label: dayLabel(timer.begin), timers: [] }
      days.push(day)
    }
    day.timers.push(timer)
  }
  return days
})

function formatDateTime(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'short', timeZone: timeZone.value }).format(unixSeconds * 1000)
}

async function load() {
  try {
    const res = await api.timers()
    timers.value = res.timers
    tuners.value = res.tuners
    loadFailed.value = false
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
}

// --- delete: preview, then confirm ---------------------------------------

const deleting = ref<DeleteState | null>(null)

async function previewDelete(timer: Timer) {
  notice.value = ''
  const key = rowKey(timer)
  deleting.value = { key, kind: 'delete', busy: true }
  try {
    const res = await api.timerDeletePlan(timer)
    if (deleting.value?.key === key) deleting.value = { key, kind: 'delete', plan: res.plan, busy: false }
  } catch (err) {
    if (deleting.value?.key === key) deleting.value = { key, kind: 'delete', busy: false, error: planErrorMessage(err) }
  }
}

// Disable/enable (decision D5): taken out and put back with the other
// flag, all or nothing — the backend restores the timer if the box
// refuses (POST /api/v1/timers/toggle/plan).
async function previewToggle(timer: Timer) {
  notice.value = ''
  const key = rowKey(timer)
  const kind = timer.disabled ? 'enable' : 'disable'
  deleting.value = { key, kind, busy: true }
  try {
    const res = await api.timerTogglePlan(timer, timer.disabled)
    if (deleting.value?.key === key) deleting.value = { key, kind, plan: res.plan, busy: false }
  } catch (err) {
    if (deleting.value?.key === key) deleting.value = { key, kind, busy: false, error: planErrorMessage(err) }
  }
}

async function confirmDelete() {
  const state = deleting.value
  if (!state?.plan) return
  deleting.value = { ...state, busy: true, error: undefined }
  try {
    await api.plans.apply(state.plan.id)
    const before = state.plan.changes[0]?.before as Timer | undefined
    deleting.value = null
    notice.value = t(`timers.done.${state.kind}`, { name: before?.name ?? '' })
    await load()
  } catch (err) {
    // A failed or stale plan is used up — the user previews again.
    deleting.value = { key: state.key, kind: state.kind, busy: false, error: planErrorMessage(err) }
  }
}

// --- clean up: remove every finished one-off timer ------------------------

const cleanup = ref<{ plan: Plan; busy: boolean; error?: string } | null>(null)
// Same rule as the backend: a switched-off timer is reported as ended too.
const cleanable = computed(() => finished.value.filter((x) => !x.disabled && !x.repeated).length)

async function previewCleanup() {
  notice.value = ''
  try {
    const res = await api.timerCleanupPlan()
    cleanup.value = res.plan ? { plan: res.plan, busy: false } : null
  } catch (err) {
    notice.value = planErrorMessage(err)
  }
}

async function confirmCleanup() {
  const state = cleanup.value
  if (!state) return
  cleanup.value = { ...state, busy: true, error: undefined }
  try {
    await api.plans.apply(state.plan.id)
    notice.value = t('timers.cleanup.done', { count: state.plan.changes.length }, state.plan.changes.length)
    cleanup.value = null
    await load()
  } catch (err) {
    cleanup.value = { plan: state.plan, busy: false, error: planErrorMessage(err) }
  }
}

// --- AutoTimer (optional plugin, read-only) -------------------------------

const autoTimers = ref<AutoTimers | null>(null)
const autoTimerError = ref('')
const simulated = ref<SimulatedTimer[] | null>(null)
const simulating = ref(false)

// The plugin's own editor lives on WebControl (port 80), same host.
const autoTimerEditorUrl = computed(() =>
  autoTimers.value?.installed ? `${window.location.protocol}//${window.location.hostname}${autoTimers.value.editorPath}` : '',
)

async function loadAutoTimers() {
  try {
    autoTimers.value = await api.autoTimers()
  } catch (err) {
    const code = err instanceof ApiError ? err.code : 'network_error'
    autoTimerError.value = code === 'webcontrol_unauthorized' ? t('error.plan.webcontrol_unauthorized') : t('timers.autoTimer.unavailable')
  }
}

async function simulate() {
  simulating.value = true
  try {
    simulated.value = (await api.autoTimerSimulate()).timers
  } catch {
    autoTimerError.value = t('timers.autoTimer.unavailable')
  } finally {
    simulating.value = false
  }
}

onMounted(() => {
  void load()
  void loadAutoTimers()
})
</script>
