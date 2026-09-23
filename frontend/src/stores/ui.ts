// Transient, session-only UI state that doesn't belong on the backend and
// isn't worth persisting (unlike preferences.ts). Currently just whether
// the mobile navigation drawer is open (DESIGN.md: "≤ 900px → sidebar
// becomes a drawer").
import { defineStore } from 'pinia'

export const useUiStore = defineStore('ui', {
  state: () => ({ mobileNavOpen: false }),
  actions: {
    openMobileNav() {
      this.mobileNavOpen = true
    },
    closeMobileNav() {
      this.mobileNavOpen = false
    },
  },
})
