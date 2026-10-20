<!--
  Copy the channel list from another box (POST /api/v1/channels/sync/plan,
  docs/MULTIDEVICE_PLAN.md M8): pick the box, see per bouquet what happens
  (created, brought in line, unchanged) and which channels this box doesn't
  know, then one confirm. Only offered with two or more boxes.
-->
<template>
  <details class="mt-6 rounded-sm border border-line bg-tile p-3">
    <summary class="cursor-pointer text-[14px] font-semibold">{{ t('channels.sync.title') }}</summary>
    <p class="mt-1 text-[13px] text-muted">{{ t('channels.sync.explain', { box: boxes.active?.name ?? '' }) }}</p>

    <form v-if="!result" class="mt-3 flex flex-wrap items-end gap-2" @submit.prevent="review">
      <label class="flex flex-col gap-0.5 text-[13px]">
        <span class="text-faint">{{ t('channels.sync.from') }}</span>
        <select v-model="from" class="rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text">
          <option v-for="b in boxes.others" :key="b.id" :value="b.id">{{ b.name }}</option>
        </select>
      </label>
      <button type="submit" class="rounded-sm bg-accent px-3 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-50" :disabled="busy || !from">
        {{ t('channels.edit.review') }}
      </button>
      <p v-if="error" class="w-full text-[13px] text-warn" role="alert">{{ error }}</p>
    </form>

    <PlanPreview
      v-else
      class="mt-3"
      :plan="result.plan"
      :confirm-label="t('channels.sync.confirm')"
      :busy="busy"
      :error="error"
      @confirm="apply"
      @cancel="reset"
    >
      <ul class="flex flex-col gap-1.5 text-[13.5px]">
        <li v-for="b in result.bouquets" :key="b.kind + b.name">
          <span class="font-semibold text-text">{{ b.name }}</span>
          <span class="text-muted"> — {{ t(`channels.sync.action.${b.action}`, { n: b.channels }) }}</span>
          <span v-if="b.missing?.length" class="block text-[12.5px] text-warn">{{ t('channels.sync.missing', { names: b.missing.join(', ') }, b.missing.length) }}</span>
        </li>
      </ul>
      <p class="mt-2 text-[12.5px] text-faint">{{ t('channels.edit.backupNote') }}</p>
    </PlanPreview>
  </details>
</template>

<script setup lang="ts">
import { ref, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { usePlanError } from '@/composables/usePlanError'
import { useBoxesStore } from '@/stores/boxes'
import PlanPreview from '@/components/PlanPreview.vue'
import type { ChannelSyncResult } from '@/api/types'

const emit = defineEmits<{ done: [] }>()
const { t } = useI18n()
const { planErrorMessage } = usePlanError()
const boxes = useBoxesStore()

const from = ref('')
const busy = ref(false)
const error = ref('')
const result = ref<ChannelSyncResult | null>(null)

watchEffect(() => {
  if (!boxes.others.some((b) => b.id === from.value)) from.value = boxes.others[0]?.id ?? ''
})

function reset() {
  result.value = null
  error.value = ''
}

async function review() {
  busy.value = true
  error.value = ''
  try {
    result.value = await api.channels.planSync(from.value)
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}

async function apply() {
  if (!result.value) return
  busy.value = true
  error.value = ''
  try {
    await api.plans.apply(result.value.plan.id)
    reset()
    emit('done')
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}
</script>
