// What counts as "needs attention". An integration that was never set up
// (TMDB, Plex, the AI key …) is an optional extra, not a fault: it does not
// count, and the pages do not repeat it. A page that cannot work without it
// still says so itself (HubStatusBanner), and the sidebar shows "Not set up
// yet". Only the real trouble counts: a box or service that does not answer,
// a login that fails, a missing dependency.
import type { Problem } from '@/api/types'

// A page that only a box with DreamHub has (box.notFull) is the same kind of
// thing on a Basic box: not a fault, the page says what it needs.
export const isNotSetUp = (p: Problem): boolean => p.code === 'connector.notConfigured' || p.code === 'box.notFull'

// The problems that are trouble: everything except an integration not set up.
export const realProblems = (problems: Problem[] | undefined): Problem[] => (problems ?? []).filter((p) => !isNotSetUp(p))

// Whether a hub or page in this state needs the household's attention.
export function needsAttention(status: string | undefined, problems: Problem[] | undefined): boolean {
  if (!status || status === 'available') return false
  // No reason given: something is wrong, say so. Only "not set up" reasons: not an alarm.
  return !problems?.length || realProblems(problems).length > 0
}
