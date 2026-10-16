// Routes mirror the hub paths declared in backend/internal/hub/v1.go
// (Descriptor.Path) and the core routes in Navigation() (dashboard,
// settings). Every v1 hub now has a real screen. Diagnostics used to be its
// own core route; it's now a tab inside DeviceHubView.vue, so /diagnostics
// redirects there instead of 404ing an old bookmark or link. SystemHub and
// AdminHub merged into DeviceHub (docs/ADMINHUB_PLAN.md, #47); /system and
// /admin redirect to /device (with the matching tab) for the same reason.
import type { Permission } from '@/api/types'
import { useAuth } from '@/composables/useAuth'
import { createRouter, createWebHistory } from 'vue-router'
import { isSetupDone } from '@/lib/onboarding'
import { routerBase } from '@/lib/box'

const router = createRouter({
  // /b/<id> when the page is about another box than the first (lib/box.ts).
  history: createWebHistory(routerBase),
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
      // Nothing to render: App.vue shows the password screen or the
      // profile picker instead of the shell while nobody is signed in, and
      // this path is where the address bar says so.
      path: '/login',
      name: 'login',
      component: { render: () => null },
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
      redirect: { path: '/device', query: { tab: 'diagnostics' } },
    },
    {
      path: '/system',
      redirect: (to) => ({ path: '/device', query: to.query }),
    },
    {
      path: '/admin',
      redirect: { path: '/device', query: { tab: 'admin' } },
    },
    {
      path: '/device',
      name: 'device',
      component: () => import('@/views/DeviceHubView.vue'),
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
      // AppsHub renamed+widened Extension Hub (docs/APPSHUB_PLAN.md,
      // App1) — an old /extensions bookmark/link still lands on the
      // same content, now the hub's default tab.
      path: '/extensions',
      redirect: { path: '/apps' },
    },
    {
      path: '/apps',
      name: 'apps',
      component: () => import('@/views/AppsHubView.vue'),
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/views/NotFoundView.vue'),
    },
  ],
})

// Signed-out visitors (password screen, profile picker) live at /login and
// come back to where they were headed. The setup wizard is offered to an
// admin only while the box has no configuration yet (lib/onboarding.ts);
// re-running it later is a deliberate navigation from Settings.
// A profile is known before the first page is chosen, so the guards below
// can ask what it may do (the server refuses anyway — this only keeps a
// viewer from landing on a page that can't show them anything).
router.beforeEach(async (to) => {
  const { authState, me, can, check } = useAuth()
  if (!authState.checked) await check()
  if (me.value === null) {
    if (to.name === 'login') return
    return { name: 'login', query: to.fullPath === '/' ? {} : { redirect: to.fullPath } }
  }
  if (to.name === 'login') {
    const target = typeof to.query.redirect === 'string' && to.query.redirect.startsWith('/') ? to.query.redirect : '/'
    return target === '/login' ? { path: '/' } : { path: target }
  }
  const need = to.meta.perm as Permission | undefined
  if (need && !can(need)) return { path: '/' }
  // The wizard sets up the box for the household: only for those who may.
  if (!can('settings.global')) {
    if (to.name === 'onboarding') return { path: '/' }
    return
  }
  if (to.name !== 'onboarding' && to.name !== 'not-found' && !(await isSetupDone())) {
    return { name: 'onboarding' }
  }
})

export default router
