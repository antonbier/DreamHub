<!--
  One film or series in PlexHub's tile view: the poster (2:3, lazy-loaded),
  title and one `muted` line under it. Seen: an `ok` check in a small
  round mark at the poster's top end corner (no pill); started: a 3 px
  `accent` bar along the poster's bottom showing how far. The whole card
  is one button that opens the film's details or the series page.
-->
<template>
  <button
    type="button"
    class="group flex w-full flex-col text-start"
    :aria-label="item.watched ? t('plex.openWatched', { title: item.title }) : t('plex.open', { title: item.title })"
    @click="$emit('open')"
  >
    <span class="relative block aspect-[2/3] w-full overflow-hidden rounded border border-line-soft bg-field transition-colors group-hover:border-accent/60">
      <PosterImage v-if="item.thumb" :plex="item.thumb" :plex-width="240" fill lazy alt="" />
      <span
        v-if="item.watched"
        class="absolute end-1.5 top-1.5 flex h-5 w-5 items-center justify-center rounded-full border border-line bg-bg text-ok"
        :title="t('plex.watched')"
      >
        <Icon name="check" :size="14" />
      </span>
      <span v-if="progress !== null" class="absolute inset-x-0 bottom-0 h-[3px] bg-field">
        <span class="block h-full bg-accent" :style="{ width: `${progress * 100}%` }" />
      </span>
    </span>
    <span class="mt-1.5 block truncate text-[13.5px] text-text group-hover:text-accent" :title="item.title">{{ item.title }}</span>
    <span class="block truncate text-[12px] text-muted">{{ sub }}</span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/Icon.vue'
import PosterImage from '@/components/enrich/PosterImage.vue'
import { usePlexText } from '@/composables/usePlexText'
import type { PlexItem } from '@/api/types'

const props = defineProps<{ item: PlexItem }>()
defineEmits<{ open: [] }>()
const { t } = useI18n()
const text = usePlexText()

const progress = computed(() => text.progress(props.item))
const sub = computed(() => {
  const it = props.item
  if (it.type === 'show') return it.leafCount ? t('plex.episodesSeen', { seen: it.viewedLeafCount ?? 0, total: it.leafCount }) : it.year ? String(it.year) : ''
  return [it.year, text.minutes(it.duration)].filter(Boolean).join(' · ')
})
</script>
