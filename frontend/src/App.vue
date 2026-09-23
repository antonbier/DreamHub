<!--
  Core shell (CONCEPT.md, "Navigation"): a persistent sidebar plus top bar
  around the routed hub views. Loads user preferences and the sidebar
  contents once at startup; both come straight from the backend, so this is
  where "the box is unreachable" first becomes visible.
-->
<template>
  <div class="flex h-screen w-screen overflow-hidden bg-bg text-text">
    <AppSidebar :sections="navigation.navigation?.sections ?? fallbackSections" />
    <div class="flex min-w-0 flex-1 flex-col">
      <AppTopBar />
      <div
        v-if="!preferences.reachable || !navigation.reachable"
        class="flex items-center gap-2 border-b border-line bg-field px-4 py-1.5 text-[13px] text-warn"
      >
        <Icon name="warning" :size="14" />
        {{ t('common.backendUnreachable') }}
      </div>
      <main class="flex-1 overflow-y-auto">
        <RouterView />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { RouterView } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppSidebar from './components/AppSidebar.vue'
import AppTopBar from './components/AppTopBar.vue'
import Icon from './components/Icon.vue'
import { usePreferencesStore } from './stores/preferences'
import { useNavigationStore } from './stores/navigation'
import type { NavSection } from './api/types'

const { t } = useI18n()
const preferences = usePreferencesStore()
const navigation = useNavigationStore()

// Shown only while the backend has never answered yet, so the sidebar
// isn't just empty — Dashboard and Settings always exist.
const fallbackSections: NavSection[] = [
  { id: 'main', items: [{ id: 'dashboard', kind: 'core', label: 'nav.dashboard', icon: 'dashboard', path: '/' }] },
  { id: 'bottom', items: [{ id: 'settings', kind: 'core', label: 'nav.settings', icon: 'settings', path: '/settings' }] },
]

onMounted(() => {
  void preferences.load()
  void navigation.load()
})

// The backend picks descriptive-vs-hub labels per navLabels, so re-fetch
// the sidebar whenever that preference changes (skip the initial load).
watch(
  () => preferences.navLabels,
  () => {
    if (preferences.loaded) void navigation.load()
  },
)
</script>
