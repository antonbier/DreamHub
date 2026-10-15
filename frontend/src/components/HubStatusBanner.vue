<!--
  What is wrong with a page, in plain sentences: no "Degraded" / "Unavailable"
  label, just the reasons (and what to do) the backend reported. Shown by
  every page above its content when its sidebar entry isn't fine.
-->
<template>
  <div v-if="item && item.status && item.status !== 'available'" class="mt-4 flex gap-2.5 rounded border border-line bg-tile p-4">
    <span class="mt-[7px] h-1.5 w-1.5 shrink-0 rounded-full" :class="item.status === 'degraded' ? 'bg-warn' : 'bg-faint'" />
    <ul class="flex flex-col gap-1 text-[13.5px] text-muted">
      <li v-for="(p, i) in item.problems ?? []" :key="i">
        {{ describe(p) }}
        <span v-if="describeFix(p)" class="text-faint">{{ ' — ' + describeFix(p) }}</span>
      </li>
      <li v-if="!item.problems?.length">{{ t('status.noDetail') }}</li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useProblem } from '@/composables/useProblem'
import type { NavItem } from '@/api/types'

defineProps<{ item?: NavItem | null }>()
const { t } = useI18n()
const { describe, describeFix } = useProblem()
</script>
