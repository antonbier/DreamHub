<!--
  Tile chrome (DESIGN.md, "Components: Tile" and its edit-mode variant):
  header with title, body in the default slot. In edit mode: dashed
  border, a drag handle (the actual native drag is wired on the grid item
  in DashboardView.vue — this is the visual grip; the only way to reorder)
  and a remove-from/show-on-dashboard button. One visual weight for every tile; nothing gets a
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
      <h2 class="truncate text-[15.5px] font-semibold">{{ title }}</h2>
      <span v-if="editing && tile.hidden" class="ms-1 text-[11.5px] text-faint">{{ t('dashboard.tileFrame.hidden') }}</span>
      <span v-if="!editing && meta" class="ms-auto shrink-0 text-[12.5px] text-faint">{{ meta }}</span>

      <div v-if="editing" class="ms-auto flex items-center gap-1">
        <button
          type="button"
          class="tap rounded px-2 py-1 text-[12px] text-accent hover:bg-tile-2"
          @click="$emit('hide')"
        >
          {{ tile.hidden ? t('dashboard.tileFrame.show') : t('dashboard.tileFrame.hide') }}
        </button>
        <button
          v-if="removable"
          type="button"
          class="tap rounded px-2 py-1 text-[12px] text-onair hover:bg-tile-2"
          @click="$emit('remove')"
        >
          {{ t('dashboard.tileFrame.remove') }}
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

defineProps<{
  tile: DashboardTile
  title: string
  editing: boolean
  // A small faint note beside the title (custom tiles: "Custom view").
  meta?: string
  // Custom tiles can be removed for good; built-ins only hidden.
  removable?: boolean
}>()
defineEmits<{ hide: []; remove: [] }>()
const { t } = useI18n()
</script>
