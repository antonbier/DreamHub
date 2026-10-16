<!--
  The box selector in the top bar (docs/MULTIDEVICE_PLAN.md, M3). Only with
  two or more boxes the profile may use — a household with one box sees
  nothing new. A box is told apart by its name and its colour, never by
  colour alone; one that does not answer is greyed and says so.
-->
<template>
  <DropdownMenu v-if="boxes.multi">
    <template #trigger="{ open }">
      <button
        type="button"
        class="flex h-8 max-w-[180px] shrink-0 items-center gap-1.5 rounded-sm px-1.5 text-[13px] text-text hover:bg-tile-2"
        :class="{ 'bg-tile-2': open }"
        :aria-label="t('boxes.selector.label')"
        aria-haspopup="menu"
      >
        <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ background: dot(boxes.active?.color) }" aria-hidden="true" />
        <span class="truncate font-semibold below-sm:max-w-[5rem]">{{ boxes.active?.name }}</span>
        <Icon name="chevron-down" :size="14" class="shrink-0 text-faint" />
      </button>
    </template>
    <template #panel="{ close }">
      <div class="px-3 pb-1 pt-0.5 text-[12px] text-faint">{{ t('boxes.selector.title') }}</div>
      <button
        v-for="b in boxes.allowed"
        :key="b.id"
        type="button"
        class="flex w-full items-center gap-2.5 px-3 py-1.5 text-start text-[13.5px] hover:bg-tile-2"
        :class="b.online ? 'text-text' : 'text-faint'"
        :aria-current="b.id === boxes.activeId ? 'true' : undefined"
        @click="close(); boxes.select(b.id)"
      >
        <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ background: dot(b.color) }" :class="{ 'opacity-40': !b.online }" aria-hidden="true" />
        <span class="min-w-0 flex-1 truncate">{{ b.name }}</span>
        <span v-if="!b.online" class="shrink-0 text-[12px]">{{ t('boxes.selector.offline') }}</span>
        <span v-else-if="b.id === boxes.activeId" class="shrink-0 text-accent">{{ '✓' }}</span>
      </button>
    </template>
  </DropdownMenu>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import DropdownMenu from './DropdownMenu.vue'
import Icon from './Icon.vue'
import { useBoxesStore } from '@/stores/boxes'

const { t } = useI18n()
const boxes = useBoxesStore()

// The boxes use the profiles' colour tokens (style.css, --dh-profile-*).
const dot = (color?: string) => `var(--dh-profile-${color || 'slate'})`
</script>
