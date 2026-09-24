<!--
  SeriesHub's real screen (ROADMAP.md, "v1 — hubs"): series tracked in
  SerienRecorder and their upcoming episodes (GET /api/v1/series/markers,
  GET /api/v1/series/upcoming). Episodes-on-disk/missing-episodes
  cross-referencing against recordings is still open. Same
  status-banner-plus-content pattern as MediaHubView.vue.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="series" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ t(item ? item.label : 'hub.series') }}</h1>
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
      <section class="mt-5 flex flex-wrap items-center justify-between gap-3 rounded border border-line bg-tile p-4">
        <div>
          <h2 class="text-[14px] font-semibold text-text">{{ t('series.webInterface.title') }}</h2>
          <p class="mt-0.5 text-[12.5px] text-muted">{{ t('series.webInterface.body') }}</p>
        </div>
        <div class="flex shrink-0 gap-2">
          <button type="button" class="rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field" @click="showEmbed = !showEmbed">
            {{ showEmbed ? t('series.webInterface.hide') : t('series.webInterface.show') }}
          </button>
          <a
            :href="serienRecorderUrl"
            target="_blank"
            rel="noopener"
            class="rounded bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg"
          >
            {{ t('series.webInterface.openTab') }}
          </a>
        </div>
      </section>
      <iframe
        v-if="showEmbed"
        :src="serienRecorderUrl"
        :title="t('series.webInterface.title')"
        class="mt-3 h-[600px] w-full rounded border border-line"
      />

      <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted">{{ t('common.backendUnreachable') }}</p>
      <template v-else>
        <section>
          <h2 class="mt-5 text-[15px] font-semibold text-text">{{ t('series.tracked') }}</h2>
          <p v-if="markers.length === 0" class="mt-2 text-[14px] text-muted">{{ t('series.trackedEmpty') }}</p>
          <ul v-else class="mt-2 flex flex-col">
            <li
              v-for="m in markers"
              :key="m.id"
              class="flex items-start gap-3 border-b border-line-soft py-2.5 last:border-none"
              :class="{ 'opacity-60': !m.active }"
            >
              <span class="min-w-0 flex-1">
                <span class="block truncate text-[14px] text-text">{{ m.name }}</span>
                <span class="block truncate text-[12.5px] text-muted">{{ m.info }}</span>
              </span>
              <span v-if="!m.active" class="shrink-0 text-[12.5px] text-faint">{{ t('series.inactive') }}</span>
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
                <span class="block truncate text-[14px] text-text">{{ ep.series }}</span>
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
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import type { SeriesMarker, UpcomingEpisode } from '@/api/types'

const { t, locale } = useI18n()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/series'))

const markers = ref<SeriesMarker[]>([])
const upcoming = ref<UpcomingEpisode[]>([])
const loading = ref(true)
const loadFailed = ref(false)
const showEmbed = ref(false)

// SerienRecorder's own web GUI (confirmed reachable, no X-Frame-Options/CSP
// that would block embedding — ARCHITECTURE.md, #10) lives on the same
// device, same host, just WebControl's port (80) rather than DreamHub's
// (8080) — this only resolves correctly when DreamHub is actually deployed
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
