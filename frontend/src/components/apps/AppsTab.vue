<!--
  AppsHub's "Apps" tab (docs/APPSHUB_PLAN.md, App2+): Docker container
  management, Unraid-style — not built yet. A plain placeholder rather
  than a fake status/empty-state machine for a backend that doesn't
  exist yet; becomes a real connector-style available/degraded/
  unavailable view once App1's backend lands.
-->
<template>
  <p class="mt-4 text-[13px] text-faint">{{ t('apps.comingSoon') }}</p>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>
