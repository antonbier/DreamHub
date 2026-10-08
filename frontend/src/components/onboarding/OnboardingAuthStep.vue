<template>
  <OnboardingStepFrame
    :title="t('onboarding.auth.title')"
    :subtitle="t('onboarding.auth.subtitle')"
    show-back
    :busy="saving"
    :error="error"
    @next="save"
    @back="$emit('back')"
  >
    <button
      type="button"
      role="switch"
      :aria-checked="enabled"
      class="mb-4 flex h-6 w-10 items-center rounded-full p-0.5 transition-colors"
      :class="enabled ? 'bg-accent justify-end' : 'bg-field justify-start'"
      @click="enabled = !enabled"
    >
      <span class="h-5 w-5 rounded-full bg-tile shadow" />
    </button>
    <span class="mb-4 block text-[14px] text-text">{{ t('settings.auth.enabled') }}</span>

    <div v-if="enabled" class="flex flex-col gap-3">
      <label class="block">
        <span class="mb-1 block text-[13px] text-muted">{{ t('settings.auth.username') }}</span>
        <input v-model="username" type="text" autocomplete="username" class="w-full max-w-[320px] rounded border border-line bg-field px-2.5 py-1.5 text-[14px] text-text" />
      </label>
      <label class="block">
        <span class="mb-1 block text-[13px] text-muted">{{ t('settings.auth.password') }}</span>
        <input v-model="password" type="password" autocomplete="new-password" class="w-full max-w-[320px] rounded border border-line bg-field px-2.5 py-1.5 text-[14px] text-text" />
      </label>
    </div>
  </OnboardingStepFrame>
</template>

<script setup lang="ts">
import { useFriendlyError } from '@/composables/useFriendlyError'
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import OnboardingStepFrame from './OnboardingStepFrame.vue'
import { api, ApiError } from '@/api/client'
import { useAuth } from '@/composables/useAuth'
import { useSettingsText } from '@/composables/useSettingsText'

const emit = defineEmits<{ next: []; back: [] }>()
const { t } = useI18n()
const { friendly } = useFriendlyError()
const { login } = useAuth()
const { fieldError } = useSettingsText()

const enabled = ref(false)
const username = ref('')
const password = ref('')
const saving = ref(false)
const error = ref('')

async function save() {
  error.value = ''
  if (!enabled.value) {
    // Nothing to configure; make sure it's actually off (re-running the
    // wizard might be turning a previous "on" back off).
    saving.value = true
    try {
      await api.settings.update({ 'auth.enabled': false })
    } catch {
      // Non-fatal — worst case auth stays as it was; the settings page
      // can always fix it.
    }
    saving.value = false
    emit('next')
    return
  }

  saving.value = true
  try {
    await api.settings.update({
      'auth.enabled': true,
      'auth.username': username.value,
      'auth.password': password.value,
    })
    // Adopt the credentials we just set so the rest of this session (and
    // the rest of the wizard) doesn't immediately lock itself out.
    const result = await login(username.value, password.value)
    if (!result.ok) {
      error.value = t('login.errorWrong')
      return
    }
    emit('next')
  } catch (err) {
    if (err instanceof ApiError && err.details) {
      const first = Object.values(err.details)[0]
      error.value = first ? fieldError(first) : friendly(err).text
    } else {
      error.value = friendly(err).text
    }
  } finally {
    saving.value = false
  }
}
</script>
