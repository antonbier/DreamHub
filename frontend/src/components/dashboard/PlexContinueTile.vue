<!--
  Optional dashboard tile "Continue watching" (Plex): Plex's own list of
  started films and next episodes (GET /api/v1/plex/ondeck) as one row of
  small covers with a progress bar — the cover row CONCEPT.md allows as a
  tile, hidden until switched on in edit mode. Each cover opens it in
  PlexHub. Offline it shows the last known list and says so.
-->
<template>
  <p v-if="loading" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
  <p v-else-if="state === 'notConfigured'" class="text-[13px] text-faint">
    {{ t('dashboard.plexTile.notConfigured') }}
    <RouterLink to="/settings/services#plex" class="text-accent hover:underline">{{ t('dashboard.plexTile.setUp') }}</RouterLink>
  </p>
  <p v-else-if="state === 'failed'" class="text-[13px] text-faint">{{ t('dashboard.plexTile.unavailable') }}</p>
  <p v-else-if="items.length === 0" class="text-[13px] text-faint">{{ t('dashboard.plexTile.empty') }}</p>
  <template v-else>
    <ul class="flex gap-3 overflow-x-auto pb-1">
      <li v-for="it in items" :key="it.ratingKey" class="w-[96px] shrink-0">
        <RouterLink :to="`/plex?item=${it.ratingKey}`" class="group block" :title="text.title(it)">
          <span class="relative block aspect-[2/3] w-full overflow-hidden rounded border border-line-soft bg-field group-hover:border-accent/60">
            <PosterImage :plex="it.thumb" :plex-width="240" fill lazy alt="" />
            <span v-if="text.progress(it) !== null" class="absolute inset-x-0 bottom-0 h-[3px] bg-field">
              <span class="block h-full bg-accent" :style="{ width: `${(text.progress(it) ?? 0) * 100}%` }" />
            </span>
          </span>
          <span class="mt-1 block truncate text-[12.5px] text-text group-hover:text-accent">{{ it.type === 'episode' ? it.showTitle : it.title }}</span>
          <span class="block truncate text-[11.5px] text-muted">{{ it.type === 'episode' ? text.episodeCode(it) : it.year }}</span>
        </RouterLink>
      </li>
    </ul>
    <p v-if="!online" class="mt-1 text-[12px] text-warn">{{ t('dashboard.plexTile.offline') }}</p>
  </template>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import PosterImage from '@/components/enrich/PosterImage.vue'
import { usePlexText } from '@/composables/usePlexText'
import type { PlexItem } from '@/api/types'

const { t } = useI18n()
const text = usePlexText()
const items = ref<PlexItem[]>([])
const online = ref(true)
const loading = ref(true)
const state = ref<'ok' | 'notConfigured' | 'failed'>('ok')

onMounted(async () => {
  try {
    const res = await api.plex.onDeck()
    items.value = res.items.slice(0, 12)
    online.value = res.meta.online
  } catch (err) {
    state.value = err instanceof ApiError && err.code === 'plex_not_configured' ? 'notConfigured' : 'failed'
  } finally {
    loading.value = false
  }
})
</script>
