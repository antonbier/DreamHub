// A list read elsewhere (the Netcast tab) on its way to the IPTV tab, which
// picks it up when it opens. In memory only.
import { ref } from 'vue'
import type { IptvList } from '@/api/types'

const handoff = ref<IptvList | null>(null)

export function useIptvHandoff() {
  return {
    offer(l: IptvList) {
      handoff.value = l
    },
    take(): IptvList | null {
      const l = handoff.value
      handoff.value = null
      return l
    },
  }
}
