// Whether the onboarding wizard (CONCEPT.md, "Onboarding wizard") has run
// at least once. Tracked per browser (localStorage), not server-side: it's
// a UI convenience ("don't show this again on this device"), not shared
// state — re-running it is always available from Settings regardless.
const KEY = 'dreamhub.onboardingComplete'

export function isOnboardingComplete(): boolean {
  try {
    return localStorage.getItem(KEY) === '1'
  } catch {
    return true // can't tell — don't force the wizard on every load
  }
}

export function markOnboardingComplete() {
  try {
    localStorage.setItem(KEY, '1')
  } catch {
    // Private browsing or a blocked store — the wizard just reappears
    // next visit, which is harmless.
  }
}
