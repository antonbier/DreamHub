// Shared status data for StatusStrip.vue and NotificationsBell.vue, so the
// two don't each fetch GET /api/v1/health independently. The clock-skew
// check runs once (module-level state) and both components read it.
import { computed, ref } from 'vue'
import { clockSkewMinutes as fetchClockSkew } from '@/lib/clock'
import { useNavigationStore } from '@/stores/navigation'
import { usePreferencesStore } from '@/stores/preferences'

const clockSkew = ref<number | null>(null)
const clockChecked = ref(false)
let checkPromise: Promise<void> | null = null

export const CLOCK_SKEW_WARN_MINUTES = 1

function ensureClockChecked() {
  if (!checkPromise) {
    checkPromise = fetchClockSkew().then((m) => {
      clockSkew.value = m
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
    () => navigation.navigation?.sections.flatMap((s) => s.items).filter((i) => i.status && i.status !== 'available').length ?? 0,
  )

  const clockOk = computed(() => clockSkew.value === null || clockSkew.value < CLOCK_SKEW_WARN_MINUTES)

  return { reachable, issueCount, clockSkew, clockChecked, clockOk, checkClock: ensureClockChecked }
}
