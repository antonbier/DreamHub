<!--
  The inline preview every mutating action shows before anything happens
  (PRINCIPLES.md, "Preview and confirm"; DESIGN.md, "Preview dialog"): an
  eyebrow saying nothing has changed yet, the caller's own summary of the
  change (default slot), the plan's warnings in `warn`, then exactly one
  primary (confirm) and one secondary (cancel) action. Inline rather than a
  modal, like DiscoveryHub's recommendation cards — the row it belongs to
  already has the room.
-->
<template>
  <div class="rounded-sm border border-line-soft bg-bg p-3" role="group" :aria-label="t('plan.eyebrow')">
    <p class="mb-1.5 text-[12px] text-faint">{{ t('plan.eyebrow') }}</p>
    <div class="text-[13.5px] text-text"><slot /></div>
    <ul v-if="plan.warnings.length > 0" class="mt-2 flex flex-col gap-1 text-[12.5px] text-warn">
      <li v-for="(w, i) in plan.warnings" :key="i">{{ describe(w) }}</li>
    </ul>
    <p v-if="error" class="mt-2 text-[12.5px] text-warn">{{ error }}</p>
    <div class="mt-3 flex justify-end gap-2">
      <button
        type="button"
        class="rounded-sm px-3 py-1.5 text-[13px] text-text hover:bg-field"
        :disabled="busy"
        @click="emit('cancel')"
      >
        {{ t('plan.cancel') }}
      </button>
      <button
        ref="confirmButton"
        type="button"
        class="rounded-sm px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60"
        :class="danger ? 'bg-onair' : 'bg-accent'"
        :disabled="busy"
        @click="emit('confirm')"
      >
        {{ busy ? t('plan.applying') : confirmLabel }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useProblem } from '@/composables/useProblem'
import type { Plan } from '@/api/types'

defineProps<{
  plan: Plan
  confirmLabel: string
  busy?: boolean
  error?: string
  // A destructive change (deleting): the confirm button uses `onair`.
  danger?: boolean
}>()
const emit = defineEmits<{ confirm: []; cancel: [] }>()

const { t } = useI18n()
const { describe } = useProblem()

// DESIGN.md: dialogs focus their primary action on open.
const confirmButton = ref<HTMLButtonElement | null>(null)
onMounted(() => confirmButton.value?.focus())
</script>
