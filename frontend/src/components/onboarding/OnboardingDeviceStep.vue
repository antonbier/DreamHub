<template>
  <OnboardingStepFrame
    :title="t('onboarding.device.title')"
    :subtitle="t('onboarding.device.subtitle')"
    show-back
    @next="$emit('next')"
    @back="$emit('back')"
  >
    <p v-if="loading" class="text-[14px] text-muted">{{ t('common.loading') }}</p>

    <div v-else-if="!info" class="rounded border border-line bg-field p-3.5">
      <p class="flex items-center gap-2 text-[13.5px] text-warn">
        <Icon name="warning" :size="14" />
        {{ t('onboarding.device.unreachable') }}
      </p>
    </div>

    <div v-else class="flex flex-col gap-3">
      <dl class="grid grid-cols-[140px_1fr] gap-x-3 gap-y-1.5 text-[13.5px]">
        <dt class="text-muted">{{ t('onboarding.device.name') }}</dt>
        <dd class="text-text">{{ info.deviceName }}</dd>
        <dt class="text-muted">{{ t('onboarding.device.image') }}</dt>
        <dd class="font-mono text-text">{{ info.imageVersion }}</dd>
        <dt class="text-muted">{{ t('onboarding.device.enigma') }}</dt>
        <dd class="font-mono text-text">{{ info.enigmaVersion }}</dd>
        <dt class="text-muted">{{ t('onboarding.device.tuners') }}</dt>
        <dd class="text-text">{{ info.frontends.map((f) => f.model).join(', ') || '—' }}</dd>
      </dl>

      <div class="mt-1 flex items-center gap-2 rounded border border-line bg-field p-3">
        <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="clockDotClass" />
        <span class="text-[13px] text-text">{{ clockText }}</span>
      </div>
    </div>
  </OnboardingStepFrame>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import OnboardingStepFrame from './OnboardingStepFrame.vue'
import Icon from '../Icon.vue'
import { api } from '@/api/client'
import { clockSkewMinutes } from '@/lib/clock'
import type { DeviceInfo } from '@/api/types'

defineEmits<{ next: []; back: [] }>()
const { t } = useI18n()

const loading = ref(true)
const info = ref<DeviceInfo | null>(null)
const skew = ref<number | null>(null)

onMounted(async () => {
  const [deviceResult, skewResult] = await Promise.allSettled([api.deviceInfo(), clockSkewMinutes()])
  if (deviceResult.status === 'fulfilled') info.value = deviceResult.value
  if (skewResult.status === 'fulfilled') skew.value = skewResult.value
  loading.value = false
})

const clockText = computed(() => {
  if (skew.value === null) return t('onboarding.device.clockUnknown')
  if (skew.value < 1) return t('onboarding.device.clockOk')
  return t('onboarding.device.clockOff', { minutes: skew.value })
})
const clockDotClass = computed(() => (skew.value !== null && skew.value >= 1 ? 'bg-warn' : 'bg-ok'))
</script>
