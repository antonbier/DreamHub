<!--
  The onboarding wizard (CONCEPT.md, "Onboarding wizard"). Runs on first
  start (router/index.ts sends an admin here until the box remembers the
  setup, lib/onboarding.ts) and is re-runnable any time from Settings.
  Every step works against real endpoints; nothing here is simulated.
-->
<template>
  <div class="mx-auto flex min-h-full max-w-[560px] flex-col justify-center p-6 below-md:p-4">
    <div class="mb-6 flex gap-1.5">
      <span
        v-for="(s, i) in STEPS"
        :key="s"
        class="h-1 flex-1 rounded-full"
        :class="i <= step ? 'bg-accent' : 'bg-line'"
      />
    </div>

    <component
      :is="stepComponent"
      @next="next"
      @back="back"
      @skip="next"
      @finish="finish"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import OnboardingLanguageStep from '@/components/onboarding/OnboardingLanguageStep.vue'
import OnboardingAuthStep from '@/components/onboarding/OnboardingAuthStep.vue'
import OnboardingDeviceStep from '@/components/onboarding/OnboardingDeviceStep.vue'
import OnboardingHubsStep from '@/components/onboarding/OnboardingHubsStep.vue'
import OnboardingAiStep from '@/components/onboarding/OnboardingAiStep.vue'
import OnboardingDoneStep from '@/components/onboarding/OnboardingDoneStep.vue'
import { markSetupDone } from '@/lib/onboarding'

const STEPS = ['language', 'auth', 'device', 'hubs', 'ai', 'done'] as const
const COMPONENTS = {
  language: OnboardingLanguageStep,
  auth: OnboardingAuthStep,
  device: OnboardingDeviceStep,
  hubs: OnboardingHubsStep,
  ai: OnboardingAiStep,
  done: OnboardingDoneStep,
}

const router = useRouter()
const step = ref(0)
const stepComponent = computed(() => COMPONENTS[STEPS[step.value]])

function next() {
  if (step.value < STEPS.length - 1) step.value++
}
function back() {
  if (step.value > 0) step.value--
}
async function finish() {
  await markSetupDone()
  void router.push('/')
}
</script>
