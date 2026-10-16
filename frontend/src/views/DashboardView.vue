<!--
  Dashboard — CONCEPT.md, "Dashboard (decided: tiles)". The grid follows
  DESIGN.md exactly (4 columns, 14px gap, the three breakpoints). Tiles,
  their order, width and visibility come from GET/PUT /api/v1/dashboard
  (backend/internal/dashboard): the built-in tiles, plus custom tiles —
  saved views with criteria over a hub's view source, created and edited
  in CustomTileBuilder.vue from edit mode.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="mb-4 flex flex-wrap items-center gap-2">
      <Icon name="dashboard" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ template ? t('dashboard.template.title') : t('dashboard.title') }}</h1>
      <button
        v-if="canReset"
        type="button"
        class="ms-auto rounded-sm px-3 py-1.5 text-[13.5px] text-muted hover:bg-tile-2 hover:text-text"
        @click="resetToTemplate"
      >
        {{ t('dashboard.template.reset') }}
      </button>
      <button
        type="button"
        class="tap rounded-sm px-3 py-1.5 text-[13.5px] font-semibold"
        :class="[dash.editing.value ? 'bg-accent text-bg' : 'text-accent hover:bg-tile-2', canReset ? '' : 'ms-auto']"
        @click="dash.editing.value = !dash.editing.value"
      >
        {{ dash.editing.value ? t('dashboard.done') : t('dashboard.edit') }}
      </button>
    </div>

    <BoxStrip v-if="!template" />

    <p v-if="template" class="mb-4 rounded-sm border border-accent bg-tile px-4 py-3 text-[13.5px] text-muted">
      {{ t('dashboard.template.explain') }}
      <RouterLink to="/settings/profiles" class="ms-1 text-accent hover:underline">{{ t('dashboard.template.back') }}</RouterLink>
    </p>

    <p v-if="dash.loading.value" class="text-[14px] text-muted">{{ t('common.loading') }}</p>
    <div v-else-if="dash.loadFailed.value" class="rounded-sm border border-line bg-tile p-5">
      <p class="mb-3 text-[14px] text-muted">{{ t('dashboard.loadFailed') }}</p>
      <button type="button" class="text-[13.5px] text-accent hover:underline" @click="dash.load()">
        {{ t('common.retry') }}
      </button>
    </div>

    <div v-else-if="visibleTiles.length === 0 && !dash.editing.value" class="rounded-sm border border-line bg-tile p-6">
      <h2 class="mb-1.5 text-[15.5px] font-semibold">{{ t('dashboard.allHidden.title') }}</h2>
      <p class="text-[14px] text-muted">{{ t('dashboard.allHidden.body') }}</p>
    </div>

    <div v-else class="grid grid-cols-4 gap-[14px] below-lg:grid-cols-2 below-sm:grid-cols-1">
      <div
        v-for="(tile, i) in shownTiles"
        :key="tileKey(tile)"
        :draggable="dash.editing.value"
        class="transition-opacity"
        :class="[WIDTH_CLASSES[tile.width], tile.hidden ? 'opacity-50' : '', draggingKey === tileKey(tile) ? 'opacity-30' : '']"
        @dragstart="onDragStart(tileKey(tile), $event)"
        @dragend="draggingKey = null"
        @dragover.prevent
        @drop.prevent="onDrop(tileKey(tile))"
      >
        <DashboardTileFrame
          :tile="tile"
          :title="tile.custom ? tile.custom.title : t(tile.label)"
          :meta="tile.custom ? t('dashboard.custom.meta') : undefined"
          :removable="tile.type === 'custom'"
          :editing="dash.editing.value"
          :can-up="i > 0"
          :can-down="i < shownTiles.length - 1"
          @up="dash.move(tileKey(tile), -1)"
          @down="dash.move(tileKey(tile), 1)"
          @hide="dash.toggleHidden(tileKey(tile))"
          @remove="dash.removeCustom(tileKey(tile))"
        >
          <CustomTile v-if="tile.custom" :custom="tile.custom" editable @edit="openBuilder(tile)" />
          <NowPlayingTile v-else-if="tile.type === 'control.nowPlaying'" />
          <TimersUpcomingTile v-else-if="tile.type === 'timers.upcoming'" />
          <HubsStatusTile v-else-if="tile.type === 'hubs.status'" />
          <MediaDiskSpaceTile v-else-if="tile.type === 'media.diskSpace'" />
          <SeriesUpcomingTile v-else-if="tile.type === 'series.upcoming'" />
          <TimersConflictsTile v-else-if="tile.type === 'timers.conflicts'" />
          <PlexContinueTile v-else-if="tile.type === 'plex.continue'" />
          <LiveTvTile v-else-if="tile.type === 'stream.channels'" />
          <RecentRecordingsTile v-else-if="tile.type === 'media.recent'" />
          <ExtensionsTile v-else-if="tile.type === 'extensions.tiles'" :cards="extensionCards" />
        </DashboardTileFrame>
      </div>

      <!-- Edit mode: add a custom tile (DESIGN mockup's "Add tile" block). -->
      <div v-if="dash.editing.value" class="col-span-2 flex flex-col items-start gap-2 rounded-sm border border-dashed border-line p-4 below-sm:col-span-1">
        <h3 class="text-[15.5px] font-semibold">{{ t('dashboard.add.title') }}</h3>
        <button type="button" class="rounded-sm py-1 text-[13.5px] text-accent hover:underline" @click="openBuilder()">
          {{ t('dashboard.add.custom') }}
        </button>
        <p class="text-[12.5px] text-faint">{{ t('dashboard.add.hint') }}</p>
      </div>
    </div>

    <CustomTileBuilder
      v-if="builder.open"
      ref="builderEl"
      :initial="builder.tile?.custom"
      @close="closeBuilder"
      @save="saveBuilder"
    />
  </div>
</template>

<script setup lang="ts">
import BoxStrip from '@/components/dashboard/BoxStrip.vue'
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/Icon.vue'
import DashboardTileFrame from '@/components/dashboard/DashboardTileFrame.vue'
import CustomTile from '@/components/dashboard/CustomTile.vue'
import CustomTileBuilder from '@/components/dashboard/CustomTileBuilder.vue'
import TimersUpcomingTile from '@/components/dashboard/TimersUpcomingTile.vue'
import NowPlayingTile from '@/components/dashboard/NowPlayingTile.vue'
import LiveTvTile from '@/components/dashboard/LiveTvTile.vue'
import HubsStatusTile from '@/components/dashboard/HubsStatusTile.vue'
import MediaDiskSpaceTile from '@/components/dashboard/MediaDiskSpaceTile.vue'
import SeriesUpcomingTile from '@/components/dashboard/SeriesUpcomingTile.vue'
import TimersConflictsTile from '@/components/dashboard/TimersConflictsTile.vue'
import PlexContinueTile from '@/components/dashboard/PlexContinueTile.vue'
import RecentRecordingsTile from '@/components/dashboard/RecentRecordingsTile.vue'
import ExtensionsTile from '@/components/dashboard/ExtensionsTile.vue'
import { api } from '@/api/client'
import { useAuth } from '@/composables/useAuth'
import { tileKey, useDashboard } from '@/composables/useDashboard'
import type { CustomTileView, DashboardTile, ExtensionCard } from '@/api/types'

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
const { me, can } = useAuth()
// /?template=1: an admin designs the viewers' start page
// (docs/archive/PROFILES_PLAN.md, P4) — in edit mode from the start.
const route = useRoute()
const template = route.query.template === '1' && can('profiles')
const dash = useDashboard(template)
// What the extensions chose to show on the start page: only their widgets
// flagged "tile", and only the extensions that flagged any. Without any,
// the tile is left out (except in edit mode, where it can be moved).
const extensionCards = ref<ExtensionCard[]>([])
async function loadExtensionTiles() {
  try {
    const { extensions } = await api.extensions.list()
    extensionCards.value = extensions
      .map((c) => ({ ...c, widgets: c.widgets.filter((w) => w.tile && w.type !== 'action') }))
      .filter((c) => c.widgets.length > 0)
  } catch {
    extensionCards.value = []
  }
}

onMounted(async () => {
  void loadExtensionTiles()
  await dash.load()
  if (template) dash.editing.value = true
})

// A viewer who customized can go back to the template — in edit mode only,
// so it isn't tapped by accident.
const canReset = computed(() => dash.editing.value && !template && me.value?.role === 'user' && !dash.layout.value.fromTemplate)

// A viewer's own start page goes; the template comes back.
async function resetToTemplate() {
  try {
    await dash.reset()
    dash.editing.value = false
  } catch {
    // Stays as it was; the button can be tried again.
  }
}

const visibleTiles = computed(() =>
  dash.layout.value.tiles.filter((t) => !t.hidden && !(t.type === 'extensions.tiles' && extensionCards.value.length === 0)),
)
// In edit mode, hidden tiles stay visible (dimmed) so they can be brought
// back; otherwise only visible ones render.
const shownTiles = computed(() => (dash.editing.value ? dash.layout.value.tiles : visibleTiles.value))

const draggingKey = ref<string | null>(null)
function onDragStart(key: string, e: DragEvent) {
  draggingKey.value = key
  e.dataTransfer?.setData('text/plain', key)
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
}
function onDrop(targetKey: string) {
  if (draggingKey.value) dash.reorder(draggingKey.value, targetKey)
  draggingKey.value = null
}

// The custom tile builder: a new tile, or an existing one's criteria.
const builder = reactive<{ open: boolean; tile?: DashboardTile }>({ open: false })
const builderEl = ref<InstanceType<typeof CustomTileBuilder> | null>(null)
let returnFocus: HTMLElement | null = null
function openBuilder(tile?: DashboardTile) {
  returnFocus = document.activeElement as HTMLElement | null
  builder.tile = tile
  builder.open = true
}
function closeBuilder() {
  builder.open = false
  builder.tile = undefined
  void nextTick(() => returnFocus?.focus())
}
async function saveBuilder(custom: CustomTileView) {
  try {
    if (builder.tile) await dash.updateCustom(tileKey(builder.tile), custom)
    else await dash.addCustom(custom)
    closeBuilder()
  } catch {
    builderEl.value?.saveFailed()
  }
}
</script>
