// Compares the box's clock (GET /api/v1/health, UTC) against this
// browser's own clock — PRINCIPLES.md, "Clock check": a wrong box clock
// means wrong timers. Used by the onboarding wizard's device step and the
// notifications bell.
import { api } from '@/api/client'

/** Absolute skew in whole minutes, or null if the backend is unreachable. */
export async function clockSkewMinutes(): Promise<number | null> {
  try {
    const health = await api.health()
    const boxMs = new Date(health.time).getTime()
    return Math.round(Math.abs(Date.now() - boxMs) / 60000)
  } catch {
    return null
  }
}
