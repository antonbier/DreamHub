<!--
  Core shell (CONCEPT.md, "Navigation"): a persistent sidebar plus top bar
  around the routed hub views. Loads user preferences and the sidebar
  contents once at startup; both come straight from the backend, so this is
  where "the box is unreachable" first becomes visible.

  Gated by auth (src/composables/useAuth.ts): while whoami hasn't answered
  yet, nothing renders; once it has, either LoginScreen or the shell does.
-->
<template>
  <div v-if="!authState.checked" class="h-screen w-screen bg-bg" />
  <LoginScreen v-else-if="!authState.authenticated" @success="onAuthenticated" />
  <div v-else class="flex h-screen w-screen overflow-hidden bg-bg text-text">
    <AppSidebar :sections="navigation.navigation?.sections ?? fallbackSections" />
    <div class="flex min-w-0 flex-1 flex-col">
      <AppTopBar />
      <StatusStrip />
      <main class="flex-1 overflow-y-auto">
        <RouterView />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { RouterView } from 'vue-router'
import AppSidebar from './components/AppSidebar.vue'
import AppTopBar from './components/AppTopBar.vue'
import StatusStrip from './components/StatusStrip.vue'
import LoginScreen from './components/LoginScreen.vue'
import { usePreferencesStore } from './stores/preferences'
import { useNavigationStore } from './stores/navigation'
import { useAuth } from './composables/useAuth'
import { useGlobalShortcuts } from './composables/useGlobalShortcuts'
import type { NavSection } from './api/types'

const preferences = usePreferencesStore()
const navigation = useNavigationStore()
const { authState, check } = useAuth()
useGlobalShortcuts()

// Shown only while the backend has never answered yet, so the sidebar
// isn't just empty — Dashboard and Settings always exist.
const fallbackSections: NavSection[] = [
  { id: 'main', items: [{ id: 'dashboard', kind: 'core', label: 'nav.dashboard', icon: 'dashboard', path: '/' }] },
  { id: 'bottom', items: [{ id: 'settings', kind: 'core', label: 'nav.settings', icon: 'settings', path: '/settings' }] },
]

function loadShellData() {
  void preferences.load()
  void navigation.load()
}

onMounted(async () => {
  await check()
  if (authState.authenticated) loadShellData()
})

function onAuthenticated() {
  loadShellData()
}

// A 401 on some later request (e.g. the admin changed the password
// elsewhere) flips authState.authenticated back to false — client.ts does
// that directly since it's the one that sees the 401. Nothing else to do
// here; the template above reacts on its own.

// The backend picks descriptive-vs-hub labels per navLabels, so re-fetch
// the sidebar whenever that preference changes (skip the initial load).
watch(
  () => preferences.navLabels,
  () => {
    if (preferences.loaded) void navigation.load()
  },
)
</script>
