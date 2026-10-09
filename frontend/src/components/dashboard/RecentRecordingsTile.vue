<!--
  Built-in tile "Recent recordings" (docs/archive/PROFILES_PLAN.md, P4): the newest
  recordings, each one tap from watching in the browser (StreamHub). The
  same data as a custom tile over MediaHub's "recordings" source, newest
  first — with a look made for watching rather than tidying up.
-->
<template>
  <p v-if="loading" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
  <p v-else-if="failed" class="text-[13px] text-faint"><ErrorText /></p>
  <p v-else-if="items.length === 0" class="text-[13px] text-faint">{{ t('dashboard.recent.empty') }}</p>
  <ul v-else class="flex flex-col">
    <li v-for="item in items" :key="item.recording ?? item.title" class="border-b border-line-soft last:border-none">
      <RouterLink
        :to="item.recording ? { path: '/stream', query: { recording: item.recording, title: item.title } } : (item.path ?? '/media')"
        class="-mx-2 flex items-center gap-3 rounded px-2 py-1.5 hover:bg-tile-2"
      >
        <Icon name="play" :size="16" class="shrink-0 text-accent" />
        <span class="min-w-0 flex-1">
          <span class="block truncate text-[14px] text-text">{{ item.title }}</span>
          <span class="block truncate text-[12.5px] text-muted">{{ when(item.time) }}</span>
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
import Icon from '@/components/Icon.vue'
import { api } from '@/api/client'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { ViewItem } from '@/api/types'

const MAX = 6
const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()
const loading = ref(true)
const failed = ref(false)
const items = ref<ViewItem[]>([])

function when(unix?: number): string {
  if (!unix) return ''
  return new Intl.DateTimeFormat(locale.value, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', timeZone: timeZone.value }).format(unix * 1000)
}

onMounted(async () => {
  try {
    const res = await api.dashboard.view('media.recordings', { sort: 'newest', limit: MAX }, timeZone.value ?? 'UTC')
    items.value = res.items
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
})
</script>
