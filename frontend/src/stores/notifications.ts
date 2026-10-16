// The core event bus's inbox (docs/archive/V1_PLAN.md, WP7; CONCEPT.md,
// "Notifications"): the persisted feed (GET /api/v1/notifications) kept
// live via the SSE stream (GET /api/v1/events/stream, useEventStream.ts).
// One shared connection for the whole app — started from App.vue once
// authenticated, not per-component — so NotificationsBell.vue can mount
// and unmount freely without dropping and reopening the stream.
import { defineStore } from 'pinia'
import { api } from '@/api/client'
import { openEventStream } from '@/composables/useEventStream'
import type { Job, Notification } from '@/api/types'
import { useJobsStore } from '@/stores/jobs'
import { useBoxesStore } from '@/stores/boxes'

let stopStream: (() => void) | null = null

// "Only this box" is a per-device choice (like the remembered box).
const ONLY_KEY = 'dreamhub.notifications.onlyThisBox'
function readOnly(): boolean {
  try {
    return localStorage.getItem(ONLY_KEY) === '1'
  } catch {
    return false
  }
}

export const useNotificationsStore = defineStore('notifications', {
  state: () => ({
    items: [] as Notification[],
    unread: 0,
    loaded: false,
    onlyThisBox: readOnly(),
  }),
  getters: {
    // What the bell lists: with the switch on, only the selected box's events.
    visible(state): Notification[] {
      if (!state.onlyThisBox) return state.items
      const active = useBoxesStore().activeId
      return state.items.filter((n) => !n.box || n.box.id === active)
    },
    hasUnread(): boolean {
      return this.onlyThisBox ? this.visible.some((n) => !n.read) : this.unread > 0
    },
  },
  actions: {
    setOnlyThisBox(on: boolean) {
      this.onlyThisBox = on
      try {
        localStorage.setItem(ONLY_KEY, on ? '1' : '0')
      } catch {
        // Not remembered; it still applies until the page is closed.
      }
    },
    async load() {
      try {
        const res = await api.notifications.list(20)
        this.items = res.notifications
        this.unread = res.unread
      } catch {
        // Left as-is; the bell just shows what it already had.
      } finally {
        this.loaded = true
      }
    },
    async markRead() {
      if (this.unread === 0) return
      try {
        await api.notifications.markRead()
        this.unread = 0
        this.items = this.items.map((n) => ({ ...n, read: true }))
      } catch {
        // Best-effort; the next load() reconciles either way.
      }
    },
    connect() {
      if (stopStream) return // already connected
      stopStream = openEventStream('/api/v1/events/stream', (e) => {
        // Background jobs ride the same stream.
        if (e.event === 'job') {
          try {
            useJobsStore().apply(JSON.parse(e.data) as Job)
          } catch {
            // Malformed push — ignore it.
          }
          return
        }
        if (e.event !== 'notification') return
        try {
          const n = JSON.parse(e.data) as Notification
          this.items = [n, ...this.items].slice(0, 20)
          if (!n.read) this.unread++
        } catch {
          // Malformed push — ignore it, the next load() catches up.
        }
      })
    },
    disconnect() {
      stopStream?.()
      stopStream = null
    },
  },
})
