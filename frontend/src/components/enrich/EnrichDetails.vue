<!--
  What enrichment knows about a title, for a detail area (series page,
  selected programme, recommendation card): poster, the database's title
  when it differs, year, genres, rating with its vote count, and the
  overview. Renders nothing at all when nothing is known, so a screen
  without enrichment looks exactly as before. Names its source, as TMDB's
  terms ask.
-->
<template>
  <div v-if="info" class="flex gap-4">
    <PosterImage :path="info.poster" size="w185" :width="posterWidth" :height="Math.round(posterWidth * 1.5)" :alt="info.title" />
    <div class="min-w-0 flex-1">
      <p class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[13px] text-muted">
        <span v-if="showTitle" class="font-semibold text-text">{{ info.title }}</span>
        <span v-if="info.year" class="tabular-nums">{{ info.year }}</span>
        <span v-if="info.genres?.length">{{ info.genres.join(', ') }}</span>
        <RatingBadge :rating="info.rating" :votes="info.votes" />
        <span v-if="info.fsk" class="rounded-sm border border-line px-1 text-[11.5px] text-muted">{{ t('enrich.fsk', { age: info.fsk }) }}</span>
      </p>
      <p v-if="info.overview" class="mt-1.5 text-[13.5px] text-text" :class="{ 'line-clamp-4': clamp }">{{ info.overview }}</p>
      <p class="mt-1.5 text-[11.5px] text-faint">{{ t('enrich.source', { source: sourceName(info) }) }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PosterImage from './PosterImage.vue'
import RatingBadge from './RatingBadge.vue'
import type { EnrichInfo } from '@/api/types'
import { titlesDiffer } from '@/lib/titles'
import { sourceName } from '@/lib/enrichSource'

const props = withDefaults(
  defineProps<{ info?: EnrichInfo | null; shownTitle?: string; posterWidth?: number; clamp?: boolean }>(),
  { posterWidth: 92, clamp: true },
)
const { t } = useI18n()

// The database's title only when it adds something ("Bones - Die
// Knochenjägerin" for "Bones"), not a repeat of the heading.
const showTitle = computed(() => !!props.info && titlesDiffer(props.info.title, props.shownTitle))
</script>
