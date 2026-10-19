<!--
  SeriesHub's real screen (ROADMAP.md, "v1 — hubs"): series tracked in
  SerienRecorder and their upcoming episodes (GET /api/v1/series/markers,
  GET /api/v1/series/upcoming); each series opens its own detail page
  (SeriesDetailView.vue). Same status-banner-plus-content pattern as
  MediaHubView.vue.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="series" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ hubTitle('series', 'hub.series') }}</h1>
    </div>

    <HubStatusBanner :item="item" />

    <template v-if="!item || item.status !== 'unavailable'">
      <section class="mt-5 flex flex-wrap items-center justify-between gap-3 rounded-sm border border-line bg-tile p-4">
        <div>
          <h2 class="text-[14px] font-semibold text-text">{{ t('series.webInterface.title') }}</h2>
          <p class="mt-0.5 text-[12.5px] text-muted">{{ t('series.webInterface.body') }}</p>
        </div>
        <div class="flex shrink-0 gap-2">
          <button type="button" class="rounded-sm border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field" @click="showEmbed = !showEmbed">
            {{ showEmbed ? t('series.webInterface.hide') : t('series.webInterface.show') }}
          </button>
          <a
            :href="serienRecorderUrl"
            target="_blank"
            rel="noopener"
            class="rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg"
          >
            {{ t('series.webInterface.openTab') }}
          </a>
        </div>
      </section>
      <iframe
        v-if="showEmbed"
        :src="serienRecorderUrl"
        :title="t('series.webInterface.title')"
        class="mt-3 h-[600px] w-full rounded-sm border border-line"
      />

      <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted"><ErrorText /></p>
      <template v-else>
        <section>
          <h2 class="mt-5 text-[15px] font-semibold text-text">{{ t('series.tracked') }}</h2>
          <p v-if="markers.length === 0" class="mt-2 text-[14px] text-muted">{{ t('series.trackedEmpty') }}</p>
          <ul v-else class="mt-2 flex flex-col">
            <li v-for="m in markers" :key="m.id" class="border-b border-line-soft last:border-none">
              <RouterLink
                :to="`/series/${m.id}`"
                class="-mx-2 flex items-center gap-3 rounded-sm px-2 py-2.5 hover:bg-tile-2"
                :class="{ 'opacity-60': !m.active }"
              >
                <PosterImage v-if="enrich.enabled" :path="enrich.info(m.name, 'tv')?.poster" :alt="m.name" />
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-[14px] text-text">{{ m.name }}</span>
                  <span class="block truncate text-[12.5px] text-muted">{{ m.info }}</span>
                </span>
                <span v-if="!m.active" class="shrink-0 text-[12.5px] text-faint">{{ t('series.inactive') }}</span>
                <Icon name="chevron-right" :size="16" class="shrink-0 text-faint rtl:rotate-180" />
              </RouterLink>
            </li>
          </ul>
        </section>

        <section>
          <h2 class="mt-6 text-[15px] font-semibold text-text">{{ t('series.upcoming') }}</h2>
          <p v-if="upcoming.length === 0" class="mt-2 text-[14px] text-muted">{{ t('series.upcomingEmpty') }}</p>
          <ul v-else class="mt-2 flex flex-col">
            <li v-for="(ep, i) in upcoming" :key="i" class="flex items-center gap-3 border-b border-line-soft py-2 last:border-none">
              <span class="w-28 shrink-0 whitespace-nowrap text-[12.5px] font-mono text-faint">{{ formatTime(ep.startTime) }}</span>
              <span class="min-w-0 flex-1">
                <RouterLink
                  v-if="markerIdByFsid[ep.fsID]"
                  :to="`/series/${markerIdByFsid[ep.fsID]}`"
                  class="block truncate text-[14px] text-text hover:text-accent"
                >
                  {{ ep.series }}
                </RouterLink>
                <span v-else class="block truncate text-[14px] text-text">{{ ep.series }}</span>
                <span class="block truncate text-[12.5px] text-muted">
                  {{ t('series.episode', { season: ep.season, episode: ep.episode, title: ep.title }) }} · {{ ep.channel }}
                </span>
              </span>
            </li>
          </ul>
        </section>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import HubStatusBanner from '@/components/HubStatusBanner.vue'
import { useHubLabel } from '@/composables/useHubLabel'
import ErrorText from '@/components/ErrorText.vue'
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import PosterImage from '@/components/enrich/PosterImage.vue'
import { useEnrichStore } from '@/stores/enrich'
import { useNavigationStore } from '@/stores/navigation'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { SeriesMarker, UpcomingEpisode } from '@/api/types'

const { t, locale } = useI18n()
const { hubTitle } = useHubLabel()
const navigation = useNavigationStore()
const enrich = useEnrichStore()
const { timeZone } = useDisplayTimeZone()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/series'))

const markers = ref<SeriesMarker[]>([])
const upcoming = ref<UpcomingEpisode[]>([])
const loading = ref(true)
const loadFailed = ref(false)
const showEmbed = ref(false)
const markerIdByFsid = computed(() => Object.fromEntries(markers.value.map((m) => [m.fsid, m.id])))

// SerienRecorder's own web GUI (confirmed reachable, no X-Frame-Options/CSP
// that would block embedding — ARCHITECTURE.md, #10) lives on the same
// device, same host, just WebControl's port (80) rather than DreamHub's
// (8765) — this only resolves correctly when DreamHub is actually deployed
// on the box itself (PROJECT_PLAN.md's packaging plan), not in local dev
// against a different device. DreamHub deliberately doesn't rebuild
// SerienRecorder's own UI (PRINCIPLES.md) — this just gets the user to it.
const serienRecorderUrl = computed(() => `${window.location.protocol}//${window.location.hostname}/serienrecorderui/`)

function formatTime(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
    timeZone: timeZone.value,
  }).format(unixSeconds * 1000)
}

onMounted(async () => {
  try {
    const [markersRes, upcomingRes] = await Promise.all([api.series.markers(), api.series.upcoming()])
    markers.value = markersRes.markers
    upcoming.value = upcomingRes.episodes
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
})
</script>
