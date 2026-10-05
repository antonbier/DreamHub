<!--
  The body of a "mark a season/series as seen" preview: one sentence and
  every episode that changes, in monospace like the other previews.
-->
<template>
  <p>{{ t(watched ? 'plex.previewWatched' : 'plex.previewUnwatched', { count: plan.changes.length }, plan.changes.length) }}</p>
  <ul class="mt-2 flex max-h-64 flex-col gap-0.5 overflow-y-auto font-mono text-[12.5px]">
    <li v-for="(c, i) in plan.changes" :key="i" class="truncate">{{ titleOf(c.after) }}</li>
  </ul>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Plan } from '@/api/types'

const props = defineProps<{ plan: Plan }>()
const { t } = useI18n()

const watched = computed(() => (props.plan.changes[0]?.after as { watched?: boolean } | undefined)?.watched ?? true)
function titleOf(v: unknown): string {
  return (v as { title?: string } | undefined)?.title ?? ''
}
</script>
