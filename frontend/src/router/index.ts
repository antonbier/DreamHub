// Routes mirror the hub paths declared in backend/internal/hub/v1.go
// (Descriptor.Path) and the core routes in Navigation() (dashboard,
// settings). Every v1 hub now has a real screen. Diagnostics used to be its
// own core route; it's now a tab inside SystemHubView.vue, so /diagnostics
// redirects there instead of 404ing an old bookmark or link.
import type { Permission } from '@/api/types'
import { useAuth } from '@/composables/useAuth'
import { createRouter, createWebHistory } from 'vue-router'
import { isOnboardingComplete } from '@/lib/onboarding'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'dashboard',
      component: () => import('@/views/DashboardView.vue'),
    },
    {
      // /settings/<tab>, the tab ids of src/lib/settingsTabs.ts; plain
      // /settings opens the first one.
      path: '/settings/:tab?',
      name: 'settings',
      component: () => import('@/views/SettingsView.vue'),
    },
    {
      path: '/onboarding',
      name: 'onboarding',
      component: () => import('@/views/OnboardingView.vue'),
      meta: { perm: 'settings.global' },
    },
    {
      // MediaHub, GuideHub and SeriesHub have their own real screens
      // (ROADMAP.md, "v1 — hubs"); the rest still render the shared
      // placeholder below.
      path: '/media',
      name: 'media',
      component: () => import('@/views/MediaHubView.vue'),
    },
    {
      path: '/guide',
      name: 'guide',
      component: () => import('@/views/GuideHubView.vue'),
    },
    {
      // PlexHub, "Plex Mediathek": ?library= picks the library, ?item= opens
      // a series/season page or a film's details.
      path: '/plex',
      name: 'plex',
      component: () => import('@/views/PlexHubView.vue'),
    },
    {
      // StreamHub: live TV in the browser; ?ref= starts a channel,
      // ?recording=<path>&title= plays a recording.
      path: '/stream',
      name: 'stream',
      component: () => import('@/views/StreamHubView.vue'),
    },
    {
      path: '/radio',
      name: 'radio',
      component: () => import('@/views/RadioHubView.vue'),
    },
    {
      path: '/series',
      name: 'series',
      component: () => import('@/views/SeriesHubView.vue'),
    },
    {
      path: '/series/:id(\\d+)',
      name: 'series-detail',
      component: () => import('@/views/SeriesDetailView.vue'),
      props: (route) => ({ id: Number(route.params.id) }),
    },
    {
      path: '/discovery',
      name: 'discovery',
      component: () => import('@/views/DiscoveryHubView.vue'),
    },
    {
      path: '/timers',
      name: 'timers',
      component: () => import('@/views/TimerHubView.vue'),
    },
    {
      path: '/diagnostics',
      redirect: { path: '/system', query: { tab: 'diagnostics' } },
    },
    {
      path: '/system',
      name: 'system',
      component: () => import('@/views/SystemHubView.vue'),
      meta: { perm: 'system' },
    },
    {
      path: '/admin',
      name: 'admin',
      component: () => import('@/views/AdminHubView.vue'),
      meta: { perm: 'system' },
    },
    {
      path: '/channels',
      name: 'channels',
      component: () => import('@/views/ChannelHubView.vue'),
      meta: { perm: 'channels.manage' },
    },
    {
      path: '/control',
      name: 'control',
      component: () => import('@/views/ControlHubView.vue'),
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/views/NotFoundView.vue'),
    },
  ],
})

// First visit on this browser (CONCEPT.md, "Onboarding wizard: on first
// start"): send everything to /onboarding until it finishes. Re-running it
// later is a deliberate navigation from Settings, not this guard.
// A profile is known before the first page is chosen, so the guards below
// can ask what it may do (the server refuses anyway — this only keeps a
// viewer from landing on a page that can't show them anything).
router.beforeEach(async (to) => {
  const { authState, me, can, check } = useAuth()
  if (!authState.checked) await check()
  const signedIn = me.value !== null
  const need = to.meta.perm as Permission | undefined
  if (signedIn && need && !can(need)) return { path: '/' }
  // The wizard sets up the box for the household: only for those who may.
  if (signedIn && !can('settings.global')) return
  if (to.name !== 'onboarding' && to.name !== 'not-found' && !isOnboardingComplete()) {
    return { name: 'onboarding' }
  }
})

export default router
