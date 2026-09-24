<!--
  MediaHub's real screen (ROADMAP.md, "v1 — hubs"): the recordings list
  (GET /api/v1/media/movies) and disk usage (GET /api/v1/media/disk).
  Read-only for now — rename/delete need their own preview-and-confirm
  design (PRINCIPLES.md) and aren't built yet. Shows the hub's live
  status/problems first, same as HubView, per frontend/README.md's "a hub
  view without a backend implementation yet is a real placeholder" rule —
  this one just also has real content once the hub is available.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="recordings" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ t(item ? item.label : 'hub.media') }}</h1>
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

    <template v-else>
      <section v-if="disks.length > 0" class="mt-5 flex flex-col gap-3 rounded border border-line bg-tile p-4">
        <div v-for="(disk, i) in disks" :key="i">
          <div class="mb-1 flex items-baseline justify-between gap-2 text-[13px]">
            <span class="text-text">{{ disk.model }}</span>
            <span class="text-muted">{{ t('media.diskFree', { free: disk.freeText, capacity: disk.capacityText }) }}</span>
          </div>
          <div class="h-1.5 overflow-hidden rounded-full bg-line-soft">
            <div
              class="h-full rounded-full"
              :class="disk.usedPercent != null && disk.usedPercent >= 90 ? 'bg-warn' : 'bg-accent'"
              :style="{ width: (disk.usedPercent ?? 0) + '%' }"
            />
          </div>
        </div>
      </section>

      <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted">{{ t('common.backendUnreachable') }}</p>
      <p v-else-if="movies.length === 0" class="mt-5 text-[14px] text-muted">{{ t('media.empty') }}</p>
      <ul v-else class="mt-5 flex flex-col">
        <li
          v-for="movie in movies"
          :key="movie.serviceReference"
          class="flex items-start gap-3 border-b border-line-soft py-2.5 last:border-none"
          :class="{ 'opacity-60': !movie.isRecording }"
        >
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[14px] text-text">{{ movie.title }}</span>
            <span class="block truncate text-[12.5px] text-muted">
              <template v-if="movie.isRecording">{{ movie.serviceName }} · {{ formatTime(movie.time) }} · {{ movie.length }}</template>
              <template v-else>{{ t('media.notARecording') }}</template>
            </span>
          </span>
          <span class="shrink-0 whitespace-nowrap text-[12.5px] font-mono text-faint">{{ formatSize(movie.fileSize) }}</span>
        </li>
      </ul>
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
import type { Disk, Movie } from '@/api/types'

const { t, locale } = useI18n()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/media'))

const movies = ref<Movie[]>([])
const disks = ref<Disk[]>([])
const loading = ref(true)
const loadFailed = ref(false)

function formatTime(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(unixSeconds * 1000)
}

// Binary units (GiB, not GB) since fileSize is an exact byte count from the
// API, unlike Disk's already-formatted capacity/free strings (webcontrol.ParseSize).
function formatSize(bytes: number): string {
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

onMounted(async () => {
  try {
    const [moviesRes, diskRes] = await Promise.all([api.media.movies(), api.media.disk()])
    movies.value = moviesRes.movies
    disks.value = diskRes.disks
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
})
</script>
