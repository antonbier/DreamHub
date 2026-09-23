<!--
  Shown instead of the whole shell whenever GET /api/v1/whoami says we
  aren't authenticated (App.vue) — see src/api/authState.ts for why the app
  needs its own login screen instead of the browser's native Basic Auth
  prompt.
-->
<template>
  <div class="flex h-screen w-screen items-center justify-center bg-bg px-4">
    <form class="w-full max-w-[360px] rounded-lg border border-line bg-tile p-6" @submit.prevent="submit">
      <h1 class="mb-1 text-[21px] font-bold">{{ t('app.name') }}</h1>
      <p class="mb-5 text-[13.5px] text-muted">{{ t('login.subtitle') }}</p>

      <label class="mb-3 block">
        <span class="mb-1 block text-[13px] text-muted">{{ t('login.username') }}</span>
        <input
          v-model="username"
          type="text"
          autocomplete="username"
          autofocus
          class="w-full rounded border border-line bg-field px-2.5 py-1.5 text-[14px] text-text"
        />
      </label>
      <label class="mb-4 block">
        <span class="mb-1 block text-[13px] text-muted">{{ t('login.password') }}</span>
        <input
          v-model="password"
          type="password"
          autocomplete="current-password"
          class="w-full rounded border border-line bg-field px-2.5 py-1.5 text-[14px] text-text"
        />
      </label>

      <p v-if="error" class="mb-4 text-[13px] text-warn">{{ error }}</p>

      <button
        type="submit"
        class="w-full rounded bg-accent py-2 text-[14px] font-semibold text-bg disabled:opacity-60"
        :disabled="submitting || !username || !password"
      >
        {{ submitting ? t('login.submitting') : t('login.submit') }}
      </button>
    </form>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuth } from '@/composables/useAuth'

const emit = defineEmits<{ success: [] }>()
const { t } = useI18n()
const { login } = useAuth()

const username = ref('')
const password = ref('')
const submitting = ref(false)
const error = ref('')

async function submit() {
  submitting.value = true
  error.value = ''
  const result = await login(username.value, password.value)
  submitting.value = false
  if (result.ok) {
    emit('success')
    return
  }
  error.value = result.message === 'wrong' ? t('login.errorWrong') : t('login.errorNetwork')
  password.value = ''
}
</script>
