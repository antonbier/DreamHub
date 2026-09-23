<!--
  Tile chrome (DESIGN.md, "Components: Tile" and its edit-mode variant):
  header with title, body in the default slot. In edit mode: dashed
  border, a drag handle (the actual native drag is wired on the grid item
  in DashboardView.vue — this is the visual grip), width ←/→ buttons and
  a hide/show toggle. One visual weight for every tile; nothing gets a
  louder treatment than another.
-->
<template>
  <div
    class="flex h-full flex-col rounded border bg-tile p-4"
    :class="editing ? 'border-dashed border-line' : 'border-line'"
  >
    <div class="mb-3 flex items-center gap-2">
      <Icon
        v-if="editing"
        name="menu"
        :size="15"
        class="shrink-0 cursor-grab text-faint"
        :title="t('dashboard.tileFrame.dragHandle')"
      />
      <h3 class="truncate text-[15.5px] font-semibold">{{ title }}</h3>
      <span v-if="editing && tile.hidden" class="ms-1 text-[11.5px] text-faint">{{ t('dashboard.tileFrame.hidden') }}</span>

      <div v-if="editing" class="ms-auto flex items-center gap-1">
        <button
          type="button"
          class="flex h-6 w-6 items-center justify-center rounded text-muted hover:bg-tile-2 hover:text-text disabled:opacity-30"
          :disabled="tile.width <= 1"
          :title="t('dashboard.tileFrame.widthDecrease')"
          @click="$emit('width', tile.width - 1)"
        >
          <Icon name="chevron-left" :size="14" />
        </button>
        <span class="w-3 text-center text-[12px] text-faint">{{ tile.width }}</span>
        <button
          type="button"
          class="flex h-6 w-6 items-center justify-center rounded text-muted hover:bg-tile-2 hover:text-text disabled:opacity-30"
          :disabled="tile.width >= 4"
          :title="t('dashboard.tileFrame.widthIncrease')"
          @click="$emit('width', tile.width + 1)"
        >
          <Icon name="chevron-right" :size="14" />
        </button>
        <button
          type="button"
          class="ms-1 rounded px-2 py-1 text-[12px] text-accent hover:bg-tile-2"
          @click="$emit('hide')"
        >
          {{ tile.hidden ? t('dashboard.tileFrame.show') : t('dashboard.tileFrame.hide') }}
        </button>
      </div>
    </div>
    <div class="min-h-0 flex-1"><slot /></div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/Icon.vue'
import type { DashboardTile } from '@/api/types'

defineProps<{ tile: DashboardTile; title: string; editing: boolean }>()
defineEmits<{ hide: []; width: [number] }>()
const { t } = useI18n()
</script>
