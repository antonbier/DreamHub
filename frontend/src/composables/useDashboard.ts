// Drives the dashboard grid (src/views/DashboardView.vue) against
// GET/PUT /api/v1/dashboard (backend/internal/dashboard). Every change in
// edit mode — reorder, resize, hide/show — persists immediately; there's
// no separate Save step (same direct-save exception as preferences.ts and
// the settings menu: this is DreamHub's own state, not a device change).
//
// Tiles are addressed by tileKey(): the type for a built-in tile (one of
// each), the ID for a custom tile (any number of them).
import { ref } from 'vue'
import { api } from '@/api/client'
import type { CustomTileView, DashboardLayout, DashboardTile } from '@/api/types'

export function tileKey(tile: DashboardTile): string {
  return tile.type === 'custom' ? `custom:${tile.id ?? ''}` : tile.type
}

// template: edit the viewers' start page (an admin's job) instead of the
// profile's own.
export function useDashboard(template = false) {
  const get = () => (template ? api.dashboard.template() : api.dashboard.get())
  const save = (l: DashboardLayout) => (template ? api.dashboard.saveTemplate(l) : api.dashboard.save(l))
  const layout = ref<DashboardLayout>({ tiles: [] })
  const loading = ref(true)
  const loadFailed = ref(false)
  const editing = ref(false)

  async function load() {
    loading.value = true
    loadFailed.value = false
    try {
      layout.value = await get()
    } catch {
      loadFailed.value = true
    } finally {
      loading.value = false
    }
  }

  async function persist() {
    try {
      // Adopt the server's (reconciled) response as the new source of
      // truth, same as preferences.ts and the settings form.
      layout.value = await save(layout.value)
    } catch {
      // The layout still applies locally for this session; it just might
      // not survive a reload. No toast — a background save stays quiet.
    }
  }

  const find = (key: string) => layout.value.tiles.find((t) => tileKey(t) === key)

  function toggleHidden(key: string) {
    const tile = find(key)
    if (!tile) return
    tile.hidden = !tile.hidden
    void persist()
  }

  function setWidth(key: string, width: number) {
    const tile = find(key)
    if (!tile) return
    tile.width = Math.min(4, Math.max(1, width))
    void persist()
  }

  function reorder(fromKey: string, toKey: string) {
    const tiles = layout.value.tiles
    const from = tiles.findIndex((t) => tileKey(t) === fromKey)
    const to = tiles.findIndex((t) => tileKey(t) === toKey)
    if (from === -1 || to === -1 || from === to) return
    const [moved] = tiles.splice(from, 1)
    tiles.splice(to, 0, moved)
    void persist()
  }

  // The touch-friendly counterpart of drag: one step up (-1) or down (+1).
  function move(key: string, delta: -1 | 1) {
    const tiles = layout.value.tiles
    const from = tiles.findIndex((t) => tileKey(t) === key)
    const to = from + delta
    if (from === -1 || to < 0 || to >= tiles.length) return
    const [moved] = tiles.splice(from, 1)
    tiles.splice(to, 0, moved)
    void persist()
  }

  // Custom tiles: unlike the background saves above, these report failure,
  // since the builder dialog has to stay open and say so.
  async function addCustom(custom: CustomTileView) {
    const next = { tiles: [...layout.value.tiles, { type: 'custom', label: '', width: 2, hidden: false, custom }] }
    layout.value = await save(next)
  }

  async function updateCustom(key: string, custom: CustomTileView) {
    const next = { tiles: layout.value.tiles.map((t) => (tileKey(t) === key ? { ...t, custom } : t)) }
    layout.value = await save(next)
  }

  function removeCustom(key: string) {
    layout.value.tiles = layout.value.tiles.filter((t) => tileKey(t) !== key)
    void persist()
  }

  // Back to the role's template: the profile's own start page is dropped.
  async function reset() {
    layout.value = await api.dashboard.reset()
  }

  return { layout, loading, loadFailed, editing, load, reset, toggleHidden, setWidth, reorder, move, addCustom, updateCustom, removeCustom }
}
