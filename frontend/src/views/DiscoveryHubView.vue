<!--
  DiscoveryHub's real screen (ROADMAP.md, "v1 — hubs"): AI recommendations
  from a chosen bouquet's EPG. The AI only proposes — "Create timer" is a
  plain call to TimerHub's own POST /api/v1/timers/plan and
  POST /api/v1/plans/{id}/apply (PRINCIPLES.md, "Preview and confirm";
  CONCEPT.md: DiscoveryHub calls TimerHub's API, not its Go code). Nothing
  here calls the AI automatically — every real API call costs money, so
  generating is always an explicit click, never on page load.

  The last generated set is kept server-side (GET
  /api/v1/discovery/recommendations), so leaving the page or reloading
  shows it again without a new AI call; ended broadcasts drop out. Picks
  are cards grouped by day; a card opens the details (RecommendationDetail:
  picture, the data sources' info, why it was picked, Create timer).

  "Use my taste" sends what the user records, follows and has planned
  (titles only) along, so the picks fit them. It's the user's own viewing
  data going to an external provider, so it's an explicit, remembered
  choice, and "What gets sent" shows exactly that list before anything is
  sent (GET /api/v1/discovery/taste sends nothing).
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="recommendations" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ t(item ? item.label : 'hub.discovery') }}</h1>
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
        <button
          type="button"
          class="rounded bg-accent px-3.5 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-60"
          :disabled="generating || !selectedBouquet || budgetExhausted"
          @click="generate"
        >
          {{ generating ? t('discovery.generating') : hasGenerated ? t('discovery.regenerate') : t('discovery.generate') }}
        </button>
        <span v-if="budget" class="text-[12.5px] text-muted">{{ t('discovery.budget', { used: budget.usedToday, limit: budget.dailyLimit }) }}</span>
      </div>
      <p v-if="generatedAt" class="mt-2 text-[12.5px] text-faint">
        {{ t('discovery.generatedAt', { time: formatGenerated(generatedAt), bouquet: generatedBouquetName }) }}
      </p>

      <div class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1">
        <label class="flex cursor-pointer items-center gap-2 text-[13.5px] text-text">
          <input v-model="useTaste" type="checkbox" class="rounded" />
          {{ t('discovery.taste.use') }}
        </label>
        <button
          type="button"
          class="rounded px-1.5 py-0.5 text-[13px] text-accent hover:bg-tile-2"
          :aria-expanded="tasteShown"
          aria-controls="taste-panel"
          @click="toggleTaste"
        >
          {{ tasteShown ? t('discovery.taste.hide') : t('discovery.taste.show') }}
        </button>
      </div>
      <section v-if="tasteShown" id="taste-panel" class="mt-2 rounded border border-line bg-tile p-4">
        <p class="text-[12.5px] text-faint">{{ t('discovery.taste.explain') }}</p>
        <p v-if="tasteLoading" class="mt-2 text-[13px] text-muted">{{ t('common.loading') }}</p>
        <p v-else-if="!taste" class="mt-2 text-[13px] text-muted"><ErrorText /></p>
        <dl v-else class="mt-2 grid grid-cols-[160px_1fr] gap-x-4 gap-y-2 text-[13px] below-sm:grid-cols-1 below-sm:gap-y-0.5">
          <template v-for="part in tasteParts" :key="part.key">
            <dt class="text-muted below-sm:mt-2">{{ t(`discovery.taste.${part.key}`) }}</dt>
            <dd class="text-text">{{ part.items.length ? part.items.join(' · ') : t('discovery.taste.none') }}</dd>
          </template>
        </dl>
      </section>

      <p v-if="genError" class="mt-3 text-[13px] text-warn">{{ genError }}</p>
      <p v-if="usedTaste && hasGenerated" class="mt-3 text-[12.5px] text-faint">{{ usedTaste }}</p>

      <div v-if="!hasGenerated && !loading" class="mt-6 flex flex-col items-center gap-3 rounded border border-dashed border-line px-6 py-10 text-center">
        <Icon name="recommendations" :size="28" class="text-faint" />
        <p class="max-w-md text-[14px] text-muted">{{ bouquets.length > 1 ? t('discovery.empty') : t('discovery.emptySingle') }}</p>
      </div>
      <p v-else-if="hasGenerated && recommendations.length === 0 && !generating" class="mt-5 text-[14px] text-muted">
        {{ t('discovery.noPicks') }}
      </p>
      <template v-else>
        <section v-for="g in groups" :key="g.key" class="mt-6">
          <h2 class="text-[15.5px] font-semibold text-text">{{ g.label }}</h2>
          <ul class="mt-2 grid grid-cols-2 gap-3 below-md:grid-cols-1">
            <li v-for="rec in g.items" :key="keyOf(rec)">
              <RecommendationCard
                :rec="rec"
                :info="enrich.info(rec.title)"
                :time="formatClock(rec.start)"
                :now="nowSec"
                :has-timer="hasTimer(rec)"
                @open="openKey = keyOf(rec)"
              />
            </li>
          </ul>
        </section>
      </template>
    </template>

    <RecommendationDetail
      v-if="opened"
      :rec="opened"
      :info="enrich.info(opened.title)"
      :when="formatTime(opened.start)"
      :now="nowSec"
      :has-timer="hasTimer(opened)"
      :state="timerState[keyOf(opened)]"
      :can-create-timers="canCreateTimers"
      @close="openKey = ''"
      @preview-timer="previewTimer(opened)"
      @confirm-timer="confirmTimer(opened)"
      @cancel-timer="delete timerState[keyOf(opened)]"
    />
  </div>
</template>

<script setup lang="ts">
import ErrorText from '@/components/ErrorText.vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import Icon from '@/components/Icon.vue'
import RecommendationCard from '@/components/discovery/RecommendationCard.vue'
import RecommendationDetail from '@/components/discovery/RecommendationDetail.vue'
import type { TimerState } from '@/components/discovery/types'
import { useEnrichStore } from '@/stores/enrich'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { usePlanError } from '@/composables/usePlanError'
import { dayKey as zonedDayKey } from '@/lib/zonedTime'
import type { Bouquet, DiscoveryStatus, Recommendation, TasteCounts, TasteProfile } from '@/api/types'

const { t, locale } = useI18n()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()
const { timeZone } = useDisplayTimeZone()
const { planErrorMessage } = usePlanError()
const enrich = useEnrichStore()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/discovery'))
// TimerHub is an optional requirement (CONCEPT.md); DiscoveryHub itself can
// still be "available" without it, but timer creation specifically needs it.
const canCreateTimers = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.id === 'timer')?.status !== 'unavailable')

const bouquets = ref<Bouquet[]>([])
const selectedBouquet = ref('')
const budget = ref<DiscoveryStatus | null>(null)
const budgetExhausted = computed(() => (budget.value ? budget.value.usedToday >= budget.value.dailyLimit : false))

const loading = ref(true)
const generating = ref(false)
const hasGenerated = ref(false)
const genError = ref('')
const recommendations = ref<Recommendation[]>([])

// The taste profile: an explicit, per-browser choice (see the comment on
// top), off until the user turns it on.
const TASTE_KEY = 'dreamhub.discoveryUseTaste'
function readUseTaste(): boolean {
  try {
    return localStorage.getItem(TASTE_KEY) === '1'
  } catch {
    return false
  }
}
const useTaste = ref(readUseTaste())
watch(useTaste, (on) => {
  try {
    localStorage.setItem(TASTE_KEY, on ? '1' : '0')
  } catch {
    // Not remembered, still applies to this visit.
  }
})

const tasteShown = ref(false)
const tasteLoading = ref(false)
const taste = ref<TasteProfile | null>(null)
async function toggleTaste() {
  tasteShown.value = !tasteShown.value
  if (!tasteShown.value) return
  tasteLoading.value = true
  try {
    taste.value = await api.discovery.taste()
  } catch {
    taste.value = null
  } finally {
    tasteLoading.value = false
  }
}
const tasteParts = computed(() =>
  taste.value
    ? (['series', 'recordings', 'timers', 'autoTimers'] as const).map((key) => ({ key, items: taste.value![key] ?? [] }))
    : [],
)

// "Considered: 3 series, 38 recordings, 2 timers" under the results.
const lastTaste = ref<TasteCounts | null>(null)
const usedTaste = computed(() => {
  const c = lastTaste.value
  if (!c) return ''
  const parts = (['series', 'recordings', 'timers', 'autoTimers'] as const)
    .filter((k) => c[k] > 0)
    .map((k) => t(`discovery.taste.count.${k}`, { n: c[k] }, c[k]))
  return parts.length ? t('discovery.taste.considered', { list: parts.join(', ') }) : ''
})

// Timer state per recommendation, by channel and start (stable across a
// reload of the saved set, unlike a list index).
const keyOf = (r: Recommendation) => `${r.serviceReference}@${r.start}`
const timerState = reactive<Record<string, TimerState>>({})
const hasTimer = (r: Recommendation) => !!r.hasTimer || timerState[keyOf(r)]?.status === 'created'

// The detail dialog: which recommendation is open.
const openKey = ref('')
const opened = computed(() => recommendations.value.find((r) => keyOf(r) === openKey.value) ?? null)

// When the shown set was generated, and from which bouquet.
const generatedAt = ref(0)
const generatedBouquet = ref('')
const generatedBouquetName = computed(
  () => bouquets.value.find((b) => b.serviceReference === generatedBouquet.value)?.serviceName ?? '',
)

// "Now" for the on-air marker, ticking once a minute.
const nowSec = ref(Math.floor(Date.now() / 1000))
const ticker = setInterval(() => (nowSec.value = Math.floor(Date.now() / 1000)), 60_000)
onBeforeUnmount(() => clearInterval(ticker))

// Day of a Unix time in the display zone, as YYYY-MM-DD (for grouping).
const dayKey = (unixSeconds: number) => zonedDayKey(unixSeconds, timeZone.value)
// Picks grouped by day ("Today", "Tomorrow", else the date), in airing order.
const groups = computed(() => {
  const today = dayKey(nowSec.value)
  const tomorrow = dayKey(nowSec.value + 86400)
  const sorted = [...recommendations.value].sort((a, b) => a.start - b.start)
  const out: { key: string; label: string; items: Recommendation[] }[] = []
  for (const r of sorted) {
    const k = dayKey(r.start)
    let g = out.find((x) => x.key === k)
    if (!g) {
      const label =
        k === today
          ? t('discovery.today')
          : k === tomorrow
            ? t('discovery.tomorrow')
            : new Intl.DateTimeFormat(locale.value, { weekday: 'long', day: 'numeric', month: 'long', timeZone: timeZone.value }).format(r.start * 1000)
      g = { key: k, label, items: [] }
      out.push(g)
    }
    g.items.push(r)
  }
  return out
})

function formatClock(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, { timeStyle: 'short', timeZone: timeZone.value }).format(unixSeconds * 1000)
}

// "today, 07:15" / a full date for an older set.
function formatGenerated(unixSeconds: number): string {
  if (dayKey(unixSeconds) === dayKey(nowSec.value)) return t('discovery.todayAt', { time: formatClock(unixSeconds) })
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short', timeZone: timeZone.value }).format(unixSeconds * 1000)
}

function showPicks(recs: Recommendation[], at: number, bouquet: string) {
  recommendations.value = recs
  generatedAt.value = at
  generatedBouquet.value = bouquet
  hasGenerated.value = true
}

function translateOrRaw(key: string, raw: string): string {
  const result = t(key)
  return result === key ? raw : result
}

function formatTime(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short', timeZone: timeZone.value }).format(
    unixSeconds * 1000,
  )
}

async function generate() {
  if (!selectedBouquet.value) return
  generating.value = true
  genError.value = ''
  try {
    const res = await api.discovery.generate(selectedBouquet.value, useTaste.value)
    Object.keys(timerState).forEach((k) => delete timerState[k])
    openKey.value = ''
    showPicks(res.recommendations, res.generatedAt, res.bouquet)
    lastTaste.value = res.taste ?? null
    budget.value = { usedToday: res.usedToday, dailyLimit: res.dailyLimit }
  } catch (err) {
    const code = err instanceof ApiError ? err.code : 'network_error'
    genError.value = translateOrRaw(`error.discovery.${code}`, code)
    if (err instanceof ApiError && err.code !== 'network_error') {
      // The backend still reports the up-to-date budget even on a
      // budget/config error — refresh so the button/label stay honest.
      void loadStatus()
    }
  } finally {
    generating.value = false
  }
}

async function previewTimer(rec: Recommendation) {
  const k = keyOf(rec)
  try {
    const res = await api.timerPlan({
      serviceReference: rec.serviceReference,
      serviceName: rec.serviceName,
      name: rec.title,
      description: rec.reason,
      begin: rec.start,
      end: rec.start + rec.duration,
    })
    timerState[k] = { status: 'previewing', plan: res.plan }
  } catch (err) {
    timerState[k] = { status: 'error', error: planErrorMessage(err) }
  }
}

async function confirmTimer(rec: Recommendation) {
  const k = keyOf(rec)
  const st = timerState[k]
  if (!st?.plan) return
  st.busy = true
  try {
    await api.plans.apply(st.plan.id)
    timerState[k] = { status: 'created' }
  } catch (err) {
    timerState[k] = { status: 'previewing', plan: st.plan, error: planErrorMessage(err) }
  }
}

async function loadStatus() {
  try {
    budget.value = await api.discovery.status()
  } catch {
    // Non-fatal — the generate button just won't show a budget hint yet.
  }
}

onMounted(async () => {
  const [, saved] = await Promise.all([loadStatus(), api.discovery.saved().catch(() => null)])
  if (saved?.saved?.recommendations.length) {
    showPicks(saved.saved.recommendations, saved.saved.generatedAt, saved.saved.bouquet)
    lastTaste.value = saved.saved.taste ?? null
  }
  try {
    const res = await api.guide.bouquets()
    bouquets.value = res.bouquets
    const keep = res.bouquets.find((b) => b.serviceReference === generatedBouquet.value)
    if (keep) selectedBouquet.value = keep.serviceReference
    else if (res.bouquets.length > 0) selectedBouquet.value = res.bouquets[0].serviceReference
  } catch {
    // Bouquet list unavailable — the generate button stays disabled
    // (selectedBouquet stays empty), no separate error banner needed.
  } finally {
    loading.value = false
  }
})
</script>
