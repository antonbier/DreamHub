<!--
  A failure in everyday words, with the technical part behind "Details"
  (src/composables/useFriendlyError.ts). Without an error, it describes the
  last failed request — for views that only keep a "failed" flag. Inline:
  it sits inside the view's own paragraph and takes its style.
-->
<template>
  <span role="status">
    {{ shown.text }}
    <details v-if="shown.details" class="mt-1 inline-block w-full text-[12px] text-faint">
      <summary class="cursor-pointer select-none">{{ t('error.friendly.details') }}</summary>
      <span class="font-mono">{{ shown.details }}</span>
    </details>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useFriendlyError } from '@/composables/useFriendlyError'

const props = defineProps<{ error?: unknown }>()
const { t } = useI18n()
const { friendly, lastFriendly } = useFriendlyError()
// Taken once when shown: a later failure elsewhere mustn't rewrite it.
const snapshot = props.error === undefined ? lastFriendly() : null
const shown = computed(() => (props.error === undefined ? snapshot! : friendly(props.error)))
</script>
