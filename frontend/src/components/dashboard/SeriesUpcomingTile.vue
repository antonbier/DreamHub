<!--
  Built-in dashboard tile: the next few episodes SerienRecorder will record
  (GET /api/v1/series/upcoming) — same shape as TimersUpcomingTile.vue.
-->
<template>
  <p v-if="loading" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
  <p v-else-if="unavailable" class="text-[13px] text-faint">{{ t('dashboard.seriesTile.unavailable') }}</p>
  <p v-else-if="upcoming.length === 0" class="text-[13px] text-faint">{{ t('dashboard.seriesTile.empty') }}</p>
  <ul v-else class="flex flex-col">
    <li
      v-for="(ep, i) in upcoming"
      :key="i"
      class="flex items-center gap-3 border-b border-line-soft py-2 last:border-none"
    >
      <span class="w-24 shrink-0 whitespace-nowrap text-[12.5px] font-mono tabular-nums text-faint">{{ formatTime(ep.startTime) }}</span>
      <span class="min-w-0 flex-1">
        <span class="block truncate text-[14px] text-text">{{ ep.series }}</span>
        <span class="block truncate text-[12.5px] text-muted">{{ ep.channel }}</span>
      </span>
    </li>
  </ul>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { formatWhen } from '@/lib/format'
import type { UpcomingEpisode } from '@/api/types'

const MAX_ROWS = 5

const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()
const episodes = ref<UpcomingEpisode[]>([])
const loading = ref(true)
const unavailable = ref(false)

const upcoming = computed(() => episodes.value.slice(0, MAX_ROWS))

function formatTime(unixSeconds: number): string {
  return formatWhen(unixSeconds, locale.value, timeZone.value)
}

onMounted(async () => {
  try {
    const res = await api.series.upcoming()
    episodes.value = res.episodes
  } catch {
    unavailable.value = true
  } finally {
    loading.value = false
  }
})
</script>
