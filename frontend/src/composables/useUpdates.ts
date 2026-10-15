// The one update notion (docs/APPSHUB_PLAN.md, App14): a single summary of
// what can be updated — packages and containers — shared by the Apps page
// header and the notification inbox, so both always say the same. It is
// fetched on demand (the answer needs a package list and a look at the
// registry) and kept for a few minutes.
import { ref } from 'vue'
import { api } from '@/api/client'
import type { UpdatesView } from '@/api/types'

const summary = ref<UpdatesView | null>(null)
const loading = ref(false)
let fetchedAt = 0
const FRESH_MS = 5 * 60 * 1000

export function useUpdates() {
  // reload asks again; without force a recent answer is kept.
  async function reload(force = false) {
    if (loading.value) return
    if (!force && summary.value && Date.now() - fetchedAt < FRESH_MS) return
    loading.value = true
    try {
      summary.value = await api.apps.updates()
      fetchedAt = Date.now()
    } catch {
      // Neither apt nor Docker answered: there is no summary to show.
      summary.value = null
    } finally {
      loading.value = false
    }
  }
  return { summary, loading, reload }
}
