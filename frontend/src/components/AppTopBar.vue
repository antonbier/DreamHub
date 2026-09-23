<!--
  Top bar — DESIGN.md, "Components: Top bar". Search and notifications are
  placeholders (both are still-open Foundation items, see ROADMAP.md) shown
  disabled with an explanatory tooltip rather than omitted, so the layout
  matches the approved mockup and nothing looks silently missing. Language
  and appearance are fully wired to GET/PATCH /api/v1/settings.
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
    <div class="flex flex-1 justify-center">
      <label class="relative w-full max-w-[420px]">
        <Icon name="search" :size="16" class="pointer-events-none absolute start-2.5 top-1/2 -translate-y-1/2 text-faint" />
        <input
          type="search"
          disabled
          :title="t('common.searchNotBuilt')"
          :placeholder="t('topbar.searchPlaceholder')"
          class="w-full rounded border border-line bg-field py-1.5 ps-8 pe-3 text-[13.5px] text-muted placeholder:text-faint disabled:cursor-not-allowed"
        />
      </label>
    </div>

    <button
      type="button"
      disabled
      :title="t('common.notificationsNotBuilt')"
      :aria-label="t('topbar.notifications')"
      class="flex h-8 w-8 shrink-0 items-center justify-center rounded text-muted disabled:cursor-not-allowed disabled:opacity-50"
    >
      <Icon name="bell" :size="18" />
    </button>

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
          <Icon name="user" :size="17" />
        </button>
      </template>
      <template #panel>
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

        <div class="px-3 pb-2 pt-1">
          <div class="mb-1.5 text-[12.5px] text-faint">{{ t('userMenu.navLabels.label') }}</div>
          <div class="flex gap-1 rounded bg-field p-1">
            <button
              type="button"
              class="flex-1 rounded px-1.5 py-1 text-[12.5px]"
              :class="preferences.navLabels === 'descriptive' ? 'bg-tile text-text' : 'text-muted hover:text-text'"
              @click="preferences.setNavLabels('descriptive')"
            >
              {{ t('userMenu.navLabels.descriptive') }}
            </button>
            <button
              type="button"
              class="flex-1 rounded px-1.5 py-1 text-[12.5px]"
              :class="preferences.navLabels === 'hub' ? 'bg-tile text-text' : 'text-muted hover:text-text'"
              @click="preferences.setNavLabels('hub')"
            >
              {{ t('userMenu.navLabels.hub') }}
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
      </template>
    </DropdownMenu>
  </header>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
import DropdownMenu from './DropdownMenu.vue'
import { usePreferencesStore } from '@/stores/preferences'
import { useUiStore } from '@/stores/ui'
import { LOCALE_NAMES, SUPPORTED_LOCALES } from '@/i18n'

const { t } = useI18n()
const preferences = usePreferencesStore()
const ui = useUiStore()

const appearanceOptions = [
  { value: 'dark' as const, icon: 'moon', label: 'userMenu.appearance.dark' },
  { value: 'light' as const, icon: 'sun', label: 'userMenu.appearance.light' },
  { value: 'system' as const, icon: 'laptop', label: 'userMenu.appearance.system' },
]
</script>
