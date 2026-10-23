<!--
  DeviceHub's reception card: signal quality (SNR) and strength (AGC) of
  the tuner showing the current channel, the bit error rate and, where the
  box gives a real one, the SNR in dB (GET /api/v1/deviceinfo/signal).
  Read again every few seconds while the page is open and visible — handy
  when aligning a dish. Hidden on a box without the call; a box that gives
  no numbers (standby, no tuner in use) says so.
-->
<template>
  <section v-if="available" class="mt-4 rounded-sm border border-line bg-tile p-4">
    <h2 class="text-[14px] font-semibold text-text">{{ t('system.signal.title') }}</h2>
    <p class="mt-0.5 text-[12.5px] text-faint">{{ t('system.signal.hint') }}</p>
    <p v-if="!signal" class="mt-2 text-[13px] text-muted">{{ t('system.signal.none') }}</p>
    <dl v-else class="mt-2 grid grid-cols-[max-content_1fr] items-center gap-x-6 gap-y-2 text-[13px] below-sm:grid-cols-1 below-sm:gap-y-0">
      <template v-for="m in meters" :key="m.key">
        <dt class="text-muted below-sm:mt-1.5">{{ t(`system.signal.${m.key}`) }}</dt>
        <dd class="flex min-w-0 items-center gap-3">
          <div
            class="h-1.5 max-w-60 flex-1 overflow-hidden rounded-full bg-field"
            role="meter"
            :aria-valuenow="m.value"
            aria-valuemin="0"
            aria-valuemax="100"
            :aria-label="t(`system.signal.${m.key}`)"
          >
            <div class="h-full" :class="m.value < WEAK ? 'bg-warn' : 'bg-ok'" :style="{ width: clamp(m.value) + '%' }" />
          </div>
          <span class="shrink-0 tabular-nums text-text">{{ percent(m.value) }}</span>
          <span v-if="m.key === 'snr' && signal.snrDb !== undefined" class="shrink-0 tabular-nums text-faint">
            {{ t('system.signal.db', { value: decimal(signal.snrDb) }) }}
          </span>
        </dd>
      </template>
      <dt class="text-muted below-sm:mt-1.5">{{ t('system.signal.ber') }}</dt>
      <dd class="tabular-nums text-text">{{ decimal(signal.ber) }}</dd>
    </dl>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import type { TunerSignal } from '@/api/types'

const { t, locale } = useI18n()

// Below this many percent a value shows as weak.
const WEAK = 40
const EVERY_MS = 5_000

const available = ref(false)
const signal = ref<TunerSignal | null>(null)
const meters = computed(() => {
  const s = signal.value
  if (!s) return []
  return s.agc === undefined ? [{ key: 'snr', value: s.snr }] : [{ key: 'snr', value: s.snr }, { key: 'agc', value: s.agc }]
})

function clamp(v: number): number {
  return Math.min(100, Math.max(0, v))
}
function percent(v: number): string {
  return new Intl.NumberFormat(locale.value, { style: 'percent' }).format(clamp(v) / 100)
}
function decimal(v: number): string {
  return new Intl.NumberFormat(locale.value, { maximumFractionDigits: 1 }).format(v)
}

async function load() {
  if (document.visibilityState === 'hidden') return
  try {
    const res = await api.tunerSignal()
    available.value = res.available
    signal.value = res.signal
  } catch {
    // Keep the last reading; the device card above shows a box that's gone.
  }
}

let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  void load()
  timer = setInterval(() => void load(), EVERY_MS)
})
onUnmounted(() => clearInterval(timer))
</script>
