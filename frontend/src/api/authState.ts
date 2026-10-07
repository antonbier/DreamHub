// Holds the Basic Auth credentials the web UI uses once a login succeeds
// (see src/composables/useAuth.ts), and the reactive "are we currently
// authenticated" flag client.ts updates on every response.
//
// Why the app manages Basic Auth itself instead of relying on the browser's
// native credentials prompt: only /api/v1/* sits behind auth, not the SPA
// shell (backend/README.md) — a plain fetch() to a protected endpoint gets
// a 401 but nothing in the browser ever surfaces a prompt for it. So
// DreamHub asks for credentials itself (LoginScreen.vue) and attaches them
// to every request.
//
// Trade-off, deliberately accepted for a LAN appliance (PRINCIPLES.md,
// "Authentication: simple by default"): the password is kept in
// sessionStorage, in the clear, for this tab's lifetime, so it's readable
// by any script that can already run on this page. That's the same
// exposure the browser's own native Basic Auth prompt would have; nothing
// here is worse, but nothing hides it either.
import { reactive } from 'vue'

const STORAGE_KEY = 'dreamhub.credentials'
// The device's profile session (docs/PROFILES_PLAN.md): kept in
// localStorage, unlike the door's password — "remember the profile on this
// device" was the owner's choice. The token only picks a profile; it opens
// nothing the door hasn't already opened.
const PROFILE_KEY = 'dreamhub.profile'
export const PROFILE_HEADER = 'X-DreamHub-Profile'

function loadProfileToken(): string | null {
  try {
    return localStorage.getItem(PROFILE_KEY)
  } catch {
    return null
  }
}

export interface Credentials {
  username: string
  password: string
}

function loadStored(): Credentials | null {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    return raw ? (JSON.parse(raw) as Credentials) : null
  } catch {
    return null
  }
}

export const authState = reactive({
  credentials: loadStored() as Credentials | null,
  // True once /api/v1/whoami has been tried at least once (success or
  // not) — lets the shell tell "still deciding" apart from "log in".
  checked: false,
  authenticated: false,
  // True when that check failed for another reason than a 401 — DreamHub
  // (or the box it runs on) didn't answer. App.vue shows "can't reach"
  // with a retry instead of a login form that couldn't work either.
  unreachable: false,
  // The door is open but no profile is chosen on this device (a 401
  // profile_required / profile_session_invalid) — App.vue shows the picker.
  profileRequired: false,
  profileToken: loadProfileToken(),
})

export function authHeader(): string | undefined {
  if (!authState.credentials) return undefined
  const { username, password } = authState.credentials
  return 'Basic ' + btoa(`${username}:${password}`)
}

// Every header a request to DreamHub needs to say who it is: the door's
// credentials and the device's profile. Used by every fetch — JSON, images,
// the live-TV stream and the event stream.
export function authHeaders(): Record<string, string> {
  const headers: Record<string, string> = {}
  const auth = authHeader()
  if (auth) headers['Authorization'] = auth
  if (authState.profileToken) headers[PROFILE_HEADER] = authState.profileToken
  return headers
}

export function setProfileToken(token: string | null) {
  authState.profileToken = token
  try {
    if (token) localStorage.setItem(PROFILE_KEY, token)
    else localStorage.removeItem(PROFILE_KEY)
  } catch {
    // See setCredentials.
  }
}

// handleUnauthorized decides what a 401 means: the profile's session is
// missing or over (pick a profile) or the door's credentials are (log in).
export function handleUnauthorized(code: string | undefined) {
  if (code === 'profile_required' || code === 'profile_session_invalid') {
    setProfileToken(null)
    authState.profileRequired = true
    return
  }
  clearCredentials()
  authState.authenticated = false
}

export function setCredentials(c: Credentials) {
  authState.credentials = c
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(c))
  } catch {
    // Private browsing or a blocked store — credentials still work for
    // this request cycle, they just won't survive a reload.
  }
}

export function clearCredentials() {
  authState.credentials = null
  try {
    sessionStorage.removeItem(STORAGE_KEY)
  } catch {
    // See setCredentials.
  }
}
