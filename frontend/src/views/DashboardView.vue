<!--
  Dashboard — CONCEPT.md, "Dashboard (decided: tiles)". The grid follows
  DESIGN.md exactly (4 columns, 14px gap, the three breakpoints). Tiles,
  their order, width and visibility come from GET/PUT /api/v1/dashboard
  (backend/internal/dashboard); v1's tile catalog is small and built-in —
  custom tiles as saved views need hub search providers first (ROADMAP.md).
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="mb-4 flex items-center">
      <h1 class="text-[26px] font-bold">{{ t('dashboard.title') }}</h1>
      <button
        type="button"
        class="ms-auto rounded px-3 py-1.5 text-[13.5px] font-semibold"
        :class="dash.editing.value ? 'bg-accent text-bg' : 'text-accent hover:bg-tile-2'"
        @click="dash.editing.value = !dash.editing.value"
      >
        {{ dash.editing.value ? t('dashboard.done') : t('dashboard.edit') }}
      </button>
    </div>

    <p v-if="dash.loading.value" class="text-[14px] text-muted">{{ t('common.loading') }}</p>
    <div v-else-if="dash.loadFailed.value" class="rounded border border-line bg-tile p-5">
      <p class="mb-3 text-[14px] text-muted">{{ t('dashboard.loadFailed') }}</p>
      <button type="button" class="text-[13.5px] text-accent hover:underline" @click="dash.load()">
        {{ t('common.retry') }}
      </button>
    </div>

    <div v-else-if="visibleTiles.length === 0 && !dash.editing.value" class="rounded border border-line bg-tile p-6">
      <h2 class="mb-1.5 text-[15.5px] font-semibold">{{ t('dashboard.allHidden.title') }}</h2>
      <p class="text-[14px] text-muted">{{ t('dashboard.allHidden.body') }}</p>
    </div>

    <div v-else class="grid grid-cols-4 gap-[14px] below-lg:grid-cols-2 below-sm:grid-cols-1">
      <div
        v-for="tile in shownTiles"
        :key="tile.type"
        :draggable="dash.editing.value"
        class="transition-opacity"
        :class="[WIDTH_CLASSES[tile.width], tile.hidden ? 'opacity-50' : '', draggingType === tile.type ? 'opacity-30' : '']"
        @dragstart="onDragStart(tile.type, $event)"
        @dragend="draggingType = null"
        @dragover.prevent
        @drop.prevent="onDrop(tile.type)"
      >
        <DashboardTileFrame
          :tile="tile"
          :title="t(tile.label)"
          :editing="dash.editing.value"
          @hide="dash.toggleHidden(tile.type)"
          @width="dash.setWidth(tile.type, $event)"
        >
          <TimersUpcomingTile v-if="tile.type === 'timers.upcoming'" />
          <HubsStatusTile v-else-if="tile.type === 'hubs.status'" />
          <MediaDiskSpaceTile v-else-if="tile.type === 'media.diskSpace'" />
          <SeriesUpcomingTile v-else-if="tile.type === 'series.upcoming'" />
        </DashboardTileFrame>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import DashboardTileFrame from '@/components/dashboard/DashboardTileFrame.vue'
import TimersUpcomingTile from '@/components/dashboard/TimersUpcomingTile.vue'
import HubsStatusTile from '@/components/dashboard/HubsStatusTile.vue'
import MediaDiskSpaceTile from '@/components/dashboard/MediaDiskSpaceTile.vue'
import SeriesUpcomingTile from '@/components/dashboard/SeriesUpcomingTile.vue'
import { useDashboard } from '@/composables/useDashboard'

// DESIGN.md, "Dashboard grid": spans 3-4 collapse to 2 columns below 1180px,
// everything is 1 column below 640px. Tailwind needs the full class names
// written out somewhere to include them in the build — this is that place.
const WIDTH_CLASSES: Record<number, string> = {
  1: 'col-span-1 below-lg:col-span-1 below-sm:col-span-1',
  2: 'col-span-2 below-lg:col-span-2 below-sm:col-span-1',
  3: 'col-span-3 below-lg:col-span-2 below-sm:col-span-1',
  4: 'col-span-4 below-lg:col-span-2 below-sm:col-span-1',
}

const { t } = useI18n()
const dash = useDashboard()
onMounted(() => dash.load())

const visibleTiles = computed(() => dash.layout.value.tiles.filter((t) => !t.hidden))
// In edit mode, hidden tiles stay visible (dimmed) so they can be brought
// back; otherwise only visible ones render.
const shownTiles = computed(() => (dash.editing.value ? dash.layout.value.tiles : visibleTiles.value))

const draggingType = ref<string | null>(null)
function onDragStart(type: string, e: DragEvent) {
  draggingType.value = type
  e.dataTransfer?.setData('text/plain', type)
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
}
function onDrop(targetType: string) {
  if (draggingType.value) dash.reorder(draggingType.value, targetType)
  draggingType.value = null
}
</script>
