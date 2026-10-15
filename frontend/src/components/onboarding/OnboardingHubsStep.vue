<template>
  <OnboardingStepFrame
    :title="t('onboarding.hubs.title')"
    :subtitle="t('onboarding.hubs.subtitle')"
    show-back
    :busy="saving"
    :error="error"
    @next="save"
    @back="$emit('back')"
  >
    <p v-if="loading" class="text-[14px] text-muted">{{ t('common.loading') }}</p>
    <div v-else class="flex flex-col gap-2">
      <label
        v-for="field in fields"
        :key="field.key"
        class="flex items-center justify-between rounded-sm border border-line px-3.5 py-2.5"
      >
        <span class="text-[14px] text-text">{{ t(field.label) }}</span>
        <button
          type="button"
          role="switch"
          :aria-checked="draft[field.key]"
          class="flex h-6 w-10 items-center rounded-full p-0.5 transition-colors"
          :class="draft[field.key] ? 'bg-accent justify-end' : 'bg-field justify-start'"
          @click="draft[field.key] = !draft[field.key]"
        >
          <span class="h-5 w-5 rounded-full bg-tile shadow-sm" />
        </button>
      </label>
    </div>
  </OnboardingStepFrame>
</template>

<script setup lang="ts">
import { useFriendlyError } from '@/composables/useFriendlyError'
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import OnboardingStepFrame from './OnboardingStepFrame.vue'
import { api } from '@/api/client'
import type { SettingField } from '@/api/types'

const emit = defineEmits<{ next: []; back: [] }>()
const { t } = useI18n()
const { friendly } = useFriendlyError()

const loading = ref(true)
const saving = ref(false)
const error = ref('')
const fields = ref<SettingField[]>([])
const draft = reactive<Record<string, boolean>>({})

onMounted(async () => {
  try {
    const view = await api.settings.get()
    const section = view.sections.find((s) => s.id === 'hubs')
    fields.value = section?.fields ?? []
    for (const f of fields.value) draft[f.key] = Boolean(f.value)
  } catch {
    // An empty list still lets the wizard continue; the settings page has
    // the full picture either way.
  } finally {
    loading.value = false
  }
})

async function save() {
  saving.value = true
  error.value = ''
  try {
    await api.settings.update({ ...draft })
    emit('next')
  } catch (err) {
    error.value = friendly(err).text
  } finally {
    saving.value = false
  }
}
</script>
