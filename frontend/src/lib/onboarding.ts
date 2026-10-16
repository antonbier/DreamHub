// Whether the setup wizard (CONCEPT.md, "Onboarding wizard") has run on
// this box. The answer lives on the box (the hidden setting
// general.setupDone), so a second phone or a new browser doesn't offer the
// wizard to a box that is long set up; the flag in localStorage is only
// the fast path that saves the request. A box that already has settings
// of its own (someone configured it before this flag existed) counts as
// set up too. Re-running the wizard stays available from Settings.
import { api } from '@/api/client'

const KEY = 'dreamhub.onboardingComplete'
const SETUP_DONE = 'general.setupDone'

function localFlag(): boolean | null {
  try {
    return localStorage.getItem(KEY) === '1'
  } catch {
    return null // can't tell
  }
}

function setLocalFlag() {
  try {
    localStorage.setItem(KEY, '1')
  } catch {
    // Private browsing or a blocked store — the box still remembers.
  }
}

let asking: Promise<boolean> | null = null

/** True when the wizard should not be offered (anymore). Never throws. */
export function isSetupDone(): Promise<boolean> {
  const local = localFlag()
  if (local === null || local) return Promise.resolve(true)
  asking ??= ask().finally(() => {
    asking = null
  })
  return asking
}

async function ask(): Promise<boolean> {
  try {
    const view = await api.settings.get()
    const fields = view.sections.flatMap((s) => s.fields)
    const done = fields.some((f) => f.key === SETUP_DONE && f.value === true)
    // Anything the household set on the box itself means it is set up.
    const configured = fields.some((f) => f.scope === 'global' && !f.hidden && f.key !== SETUP_DONE && f.hasValue)
    if (done || configured) {
      setLocalFlag()
      return true
    }
    return false
  } catch {
    return true // can't tell — don't force the wizard on every load
  }
}

/** The wizard ended (finished or skipped): remember it on the box and here. */
export async function markSetupDone() {
  setLocalFlag()
  try {
    await api.settings.update({ [SETUP_DONE]: true })
  } catch {
    // The local flag still holds on this device; another one may ask again.
  }
}
