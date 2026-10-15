<!--
  One broadcast's details as a dialog — shared by DiscoveryHub's picks and
  every GuideHub view: the backdrop across the top when enrichment has
  one, the title, the broadcast line (on-air marker, day and time,
  channel, length), everything the data sources know (EnrichDetails:
  poster, logo, year, genres, ratings, FSK, the full overview, credited),
  then the caller's own sections (default slot: the EPG text, why the AI
  picked it, a recording's description) and actions (slot "actions": record, switch now). Escape, the
  close button and the backdrop close it; Close gets focus on open, Tab
  stays inside (useFocusTrap), and focus goes back to whatever opened it. Full screen below sm.
-->
<template>
  <div class="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/60 px-4 py-[6vh] below-sm:px-0 below-sm:py-0" @click.self="$emit('close')">
    <div
      ref="dialogEl"
      role="dialog"
      aria-modal="true"
      aria-labelledby="programme-title"
      class="relative w-full max-w-[680px] overflow-hidden rounded-lg border border-line bg-tile shadow-lg below-sm:min-h-full below-sm:rounded-none below-sm:border-0"
    >
      <div v-if="info?.backdrop" class="relative h-[220px] below-sm:h-[180px]">
        <PosterImage :path="info.backdrop" size="w780" fill alt="" />
        <div class="absolute inset-0 bg-linear-to-t/srgb from-tile to-transparent" />
      </div>
      <button
        ref="closeButton"
        type="button"
        class="absolute inset-e-3 top-3 rounded-sm px-2.5 py-1 text-[13px] text-text backdrop-blur-sm hover:bg-tile-2"
        @click="$emit('close')"
      >
        {{ t('plan.close') }}
      </button>

      <div class="p-5" :class="{ 'relative -mt-12': info?.backdrop }">
        <h2 id="programme-title" class="pe-20 text-[21px] font-bold leading-tight text-text">{{ title }}</h2>
        <p v-if="subtitle" class="mt-1 text-[13.5px] text-muted">{{ subtitle }}</p>
        <p v-else class="mt-1 flex flex-wrap items-center gap-x-1.5 text-[13.5px] text-muted">
          <span v-if="airingNow" class="flex items-center gap-1 text-onair"><span class="h-1.5 w-1.5 rounded-full bg-onair" />{{ t('discovery.now') }}</span>
          <span class="tabular-nums">{{ when }}</span>
          <span>· {{ serviceName }}</span>
          <span class="tabular-nums">· {{ t('discovery.minutes', { n: Math.round(duration / 60) }) }}</span>
        </p>

        <template v-if="plain" />
        <EnrichDetails v-else-if="info" class="mt-4" :info="info" :shown-title="title" :poster-width="120" :clamp="false" :recording="recording" />
        <PosterImage v-else-if="recording" class="mt-4" :recording="recording" size="w185" :width="120" :height="180" :alt="title" />
        <p v-else-if="info === null" class="mt-4 text-[13px] text-faint">{{ t('discovery.noDetails') }}</p>
        <p v-else class="mt-4 text-[13px] text-faint">{{ t('common.loading') }}</p>

        <slot />

        <div class="mt-4">
          <slot name="actions" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import PosterImage from '@/components/enrich/PosterImage.vue'
import EnrichDetails from '@/components/enrich/EnrichDetails.vue'
import { useFocusTrap } from '@/composables/useFocusTrap'
import type { EnrichInfo } from '@/api/types'

const props = defineProps<{
  title: string
  serviceName: string
  start: number
  duration: number
  when: string
  now: number
  info?: EnrichInfo | null
  // A recording: its own line instead of the broadcast's, and its own
  // cover on the box (preferred over TMDB's poster).
  subtitle?: string
  recording?: string
  // No film/series details to look up (a radio programme): skips the
  // details block and its "nothing found" line.
  plain?: boolean
}>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()

const airingNow = computed(() => props.start <= props.now && props.now < props.start + props.duration)

const closeButton = ref<HTMLButtonElement | null>(null)
const dialogEl = ref<HTMLElement | null>(null)
useFocusTrap(dialogEl, { onEscape: () => emit('close') })
let opener: HTMLElement | null = null
onMounted(() => {
  opener = document.activeElement as HTMLElement | null
  closeButton.value?.focus()
})
onBeforeUnmount(() => opener?.focus())
</script>
