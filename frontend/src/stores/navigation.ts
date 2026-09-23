// Sidebar contents from GET /api/v1/navigation (backend/internal/hub,
// Navigation()). The backend already applies the role filter, hides
// disabled hubs and picks descriptive-vs-hub labels — the frontend just
// renders what it's given.
import { defineStore } from 'pinia'
import { api } from '@/api/client'
import type { Navigation } from '@/api/types'

export const useNavigationStore = defineStore('navigation', {
  state: () => ({
    navigation: null as Navigation | null,
    loading: false,
    reachable: true,
  }),
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
