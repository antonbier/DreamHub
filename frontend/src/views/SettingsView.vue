<!--
  Placeholder for the central settings menu (CONCEPT.md, "Settings"): a
  full schema-driven renderer of GET /api/v1/settings is a separate,
  still-open Foundation item (ROADMAP.md). Language and appearance already
  work from the user menu in the meantime.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <h1 class="text-[26px] font-bold">{{ t('settingsPlaceholder.title') }}</h1>
    <p class="mt-2 max-w-[640px] text-[14px] text-muted">{{ t('settingsPlaceholder.body') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
</script>
