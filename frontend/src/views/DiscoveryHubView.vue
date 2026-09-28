<!--
  DiscoveryHub's real screen (ROADMAP.md, "v1 — hubs"): AI recommendations
  from a chosen bouquet's EPG. The AI only proposes — "Create timer" is a
  plain call to TimerHub's own POST /api/v1/timers/plan and
  POST /api/v1/plans/{id}/apply (PRINCIPLES.md, "Preview and confirm";
  CONCEPT.md: DiscoveryHub calls TimerHub's API, not its Go code). Nothing
  here calls the AI automatically — every real API call costs money, so
  generating is always an explicit click, never on page load.

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
          {{ generating ? t('discovery.generating') : t('discovery.generate') }}
        </button>
        <span v-if="budget" class="text-[12.5px] text-muted">{{ t('discovery.budget', { used: budget.usedToday, limit: budget.dailyLimit }) }}</span>
      </div>

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
        <p v-else-if="!taste" class="mt-2 text-[13px] text-muted">{{ t('common.backendUnreachable') }}</p>
        <dl v-else class="mt-2 grid grid-cols-[160px_1fr] gap-x-4 gap-y-2 text-[13px] below-sm:grid-cols-1 below-sm:gap-y-0.5">
          <template v-for="part in tasteParts" :key="part.key">
            <dt class="text-muted below-sm:mt-2">{{ t(`discovery.taste.${part.key}`) }}</dt>
            <dd class="text-text">{{ part.items.length ? part.items.join(' · ') : t('discovery.taste.none') }}</dd>
          </template>
        </dl>
      </section>

      <p v-if="genError" class="mt-3 text-[13px] text-warn">{{ genError }}</p>
      <p v-if="usedTaste && hasGenerated" class="mt-3 text-[12.5px] text-faint">{{ usedTaste }}</p>

      <p v-if="!hasGenerated && !loading" class="mt-5 text-[14px] text-muted">{{ t('discovery.empty') }}</p>
      <p v-else-if="hasGenerated && recommendations.length === 0 && !generating" class="mt-5 text-[14px] text-muted">
        {{ t('discovery.noPicks') }}
      </p>
      <ul v-else class="mt-5 flex flex-col gap-3">
        <li v-for="(rec, i) in recommendations" :key="i" class="rounded border border-line bg-tile p-4">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <span class="block truncate text-[14.5px] font-semibold text-text">{{ rec.title }}</span>
              <span class="block truncate text-[12.5px] text-muted">{{ rec.serviceName }} · {{ formatTime(rec.start) }}</span>
              <span class="mt-1 block text-[13px] text-text">{{ rec.reason }}</span>
              <EnrichDetails class="mt-3" :info="enrich.info(rec.title)" :shown-title="rec.title" :poster-width="60" />
            </div>
            <button
              v-if="!timerState[i] || timerState[i]?.status === 'error'"
              type="button"
              class="shrink-0 rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field"
              :disabled="!canCreateTimers"
              :title="canCreateTimers ? undefined : t('discovery.timerHubUnavailable')"
              @click="previewTimer(i, rec)"
            >
              {{ t('discovery.createTimer') }}
            </button>
            <span v-else-if="timerState[i]?.status === 'created'" class="shrink-0 text-[13px] text-ok">{{ t('discovery.created') }}</span>
          </div>

          <div v-if="timerState[i]?.status === 'previewing'" class="mt-3 rounded border border-line-soft bg-bg p-3">
            <ul v-if="(timerState[i]?.plan?.warnings.length ?? 0) > 0" class="mb-2 flex flex-col gap-1 text-[12.5px] text-warn">
              <li v-for="(w, wi) in timerState[i]?.plan?.warnings" :key="wi">{{ describe(w) }}</li>
            </ul>
            <div class="flex gap-2">
              <button type="button" class="rounded bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg" @click="confirmTimer(i)">
                {{ t('discovery.confirm') }}
              </button>
              <button type="button" class="rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field" @click="cancelTimer(i)">
                {{ t('discovery.cancel') }}
              </button>
            </div>
          </div>
          <p v-else-if="timerState[i]?.status === 'error'" class="mt-2 text-[12.5px] text-warn">{{ timerState[i]?.error }}</p>
        </li>
      </ul>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import Icon from '@/components/Icon.vue'
import EnrichDetails from '@/components/enrich/EnrichDetails.vue'
import { useEnrichStore } from '@/stores/enrich'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { usePlanError } from '@/composables/usePlanError'
import type { Bouquet, DiscoveryStatus, Plan, Recommendation, TasteCounts, TasteProfile } from '@/api/types'

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

type TimerState = { status: 'previewing' | 'created' | 'error'; plan?: Plan; error?: string }
const timerState = reactive<Record<number, TimerState>>({})

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
    recommendations.value = res.recommendations
    lastTaste.value = res.taste ?? null
    budget.value = { usedToday: res.usedToday, dailyLimit: res.dailyLimit }
    hasGenerated.value = true
    Object.keys(timerState).forEach((k) => delete timerState[Number(k)])
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

async function previewTimer(i: number, rec: Recommendation) {
  try {
    const res = await api.timerPlan({
      serviceReference: rec.serviceReference,
      serviceName: rec.serviceName,
      name: rec.title,
      description: rec.reason,
      begin: rec.start,
      end: rec.start + rec.duration,
    })
    timerState[i] = { status: 'previewing', plan: res.plan }
  } catch (err) {
    timerState[i] = { status: 'error', error: planErrorMessage(err) }
  }
}

async function confirmTimer(i: number) {
  const plan = timerState[i]?.plan
  if (!plan) return
  try {
    await api.plans.apply(plan.id)
    timerState[i] = { status: 'created' }
  } catch (err) {
    timerState[i] = { status: 'error', error: planErrorMessage(err) }
  }
}

function cancelTimer(i: number) {
  delete timerState[i]
}

async function loadStatus() {
  try {
    budget.value = await api.discovery.status()
  } catch {
    // Non-fatal — the generate button just won't show a budget hint yet.
  }
}

onMounted(async () => {
  await loadStatus()
  try {
    const res = await api.guide.bouquets()
    bouquets.value = res.bouquets
    if (res.bouquets.length > 0) selectedBouquet.value = res.bouquets[0].serviceReference
  } catch {
    // Bouquet list unavailable — the generate button stays disabled
    // (selectedBouquet stays empty), no separate error banner needed.
  } finally {
    loading.value = false
  }
})
</script>
