<!--
  Shown instead of the shell when the very first GET /api/v1/whoami gets no
  answer at all (App.vue, authState.unreachable): the box is off or out of
  reach, or the installed app was opened away from home. Not the login
  form — that would suggest a password problem. Retries by itself when
  the browser comes back online, and every 15 s while it stays open.
-->
<template>
  <div class="flex h-screen w-screen items-center justify-center bg-bg px-4">
    <div class="w-full max-w-[380px] rounded-lg border border-line bg-tile p-6 text-center" role="alert">
      <img src="/icons/icon-192.png" alt="" class="mx-auto mb-4 h-12 w-12 rounded-lg" />
      <h1 class="mb-1 text-[19px] font-bold">{{ t('unreachable.title') }}</h1>
      <p class="mb-5 text-[13.5px] text-muted">{{ t('unreachable.body') }}</p>
      <button
        type="button"
        class="w-full rounded-sm bg-accent py-2 text-[14px] font-semibold text-bg disabled:opacity-60"
        :disabled="retrying"
        @click="retry"
      >
        {{ retrying ? t('unreachable.retrying') : t('unreachable.retry') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

const emit = defineEmits<{ retry: [] }>()
const props = defineProps<{ check: () => Promise<void> }>()
const { t } = useI18n()

const retrying = ref(false)
async function retry() {
  if (retrying.value) return
  retrying.value = true
  try {
    await props.check()
    emit('retry')
  } finally {
    retrying.value = false
  }
}

let timer: number | undefined
onMounted(() => {
  window.addEventListener('online', retry)
  timer = window.setInterval(retry, 15_000)
})
onBeforeUnmount(() => {
  window.removeEventListener('online', retry)
  window.clearInterval(timer)
})
</script>
