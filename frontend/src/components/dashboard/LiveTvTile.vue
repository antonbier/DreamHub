<!--
  Optional dashboard tile "Live TV": the first channels of the bouquet
  StreamHub last showed, each starting it in the browser (/stream?ref=).
  Backed by StreamHub's GET /api/v1/stream/bouquets and /channels; hidden
  until switched on in edit mode.
-->
<template>
  <p v-if="loading" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
  <p v-else-if="failed" class="text-[13px] text-faint"><ErrorText /></p>
  <ul v-else class="grid grid-cols-2 gap-1.5 below-sm:grid-cols-1">
    <li v-for="ch in channels" :key="ch.serviceReference">
      <RouterLink
        :to="{ path: '/stream', query: { ref: ch.serviceReference } }"
        class="flex items-center gap-2 rounded-sm px-1.5 py-1.5 hover:bg-tile-2"
      >
        <span class="flex w-9 shrink-0 justify-center"><ChannelLogo source="stream" :service-reference="ch.serviceReference" :service-name="ch.serviceName" :height="18" /></span>
        <span class="min-w-0 flex-1">
          <span class="block truncate text-[13px] font-semibold text-text">{{ ch.serviceName }}</span>
          <span v-if="ch.now" class="block truncate text-[12px] text-muted">{{ ch.now.title }}</span>
          <span v-if="ch.now && progress(ch.now) !== null" class="mt-1 block h-[3px] w-full rounded-sm bg-field">
            <span class="block h-full rounded-sm bg-accent" :style="{ width: `${(progress(ch.now) ?? 0) * 100}%` }" />
          </span>
        </span>
      </RouterLink>
    </li>
  </ul>
</template>

<script setup lang="ts">
import ErrorText from '@/components/ErrorText.vue'
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import ChannelLogo from '@/components/guide/ChannelLogo.vue'
import type { NowNext } from '@/api/types'

const MAX = 8
const { t } = useI18n()
const loading = ref(true)
const failed = ref(false)
const channels = ref<NowNext[]>([])

// How far the programme on now has run, 0–1; null without a known length.
function progress(e: { start: number; duration: number }): number | null {
  if (!e.duration) return null
  return Math.min(1, Math.max(0, (Date.now() / 1000 - e.start) / e.duration))
}

onMounted(async () => {
  try {
    const { bouquets } = await api.stream.bouquets()
    let remembered: string | null = null
    try {
      remembered = localStorage.getItem('dreamhub.streamBouquet')
    } catch {
      remembered = null
    }
    const b = bouquets.find((x) => x.serviceReference === remembered) ?? bouquets[0]
    if (b) channels.value = (await api.stream.channels(b.serviceReference)).channels.slice(0, MAX)
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
})
</script>
