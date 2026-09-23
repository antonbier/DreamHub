// Drives the login gate (App.vue) and LoginScreen.vue against
// GET /api/v1/whoami — see src/api/authState.ts for why the app manages
// Basic Auth itself instead of the browser's native prompt.
import { ref } from 'vue'
import { api, ApiError } from '@/api/client'
import { authState, clearCredentials, setCredentials } from '@/api/authState'

const username = ref<string | null>(null)
const role = ref<'admin' | 'user' | null>(null)

export function useAuth() {
  // Tries whoami with whatever credentials (if any) are already on hand —
  // called once at startup. A 200 means either auth is disabled, or a
  // credential from an earlier login this tab session is still good.
  async function check() {
    try {
      const who = await api.whoami()
      authState.authenticated = true
      username.value = who.username
      role.value = who.role
    } catch {
      authState.authenticated = false
    } finally {
      authState.checked = true
    }
  }

  // Submits credentials and confirms them against whoami before
  // committing to them, so a wrong password never gets stored.
  async function login(user: string, password: string): Promise<{ ok: true } | { ok: false; message: string }> {
    setCredentials({ username: user, password })
    try {
      const who = await api.whoami()
      authState.authenticated = true
      username.value = who.username
      role.value = who.role
      return { ok: true }
    } catch (err) {
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
    username.value = null
    role.value = null
  }

  return { authState, username, role, check, login, logout }
}
