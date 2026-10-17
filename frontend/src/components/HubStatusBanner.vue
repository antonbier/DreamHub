<!--
  What is wrong with a page, in plain sentences: no "Degraded" / "Unavailable"
  label, just the reasons (and what to do) the backend reported. Shown by
  every page above its content when its sidebar entry isn't fine.
-->
<template>
  <div v-if="item && item.status && item.status !== 'available' && !onlyOptional" class="mt-4 flex gap-2.5 rounded-sm border border-line bg-tile p-4">
    <span class="mt-[7px] h-1.5 w-1.5 shrink-0 rounded-full" :class="item.status === 'degraded' ? 'bg-warn' : 'bg-faint'" />
    <ul class="flex flex-col gap-1 text-[13.5px] text-muted">
      <li v-for="(p, i) in shown" :key="i">
        {{ describe(p) }}
        <span v-if="describeFix(p)" class="text-faint">{{ ' — ' + describeFix(p) }}</span>
      </li>
      <li v-if="!item.problems?.length">{{ t('status.noDetail') }}</li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useProblem } from '@/composables/useProblem'
import { needsAttention, realProblems } from '@/lib/attention'
import type { NavItem } from '@/api/types'

const props = defineProps<{ item?: NavItem | null }>()
const { t } = useI18n()
const { describe, describeFix } = useProblem()

// An optional extra that is not set up (the film database) is no reason to
// hold up a page that works without it; it is said once, on the dashboard.
// A page that cannot work without a connector that is not set up (Plex)
// still says why.
const onlyOptional = computed(() => props.item?.status === 'degraded' && !needsAttention(props.item.status, props.item.problems))
const shown = computed(() => (props.item?.status === 'unavailable' ? (props.item.problems ?? []) : realProblems(props.item?.problems)))
</script>
