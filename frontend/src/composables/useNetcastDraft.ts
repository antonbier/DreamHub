// A guide source (or playlist) on its way to the Netcast tab: the IPTV tab's
// coverage check hands it over, the Netcast tab fills its "add" form with it.
// In memory only — the address may carry a login and does not belong in a URL.
import { ref } from 'vue'

export interface NetcastDraft {
  kind: 'server' | 'epg'
  name: string
  address: string
}

const draft = ref<NetcastDraft | null>(null)

export function useNetcastDraft() {
  return {
    draft,
    offer(d: NetcastDraft) {
      draft.value = d
    },
    take(): NetcastDraft | null {
      const d = draft.value
      draft.value = null
      return d
    },
  }
}
