// The boxes this profile may use and the one this page is about
// (docs/MULTIDEVICE_PLAN.md, M3). Which box is chosen by the address
// (lib/box.ts); changing it loads the page again at the new address.
import { defineStore } from 'pinia'
import { api, ApiError } from '@/api/client'
import { activeBoxId, boxUrl, pathWithoutBox, rememberBox } from '@/lib/box'
import type { BoxInfo } from '@/api/types'

export const useBoxesStore = defineStore('boxes', {
  state: () => ({
    boxes: [] as BoxInfo[],
    defaultId: '',
    loaded: false,
  }),
  getters: {
    // The boxes the profile may use: what the selector offers.
    allowed: (state): BoxInfo[] => state.boxes.filter((b) => b.allowed),
    // A household with one box sees nothing new: no selector, no strip.
    multi(): boolean {
      return this.allowed.length >= 2
    },
    activeId: (state): string => activeBoxId || state.defaultId,
    active(): BoxInfo | undefined {
      return this.boxes.find((b) => b.id === this.activeId)
    },
    others(): BoxInfo[] {
      return this.allowed.filter((b) => b.id !== this.activeId)
    },
  },
  actions: {
    async load() {
      try {
        const res = await api.boxes.list()
        this.boxes = res.boxes
        this.defaultId = res.default
      } catch (err) {
        // The address (or the remembered box) names a box that is gone or
        // not this profile's: back to the first box.
        if (err instanceof ApiError && (err.code === 'box_not_found' || err.code === 'box_forbidden') && activeBoxId) {
          rememberBox('')
          window.location.replace(pathWithoutBox())
        }
      } finally {
        this.loaded = true
      }
    },
    // Who is reachable changes without anyone asking: look again quietly.
    async refresh() {
      try {
        const res = await api.boxes.list()
        this.boxes = res.boxes
        this.defaultId = res.default
      } catch {
        // Keep what is shown.
      }
    },
    select(id: string) {
      if (id === this.activeId) return
      const target = id === this.defaultId ? '' : id
      rememberBox(target)
      window.location.assign(boxUrl(target, pathWithoutBox()))
    },
  },
})
