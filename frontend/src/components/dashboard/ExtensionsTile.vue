<!--
  Dashboard tile: what the installed extensions chose to show here — the
  widgets they flag with "tile" (docs/EXTENSIONHUB_PLAN.md, schema 3). One
  small block per extension; an extension that flags nothing has no block.
  Read-only: switching stays on the Apps page, with its preview.
-->
<template>
  <p v-if="cards.length === 0" class="text-[13px] text-faint">{{ t('dashboard.extensionsTile.empty') }}</p>
  <div v-else class="flex flex-col gap-4">
    <section v-for="card in cards" :key="card.id">
      <h3 class="mb-1.5 text-[12.5px] font-semibold uppercase tracking-wide text-faint">{{ card.name }}</h3>
      <div class="flex flex-col gap-2">
        <template v-for="(w, i) in card.widgets" :key="i">
          <div v-if="w.type === 'status'" class="flex items-center gap-2 text-[13.5px]">
            <span class="h-2 w-2 shrink-0 rounded-full" :class="toneDot(w.tone)" />
            <span class="text-muted">{{ w.label }}</span>
            <span class="ms-auto font-medium text-text">{{ w.value }}</span>
          </div>
          <div v-else-if="w.type === 'meter'">
            <div class="flex items-baseline justify-between text-[12.5px] text-muted">
              <span>{{ w.label }}</span>
              <span>{{ w.current }}{{ w.unit }} / {{ w.max }}{{ w.unit }}</span>
            </div>
            <div class="mt-1 h-1.5 overflow-hidden rounded-full bg-tile-2">
              <div class="h-full rounded-full bg-accent" :style="{ width: meterPercent(w) + '%' }" />
            </div>
          </div>
          <div v-else-if="w.type === 'list'">
            <p class="mb-0.5 text-[12px] text-faint">{{ w.label }}</p>
            <ul class="flex flex-col gap-1">
              <li v-for="(row, j) in w.rows" :key="j" class="flex items-center gap-2 text-[13px]">
                <span class="text-muted">{{ row.label }}</span>
                <span class="ms-auto text-text">{{ row.value }}</span>
              </li>
            </ul>
          </div>
        </template>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { ExtensionCard, ExtensionWidget } from '@/api/types'

defineProps<{ cards: ExtensionCard[] }>()
const { t } = useI18n()

function toneDot(tone?: string) {
  if (tone === 'good') return 'bg-ok'
  if (tone === 'bad') return 'bg-onair'
  return 'bg-faint'
}

function meterPercent(w: ExtensionWidget): number {
  if (!w.max) return 0
  return Math.max(0, Math.min(100, (100 * (w.current ?? 0)) / w.max))
}
</script>
