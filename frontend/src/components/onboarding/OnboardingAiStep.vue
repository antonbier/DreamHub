<template>
  <OnboardingStepFrame
    :title="t('onboarding.ai.title')"
    :subtitle="t('onboarding.ai.subtitle')"
    show-back
    show-skip
    :busy="saving"
    :error="error"
    @next="save"
    @back="$emit('back')"
    @skip="$emit('next')"
  >
    <label class="mb-3 block max-w-[320px]">
      <span class="mb-1 block text-[13px] text-muted">{{ t('settings.ai.provider') }}</span>
      <select disabled class="w-full rounded border border-line bg-field px-2.5 py-1.5 text-[14px] text-text opacity-70">
        <option>OpenAI</option>
      </select>
    </label>
    <label class="block max-w-[320px]">
      <span class="mb-1 block text-[13px] text-muted">{{ t('settings.ai.openai.apiKey') }}</span>
      <input
        v-model="apiKey"
        type="password"
        autocomplete="off"
        :placeholder="t('settings.page.secretPlaceholder')"
        class="w-full rounded border border-line bg-field px-2.5 py-1.5 text-[14px] text-text"
      />
    </label>
  </OnboardingStepFrame>
</template>

<script setup lang="ts">
import { useFriendlyError } from '@/composables/useFriendlyError'
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import OnboardingStepFrame from './OnboardingStepFrame.vue'
import { api } from '@/api/client'

const emit = defineEmits<{ next: []; back: [] }>()
const { t } = useI18n()
const { friendly } = useFriendlyError()

const apiKey = ref('')
const saving = ref(false)
const error = ref('')

async function save() {
  if (!apiKey.value) {
    emit('next')
    return
  }
  saving.value = true
  error.value = ''
  try {
    await api.settings.update({ 'ai.openai.apiKey': apiKey.value })
    emit('next')
  } catch (err) {
    error.value = friendly(err).text
  } finally {
    saving.value = false
  }
}
</script>
