<!--
  One timer in TimerHub's list — DESIGN.md's three-column Row: time range
  (leading), name plus channel/source (main), state and a text action
  (trailing). A likely conflict tints the time in `warn` and says so in the
  secondary line; the state is a dot plus a word, never a pill. Disable/enable and
  delete preview in place below the row (PlanPreview).
-->
<template>
  <li class="border-b border-line-soft last:border-none">
    <div class="group flex items-center gap-3 py-2 hover:bg-tile-2 below-sm:flex-wrap">
      <span
        class="w-36 shrink-0 whitespace-nowrap font-mono text-[12.5px] tabular-nums below-sm:w-auto below-sm:basis-full"
        :class="timer.conflict ? 'text-warn' : 'text-faint'"
      >
        {{ timeRange }}
      </span>
      <span class="min-w-0 flex-1">
        <span class="block wrap-break-word text-[14px]" :class="timer.disabled ? 'text-muted' : 'text-text'">{{ timer.name }}</span>
        <span class="block wrap-break-word text-[12.5px] text-muted">
          {{ timer.serviceName }}
          <template v-if="timer.source"> · {{ t(`timers.source.${timer.source}`) }}</template>
          <template v-if="timer.justPlay"> · {{ t('timers.zap') }}</template>
          <template v-if="timer.repeated"> · {{ t('timers.repeating') }}</template>
          <span v-if="timer.conflict" class="text-warn"> · {{ t('timers.likelyConflict') }}</span>
        </span>
      </span>
      <span class="flex shrink-0 items-center gap-1.5 text-[12.5px]" :class="stateClass">
        <span class="h-1.5 w-1.5 rounded-full" :class="dotClass" />
        <span>{{ t(`timers.state.${stateKey}`) }}</span>
      </span>
      <!-- One action in sight: switch it off/on, or delete when it can't be
           switched (running, finished). The other one is under "more". -->
      <template v-if="timer.mayChange">
        <button
          type="button"
          class="tap shrink-0 rounded-sm px-2 py-1 text-[13px] text-accent hover:bg-tile"
          :class="{ invisible: deleting }"
          :disabled="!!deleting"
          @click="toggleable ? emit('toggle') : emit('delete')"
        >
          {{ toggleable ? (timer.disabled ? t('timers.enable') : t('timers.disable')) : t('timers.delete') }}
        </button>
        <DropdownMenu v-if="toggleable" :class="{ invisible: deleting }">
          <template #trigger="{ open }">
            <button
              type="button"
              class="tap flex h-7 w-7 shrink-0 items-center justify-center rounded-sm text-muted hover:bg-tile hover:text-text"
              :class="{ 'bg-tile': open }"
              :aria-label="t('timers.more')"
              :disabled="!!deleting"
            >
              <Icon name="more" :size="16" />
            </button>
          </template>
          <template #panel="{ close }">
            <button type="button" class="block w-full px-3 py-2 text-start text-[13.5px] text-onair hover:bg-tile-2" @click="close(); emit('delete')">
              {{ t('timers.delete') }}
            </button>
          </template>
        </DropdownMenu>
      </template>
    </div>
    <div v-if="deleting" class="pb-3">
      <p v-if="deleting.busy && !deleting.plan" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
      <PlanPreview
        v-else-if="deleting.plan"
        :plan="deleting.plan"
        :confirm-label="t(`timers.confirm.${deleting.kind}`)"
        :busy="deleting.busy"
        :error="deleting.error"
        :danger="deleting.kind === 'delete'"
        @confirm="emit('confirm')"
        @cancel="emit('cancel')"
      >
        {{ t(`timers.preview.${deleting.kind}`, { name: timer.name, channel: timer.serviceName, when: timeRange }) }}
      </PlanPreview>
      <div v-else-if="deleting.error" class="flex items-center gap-3 text-[12.5px] text-warn">
        {{ deleting.error }}
        <button type="button" class="text-accent hover:underline" @click="emit('cancel')">{{ t('plan.close') }}</button>
      </div>
    </div>
  </li>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PlanPreview from '@/components/PlanPreview.vue'
import DropdownMenu from '@/components/DropdownMenu.vue'
import Icon from '@/components/Icon.vue'
import type { DeleteState } from './types'
import type { Timer } from '@/api/types'

const props = defineProps<{
  timer: Timer
  timeZone: string | undefined
  deleting: DeleteState | null
  // Finished timers span several days; show the date too.
  showDate?: boolean
}>()
const emit = defineEmits<{ delete: []; toggle: []; confirm: []; cancel: [] }>()

const { t, locale } = useI18n()

const timeRange = computed(() => {
  const time = new Intl.DateTimeFormat(locale.value, { hour: '2-digit', minute: '2-digit', timeZone: props.timeZone })
  const range = `${time.format(props.timer.begin * 1000)}–${time.format(props.timer.end * 1000)}`
  if (!props.showDate) return range
  const date = new Intl.DateTimeFormat(locale.value, { day: '2-digit', month: '2-digit', timeZone: props.timeZone })
  return `${date.format(props.timer.begin * 1000)} ${range}`
})

// Enigma2 states (webcontrol.Timer.State): 0 waiting, 1 about to start,
// 2 recording, 3 ended. Disabled wins over whatever the state says.
const stateKey = computed(() => {
  if (props.timer.disabled) return 'disabled'
  switch (props.timer.state) {
    case 1:
      return 'preparing'
    case 2:
      return 'recording'
    case 3:
      return 'done'
    default:
      return 'scheduled'
  }
})
const dotClass = computed(
  () => ({ recording: 'bg-onair', preparing: 'bg-warn', scheduled: 'bg-ok', done: 'bg-faint', disabled: 'bg-faint' })[stateKey.value],
)
// Disable/enable: not while it records (taking it out would stop the
// recording), and enabling only while its time is still ahead.
const toggleable = computed(() => {
  if (stateKey.value === 'recording' || stateKey.value === 'preparing') return false
  if (props.timer.disabled) return props.timer.end > Date.now() / 1000
  return stateKey.value === 'scheduled'
})
const stateClass = computed(() => (stateKey.value === 'recording' ? 'text-onair' : 'text-muted'))
</script>
