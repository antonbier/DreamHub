<!--
  Top bar — DESIGN.md, "Components: Top bar". Search opens the global
  command palette (SearchPalette.vue); notifications are the current-issues
  inbox (NotificationsBell.vue) — see those files for what's real vs. still
  open. Language and appearance are fully wired to GET/PATCH /api/v1/settings.
-->
<template>
  <header class="flex h-14 shrink-0 items-center gap-3 border-b border-line bg-bg px-4">
    <button
      type="button"
      class="hidden h-8 w-8 shrink-0 items-center justify-center rounded text-muted hover:bg-tile-2 hover:text-text below-md:flex"
      :aria-label="t('sidebar.expand')"
      @click="ui.openMobileNav()"
    >
      <Icon name="menu" :size="18" />
    </button>
    <!-- On phones the search field shrinks to an icon: the placeholder would
         otherwise wrap into three lines and spill over the status strip. -->
    <div class="flex flex-1 justify-center below-sm:justify-end">
      <button
        type="button"
        class="relative w-full max-w-[420px] truncate rounded border border-line bg-field py-1.5 ps-8 pe-3 text-start text-[13.5px] text-faint hover:border-line-soft below-sm:flex below-sm:h-8 below-sm:w-8 below-sm:items-center below-sm:justify-center below-sm:border-0 below-sm:bg-transparent below-sm:p-0 below-sm:text-muted below-sm:hover:bg-tile-2 below-sm:hover:text-text"
        :aria-label="t('topbar.searchPlaceholder')"
        @click="searchPalette?.show()"
      >
        <Icon name="search" :size="16" class="pointer-events-none absolute start-2.5 top-1/2 -translate-y-1/2 text-faint below-sm:static below-sm:translate-y-0 below-sm:text-current" />
        <span class="below-sm:hidden">{{ t('topbar.searchPlaceholder') }}</span>
        <span class="absolute end-2.5 top-1/2 -translate-y-1/2 text-[11px] below-sm:hidden">{{ shortcutHint }}</span>
      </button>
      <SearchPalette ref="searchPalette" />
    </div>

    <button
      v-if="controlAvailable && route.path !== '/control'"
      id="remote-panel-toggle"
      type="button"
      class="flex h-8 w-8 items-center justify-center rounded text-muted hover:bg-tile-2 hover:text-text"
      :class="{ 'bg-tile-2 text-text': remote.panelOpen }"
      :aria-label="t('control.panel.toggle')"
      :title="t('control.panel.toggle')"
      :aria-expanded="remote.panelOpen"
      aria-controls="remote-panel"
      @click="remote.togglePanel()"
    >
      <Icon name="control" :size="18" />
    </button>

    <NotificationsBell />

    <DropdownMenu>
      <template #trigger="{ open }">
        <button
          type="button"
          :aria-label="t('topbar.language')"
          class="flex h-8 items-center gap-1.5 rounded px-2 text-[13px] text-muted hover:bg-tile-2"
          :class="{ 'bg-tile-2': open }"
        >
          <Icon name="globe" :size="17" />
          <span class="font-semibold uppercase">{{ preferences.language }}</span>
        </button>
      </template>
      <template #panel="{ close }">
        <button
          v-for="locale in SUPPORTED_LOCALES"
          :key="locale"
          type="button"
          class="flex w-full items-center justify-between px-3 py-1.5 text-start text-[13.5px] hover:bg-tile-2"
          :class="locale === preferences.language ? 'text-text' : 'text-muted'"
          @click="preferences.setLanguage(locale); close()"
        >
          {{ LOCALE_NAMES[locale] }}
          <span v-if="locale === preferences.language" class="text-accent">{{ '✓' }}</span>
        </button>
      </template>
    </DropdownMenu>

    <DropdownMenu>
      <template #trigger="{ open }">
        <button
          type="button"
          :aria-label="t('topbar.userMenu')"
          class="flex h-8 w-8 items-center justify-center rounded-full bg-tile-2 text-muted hover:text-text"
          :class="{ 'ring-2 ring-accent': open }"
        >
          <ProfileAvatar v-if="me?.profiles" :name="me.username" :color="me.color" :size="32" />
          <Icon v-else name="user" :size="17" />
        </button>
      </template>
      <template #panel>
        <div v-if="me?.profiles" class="flex items-center gap-2.5 border-b border-line-soft px-3 pb-2.5 pt-1.5">
          <ProfileAvatar :name="me.username" :color="me.color" :size="28" />
          <span class="min-w-0 flex-1 truncate text-[14px] font-semibold text-text">{{ me.username }}</span>
        </div>
        <div class="px-3 pb-2 pt-1">
          <div class="mb-1.5 text-[12.5px] text-faint">{{ t('userMenu.appearance.label') }}</div>
          <div class="flex gap-1 rounded bg-field p-1">
            <button
              v-for="opt in appearanceOptions"
              :key="opt.value"
              type="button"
              class="flex flex-1 items-center justify-center gap-1 rounded px-1.5 py-1 text-[12.5px]"
              :class="preferences.appearance === opt.value ? 'bg-tile text-text' : 'text-muted hover:text-text'"
              :title="t(opt.label)"
              @click="preferences.setAppearance(opt.value)"
            >
              <Icon :name="opt.icon" :size="14" />
            </button>
          </div>
        </div>

        <div class="my-1 border-t border-line-soft" />
        <RouterLink
          to="/settings"
          class="block px-3 py-1.5 text-[13.5px] text-muted hover:bg-tile-2 hover:text-text"
        >
          {{ t('userMenu.settings') }}
        </RouterLink>
        <button
          v-if="me?.profiles"
          type="button"
          class="block w-full px-3 py-1.5 text-start text-[13.5px] text-muted hover:bg-tile-2 hover:text-text"
          @click="switchProfile()"
        >
          {{ t('userMenu.switchProfile') }}
        </button>
        <button
          v-if="authState.credentials"
          type="button"
          class="block w-full px-3 py-1.5 text-start text-[13.5px] text-muted hover:bg-tile-2 hover:text-text"
          @click="logout()"
        >
          {{ t('userMenu.signOut') }}
        </button>
      </template>
    </DropdownMenu>
  </header>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
import DropdownMenu from './DropdownMenu.vue'
import ProfileAvatar from './profiles/ProfileAvatar.vue'
import SearchPalette from './SearchPalette.vue'
import NotificationsBell from './NotificationsBell.vue'
import { usePreferencesStore } from '@/stores/preferences'
import { useUiStore } from '@/stores/ui'
import { useRemoteStore } from '@/stores/remote'
import { useControlHub } from '@/composables/useControlHub'
import { useAuth } from '@/composables/useAuth'
import { LOCALE_NAMES, SUPPORTED_LOCALES } from '@/i18n'

const { t } = useI18n()
const preferences = usePreferencesStore()
const ui = useUiStore()
const remote = useRemoteStore()
const route = useRoute()
const { available: controlAvailable } = useControlHub()
const { authState, me, logout, switchProfile } = useAuth()

const searchPalette = ref<InstanceType<typeof SearchPalette> | null>(null)
const shortcutHint = /Mac/.test(navigator.platform) ? '⌘K' : 'Ctrl+K'

const appearanceOptions = [
  { value: 'dark' as const, icon: 'moon', label: 'userMenu.appearance.dark' },
  { value: 'light' as const, icon: 'sun', label: 'userMenu.appearance.light' },
  { value: 'system' as const, icon: 'laptop', label: 'userMenu.appearance.system' },
]
</script>
