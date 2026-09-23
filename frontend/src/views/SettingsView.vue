<!--
  The central settings menu (CONCEPT.md, "Settings"): rendered straight from
  GET /api/v1/settings — every section and field the backend registers
  (general, auth, network, each connector, hubs) shows up here with no
  frontend-side list to keep in sync. See src/composables/useSettingsForm.ts.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="mb-4 flex items-center">
      <h1 class="text-[26px] font-bold">{{ t('settings.page.title') }}</h1>
      <RouterLink to="/onboarding" class="ms-auto text-[13.5px] text-accent hover:underline">
        {{ t('settings.page.runWizard') }}
      </RouterLink>
    </div>

    <p v-if="form.loading.value" class="text-[14px] text-muted">{{ t('common.loading') }}</p>
    <div v-else-if="form.loadFailed.value" class="rounded border border-line bg-tile p-5">
      <p class="mb-3 text-[14px] text-muted">{{ t('settings.page.loadFailed') }}</p>
      <button type="button" class="text-[13.5px] text-accent hover:underline" @click="form.load()">
        {{ t('common.retry') }}
      </button>
    </div>
    <div v-else class="flex flex-col gap-4">
      <template v-for="section in form.sections.value" :key="section.id">
        <SettingsSectionCard :section="section" :form="form" />
        <!-- Not a schema field — tokens are a list of their own resources,
             not a single setting — so this is placed by hand right after
             Authentication rather than coming from the section loop. -->
        <ApiTokensPanel v-if="section.id === 'auth'" />
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import SettingsSectionCard from '@/components/settings/SettingsSectionCard.vue'
import ApiTokensPanel from '@/components/settings/ApiTokensPanel.vue'
import { useSettingsForm } from '@/composables/useSettingsForm'

const { t } = useI18n()
const form = useSettingsForm()
onMounted(() => form.load())
</script>
