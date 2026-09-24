<!--
  Built-in dashboard tile: upcoming timers that likely clash on this box's
  tuners (GET /api/v1/timers' per-timer `conflict` estimate — transponder-
  and delivery-system-aware, see backend/internal/tuner). Shows only; the
  fix happens in TimerHub, through preview-and-confirm (CONCEPT.md: tiles
  never act on their own).
-->
<template>
  <p v-if="loading" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
  <p v-else-if="unavailable" class="text-[13px] text-faint">{{ t('dashboard.timersTile.unavailable') }}</p>
  <p v-else-if="conflicts.length === 0" class="flex items-center gap-2 text-[13px] text-muted">
    <span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('dashboard.conflictsTile.none') }}
  </p>
  <template v-else>
    <ul class="flex flex-col">
      <li
        v-for="timer in conflicts"
        :key="timer.serviceReference + timer.begin"
        class="flex items-center gap-3 border-b border-line-soft py-2 last:border-none"
      >
        <span class="w-24 shrink-0 whitespace-nowrap font-mono text-[12.5px] tabular-nums text-warn">{{ formatTime(timer.begin) }}</span>
        <span class="min-w-0 flex-1">
          <span class="block truncate text-[14px] text-text">{{ timer.name }}</span>
          <span class="block truncate text-[12.5px] text-muted">{{ timer.serviceName }}</span>
        </span>
      </li>
    </ul>
    <RouterLink to="/timers" class="mt-2 inline-block rounded px-2 py-1 text-[13px] text-accent hover:bg-tile-2">
      {{ t('dashboard.conflictsTile.open') }}
    </RouterLink>
  </template>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { api } from '@/api/client'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { Timer } from '@/api/types'

const MAX_ROWS = 5

const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()
const timers = ref<Timer[]>([])
const loading = ref(true)
const unavailable = ref(false)

const conflicts = computed(() => {
  const nowSec = Date.now() / 1000
  return timers.value
    .filter((x) => x.conflict && x.end > nowSec)
    .sort((a, b) => a.begin - b.begin)
    .slice(0, MAX_ROWS)
})

function formatTime(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, { weekday: 'short', hour: '2-digit', minute: '2-digit', timeZone: timeZone.value }).format(
    unixSeconds * 1000,
  )
}

onMounted(async () => {
  try {
    timers.value = (await api.timers()).timers
  } catch {
    unavailable.value = true
  } finally {
    loading.value = false
  }
})
</script>
