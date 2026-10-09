// Background jobs (backend/internal/jobs, #41): what the profile started
// and how far it is. Loaded once, then kept current by "event: job" on the
// app's one event stream (stores/notifications.ts). Screens show progress
// where the work was started — there is no page of jobs.
import { defineStore } from 'pinia'
import { api } from '@/api/client'
import type { Job } from '@/api/types'

export const ACTIVE: Job['state'][] = ['queued', 'running', 'waiting']

// Jobs keep a recording's clean path; the box's movie list may report
// "//media/…".
export function cleanPath(p: string): string {
  return p.replace(/\/{2,}/g, '/')
}

export const useJobsStore = defineStore('jobs', {
  state: () => ({
    byId: {} as Record<number, Job>,
    // Why the queue is held right now ("recording", "boxUnreachable"), or "".
    blockedBy: '',
    loaded: false,
  }),
  getters: {
    // The newest job of a kind for a key (a recording's path), if any.
    latest:
      (state) =>
      (kind: string, key: string): Job | undefined => {
        key = cleanPath(key)
        let best: Job | undefined
        for (const j of Object.values(state.byId)) {
          if (j.kind === kind && j.key === key && (!best || j.id > best.id)) best = j
        }
        return best
      },
  },
  actions: {
    async load() {
      try {
        const res = await api.jobs.list()
        const byId: Record<number, Job> = {}
        for (const j of res.jobs) byId[j.id] = j
        this.byId = byId
        this.blockedBy = res.blockedBy
      } catch {
        // Left as-is; live events still arrive.
      } finally {
        this.loaded = true
      }
    },
    apply(j: Job) {
      this.byId[j.id] = j
      if (j.state === 'waiting' && j.message?.startsWith('jobs.blocked.')) this.blockedBy = j.message.slice('jobs.blocked.'.length)
      else if (j.state === 'running') this.blockedBy = ''
    },
    async cancel(id: number) {
      try {
        this.apply(await api.jobs.cancel(id))
      } catch {
        // Already finished; the stream has the final state.
      }
    },
  },
})
