<!--
  The other side of pairing (docs/MULTIDEVICE_PLAN.md, M6): this DreamHub
  shows a one-time code — here and on this box's TV — that the DreamHub of
  another box asks for under Settings → Boxes. Out of the way on purpose:
  a household with one box never needs it.
-->
<template>
  <details class="mt-5 text-[13px]" :open="route.query.pair === '1'" @toggle="onToggle">
    <summary class="cursor-pointer text-muted">{{ t('boxes.pairThis.title') }}</summary>
    <p class="mt-2 text-muted">{{ t('boxes.pairThis.hint') }}</p>
    <ol class="mt-1 list-decimal ps-5 text-muted">
      <li>{{ t('boxes.pairThis.step1') }}</li>
      <li>{{ t('boxes.pairThis.step2') }}</li>
    </ol>
    <button type="button" class="mt-2 rounded-sm border border-line px-3 py-1.5 text-[13px] text-text hover:bg-tile disabled:opacity-60" :disabled="busy" @click="show">
      {{ t('boxes.pairThis.show') }}
    </button>
    <div v-if="shown" class="mt-3" role="status">
      <p class="font-mono text-[28px] font-semibold tracking-[0.15em] text-text">{{ shown.code }}</p>
      <p class="text-[12.5px] text-faint">
        {{ t('boxes.pairThis.valid', { time: until }) }}
        <template v-if="shown.onTv"> · {{ t('boxes.pairThis.onTv') }}</template>
      </p>
    </div>
    <p v-if="error" class="mt-2 text-warn" role="alert">{{ error }}</p>
  </details>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { api } from '@/api/client'
import type { PairingCode } from '@/api/types'

const { t, locale } = useI18n()
const route = useRoute()
const shown = ref<PairingCode | null>(null)
const busy = ref(false)
const error = ref('')

const until = computed(() =>
  shown.value ? new Intl.DateTimeFormat(locale.value, { timeStyle: 'short' }).format(new Date(shown.value.expiresAt)) : '',
)

async function show() {
  busy.value = true
  error.value = ''
  try {
    shown.value = await api.peer.code()
  } catch {
    error.value = t('boxes.error.generic')
  } finally {
    busy.value = false
  }
}

// Closing the section hides the code (it stays valid until it expires).
function onToggle(e: Event) {
  if (!(e.target as HTMLDetailsElement).open) shown.value = null
}
</script>
