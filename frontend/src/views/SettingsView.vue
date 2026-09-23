<!--
  The central settings menu (CONCEPT.md, "Settings"): rendered straight from
  GET /api/v1/settings — every section and field the backend registers
  (general, auth, network, each connector, hubs) shows up here with no
  frontend-side list to keep in sync. See src/composables/useSettingsForm.ts.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <h1 class="mb-4 text-[26px] font-bold">{{ t('settings.page.title') }}</h1>

    <p v-if="form.loading.value" class="text-[14px] text-muted">{{ t('common.loading') }}</p>
    <div v-else-if="form.loadFailed.value" class="rounded border border-line bg-tile p-5">
      <p class="mb-3 text-[14px] text-muted">{{ t('settings.page.loadFailed') }}</p>
      <button type="button" class="text-[13.5px] text-accent hover:underline" @click="form.load()">
        {{ t('common.retry') }}
      </button>
    </div>
    <div v-else class="flex flex-col gap-4">
      <SettingsSectionCard v-for="section in form.sections.value" :key="section.id" :section="section" :form="form" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import SettingsSectionCard from '@/components/settings/SettingsSectionCard.vue'
import { useSettingsForm } from '@/composables/useSettingsForm'

const { t } = useI18n()
const form = useSettingsForm()
onMounted(() => form.load())
</script>
