<!--
  "Message to TV" (ControlHub, POST /api/v1/control/message): a short text
  shown as a message box on the TV, e.g. "Dinner's ready". Direct like the
  remote — a transient box on the screen, not a durable change — and the
  picture refreshes afterwards so the message is visible here too.
-->
<template>
  <form class="flex flex-col gap-2" @submit.prevent="send">
    <h2 class="text-[14px] font-semibold text-text">{{ t('control.message.title') }}</h2>
    <div class="flex flex-wrap items-center gap-2">
      <label class="sr-only" for="tv-message-text">{{ t('control.message.text') }}</label>
      <input
        id="tv-message-text"
        v-model="text"
        type="text"
        maxlength="300"
        :placeholder="t('control.message.placeholder')"
        class="min-w-0 flex-1 rounded border border-line bg-field px-2.5 py-1.5 text-[13.5px] text-text"
      />
      <label class="sr-only" for="tv-message-kind">{{ t('control.message.kind') }}</label>
      <select id="tv-message-kind" v-model="kind" class="rounded border border-line bg-field px-2 py-1.5 text-[13px] text-text">
        <option value="info">{{ t('control.message.kinds.info') }}</option>
        <option value="warning">{{ t('control.message.kinds.warning') }}</option>
        <option value="error">{{ t('control.message.kinds.error') }}</option>
      </select>
      <label class="sr-only" for="tv-message-timeout">{{ t('control.message.timeout') }}</label>
      <select id="tv-message-timeout" v-model.number="timeout" class="rounded border border-line bg-field px-2 py-1.5 text-[13px] text-text">
        <option v-for="s in [5, 10, 30, 60]" :key="s" :value="s">{{ t('control.message.seconds', { n: s }) }}</option>
        <option :value="0">{{ t('control.message.untilClosed') }}</option>
      </select>
      <button
        type="submit"
        class="rounded bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60"
        :disabled="sending || !text.trim()"
      >
        {{ t('control.message.send') }}
      </button>
    </div>
    <p class="min-h-[1em] text-[12.5px]" :class="error ? 'text-warn' : 'text-ok'" aria-live="polite">{{ error || (sent ? t('control.message.sent') : '') }}</p>
  </form>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { usePlanError } from '@/composables/usePlanError'
import { useRemoteStore } from '@/stores/remote'

const { t } = useI18n()
const { planErrorMessage } = usePlanError()
const remote = useRemoteStore()

const text = ref('')
const kind = ref<'info' | 'warning' | 'error'>('info')
const timeout = ref(10)
const sending = ref(false)
const sent = ref(false)
const error = ref('')

async function send() {
  sending.value = true
  sent.value = false
  error.value = ''
  try {
    await api.control.message(text.value.trim(), kind.value, timeout.value)
    sent.value = true
    text.value = ''
    remote.keysSent++ // the picture refreshes, showing the message box
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    sending.value = false
  }
}
</script>
