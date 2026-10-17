<!--
  One nav entry for everything that can be added (docs/APPSHUB_PLAN.md,
  App10), in three tabs in a fixed order, each opening with one sentence —
  - Packages: the box's own DreamOS software (gear: where it comes from)
  - Extensions: small add-ons for DreamHub itself (docs/EXTENSIONHUB_PLAN.md)
  - Docker: optional extra apps in containers (gear: Docker settings)
  The page remembers the last tab per viewer; ?tab= still wins, and the
  old values (apps, system) lead to their new tab.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="extension" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ hubTitle('apps', 'hub.apps') }}</h1>
    </div>

    <p v-if="!admin" class="mt-4 text-[14px] text-muted">{{ t('apps.adminOnly') }}</p>
    <BoxNeedsDreamHub v-else-if="navigation.navigation && !navigation.isFull" class="mt-4" />
    <template v-else>
    <!-- One update notion for packages and containers (App14). -->
    <UpdatesSummary @changed="reloadKey++" />
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
    <!-- Each tab opens with one plain sentence (TabIntro), so the three
         kinds of "add-on" can't be mistaken for each other. -->
    <PackagesTab v-if="tab === 'packages'" :key="reloadKey" />
    <ExtensionsTab v-else-if="tab === 'extensions'" />
    <DockerTab v-else :key="reloadKey" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { useScrollMore } from '@/composables/useScrollMore'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import BoxNeedsDreamHub from '@/components/BoxNeedsDreamHub.vue'
import Icon from '@/components/Icon.vue'
import DockerTab from '@/components/apps/DockerTab.vue'
import ExtensionsTab from '@/components/apps/ExtensionsTab.vue'
import PackagesTab from '@/components/apps/PackagesTab.vue'
import UpdatesSummary from '@/components/apps/UpdatesSummary.vue'
import { useNavigationStore } from '@/stores/navigation'
import { useHubLabel } from '@/composables/useHubLabel'
import { useAuth } from '@/composables/useAuth'

const { t } = useI18n()
const tabBar = ref<HTMLElement | null>(null)
// Bumped after "Update all": the tabs load their lists again.
const reloadKey = ref(0)
const { more: tabsMore, measure: measureTabs } = useScrollMore(tabBar)
const { hubTitle } = useHubLabel()
const admin = useAuth().can('system')
const navigation = useNavigationStore()
const route = useRoute()
const router = useRouter()

type Tab = 'packages' | 'extensions' | 'docker'
const tabs: Tab[] = ['packages', 'extensions', 'docker']
const LAST_TAB_KEY = 'dreamhub.apps.tab'
// The tab ids before App10 (Apps, System) lead to where their content is now.
const oldTabs: Record<string, Tab> = { apps: 'docker', system: 'packages' }

function lastTab(): Tab | undefined {
  try {
    return tabs.find((x) => x === localStorage.getItem(LAST_TAB_KEY))
  } catch {
    return undefined
  }
}

function fromQuery(): Tab | undefined {
  const q = route.query.tab
  return typeof q === 'string' ? (tabs.find((x) => x === q) ?? oldTabs[q]) : undefined
}

const tab = ref<Tab>(fromQuery() ?? lastTab() ?? 'packages')
watch(
  tab,
  (t) => {
    try {
      localStorage.setItem(LAST_TAB_KEY, t)
    } catch {
      // private window or blocked storage: the page just doesn't remember
    }
    router.replace({ query: { ...route.query, tab: t === 'packages' ? undefined : t } })
  },
  { immediate: true },
)
</script>
