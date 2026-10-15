<!--
  Sidebar — DESIGN.md, "Components: Sidebar" and "Layout" breakpoints.
  Desktop: persistent, collapsible to icons-only (state remembered).
  Below 900px: becomes a drawer (no collapsed state), opened from
  AppTopBar's menu button, closed by the backdrop, Escape or a navigation.
  Content is entirely driven by GET /api/v1/navigation
  (backend/internal/hub), so adding or reordering a hub is a backend
  change, not a frontend one.
-->
<template>
  <!-- Backdrop: mobile drawer only. -->
  <div
    v-if="ui.mobileNavOpen"
    class="fixed inset-0 z-20 bg-black/50 below-md:block hidden"
    @click="ui.closeMobileNav()"
  />
  <aside
    ref="asideEl"
    :inert="narrow && !ui.mobileNavOpen"
    class="z-30 flex h-full shrink-0 flex-col border-e border-line bg-side transition-[width] duration-150 below-md:fixed below-md:inset-y-0 below-md:start-0 below-md:w-[260px] below-md:transition-transform below-md:duration-200 below-md:ease-out"
    :class="ui.mobileNavOpen ? 'below-md:translate-x-0' : 'below-md:-translate-x-full below-md:rtl:translate-x-full'"
    :style="{ width: collapsed ? '60px' : '232px' }"
  >
    <div class="flex items-center gap-2 p-3">
      <button
        type="button"
        class="flex h-8 w-8 shrink-0 items-center justify-center rounded text-muted hover:bg-tile-2 hover:text-text below-md:hidden"
        :title="collapsed ? t('sidebar.expand') : t('sidebar.collapse')"
        @click="toggle"
      >
        <Icon :name="collapsed ? 'chevron-right' : 'chevron-left'" :size="18" />
      </button>
      <span v-if="!collapsed" class="truncate text-[15.5px] font-bold">{{ t('app.name') }}</span>
      <button
        type="button"
        class="ms-auto hidden h-8 w-8 shrink-0 items-center justify-center rounded text-muted hover:bg-tile-2 hover:text-text below-md:flex"
        :aria-label="t('sidebar.collapse')"
        @click="ui.closeMobileNav()"
      >
        <Icon name="close" :size="18" />
      </button>
    </div>

    <nav class="flex flex-1 flex-col gap-4 overflow-y-auto px-2 pb-2">
      <div v-for="section in mainSections" :key="section.id">
        <div
          v-if="section.label && !collapsed"
          class="px-2 pb-1 text-[12.5px] text-faint"
        >
          {{ t(section.label) }}
        </div>
        <ul class="flex flex-col gap-0.5">
          <li v-for="item in section.items" :key="item.id">
            <NavLink :item="item" :collapsed="collapsed" @navigate="ui.closeMobileNav()" />
          </li>
        </ul>
      </div>
    </nav>

    <div v-if="bottomSection" class="border-t border-line-soft px-2 py-2">
      <ul class="flex flex-col gap-0.5">
        <li v-for="item in bottomSection.items" :key="item.id">
          <NavLink :item="item" :collapsed="collapsed" @navigate="ui.closeMobileNav()" />
        </li>
      </ul>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { useHubLabel } from '@/composables/useHubLabel'
import { useAuth } from '@/composables/useAuth'
import { computed, defineComponent, h, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
import { useProblem } from '@/composables/useProblem'
import { useUiStore } from '@/stores/ui'
import { useFocusTrap } from '@/composables/useFocusTrap'
import type { NavItem, NavSection } from '@/api/types'

const props = defineProps<{ sections: NavSection[] }>()
const { t } = useI18n()
const { hubLabel } = useHubLabel()
const ui = useUiStore()

// As a drawer (below 900 px) the sidebar is a modal while open — Tab stays
// in it, Escape closes it, focus moves in and back — and out of reach
// while closed: off screen, but without `inert` its links would still be
// Tab stops nobody can see.
const asideEl = ref<HTMLElement | null>(null)
const narrowQuery = window.matchMedia('(max-width: 900px)')
const narrow = ref(narrowQuery.matches)
const onNarrowChange = (e: MediaQueryListEvent) => (narrow.value = e.matches)
onMounted(() => narrowQuery.addEventListener('change', onNarrowChange))
onBeforeUnmount(() => narrowQuery.removeEventListener('change', onNarrowChange))
useFocusTrap(asideEl, { active: () => narrow.value && ui.mobileNavOpen, onEscape: () => ui.closeMobileNav() })
let drawerOpener: HTMLElement | null = null
watch(
  () => ui.mobileNavOpen,
  async (open) => {
    if (!narrow.value) return
    if (open) {
      drawerOpener = document.activeElement as HTMLElement | null
      await nextTick()
      // The first one actually shown (the collapse button is hidden here).
      Array.from(asideEl.value?.querySelectorAll<HTMLElement>('a[href], button') ?? []).find((el) => el.offsetParent !== null)?.focus()
    } else {
      drawerOpener?.focus()
    }
  },
)

// Collapsed-to-icons is a desktop-only affordance (DESIGN.md: "no
// collapsed state" below 900px) — the drawer's own open/closed state lives
// in the ui store instead, see the template above.
const STORAGE_KEY = 'dreamhub.sidebarCollapsed'
function readCollapsed(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === '1'
  } catch {
    return false // storage blocked: start expanded instead of failing the whole shell
  }
}
const collapsed = ref(readCollapsed())
function toggle() {
  collapsed.value = !collapsed.value
  try {
    localStorage.setItem(STORAGE_KEY, collapsed.value ? '1' : '0')
  } catch {
    // Private browsing or a blocked store — the toggle still works for
    // this session, it just won't be remembered next time.
  }
}

// A hub's setup state is the admin's business (docs/archive/PROFILES_PLAN.md, P3):
// a viewer doesn't see the dots, and a hub that can't work at all is left
// out instead of shown greyed.
const { can } = useAuth()
const admin = computed(() => can('settings.global'))
// Pages that only an admin can do anything on.
const adminOnly = new Set(['apps'])
function forProfile(s: NavSection): NavSection {
  if (admin.value) return s
  return {
    ...s,
    items: s.items.filter((i) => i.status !== 'unavailable' && !adminOnly.has(i.id)).map((i) => ({ ...i, status: undefined, problems: undefined })),
  }
}
const mainSections = computed(() => props.sections.filter((s) => s.id !== 'bottom').map(forProfile).filter((s) => s.items.length > 0))
const bottomSection = computed(() => {
  const s = props.sections.find((x) => x.id === 'bottom')
  return s ? forProfile(s) : undefined
})

// A hub stays highlighted on its own sub-pages too (/series/11 under
// /series); the dashboard at "/" only on exactly "/".
function isActive(current: string, itemPath: string): boolean {
  return itemPath === '/' ? current === '/' : current === itemPath || current.startsWith(itemPath + '/')
}

// One nav row: icon, label (or a tooltip when collapsed), active state and
// an unavailable/degraded indicator with a translated explanation.
const NavLink = defineComponent({
  props: { item: { type: Object as () => NavItem, required: true }, collapsed: { type: Boolean, required: true } },
  emits: ['navigate'],
  setup(props, { emit }) {
    const { t } = useI18n()
    const { describe, describeFix } = useProblem()
    const route = useRoute()

    const tooltip = computed(() => {
      const item = props.item
      if (!item.status || item.status === 'available') return props.collapsed ? hubLabel(item) : undefined
      const parts: string[] = []
      for (const p of item.problems ?? []) {
        parts.push(describe(p))
        const fix = describeFix(p)
        if (fix) parts.push(fix)
      }
      return (props.collapsed ? `${hubLabel(item)} — ` : '') + parts.join(' · ')
    })

    return () =>
      h(
        RouterLink,
        {
          to: props.item.path,
          title: tooltip.value,
          onClick: () => emit('navigate'),
          class: [
            'flex items-center gap-2.5 rounded px-2 py-[7px] text-[14.5px] hover:bg-tile-2',
            isActive(route.path, props.item.path) ? 'bg-tile-2 text-text' : 'text-muted',
            props.item.status === 'unavailable' ? 'opacity-60' : '',
          ],
        },
        () => [
          h(Icon, { name: props.item.icon, size: 18 }),
          !props.collapsed ? h('span', { class: 'truncate' }, hubLabel(props.item)) : null,
          props.item.status && props.item.status !== 'available'
            ? h('span', {
                class: [
                  'ms-auto h-1.5 w-1.5 shrink-0 rounded-full',
                  props.item.status === 'degraded' ? 'bg-warn' : 'bg-faint',
                ],
              })
            : null,
        ],
      )
  },
})
</script>
