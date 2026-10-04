<!--
  A recommendation's details (DiscoveryHub): the shared ProgrammeDialog
  plus why the AI picked it, and "Create timer" through TimerHub's own
  plan → apply (the inline preview).
-->
<template>
  <ProgrammeDialog
    :title="rec.title"
    :service-name="rec.serviceName"
    :start="rec.start"
    :duration="rec.duration"
    :when="when"
    :now="now"
    :info="info"
    @close="$emit('close')"
  >
    <section class="mt-4 rounded border border-line-soft bg-bg p-3">
      <h3 class="text-[12.5px] text-faint">{{ t('discovery.why') }}</h3>
      <p class="mt-1 text-[14px] text-text">{{ rec.reason }}</p>
    </section>

    <template #actions>
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
    </template>
  </ProgrammeDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import ProgrammeDialog from '@/components/programme/ProgrammeDialog.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import type { EnrichInfo, Recommendation } from '@/api/types'
import type { TimerState } from './types'

defineProps<{
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
</script>
