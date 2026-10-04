<!--
  A recommendation's details (DiscoveryHub), as a dialog: the backdrop
  across the top when enrichment has one, then the title, the broadcast
  (channel, day, time, length — "now" while it airs), everything the data
  sources know (EnrichDetails: poster, logo, year, genres, ratings, FSK,
  the full overview, credited), why the AI picked it, and "Create timer"
  through TimerHub's own plan → apply (the inline preview). Escape, the
  close button and the backdrop close it; the close button gets focus on
  open, and focus goes back to the card that opened it.
-->
<template>
  <div class="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/60 px-4 py-[6vh] below-sm:px-0 below-sm:py-0" @click.self="$emit('close')">
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="rec-title"
      class="relative w-full max-w-[680px] overflow-hidden rounded-lg border border-line bg-tile shadow-lg below-sm:min-h-full below-sm:rounded-none below-sm:border-0"
      @keydown.esc.stop="$emit('close')"
    >
      <div v-if="info?.backdrop" class="relative h-[220px] below-sm:h-[180px]">
        <PosterImage :path="info.backdrop" size="w780" fill alt="" />
        <div class="absolute inset-0 bg-gradient-to-t from-tile via-tile/30 to-transparent" />
      </div>
      <button
        ref="closeButton"
        type="button"
        class="absolute end-3 top-3 rounded bg-tile/80 px-2.5 py-1 text-[13px] text-text backdrop-blur hover:bg-tile-2"
        @click="$emit('close')"
      >
        {{ t('discovery.close') }}
      </button>

      <div class="p-5" :class="{ '-mt-12 relative': info?.backdrop }">
        <h2 id="rec-title" class="pe-20 text-[21px] font-bold leading-tight text-text">{{ rec.title }}</h2>
        <p class="mt-1 flex flex-wrap items-center gap-x-1.5 text-[13.5px] text-muted">
          <span v-if="airingNow" class="flex items-center gap-1 text-onair"><span class="h-1.5 w-1.5 rounded-full bg-onair" />{{ t('discovery.now') }}</span>
          <span class="tabular-nums">{{ when }}</span>
          <span>· {{ rec.serviceName }}</span>
          <span class="tabular-nums">· {{ t('discovery.minutes', { n: Math.round(rec.duration / 60) }) }}</span>
        </p>

        <EnrichDetails v-if="info" class="mt-4" :info="info" :shown-title="rec.title" :poster-width="120" :clamp="false" />
        <p v-else-if="info === null" class="mt-4 text-[13px] text-faint">{{ t('discovery.noDetails') }}</p>
        <p v-else class="mt-4 text-[13px] text-faint">{{ t('common.loading') }}</p>

        <section class="mt-4 rounded border border-line-soft bg-bg p-3">
          <h3 class="text-[12.5px] text-faint">{{ t('discovery.why') }}</h3>
          <p class="mt-1 text-[14px] text-text">{{ rec.reason }}</p>
        </section>

        <div class="mt-4">
          <p v-if="hasTimer" class="flex items-center gap-1.5 text-[13.5px] text-ok">
            <span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('discovery.created') }}
          </p>
          <PlanPreview
            v-else-if="state?.status === 'previewing' && state.plan"
            :plan="state.plan"
            :confirm-label="t('discovery.confirm')"
            :busy="state.busy"
            :error="state.error"
            @confirm="$emit('confirmTimer')"
            @cancel="$emit('cancelTimer')"
          >
            {{ t('series.detail.recordSummary', { title: rec.title, channel: rec.serviceName, time: when }) }}
          </PlanPreview>
          <template v-else>
            <button
              type="button"
              class="rounded bg-accent px-3.5 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-60"
              :disabled="!canCreateTimers"
              :title="canCreateTimers ? undefined : t('discovery.timerHubUnavailable')"
              @click="$emit('previewTimer')"
            >
              {{ t('discovery.createTimer') }}
            </button>
            <p v-if="state?.status === 'error'" class="mt-2 text-[12.5px] text-warn">{{ state.error }}</p>
          </template>
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
import PlanPreview from '@/components/PlanPreview.vue'
import type { EnrichInfo, Recommendation } from '@/api/types'
import type { TimerState } from './types'

const props = defineProps<{
  rec: Recommendation
  info?: EnrichInfo | null
  when: string
  now: number
  hasTimer: boolean
  state?: TimerState
  canCreateTimers: boolean
}>()
defineEmits<{ close: []; previewTimer: []; confirmTimer: []; cancelTimer: [] }>()
const { t } = useI18n()

const airingNow = computed(() => props.rec.start <= props.now && props.now < props.rec.start + props.rec.duration)

const closeButton = ref<HTMLButtonElement | null>(null)
let opener: HTMLElement | null = null
onMounted(() => {
  opener = document.activeElement as HTMLElement | null
  closeButton.value?.focus()
})
onBeforeUnmount(() => opener?.focus())
</script>
