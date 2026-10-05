<!--
  One film, series or episode in PlexHub's list view — DESIGN.md's Row: a
  36×52 poster (none for episodes inside a season, where `numbered` puts
  the episode number in a tabular column instead), title and details,
  the rating, the seen state as a dot + word, and text actions that
  appear on hover (always below 900 px): mark seen / not seen, play on
  the box. The title opens the details or the series page.
-->
<template>
  <li class="border-b border-line-soft last:border-none">
    <div class="group flex items-center gap-3 py-2 hover:bg-tile-2 below-sm:flex-wrap below-sm:gap-y-1">
      <span v-if="numbered" class="w-8 shrink-0 text-end font-mono text-[12.5px] tabular-nums text-faint">{{ item.index }}</span>
      <PosterImage v-else :plex="item.thumb" :plex-width="120" lazy :width="36" :height="52" alt="" />
      <span class="min-w-0 flex-1">
        <button
          type="button"
          class="block max-w-full truncate text-start text-[14px] hover:text-accent"
          :class="item.watched ? 'text-muted' : 'text-text'"
          :title="item.title"
          @click="$emit('open')"
        >
          {{ item.title }}
        </button>
        <span class="flex items-center gap-2 truncate text-[12.5px] text-muted">
          <span class="truncate">{{ numbered ? text.minutes(item.duration) : text.details(item) }}</span>
          <span v-if="progress !== null" class="inline-block h-[3px] w-16 shrink-0 rounded-full bg-field" :title="t('plex.started')">
            <span class="block h-full rounded-full bg-accent" :style="{ width: `${progress * 100}%` }" />
          </span>
        </span>
      </span>
      <span v-if="item.rating && !numbered" class="shrink-0 whitespace-nowrap text-[12.5px] tabular-nums below-sm:hidden">
        <span class="text-warn" aria-hidden="true">★</span> <span class="text-text">{{ text.rating(item.rating) }}</span>
      </span>
      <span v-if="item.watched" class="flex w-24 shrink-0 items-center gap-1.5 text-[12.5px] text-ok below-sm:w-auto">
        <span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('plex.watched') }}
      </span>
      <span v-else class="w-24 shrink-0 below-sm:hidden" />
      <span class="flex shrink-0 below-sm:basis-full below-sm:justify-end">
        <button
          v-if="item.type !== 'show' && item.type !== 'season'"
          type="button"
          class="rounded px-2 py-1 text-[13px] text-accent opacity-0 hover:bg-tile focus:opacity-100 disabled:text-faint group-hover:opacity-100 below-md:opacity-100"
          :disabled="!online || busy"
          :title="online ? undefined : t('plex.offlineAction')"
          @click="$emit('toggleWatched')"
        >
          {{ item.watched ? t('plex.markUnwatched') : t('plex.markWatched') }}
        </button>
        <button
          v-if="canPlay && (item.type === 'movie' || item.type === 'episode')"
          type="button"
          class="rounded px-2 py-1 text-[13px] text-accent opacity-0 hover:bg-tile focus:opacity-100 disabled:text-faint group-hover:opacity-100 below-md:opacity-100"
          :disabled="!online"
          :title="online ? undefined : t('plex.offlineAction')"
          @click="$emit('play')"
        >
          {{ t('plex.playOnBox') }}
        </button>
      </span>
    </div>
    <slot />
  </li>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PosterImage from '@/components/enrich/PosterImage.vue'
import { usePlexText } from '@/composables/usePlexText'
import type { PlexItem } from '@/api/types'

const props = defineProps<{
  item: PlexItem
  online: boolean
  canPlay: boolean
  // Episodes of one season: the number instead of a poster.
  numbered?: boolean
  busy?: boolean
}>()
defineEmits<{ open: []; toggleWatched: []; play: [] }>()
const { t } = useI18n()
const text = usePlexText()
const progress = computed(() => text.progress(props.item))
</script>
