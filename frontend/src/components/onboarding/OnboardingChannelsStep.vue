<!--
  ChannelHub (channel/bouquet management) is v1.1 (ROADMAP.md), so there's
  no channel list to select from yet. An honest "not yet" beats a
  fabricated one — DiscoveryHub uses every channel until this exists.
-->
<template>
  <OnboardingStepFrame
    :title="t('onboarding.channels.title')"
    :subtitle="t('onboarding.channels.subtitle')"
    show-back
    @next="$emit('next')"
    @back="$emit('back')"
  >
    <div class="rounded border border-line bg-field p-3.5 text-[13.5px] text-muted">
      {{ t('onboarding.channels.notBuilt') }}
    </div>
  </OnboardingStepFrame>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import OnboardingStepFrame from './OnboardingStepFrame.vue'

defineEmits<{ next: []; back: [] }>()
const { t } = useI18n()
</script>
