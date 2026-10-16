// Sidebar contents from GET /api/v1/navigation (backend/internal/hub,
// Navigation()). The backend already applies the role filter and hides
// disabled hubs. Each item carries both its descriptive label and its hub
// name; which one shows is the frontend's (composables/useHubLabel.ts).
import { defineStore } from 'pinia'
import { api } from '@/api/client'
import type { Navigation } from '@/api/types'

export const useNavigationStore = defineStore('navigation', {
  state: () => ({
    navigation: null as Navigation | null,
    loading: false,
    reachable: true,
  }),
  getters: {
    // Whether the active box runs DreamHub itself. A backend that doesn't say
    // (no box information) is the single-box case, which is always Full.
    isFull: (state): boolean => state.navigation?.box?.full ?? true,
  },
  actions: {
    async load() {
      this.loading = true
      try {
        this.navigation = await api.navigation()
        this.reachable = true
      } catch {
        this.reachable = false
      } finally {
        this.loading = false
      }
    },
  },
})
