<!--
  One broadcast as a card (DiscoveryHub's picks, GuideHub's "Tonight"):
  poster, title, channel and time — "now" in `onair` while it airs —,
  what enrichment knows (film or series, year, rating), and a short text
  (default slot: the AI's reason, the EPG description), clamped to three
  lines. The whole card is one button that opens the details; a timer
  that covers it shows as an `ok` dot + "Timer set".
-->
<template>
  <button
    type="button"
    class="flex w-full gap-3 rounded border border-line bg-tile p-3 text-start transition-colors hover:border-accent/60 hover:bg-tile-2"
    :aria-label="t('discovery.openDetails', { title })"
    @click="$emit('open')"
  >
    <PosterImage :path="info?.poster" size="w185" :width="72" :height="108" alt="" />
    <span class="flex min-w-0 flex-1 flex-col">
      <span class="truncate text-[14.5px] font-semibold text-text">{{ title }}</span>
      <span class="mt-0.5 flex flex-wrap items-center gap-x-1.5 text-[12.5px] text-muted">
        <span v-if="airingNow" class="flex items-center gap-1 text-onair"><span class="h-1.5 w-1.5 rounded-full bg-onair" />{{ t('discovery.now') }}</span>
        <ChannelLogo v-if="serviceReference" :service-reference="serviceReference" :service-name="serviceName" :height="14" />
        <span class="tabular-nums">{{ time }}</span>
        <span>· {{ serviceName }}</span>
      </span>
      <span v-if="info" class="mt-1 flex flex-wrap items-center gap-x-2 text-[12.5px] text-muted">
        <span>{{ t(`discovery.kind.${info.kind}`) }}</span>
        <span v-if="info.year" class="tabular-nums">{{ info.year }}</span>
        <RatingBadge :rating="info.rating" :votes="info.votes" />
      </span>
      <span v-if="$slots.default" class="mt-1.5 line-clamp-3 text-[13px] text-text"><slot /></span>
      <span v-if="hasTimer || inPlex" class="mt-auto flex flex-wrap items-center gap-x-3 pt-1.5 text-[12.5px]">
        <span v-if="hasTimer" class="flex items-center gap-1.5 text-ok"><span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('discovery.created') }}</span>
        <span v-if="inPlex" class="flex items-center gap-1.5 text-muted"><span class="h-1.5 w-1.5 rounded-full bg-muted" />{{ t('discovery.inPlex') }}</span>
      </span>
    </span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PosterImage from '@/components/enrich/PosterImage.vue'
import RatingBadge from '@/components/enrich/RatingBadge.vue'
import ChannelLogo from '@/components/guide/ChannelLogo.vue'
import type { EnrichInfo } from '@/api/types'

const props = defineProps<{
  title: string
  serviceName: string
  serviceReference?: string
  start: number
  duration: number
  time: string
  now: number
  info?: EnrichInfo | null
  hasTimer?: boolean
  inPlex?: boolean
}>()
defineEmits<{ open: [] }>()
const { t } = useI18n()

const airingNow = computed(() => props.start <= props.now && props.now < props.start + props.duration)
</script>
