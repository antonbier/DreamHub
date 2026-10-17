// Shared status data for StatusStrip.vue and NotificationsBell.vue, so the
// two don't each fetch GET /api/v1/health independently. The clock-skew
// check runs once (module-level state) and both components read it.
import { computed, ref } from 'vue'
import { api } from '@/api/client'
import { needsAttention } from '@/lib/attention'
import { useNavigationStore } from '@/stores/navigation'
import { usePreferencesStore } from '@/stores/preferences'

const clockSkew = ref<number | null>(null)
const clockChecked = ref(false)
// The box's own IANA zone (health.serverTimeZone), for the "box" display-zone
// mode (useDisplayTimeZone.ts) — fetched alongside the clock check since
// both come from the same GET /api/v1/health call.
const serverTimeZone = ref<string | null>(null)
let checkPromise: Promise<void> | null = null

export const CLOCK_SKEW_WARN_MINUTES = 1

function ensureClockChecked() {
  if (!checkPromise) {
    checkPromise = api
      .health()
      .then((health) => {
        clockSkew.value = Math.round(Math.abs(Date.now() - new Date(health.time).getTime()) / 60000)
        serverTimeZone.value = health.serverTimeZone || null
      })
      .catch(() => {
        clockSkew.value = null
        serverTimeZone.value = null
      })
      .finally(() => {
        clockChecked.value = true
      })
  }
  return checkPromise
}

export function useDeviceStatus() {
  const navigation = useNavigationStore()
  const preferences = usePreferencesStore()

  const reachable = computed(() => preferences.reachable && navigation.reachable)

  const issueCount = computed(
    () => navigation.navigation?.sections.flatMap((s) => s.items).filter((i) => needsAttention(i.status, i.problems)).length ?? 0,
  )

  const clockOk = computed(() => clockSkew.value === null || clockSkew.value < CLOCK_SKEW_WARN_MINUTES)

  return { reachable, issueCount, clockSkew, clockChecked, clockOk, serverTimeZone, checkClock: ensureClockChecked }
}
