<!--
  One tracked series (GET /api/v1/series/markers/{id}): SerienRecorder's
  settings for it, its timered episodes, further broadcasts in the box's
  EPG (each recordable through TimerHub's own plan → apply, the same
  inline preview DiscoveryHub uses), and the recordings already on disk,
  grouped by season where the recordings carry an S01E02 code. Every part
  comes from a different source; one that couldn't be loaded says so
  instead of looking empty.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <RouterLink to="/series" class="inline-flex items-center gap-1 rounded text-[13px] text-accent hover:underline">
      <Icon name="chevron-left" :size="14" />
      {{ t('series.detail.back') }}
    </RouterLink>

    <!-- Until the series is known, the page is still headed by its hub. -->
    <h1 v-if="!detail" class="sr-only">{{ t('hub.series') }}</h1>
    <p v-if="loading" class="mt-5 text-[14px] text-muted" aria-live="polite">{{ t('common.loading') }}</p>
    <p v-else-if="notFound" class="mt-5 text-[14px] text-muted">{{ t('series.detail.notFound') }}</p>
    <p v-else-if="!detail" class="mt-5 text-[14px] text-muted"><ErrorText /></p>
    <template v-else>
      <div class="mt-3 flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h1 class="text-[26px] font-bold">{{ detail.marker.name }}</h1>
        <span class="text-[13.5px] text-muted">{{ detail.marker.info }}</span>
        <span v-if="!detail.marker.active" class="flex items-center gap-1.5 text-[13px] text-faint">
          <span class="h-1.5 w-1.5 rounded-full bg-faint" />{{ t('series.inactive') }}
        </span>
      </div>

      <!-- What the box already keeps comes first: the folder cover and
           SerienRecorder's own description; TMDB fills in what's missing. -->
      <div v-if="detail.cover || detail.info || tmdb" class="mt-4 flex max-w-[900px] gap-4">
        <PosterImage
          v-if="detail.cover || tmdb?.poster"
          :series="detail.cover ? detail.marker.id : undefined"
          :path="detail.cover ? undefined : tmdb?.poster"
          size="w185"
          :width="120"
          :height="180"
          :alt="detail.marker.name"
        />
        <div class="min-w-0 flex-1">
          <img v-if="logoSrc" :src="logoSrc" :alt="detail.marker.name" class="mb-1.5 h-9 max-w-[220px] object-contain object-left" />
          <p class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[13px] text-muted">
            <span v-if="detail.info?.year" class="tabular-nums">{{ detail.info.year }}</span>
            <span v-if="detail.info?.info">{{ detail.info.info }}</span>
            <span v-else-if="tmdb?.genres?.length">{{ tmdb.genres.join(', ') }}</span>
            <RatingBadge v-if="tmdb" :rating="tmdb.rating" :votes="tmdb.votes" />
            <span v-for="c in chips" :key="c.id" class="whitespace-nowrap rounded-sm border border-line px-1 text-[11.5px] tabular-nums text-muted" :title="c.title">
              {{ c.label }} <span class="text-text">{{ c.value }}</span>
            </span>
          </p>
          <p v-if="detail.info?.description || tmdb?.overview" class="mt-1.5 text-[13.5px] text-text">
            {{ detail.info?.description || tmdb?.overview }}
          </p>
          <p v-if="detail.info?.cast?.length" class="mt-1.5 truncate text-[12.5px] text-muted" :title="detail.info.cast.join(', ')">
            {{ t('series.detail.cast', { names: detail.info.cast.slice(0, 4).join(', ') }) }}
          </p>
          <p class="mt-1.5 text-[11.5px] text-faint">{{ t('enrich.source', { source: sourceNames }) }}</p>
        </div>
      </div>

      <dl class="mt-4 grid grid-cols-4 gap-3 below-md:grid-cols-2 below-sm:grid-cols-1">
        <div v-for="fact in facts" :key="fact.label" class="rounded border border-line bg-tile px-3 py-2.5">
          <dt class="text-[12.5px] text-faint">{{ fact.label }}</dt>
          <dd class="mt-0.5 truncate text-[14px] text-text" :class="{ 'font-mono text-[13px]': fact.mono }" :title="fact.value">
            {{ fact.value }}
          </dd>
        </div>
      </dl>

      <div class="mt-6 grid grid-cols-2 items-start gap-6 below-lg:grid-cols-1">
        <!-- Timered by SerienRecorder -->
        <section>
          <h2 class="text-[15.5px] font-semibold text-text">{{ t('series.detail.upcoming') }}</h2>
          <p v-if="isUnavailable('upcoming')" class="mt-2 text-[13.5px] text-muted">{{ t('series.detail.partUnavailable') }}</p>
          <p v-else-if="detail.upcoming.length === 0" class="mt-2 text-[13.5px] text-muted">{{ t('series.detail.upcomingEmpty') }}</p>
          <ul v-else class="mt-2 flex flex-col">
            <li v-for="(ep, i) in detail.upcoming" :key="i" class="flex items-center gap-3 border-b border-line-soft py-2 last:border-none">
              <span class="w-36 shrink-0 whitespace-nowrap text-[12.5px] tabular-nums text-faint">{{ formatTime(ep.startTime) }}</span>
              <span class="min-w-0 flex-1">
                <span class="block truncate text-[14px] text-text">{{ ep.title }}</span>
                <span class="block truncate text-[12.5px] text-muted">
                  {{ t('series.detail.code', { season: ep.season, episode: ep.episode }) }} · {{ ep.channel }}
                </span>
              </span>
            </li>
          </ul>
        </section>

        <!-- Further broadcasts in the box's EPG -->
        <section>
          <h2 class="text-[15.5px] font-semibold text-text">{{ t('series.detail.broadcasts') }}</h2>
          <p class="mt-0.5 text-[12.5px] text-faint">{{ t('series.detail.broadcastsHint') }}</p>
          <p v-if="isUnavailable('broadcasts')" class="mt-2 text-[13.5px] text-muted">{{ t('series.detail.partUnavailable') }}</p>
          <p v-else-if="detail.broadcasts.length === 0" class="mt-2 text-[13.5px] text-muted">{{ t('series.detail.broadcastsEmpty') }}</p>
          <template v-else>
            <p v-if="missingAiring > 0" class="mt-2 flex items-center gap-1.5 text-[13.5px] text-warn" role="status">
              <span class="h-1.5 w-1.5 rounded-full bg-warn" />{{ t('series.detail.missingAiring', { n: missingAiring }, missingAiring) }}
            </p>
            <ul class="mt-2 flex flex-col">
              <li v-for="b in detail.broadcasts" :key="b.serviceReference + b.start" class="border-b border-line-soft py-2 last:border-none">
                <div class="flex items-center gap-3">
                  <span class="w-36 shrink-0 whitespace-nowrap text-[12.5px] tabular-nums text-faint">{{ formatTime(b.start) }}</span>
                  <span class="min-w-0 flex-1">
                    <span class="block truncate text-[14px] text-text">
                      <span v-if="b.episode" class="me-1.5 font-mono text-[12.5px] text-muted">{{ t('series.detail.code', { season: b.season ?? 0, episode: b.episode }) }}</span>{{ b.title }}
                    </span>
                    <span class="block truncate text-[12.5px] text-muted">
                      {{ b.serviceName }}<span v-if="b.alsoOn?.length" :title="b.alsoOn.join(', ')"> · {{ t('series.detail.alsoOn', { n: b.alsoOn.length }, b.alsoOn.length) }}</span>
                      <template v-if="b.episodeStatus === 'missing' || b.episodeStatus === 'recorded'">
                        ·
                        <span :class="b.episodeStatus === 'missing' ? 'text-warn' : 'text-ok'">{{ t(`series.detail.episodeStatus.${b.episodeStatus}`) }}</span>
                      </template>
                    </span>
                  </span>
                  <span v-if="b.hasTimer || record[key(b)]?.status === 'created'" class="flex shrink-0 items-center gap-1.5 text-[12.5px] text-ok">
                    <span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('series.detail.hasTimer') }}
                  </span>
                  <button
                    v-else-if="record[key(b)]?.status !== 'previewing'"
                    type="button"
                    class="shrink-0 rounded px-2 py-1 text-[13px] text-accent hover:bg-tile-2 disabled:text-faint"
                    :disabled="!canCreateTimers"
                    :title="canCreateTimers ? undefined : t('discovery.timerHubUnavailable')"
                    @click="previewRecord(b)"
                  >
                    {{ t('series.detail.record') }}
                  </button>
                </div>
                <PlanPreview
                  v-if="record[key(b)]?.status === 'previewing' && record[key(b)]?.plan"
                  class="mt-2"
                  :plan="record[key(b)]!.plan!"
                  :confirm-label="t('series.detail.createTimer')"
                  :busy="record[key(b)]?.busy"
                  :error="record[key(b)]?.error"
                  @confirm="applyRecord(b)"
                  @cancel="delete record[key(b)]"
                >
                  {{ t('series.detail.recordSummary', { title: b.title, channel: b.serviceName, time: formatTime(b.start) }) }}
                </PlanPreview>
                <p v-else-if="record[key(b)]?.status === 'error'" class="mt-1 text-[12.5px] text-warn">{{ record[key(b)]?.error }}</p>
              </li>
            </ul>
          </template>
        </section>
      </div>

      <EpisodeGuide class="mt-8" :guide="detail.episodes" />

      <!-- On disk -->
      <section class="mt-8">
        <h2 class="text-[15.5px] font-semibold text-text">
          {{ t('series.detail.recordings') }}
          <span v-if="detail.recordings.length" class="ms-1 text-[13px] font-normal text-faint">{{ detail.recordings.length }}</span>
        </h2>
        <p v-if="isUnavailable('recordings')" class="mt-2 text-[13.5px] text-muted">{{ t('series.detail.partUnavailable') }}</p>
        <p v-else-if="detail.recordings.length === 0" class="mt-2 text-[13.5px] text-muted">{{ t('series.detail.recordingsEmpty') }}</p>
        <template v-else>
          <div v-for="group in recordingGroups" :key="group.season" class="mt-3">
            <h3 v-if="recordingGroups.length > 1 || group.season > 0" class="text-[13.5px] font-semibold text-muted">
              {{ group.season > 0 ? t('series.detail.season', { season: group.season }) : t('series.detail.noSeason') }}
            </h3>
            <ul class="mt-1 flex flex-col">
              <li v-for="r in group.items" :key="r.path" class="flex items-center gap-3 border-b border-line-soft py-2 last:border-none">
                <span class="w-36 shrink-0 whitespace-nowrap text-[12.5px] tabular-nums text-faint">{{ r.time ? formatDate(r.time) : '' }}</span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-[14px] text-text">
                    <span v-if="r.episode" class="me-1.5 text-muted">{{ t('series.detail.code', { season: r.season, episode: r.episode }) }}</span>{{ r.title }}
                  </span>
                  <span class="block truncate text-[12.5px] text-muted">{{ [r.description, r.serviceName].filter(Boolean).join(' · ') }}</span>
                </span>
                <span class="shrink-0 text-[12.5px] tabular-nums text-faint">{{ r.fileSize ? formatBytes(r.fileSize, locale) : '' }}</span>
              </li>
            </ul>
          </div>
        </template>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import ErrorText from '@/components/ErrorText.vue'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import Icon from '@/components/Icon.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import PosterImage from '@/components/enrich/PosterImage.vue'
import RatingBadge from '@/components/enrich/RatingBadge.vue'
import { sourceName } from '@/lib/enrichSource'
import EpisodeGuide from '@/components/series/EpisodeGuide.vue'
import { useEnrichStore } from '@/stores/enrich'
import { useEnrichExtras } from '@/composables/useEnrichExtras'
import { useNavigationStore } from '@/stores/navigation'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { usePlanError } from '@/composables/usePlanError'
import { formatBytes } from '@/lib/format'
import type { Plan, SeriesBroadcast, SeriesDetail, SeriesRecording } from '@/api/types'

const props = defineProps<{ id: number }>()

const { t, locale } = useI18n()
const navigation = useNavigationStore()
const enrich = useEnrichStore()
const { timeZone } = useDisplayTimeZone()
const { planErrorMessage } = usePlanError()

const detail = ref<SeriesDetail | null>(null)
const loading = ref(true)
const notFound = ref(false)

async function load() {
  loading.value = true
  notFound.value = false
  try {
    detail.value = await api.series.detail(props.id)
  } catch (err) {
    detail.value = null
    notFound.value = err instanceof ApiError && err.status === 404
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(() => props.id, load)

const tmdb = computed(() => (detail.value ? enrich.info(detail.value.marker.name, 'tv') : null))
// Ratings (OMDb) and the logo (fanart.tv) for the series, when set up.
const { logoSrc, chips, sources: extraSources } = useEnrichExtras(tmdb)
// Names the sources actually shown.
const sourceNames = computed(() => {
  const d = detail.value
  const names: string[] = []
  if (d?.info) names.push('SerienRecorder')
  if (tmdb.value && (!d?.cover || !d?.info || tmdb.value.rating)) names.push(sourceName(tmdb.value))
  names.push(...extraSources.value)
  return names.join(', ')
})

function isUnavailable(part: SeriesDetail['unavailable'][number]) {
  return detail.value?.unavailable.includes(part) ?? false
}

const facts = computed(() => {
  const m = detail.value?.marker
  if (!m) return []
  return [
    { label: t('series.detail.channels'), value: m.channels || '—' },
    { label: t('series.detail.seasons'), value: m.seasons || '—' },
    { label: t('series.detail.recordedBySR'), value: new Intl.NumberFormat(locale.value).format(m.numberOfRecords) },
    { label: t('series.detail.folder'), value: m.recordfolder || '—', mono: true },
  ]
})

// Newest season first; recordings without an S01E02 code last.
const recordingGroups = computed(() => {
  const bySeason = new Map<number, SeriesRecording[]>()
  for (const r of detail.value?.recordings ?? []) {
    if (!bySeason.has(r.season)) bySeason.set(r.season, [])
    bySeason.get(r.season)!.push(r)
  }
  return [...bySeason.entries()]
    .sort(([a], [b]) => (a === 0 ? 1 : b === 0 ? -1 : b - a))
    .map(([season, items]) => ({ season, items: items.sort((x, y) => y.episode - x.episode || y.time - x.time) }))
})

function formatTime(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, {
    weekday: 'short',
    day: '2-digit',
    month: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
    timeZone: timeZone.value,
  }).format(unixSeconds * 1000)
}
function formatDate(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeZone: timeZone.value }).format(unixSeconds * 1000)
}

// Recording a broadcast goes through TimerHub's own plan → apply.
const canCreateTimers = computed(
  () => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.id === 'timer')?.status !== 'unavailable',
)
type RecordState = { status: 'previewing' | 'created' | 'error'; plan?: Plan; busy?: boolean; error?: string }
const record = reactive<Record<string, RecordState>>({})
// Broadcasts of episodes the user hasn't recorded yet and nothing plans
// (series_broadcast_episodes.go) — "this one is missing, and it airs".
const missingAiring = computed(() => {
  const seen = new Set<string>()
  for (const b of detail.value?.broadcasts ?? []) {
    if (b.episodeStatus === 'missing' && !b.hasTimer && record[key(b)]?.status !== 'created') seen.add(`${b.season}-${b.episode}`)
  }
  return seen.size
})
const key = (b: SeriesBroadcast) => `${b.serviceReference}@${b.start}`

async function previewRecord(b: SeriesBroadcast) {
  try {
    const res = await api.timerPlan({
      serviceReference: b.serviceReference,
      serviceName: b.serviceName,
      name: b.title,
      description: b.description,
      begin: b.start,
      end: b.start + b.duration,
      eventId: b.id || undefined,
    })
    record[key(b)] = { status: 'previewing', plan: res.plan }
  } catch (err) {
    record[key(b)] = { status: 'error', error: planErrorMessage(err) }
  }
}

async function applyRecord(b: SeriesBroadcast) {
  const state = record[key(b)]
  if (!state?.plan) return
  state.busy = true
  state.error = undefined
  try {
    await api.plans.apply(state.plan.id)
    record[key(b)] = { status: 'created' }
  } catch (err) {
    state.busy = false
    state.error = planErrorMessage(err)
  }
}
</script>
