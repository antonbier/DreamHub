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
    class="z-30 flex h-full shrink-0 flex-col border-e border-line bg-side transition-[width] duration-150 below-md:fixed below-md:inset-y-0 below-md:start-0 below-md:w-[260px] below-md:transition-transform below-md:duration-200 below-md:ease-out"
    :class="ui.mobileNavOpen ? 'below-md:translate-x-0' : 'below-md:-translate-x-full below-md:rtl:translate-x-full'"
    :style="{ width: collapsed ? '60px' : '232px' }"
    @keydown.esc="ui.closeMobileNav()"
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
import { computed, defineComponent, h, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
import { useProblem } from '@/composables/useProblem'
import { useUiStore } from '@/stores/ui'
import type { NavItem, NavSection } from '@/api/types'

const props = defineProps<{ sections: NavSection[] }>()
const { t } = useI18n()
const ui = useUiStore()

// Collapsed-to-icons is a desktop-only affordance (DESIGN.md: "no
// collapsed state" below 900px) — the drawer's own open/closed state lives
// in the ui store instead, see the template above.
const STORAGE_KEY = 'dreamhub.sidebarCollapsed'
const collapsed = ref(localStorage.getItem(STORAGE_KEY) === '1')
function toggle() {
  collapsed.value = !collapsed.value
  try {
    localStorage.setItem(STORAGE_KEY, collapsed.value ? '1' : '0')
  } catch {
    // Private browsing or a blocked store — the toggle still works for
    // this session, it just won't be remembered next time.
  }
}

const mainSections = computed(() => props.sections.filter((s) => s.id !== 'bottom'))
const bottomSection = computed(() => props.sections.find((s) => s.id === 'bottom'))

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
      if (!item.status || item.status === 'available') return props.collapsed ? t(item.label) : undefined
      const parts = [t(`status.${item.status}`)]
      for (const p of item.problems ?? []) {
        parts.push(describe(p))
        const fix = describeFix(p)
        if (fix) parts.push(fix)
      }
      return (props.collapsed ? `${t(item.label)} — ` : '') + parts.join(' · ')
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
          !props.collapsed ? h('span', { class: 'truncate' }, t(props.item.label)) : null,
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
