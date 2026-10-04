<!--
  Built-in dashboard tile: the next few timers from TimerHub
  (GET /api/v1/timers). Times are formatted from the API's Unix timestamps
  via Intl in the browser's own zone (PRINCIPLES.md: UTC inside, local only
  on screen, never derived from file names) — a per-user display zone
  setting is a separate open item (ROADMAP.md).
-->
<template>
  <p v-if="loading" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
  <p v-else-if="unavailable" class="text-[13px] text-faint">{{ t('dashboard.timersTile.unavailable') }}</p>
  <p v-else-if="upcoming.length === 0" class="text-[13px] text-faint">{{ t('dashboard.timersTile.empty') }}</p>
  <ul v-else class="flex flex-col">
    <li
      v-for="timer in upcoming"
      :key="timer.serviceReference + timer.begin"
      class="flex items-center gap-3 border-b border-line-soft py-2 last:border-none"
    >
      <span class="w-24 shrink-0 whitespace-nowrap text-[12.5px] font-mono tabular-nums text-faint">{{ formatTime(timer.begin) }}</span>
      <span class="min-w-0 flex-1">
        <span class="block truncate text-[14px] text-text">{{ timer.name }}</span>
        <span class="block truncate text-[12.5px] text-muted">{{ timer.serviceName }}</span>
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
import type { Timer } from '@/api/types'

const MAX_ROWS = 5

const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()
const timers = ref<Timer[]>([])
const loading = ref(true)
const unavailable = ref(false)

const upcoming = computed(() => {
  const nowSec = Date.now() / 1000
  return timers.value
    .filter((t) => !t.disabled && t.end > nowSec)
    .sort((a, b) => a.begin - b.begin)
    .slice(0, MAX_ROWS)
})

function formatTime(unixSeconds: number): string {
  return formatWhen(unixSeconds, locale.value, timeZone.value)
}

onMounted(async () => {
  try {
    const res = await api.timers()
    timers.value = res.timers
  } catch {
    unavailable.value = true
  } finally {
    loading.value = false
  }
})
</script>
