<!--
  The preview for a trash action — restoring (a rename back) or deleting for
  good, MediaHub's only real delete (PRINCIPLES.md: "delete means move to
  trash"; emptying the trash is the explicit second step). Deleting names
  how much disk space comes back and uses the destructive confirm style.
-->
<template>
  <p v-if="state.busy && !state.plan" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
  <PlanPreview
    v-else-if="state.plan"
    :plan="state.plan"
    :confirm-label="state.kind === 'restore' ? t('media.confirmRestore') : t('media.confirmDelete')"
    :busy="state.busy"
    :error="state.error"
    :danger="state.kind === 'empty'"
    @confirm="emit('confirm')"
    @cancel="emit('cancel')"
  >
    <template v-if="state.kind === 'restore'">{{ t('media.restorePreview') }}</template>
    <template v-else>
      <p>{{ t('media.deletePreview', { count: state.paths.length, files: state.plan.changes.length, size: formatBytes(state.bytes ?? 0, locale) }, state.paths.length) }}</p>
    </template>
  </PlanPreview>
  <div v-else-if="state.error" class="flex items-center gap-3 text-[12.5px] text-warn">
    {{ state.error }}
    <button type="button" class="text-accent hover:underline" @click="emit('cancel')">{{ t('plan.close') }}</button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import PlanPreview from '@/components/PlanPreview.vue'
import { formatBytes } from '@/lib/format'
import type { TrashActionState } from './types'

defineProps<{ state: TrashActionState }>()
const emit = defineEmits<{ confirm: []; cancel: [] }>()
const { t, locale } = useI18n()
</script>
