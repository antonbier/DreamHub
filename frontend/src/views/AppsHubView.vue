<!--
  AppsHub (docs/APPSHUB_PLAN.md): one nav entry for everything that can be
  added, in three tabs with one sentence each —
  - Apps: programs in their own Docker containers (Nextcloud, Pi-hole …)
  - Extensions: small add-ons for DreamHub itself (docs/EXTENSIONHUB_PLAN.md)
  - System: the box's own DreamOS packages and where they come from
    (moved here from DeviceHub → Administration, docs/ADMINHUB_PLAN.md)
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="extension" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ hubTitle('apps', 'hub.apps') }}</h1>
    </div>

    <p v-if="!admin" class="mt-4 text-[14px] text-muted">{{ t('apps.adminOnly') }}</p>
    <template v-else>
    <div
      ref="tabBar"
      class="mt-4 flex gap-1 overflow-x-auto border-b border-line-soft"
      :class="{ 'scroll-fade': tabsMore }"
      role="tablist"
      @scroll.passive="measureTabs"
    >
      <button
        v-for="id in tabs"
        :key="id"
        type="button"
        role="tab"
        :aria-selected="tab === id"
        class="shrink-0 whitespace-nowrap border-b-2 px-3 py-2.5 text-[13.5px] font-semibold"
        :class="tab === id ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
        @click="tab = id"
      >
        {{ t(`apps.tabs.${id}`) }}
      </button>
    </div>
    <!-- One plain sentence per tab: what this is, so the three kinds of
         "add-on" can't be mistaken for each other. -->
    <p class="mt-3 text-[13px] text-muted">{{ t(`apps.intro.${tab}`) }}</p>

    <AppsTab v-if="tab === 'apps'" />
    <ExtensionsTab v-else-if="tab === 'extensions'" />
    <SystemTab v-else />
    </template>
  </div>
</template>

<script setup lang="ts">
import { useScrollMore } from '@/composables/useScrollMore'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import Icon from '@/components/Icon.vue'
import AppsTab from '@/components/apps/AppsTab.vue'
import ExtensionsTab from '@/components/apps/ExtensionsTab.vue'
import SystemTab from '@/components/apps/SystemTab.vue'
import { useHubLabel } from '@/composables/useHubLabel'
import { useAuth } from '@/composables/useAuth'

const { t } = useI18n()
const tabBar = ref<HTMLElement | null>(null)
const { more: tabsMore, measure: measureTabs } = useScrollMore(tabBar)
const { hubTitle } = useHubLabel()
const admin = useAuth().can('system')
const route = useRoute()
const router = useRouter()

type Tab = 'apps' | 'extensions' | 'system'
const tabs: Tab[] = ['apps', 'extensions', 'system']
const tab = ref<Tab>(tabs.find((x) => x === route.query.tab) ?? 'apps')
watch(tab, (t) => router.replace({ query: { ...route.query, tab: t === 'apps' ? undefined : t } }))
</script>
