<!--
  Under the Push section (SettingsView.vue): how to get notifications on
  the phone with ntfy — install the app, subscribe to the topic — a
  random topic name to copy (anyone who knows a topic on ntfy.sh can read
  it, so it should be hard to guess), and "Send a test" with the saved
  settings (POST /api/v1/push/test).
-->
<template>
  <section class="rounded border border-line bg-tile p-5">
    <h2 class="mb-1 text-[15.5px] font-semibold">{{ t('settings.pushHelp.title') }}</h2>
    <ol class="mb-3 list-decimal ps-5 text-[13px] text-muted">
      <li>{{ t('settings.pushHelp.step1') }}</li>
      <li>{{ t('settings.pushHelp.step2') }}</li>
      <li>{{ t('settings.pushHelp.step3') }}</li>
    </ol>
    <div class="mb-4 flex flex-wrap items-center gap-2 text-[13px]">
      <span class="text-faint">{{ t('settings.pushHelp.suggestion') }}</span>
      <code class="rounded bg-field px-2 py-1 font-mono text-[12.5px] text-text">{{ suggestion }}</code>
      <button type="button" class="rounded px-2 py-1 text-[12.5px] text-accent hover:bg-tile-2" @click="copy">
        {{ copied ? t('settings.tokens.copied') : t('settings.tokens.copy') }}
      </button>
    </div>
    <div class="flex flex-wrap items-center gap-3">
      <button type="button" class="rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field disabled:opacity-60" :disabled="sending" @click="sendTest">
        {{ sending ? t('settings.pushHelp.sending') : t('settings.pushHelp.test') }}
      </button>
      <span v-if="result === 'ok'" class="text-[13px] text-ok" role="status">{{ t('settings.pushHelp.sent') }}</span>
      <span v-else-if="result" class="text-[13px] text-warn" role="status">{{ result }}</span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'

const { t } = useI18n()

// "dreamhub-" and 12 random letters/digits: hard to guess on a public server.
function randomTopic() {
  const chars = 'abcdefghijkmnpqrstuvwxyz23456789'
  const bytes = new Uint8Array(12)
  crypto.getRandomValues(bytes)
  return 'dreamhub-' + Array.from(bytes, (b) => chars[b % chars.length]).join('')
}
const suggestion = ref(randomTopic())
const copied = ref(false)
async function copy() {
  try {
    await navigator.clipboard.writeText(suggestion.value)
    copied.value = true
  } catch {
    // Clipboard blocked (plain http on some browsers): the name is visible to copy by hand.
  }
}

const sending = ref(false)
const result = ref('')
async function sendTest() {
  sending.value = true
  result.value = ''
  try {
    await api.push.test()
    result.value = 'ok'
  } catch (err) {
    const code = err instanceof ApiError ? err.code : 'network_error'
    const key = `settings.pushHelp.errors.${code}`
    result.value = t(key) === key ? code : t(key)
  } finally {
    sending.value = false
  }
}
</script>
