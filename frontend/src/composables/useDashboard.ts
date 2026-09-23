// Drives the dashboard grid (src/views/DashboardView.vue) against
// GET/PUT /api/v1/dashboard (backend/internal/dashboard). Every change in
// edit mode — reorder, resize, hide/show — persists immediately; there's
// no separate Save step (same direct-save exception as preferences.ts and
// the settings menu: this is DreamHub's own state, not a device change).
import { ref } from 'vue'
import { api } from '@/api/client'
import type { DashboardLayout } from '@/api/types'

export function useDashboard() {
  const layout = ref<DashboardLayout>({ tiles: [] })
  const loading = ref(true)
  const loadFailed = ref(false)
  const editing = ref(false)

  async function load() {
    loading.value = true
    loadFailed.value = false
    try {
      layout.value = await api.dashboard.get()
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
      layout.value = await api.dashboard.save(layout.value)
    } catch {
      // The layout still applies locally for this session; it just might
      // not survive a reload. No toast — a background save stays quiet.
    }
  }

  function toggleHidden(type: string) {
    const tile = layout.value.tiles.find((t) => t.type === type)
    if (!tile) return
    tile.hidden = !tile.hidden
    void persist()
  }

  function setWidth(type: string, width: number) {
    const tile = layout.value.tiles.find((t) => t.type === type)
    if (!tile) return
    tile.width = Math.min(4, Math.max(1, width))
    void persist()
  }

  function reorder(fromType: string, toType: string) {
    const tiles = layout.value.tiles
    const from = tiles.findIndex((t) => t.type === fromType)
    const to = tiles.findIndex((t) => t.type === toType)
    if (from === -1 || to === -1 || from === to) return
    const [moved] = tiles.splice(from, 1)
    tiles.splice(to, 0, moved)
    void persist()
  }

  return { layout, loading, loadFailed, editing, load, toggleHidden, setWidth, reorder }
}
