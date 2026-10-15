// Enrichment (posters, ratings, overviews — backend/internal/enrich) for
// every screen that shows titles. Components ask for one title at a time;
// the store collects the requests of one render into batches of up to 40
// (POST /api/v1/enrich/lookup), remembers every answer for the session,
// and never asks twice for the same title. When enrichment is off or not
// configured, every answer is null and screens simply show no pictures.
import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import { api } from '@/api/client'
import type { EnrichInfo, EnrichQuery } from '@/api/types'

const BATCH = 40
const DELAY_MS = 30

export const useEnrichStore = defineStore('enrich', () => {
  // key → answer (null: nothing known); a missing key: not asked yet.
  const answers = reactive(new Map<string, EnrichInfo | null>())
  const enabled = ref(true)
  const queue = new Map<string, EnrichQuery>()
  // Keys queued or in flight, so a title is asked for once.
  const asked = new Set<string>()
  let timer: ReturnType<typeof setTimeout> | undefined

  const keyOf = (q: EnrichQuery) => `${q.kind ?? ''}|${q.title.trim().toLowerCase()}`

  async function flush() {
    timer = undefined
    const pending = [...queue.entries()]
    queue.clear()
    for (let i = 0; i < pending.length; i += BATCH) {
      const chunk = pending.slice(i, i + BATCH)
      try {
        const res = await api.enrich.lookup(chunk.map(([, q]) => q))
        enabled.value = res.enabled
        chunk.forEach(([key], n) => answers.set(key, res.results[n] ?? null))
      } catch {
        // Unreachable: leave these unasked, so a later screen tries again.
        chunk.forEach(([key]) => asked.delete(key))
      }
    }
  }

  // info returns what's known about a title (reactive: undefined while
  // pending, then the answer), queueing a lookup the first time.
  function info(title: string | undefined, kind?: 'movie' | 'tv'): EnrichInfo | null | undefined {
    if (!title || !enabled.value) return null
    const q = { title, kind }
    const key = keyOf(q)
    if (answers.has(key)) return answers.get(key)
    if (!asked.has(key)) {
      asked.add(key)
      queue.set(key, q)
      timer ??= setTimeout(() => void flush(), DELAY_MS)
    }
    return undefined
  }

  // Asks whether enrichment is on at all, without looking anything up — for
  // a screen that offers or hides controls depending on it.
  async function probe() {
    try {
      enabled.value = (await api.enrich.lookup([])).enabled
    } catch {
      // Unreachable: keep what is known.
    }
  }

  return { info, enabled, probe }
})
