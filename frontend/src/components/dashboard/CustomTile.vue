<!--
  A custom tile's body: a saved view with criteria (CONCEPT.md, "Custom
  tiles"), evaluated by POST /api/v1/dashboard/view. A one-line summary of
  the criteria and the match count first (DESIGN mockup: "Recordings · older
  than 90 days · larger than 3 GB → 12 matches, 48 GB"), then either the
  rows or, for display "count", one big number. Rows link to where the item
  lives; the tile itself never acts on anything. Refreshes every five
  minutes while shown.
-->
<template>
  <div class="flex h-full flex-col gap-2">
    <p class="text-[13px] text-muted">
      {{ summary }}
      <template v-if="result">
        →
        <span class="font-semibold text-text">{{ matchText }}</span>
      </template>
    </p>

    <p v-if="loading && !result" class="text-[13.5px] text-muted">{{ t('common.loading') }}</p>
    <p v-else-if="error" class="text-[13.5px] text-muted">{{ error }}</p>
    <template v-else-if="result">
      <p v-if="custom.display === 'count'" class="text-[34px] font-bold tabular-nums leading-tight">
        {{ formatNumber(result.total) }}
      </p>
      <p v-else-if="result.items.length === 0" class="text-[13.5px] text-muted">{{ t('dashboard.custom.noMatches') }}</p>
      <ul v-else class="flex flex-col">
        <li v-for="(item, i) in result.items" :key="i" class="border-b border-line-soft last:border-none">
          <component
            :is="item.path ? RouterLink : 'div'"
            :to="item.path"
            class="-mx-2 flex items-center gap-3 rounded px-2 py-1.5"
            :class="{ 'hover:bg-tile-2': item.path }"
          >
            <span class="w-[5.5rem] shrink-0 whitespace-nowrap text-[12.5px] tabular-nums" :class="item.warn ? 'text-warn' : 'text-faint'">
              {{ lead(item) }}
            </span>
            <span class="min-w-0 flex-1">
              <span class="block truncate text-[14px] text-text">{{ item.title }}</span>
              <span v-if="item.subtitle" class="block truncate text-[12.5px] text-muted">{{ item.subtitle }}</span>
            </span>
            <span class="shrink-0 whitespace-nowrap text-[12.5px] tabular-nums text-faint">{{ trail(item) }}</span>
          </component>
        </li>
      </ul>
    </template>

    <div v-if="editable" class="mt-auto">
      <button type="button" class="rounded py-1 text-[13px] text-accent hover:underline" @click="$emit('edit')">
        {{ t('dashboard.custom.editFilters') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { formatBytes } from '@/lib/format'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { useViewText } from '@/composables/useViewText'
import type { CustomTileView, ViewItem, ViewResult } from '@/api/types'

const props = defineProps<{ custom: CustomTileView; editable?: boolean }>()
defineEmits<{ edit: [] }>()

const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()
const { describe, zoneName } = useViewText()

const result = ref<ViewResult | null>(null)
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  try {
    result.value = await api.dashboard.view(props.custom.source, props.custom.query, zoneName())
    error.value = ''
  } catch {
    result.value = null
    error.value = t('dashboard.custom.failed')
  } finally {
    loading.value = false
  }
}

let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  void load()
  timer = setInterval(() => document.visibilityState === 'visible' && void load(), 5 * 60 * 1000)
})
onUnmounted(() => timer && clearInterval(timer))
watch(() => JSON.stringify(props.custom), () => void load())

const summary = computed(() => describe(props.custom.source, props.custom.query))
const isRecordings = computed(() => props.custom.source === 'media.recordings')

const matchText = computed(() => {
  const r = result.value
  if (!r) return ''
  const n = t('dashboard.custom.matches', { n: formatNumber(r.total) }, r.total)
  return isRecordings.value && r.totalSize ? `${n}, ${formatBytes(r.totalSize, locale.value)}` : n
})

function formatNumber(n: number) {
  return new Intl.NumberFormat(locale.value).format(n)
}

function fmt(unix: number, opts: Intl.DateTimeFormatOptions) {
  return new Intl.DateTimeFormat(locale.value, { ...opts, timeZone: timeZone.value }).format(unix * 1000)
}

// Recordings lead with their date and end with their size; everything
// still to come leads with when it starts and ends with how long it runs.
function lead(item: ViewItem) {
  if (!item.time) return ''
  if (isRecordings.value) return fmt(item.time, { dateStyle: 'medium' })
  return fmt(item.time, { weekday: 'short', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
}
function trail(item: ViewItem) {
  if (isRecordings.value) return item.size ? formatBytes(item.size, locale.value) : ''
  if (item.time && item.end) return '–' + fmt(item.end, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
  return ''
}
</script>
