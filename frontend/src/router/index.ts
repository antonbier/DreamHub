// Routes mirror the hub paths declared in backend/internal/hub/v1.go
// (Descriptor.Path) and the core routes in Navigation() (dashboard,
// settings). Every v1 hub now has a real screen. Diagnostics used to be its
// own core route; it's now a tab inside SystemHubView.vue, so /diagnostics
// redirects there instead of 404ing an old bookmark or link.
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
    },
    {
      path: '/admin',
      name: 'admin',
      component: () => import('@/views/AdminHubView.vue'),
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
