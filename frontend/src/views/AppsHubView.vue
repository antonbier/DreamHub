<!--
  AppsHub (docs/APPSHUB_PLAN.md, App1): renamed and widened from Extension
  Hub — one nav entry, growing by zero entries no matter how many
  extensions or apps are installed. Two tabs instead of one view:
  - Extensions: every registered extension's own card (ex-ExtensionsHubView,
    docs/EXTENSIONHUB_PLAN.md, E1) — unchanged, just moved here.
  - Apps: Docker container management, Unraid-style — not built yet
    (docs/APPSHUB_PLAN.md, App2+), a plain placeholder for now.
  Same one-entry-many-tabs pattern DeviceHubView.vue already uses.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="extension" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ hubTitle('apps', 'hub.apps') }}</h1>
    </div>

    <div class="mt-4 flex gap-1 border-b border-line-soft" role="tablist">
      <button
        type="button"
        role="tab"
        :aria-selected="tab === 'extensions'"
        class="border-b-2 px-3 py-2 text-[13.5px] font-semibold"
        :class="tab === 'extensions' ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
        @click="tab = 'extensions'"
      >
        {{ t('apps.tabs.extensions') }}
      </button>
      <button
        type="button"
        role="tab"
        :aria-selected="tab === 'apps'"
        class="border-b-2 px-3 py-2 text-[13.5px] font-semibold"
        :class="tab === 'apps' ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
        @click="tab = 'apps'"
      >
        {{ t('apps.tabs.apps') }}
      </button>
    </div>

    <ExtensionsTab v-if="tab === 'extensions'" />
    <AppsTab v-else />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import Icon from '@/components/Icon.vue'
import AppsTab from '@/components/apps/AppsTab.vue'
import ExtensionsTab from '@/components/apps/ExtensionsTab.vue'
import { useHubLabel } from '@/composables/useHubLabel'

const { t } = useI18n()
const { hubTitle } = useHubLabel()
const route = useRoute()
const router = useRouter()

type Tab = 'extensions' | 'apps'
const tab = ref<Tab>(route.query.tab === 'apps' ? 'apps' : 'extensions')
watch(tab, (t) => router.replace({ query: { ...route.query, tab: t === 'extensions' ? undefined : t } }))
</script>
