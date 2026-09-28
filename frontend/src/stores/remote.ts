// ControlHub's virtual remote, shared between its own page and the side
// panel that can be opened on every page (RemotePanel.vue): whether that
// panel is open (remembered per browser, like the sidebar's collapsed
// state), the box's standby state, and a counter that goes up after every
// key the box accepted — screenshots watch it to refresh themselves, so a
// key press shows its result without a manual reload.
import { defineStore } from 'pinia'
import { api } from '@/api/client'
import { KEY } from '@/lib/remoteKeys'

const PANEL_KEY = 'dreamhub.remotePanelOpen'

function readPanelOpen(): boolean {
  try {
    return localStorage.getItem(PANEL_KEY) === '1'
  } catch {
    return false
  }
}

export const useRemoteStore = defineStore('remote', {
  state: () => ({
    panelOpen: readPanelOpen(),
    inStandby: false,
    // The last failed key press (an ApiError or network error), for the
    // component showing it to translate via usePlanError; null once a
    // later key goes through.
    lastError: null as unknown,
    keysSent: 0,
  }),
  actions: {
    setPanelOpen(open: boolean) {
      this.panelOpen = open
      try {
        localStorage.setItem(PANEL_KEY, open ? '1' : '0')
      } catch {
        // Storage blocked: the panel still works, it just isn't remembered.
      }
    },
    togglePanel() {
      this.setPanelOpen(!this.panelOpen)
    },
    async loadPower() {
      try {
        this.inStandby = (await api.control.power()).inStandby
      } catch {
        // Keep the last known state rather than flashing a wrong banner.
      }
    },
    async sendKey(code: number) {
      try {
        await api.control.sendKey(code)
        this.lastError = null
        this.keysSent++
        if (code === KEY.POWER) await this.loadPower()
      } catch (err) {
        this.lastError = err
      }
    },
  },
})
