<!--
  Dashboard — CONCEPT.md, "Dashboard (decided: tiles)". The grid itself
  follows DESIGN.md exactly (4 columns, 14px gap, the three breakpoints) so
  real tiles can drop in later without touching this layout. The dashboards
  API (read/save layout) isn't built yet — ROADMAP.md, Foundation, "Open" —
  so v1 shows a single empty-state tile explaining what's coming instead of
  fabricating placeholder data.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <h1 class="mb-4 text-[26px] font-bold">{{ t('dashboard.title') }}</h1>
    <div class="grid grid-cols-4 gap-[14px] below-lg:grid-cols-2 below-sm:grid-cols-1">
      <div class="col-span-4 rounded border border-line bg-tile p-6 below-lg:col-span-2 below-sm:col-span-1">
        <h2 class="mb-1.5 text-[15.5px] font-semibold">{{ t('dashboard.empty.title') }}</h2>
        <p class="max-w-[640px] text-[14px] text-muted">{{ t('dashboard.empty.body') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
</script>
