// Routes mirror the hub paths declared in backend/internal/hub/v1.go
// (Descriptor.Path) and the core routes in Navigation() (dashboard,
// settings). Most hub routes render HubView, a placeholder until that
// hub's own screens are built (ROADMAP.md, "v1 — hubs") — the plumbing
// behind it (navigation, status, settings) is already live. MediaHub,
// GuideHub, SeriesHub and DiscoveryHub have real screens so far.
// /diagnostics is a core route, not a hub — see Navigation() in
// backend/internal/hub/hub.go.
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
      path: '/settings',
      name: 'settings',
      component: () => import('@/views/SettingsView.vue'),
    },
    {
      path: '/onboarding',
      name: 'onboarding',
      component: () => import('@/views/OnboardingView.vue'),
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
      path: '/series',
      name: 'series',
      component: () => import('@/views/SeriesHubView.vue'),
    },
    {
      path: '/discovery',
      name: 'discovery',
      component: () => import('@/views/DiscoveryHubView.vue'),
    },
    {
      path: '/diagnostics',
      name: 'diagnostics',
      component: () => import('@/views/DiagnosticsView.vue'),
    },
    {
      // TimerHub, SystemHub, AdminHub — one placeholder route per hub path.
      path: '/:hub(timers|system|admin)',
      name: 'hub',
      component: () => import('@/views/HubView.vue'),
      props: true,
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
router.beforeEach((to) => {
  if (to.name !== 'onboarding' && to.name !== 'not-found' && !isOnboardingComplete()) {
    return { name: 'onboarding' }
  }
})

export default router
