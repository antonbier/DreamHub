<!--
  A Plex film's or episode's details as a dialog (DESIGN.md: the
  Recommendation detail's shape): the backdrop across the top, a Close
  text button, title, a `muted` line (year · genres · length · FSK), the
  rating, poster beside the summary, the file's formats in `faint` (what
  the box's player has to decode), and the actions: "Play on the box" as
  the one primary action, mark seen / not seen as a text action. While
  Plex is offline both are off and say why. Escape, Close and the
  backdrop close it; focus returns to the opener.
-->
<template>
  <div class="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/60 px-4 py-[6vh] below-sm:px-0 below-sm:py-0" @click.self="$emit('close')">
    <div
      ref="dialogEl"
      role="dialog"
      aria-modal="true"
      aria-labelledby="plex-item-title"
      class="relative w-full max-w-[680px] overflow-hidden rounded-lg border border-line bg-tile shadow-lg below-sm:min-h-full below-sm:rounded-none below-sm:border-0"
    >
      <div v-if="item.art" class="relative h-[220px] below-sm:h-[180px]">
        <PosterImage :plex="item.art" :plex-width="1280" fill alt="" />
        <div class="absolute inset-0 bg-gradient-to-t from-tile via-tile/30 to-transparent" />
      </div>
      <button
        ref="closeButton"
        type="button"
        class="absolute end-3 top-3 rounded bg-tile/80 px-2.5 py-1 text-[13px] text-text backdrop-blur hover:bg-tile-2"
        @click="$emit('close')"
      >
        {{ t('plan.close') }}
      </button>

      <div class="p-5" :class="{ 'relative -mt-12': item.art }">
        <!-- The title's logo from Plex, like the enrichment logo elsewhere. -->
        <PosterImage v-if="item.logo" :plex="item.logo" :plex-width="360" logo :width="220" :height="36" :alt="item.title" class="mb-2" />
        <p v-if="item.type === 'episode'" class="text-[13px] text-muted">{{ item.showTitle }} · {{ text.episodeCode(item) }}</p>
        <h2 id="plex-item-title" class="pe-20 text-[21px] font-bold leading-tight text-text">{{ item.title }}</h2>
        <p class="mt-1 flex flex-wrap items-center gap-x-1.5 text-[13.5px] text-muted">
          <span v-for="(part, i) in line" :key="i">{{ i > 0 ? '· ' : '' }}{{ part }}</span>
          <span v-if="item.rating" class="ms-1 tabular-nums"><span class="text-warn" aria-hidden="true">★</span> <span class="text-text">{{ text.rating(item.rating) }}</span></span>
        </p>
        <p v-if="item.watched" class="mt-1.5 flex items-center gap-1.5 text-[13px] text-ok"><span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('plex.watched') }}</p>
        <p v-else-if="progress !== null" class="mt-1.5 text-[13px] text-muted">
          {{ t('plex.startedAt', { minutes: Math.round((item.viewOffset ?? 0) / 60000) }) }}
        </p>

        <div class="mt-4 flex gap-4">
          <PosterImage v-if="item.thumb && item.type !== 'episode'" :plex="item.thumb" :plex-width="240" :width="120" :height="180" alt="" />
          <p class="min-w-0 flex-1 whitespace-pre-line text-[13.5px] text-text">{{ item.summary || t('plex.noSummary') }}</p>
        </div>
        <p v-if="formats" class="mt-3 text-[12.5px] text-faint">{{ t('plex.file') }}: {{ formats }}</p>

        <div class="mt-5 flex flex-wrap items-center gap-3">
          <button
            v-if="canPlay"
            type="button"
            class="rounded bg-accent px-3.5 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-60"
            :disabled="!online || playing"
            @click="$emit('play')"
          >
            {{ playing ? t('plex.starting') : t('plex.playOnBox') }}
          </button>
          <button
            type="button"
            class="rounded px-2 py-1 text-[13.5px] text-accent hover:bg-tile-2 disabled:text-faint"
            :disabled="!online || busy"
            @click="$emit('toggleWatched')"
          >
            {{ item.watched ? t('plex.markUnwatched') : t('plex.markWatched') }}
          </button>
          <span v-if="!online" class="text-[12.5px] text-faint">{{ t('plex.offlineAction') }}</span>
        </div>
        <p v-if="canPlay" class="mt-2 text-[12px] text-faint">{{ t('plex.playHint') }}</p>
        <p v-if="error" class="mt-2 text-[12.5px] text-warn">{{ error }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import PosterImage from '@/components/enrich/PosterImage.vue'
import { useFocusTrap } from '@/composables/useFocusTrap'
import { usePlexText } from '@/composables/usePlexText'
import type { PlexItem } from '@/api/types'

const props = defineProps<{
  item: PlexItem
  online: boolean
  canPlay: boolean
  busy?: boolean
  playing?: boolean
  error?: string
}>()
const emit = defineEmits<{ close: []; play: []; toggleWatched: [] }>()
const { t } = useI18n()
const text = usePlexText()

const line = computed(() =>
  [
    props.item.year ? String(props.item.year) : '',
    props.item.genres?.slice(0, 3).join(', ') ?? '',
    text.minutes(props.item.duration),
    props.item.contentRating?.replace(/^[a-z]{2}\//, 'FSK ') ?? '',
  ].filter(Boolean),
)
const progress = computed(() => text.progress(props.item))
const formats = computed(() => text.formats(props.item.media?.[0]))

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
