<!--
  Core shell (CONCEPT.md, "Navigation"): a persistent sidebar plus top bar
  around the routed hub views. Loads user preferences and the sidebar
  contents once at startup; both come straight from the backend, so this is
  where "the box is unreachable" first becomes visible.

  Gated by auth (src/composables/useAuth.ts): while whoami hasn't answered
  yet, nothing renders; once it has, either UnreachableScreen (no answer at
  all), LoginScreen (a 401) or the shell does.
-->
<template>
  <div v-if="!authState.checked" class="h-screen w-screen bg-bg" />
  <UnreachableScreen v-else-if="authState.unreachable" :check="check" @retry="onRetried" />
  <LoginScreen v-else-if="!authState.authenticated" @success="onAuthenticated" />
  <div v-else class="flex h-screen w-screen overflow-hidden bg-bg text-text">
    <!-- First stop for keyboard users: past sidebar and top bar. -->
    <a
      href="#main"
      class="sr-only focus:not-sr-only focus:fixed focus:start-3 focus:top-3 focus:z-50 focus:rounded focus:bg-accent focus:px-3 focus:py-2 focus:text-[13.5px] focus:font-semibold focus:text-bg"
      @click.prevent="focusMain"
    >
      {{ t('app.skipToContent') }}
    </a>
    <AppSidebar :sections="navigation.navigation?.sections ?? fallbackSections" />
    <div class="flex min-w-0 flex-1 flex-col">
      <AppTopBar />
      <StatusStrip />
      <main id="main" ref="mainEl" tabindex="-1" class="flex-1 overflow-y-auto focus:outline-none">
        <RouterView />
      </main>
    </div>
    <RemotePanel />
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppSidebar from './components/AppSidebar.vue'
import AppTopBar from './components/AppTopBar.vue'
import StatusStrip from './components/StatusStrip.vue'
import LoginScreen from './components/LoginScreen.vue'
import UnreachableScreen from './components/UnreachableScreen.vue'
import RemotePanel from './components/control/RemotePanel.vue'
import { usePreferencesStore } from './stores/preferences'
import { useNavigationStore } from './stores/navigation'
import { useNotificationsStore } from './stores/notifications'
import { useAuth } from './composables/useAuth'
import { useGlobalShortcuts } from './composables/useGlobalShortcuts'
import type { NavSection } from './api/types'

const preferences = usePreferencesStore()
const navigation = useNavigationStore()
const notifications = useNotificationsStore()
const { authState, check } = useAuth()
useGlobalShortcuts()
const { t } = useI18n()

// The skip link and page changes put the keyboard (and a screen reader) at
// the start of the page's content, not back at the top of the sidebar —
// without scrolling, so a deep link like /settings#push keeps its place.
const mainEl = ref<HTMLElement | null>(null)
function focusMain() {
  mainEl.value?.focus({ preventScroll: true })
}
const route = useRoute()
const router = useRouter()
// Not on the first page load: the skip link should be the first Tab stop.
let firstPageShown = false
void router.isReady().then(() => {
  firstPageShown = true
})
watch(
  () => route.path,
  async (path, before) => {
    if (!firstPageShown || before === undefined || path === before) return
    await nextTick()
    focusMain()
  },
)

// Shown only while the backend has never answered yet, so the sidebar
// isn't just empty — Dashboard and Settings always exist.
const fallbackSections: NavSection[] = [
  { id: 'main', items: [{ id: 'dashboard', kind: 'core', label: 'nav.dashboard', icon: 'dashboard', path: '/' }] },
  { id: 'bottom', items: [{ id: 'settings', kind: 'core', label: 'nav.settings', icon: 'settings', path: '/settings' }] },
]

function loadShellData() {
  void preferences.load()
  void navigation.load()
  void notifications.load()
  notifications.connect()
}

onMounted(async () => {
  await check()
  if (authState.authenticated) loadShellData()
})

function onAuthenticated() {
  loadShellData()
}

function onRetried() {
  if (authState.authenticated) loadShellData()
}

// A 401 on some later request (e.g. the admin changed the password
// elsewhere) flips authState.authenticated back to false — client.ts does
// that directly since it's the one that sees the 401. Nothing else to do
// here for the shell itself; the template above reacts on its own. The
// event stream does need telling explicitly, though — it would otherwise
// keep quietly retrying a stream it no longer has credentials for.
watch(
  () => authState.authenticated,
  (authenticated) => {
    if (!authenticated) notifications.disconnect()
  },
)

// The backend picks descriptive-vs-hub labels per navLabels, so re-fetch
// the sidebar whenever that preference changes (skip the initial load).
watch(
  () => preferences.navLabels,
  () => {
    if (preferences.loaded) void navigation.load()
  },
)
</script>
