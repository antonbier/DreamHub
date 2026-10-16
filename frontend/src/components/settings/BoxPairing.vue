<!--
  Pairing one box with the DreamHub running on it (docs/MULTIDEVICE_PLAN.md,
  M6). Without it a box is Basic; paired, this DreamHub knows the other one
  and what it can do. The code is read from the other box's screen or TV —
  that is the proof that someone stands there — and used once.
-->
<template>
  <div class="rounded-sm border border-line bg-field p-4">
    <h3 class="mb-1 text-[14px] font-semibold">{{ t('boxes.pair.title') }}</h3>

    <template v-if="box.paired">
      <p class="mb-2 text-[13px]" :class="ok ? 'text-ok' : 'text-warn'" role="status">{{ statusText }}</p>
      <p v-if="peer?.compat && peer.compat !== 'ok'" class="mb-2 text-[13px] text-warn">
        {{ t(peer.compat === 'hub_too_old' ? 'boxes.pair.hubTooOld' : 'boxes.pair.boxTooOld') }}
      </p>
      <p v-if="peer?.state === 'revoked'" class="mb-2 text-[12.5px] text-faint">{{ t('boxes.pair.revokedHint') }}</p>
      <details class="mb-3 text-[13px]">
        <summary class="cursor-pointer text-muted">{{ t('boxes.pair.advanced') }}</summary>
        <label class="mt-2 block">
          <span class="mb-1 block text-muted">{{ t('boxes.pair.streamBase') }}</span>
          <input v-model="streamBase" type="text" inputmode="url" autocapitalize="off" spellcheck="false" placeholder="192.168.1.5:8080" class="w-full max-w-[280px] rounded-sm border border-line bg-tile px-2.5 py-1.5 font-mono text-[14px] text-text" />
          <span class="mt-1 block text-[12.5px] text-faint">{{ t('boxes.pair.streamBaseHint') }}</span>
        </label>
        <button type="button" class="mt-2 rounded-sm border border-line px-3 py-1.5 text-[13px] text-text hover:bg-tile disabled:opacity-60" :disabled="busy || streamBase.trim() === (box.streamBase ?? '')" @click="saveStreamBase">
          {{ t('boxes.form.save') }}
        </button>
      </details>
      <button type="button" class="rounded-sm px-3 py-1.5 text-[13px] text-warn hover:bg-tile disabled:opacity-60" :disabled="busy" @click="unpair">
        {{ t('boxes.pair.unpair') }}
      </button>
      <p class="mt-2 text-[12.5px] text-faint">{{ t('boxes.pair.unpairHint') }}</p>
    </template>

    <form v-else @submit.prevent="pair">
      <p class="mb-3 text-[13px] text-muted">{{ t('boxes.pair.hint') }}</p>
      <label class="mb-3 block">
        <span class="mb-1 block text-[13px] text-muted">{{ t('boxes.pair.code') }}</span>
        <input v-model="code" type="text" inputmode="numeric" autocomplete="off" placeholder="1234 5678" class="w-[160px] rounded-sm border border-line bg-tile px-2.5 py-1.5 font-mono text-[16px] tracking-wider text-text" />
      </label>
      <details class="mb-3 text-[13px]">
        <summary class="cursor-pointer text-muted">{{ t('boxes.pair.otherAddress') }}</summary>
        <input v-model="url" type="text" inputmode="url" autocapitalize="off" spellcheck="false" :placeholder="t('boxes.pair.otherAddressPlaceholder')" class="mt-2 w-full max-w-[280px] rounded-sm border border-line bg-tile px-2.5 py-1.5 font-mono text-[14px] text-text" />
      </details>
      <p v-if="error" class="mb-3 text-[13px] text-warn" role="alert">{{ error }}</p>
      <button type="submit" class="rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60" :disabled="busy || code.trim() === ''">
        {{ busy ? t('boxes.pair.pairing') : t('boxes.pair.pair') }}
      </button>
    </form>
    <p v-if="box.paired && error" class="mt-2 text-[13px] text-warn" role="alert">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { boxErrorKey } from './boxErrors'
import type { BoxInfo } from '@/api/types'

const props = defineProps<{ box: BoxInfo }>()
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()

const code = ref('')
const url = ref('')
const streamBase = ref(props.box.streamBase ?? '')
const busy = ref(false)
const error = ref('')

const peer = computed(() => props.box.peer)
const ok = computed(() => peer.value?.state === 'ok' && (peer.value.compat ?? 'ok') === 'ok')
const statusText = computed(() => {
  switch (peer.value?.state) {
    case 'ok':
      return t('boxes.pair.state.ok', { version: peer.value.version ?? '' })
    case 'revoked':
      return t('boxes.pair.state.revoked')
    case 'unreachable':
      return t('boxes.pair.state.unreachable')
    default:
      return t('boxes.pair.state.paired')
  }
})

async function run(fn: () => Promise<unknown>) {
  busy.value = true
  error.value = ''
  try {
    await fn()
    code.value = ''
    emit('changed')
  } catch (err) {
    error.value = t(boxErrorKey(err))
  } finally {
    busy.value = false
  }
}

const pair = () => run(() => api.boxes.pair(props.box.id, { code: code.value.trim(), url: url.value.trim() || undefined }))
const unpair = () => run(() => api.boxes.unpair(props.box.id))
const saveStreamBase = () => run(() => api.boxes.update(props.box.id, { streamBase: streamBase.value.trim() }))
</script>
