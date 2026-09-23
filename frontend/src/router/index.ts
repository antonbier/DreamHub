// Routes mirror the hub paths declared in backend/internal/hub/v1.go
// (Descriptor.Path) and the core routes in Navigation() (dashboard,
// settings). Each hub route renders HubView, a placeholder until that
// hub's own screens are built (ROADMAP.md, "v1 — hubs") — the plumbing
// behind it (navigation, status, settings) is already live.
import { createRouter, createWebHistory } from 'vue-router'

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
      // MediaHub, SeriesHub, GuideHub, TimerHub, DiscoveryHub, SystemHub,
      // AdminHub — one placeholder route per hub path.
      path: '/:hub(media|series|guide|timers|discovery|system|admin)',
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

export default router
