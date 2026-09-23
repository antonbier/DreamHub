<template>
  <OnboardingStepFrame :title="t('onboarding.done.title')" :subtitle="t('onboarding.done.subtitle')" show-back :next-label="t('onboarding.done.finish')" @next="$emit('finish')" @back="$emit('back')">
    <div class="flex flex-col items-center gap-3 rounded border border-line bg-field p-5">
      <img v-if="qrDataUrl" :src="qrDataUrl" :alt="t('onboarding.done.qrAlt')" width="176" height="176" class="rounded bg-white p-2" />
      <p class="max-w-[280px] text-center text-[12.5px] text-faint">{{ origin }}</p>
    </div>
  </OnboardingStepFrame>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import OnboardingStepFrame from './OnboardingStepFrame.vue'

defineEmits<{ finish: []; back: [] }>()
const { t } = useI18n()

const origin = location.origin
const qrDataUrl = ref('')

onMounted(async () => {
  try {
    qrDataUrl.value = await QRCode.toDataURL(origin, { margin: 0, width: 176 })
  } catch {
    // No QR code, still show the URL text above — good enough to type in
    // by hand.
  }
})
</script>
