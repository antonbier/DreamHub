// Drives the login gate (App.vue), LoginScreen.vue and the profile picker
// against GET /api/v1/whoami — see src/api/authState.ts for why the app
// manages Basic Auth itself instead of the browser's native prompt, and
// docs/PROFILES_PLAN.md for profiles.
import { computed, ref } from 'vue'
import { api, ApiError } from '@/api/client'
import { authState, clearCredentials, setCredentials, setProfileToken } from '@/api/authState'
import type { Permission, WhoAmI } from '@/api/types'

const me = ref<WhoAmI | null>(null)
const username = computed(() => me.value?.username ?? null)
const role = computed(() => me.value?.role ?? null)

// can asks what the server will allow — to leave out buttons it would
// refuse. The server checks anyway; this is only so viewers don't see them.
function can(p: Permission): boolean {
  return me.value?.permissions.includes(p) ?? false
}

function isProfileError(err: unknown): boolean {
  return err instanceof ApiError && err.status === 401 && (err.code === 'profile_required' || err.code === 'profile_session_invalid')
}

export function useAuth() {
  // Tries whoami with whatever credentials (if any) are already on hand —
  // called once at startup. A 200 means either auth is disabled, or a
  // credential from an earlier login this tab session is still good.
  async function check() {
    try {
      me.value = await api.whoami()
      authState.authenticated = true
      authState.profileRequired = false
      authState.unreachable = false
    } catch (err) {
      // The door opened, but this device has no profile yet.
      authState.authenticated = isProfileError(err)
      authState.profileRequired = isProfileError(err)
      authState.unreachable = !(err instanceof ApiError && err.status === 401)
    } finally {
      authState.checked = true
    }
  }

  // Submits credentials and confirms them against whoami before
  // committing to them, so a wrong password never gets stored.
  async function login(user: string, password: string): Promise<{ ok: true } | { ok: false; message: string }> {
    setCredentials({ username: user, password })
    try {
      me.value = await api.whoami()
      authState.authenticated = true
      return { ok: true }
    } catch (err) {
      if (isProfileError(err)) {
        authState.authenticated = true
        authState.profileRequired = true
        return { ok: true }
      }
      clearCredentials()
      authState.authenticated = false
      if (err instanceof ApiError && err.status === 401) {
        return { ok: false, message: 'wrong' }
      }
      return { ok: false, message: 'network' }
    }
  }

  function logout() {
    clearCredentials()
    authState.authenticated = false
    me.value = null
  }

  // Leaves the profile on this device and shows the picker again; the page
  // reloads after the next pick so no view keeps the old profile's data.
  async function switchProfile() {
    try {
      await api.profiles.endSession()
    } catch {
      // Signed out locally either way.
    }
    setProfileToken(null)
    me.value = null
    authState.profileRequired = true
  }

  return { authState, me, username, role, can, check, login, logout, switchProfile }
}
