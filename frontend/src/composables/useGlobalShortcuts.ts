// Global "g then a letter" navigation shortcuts (ROADMAP.md, Foundation:
// "keyboard shortcuts beyond search"), mounted once in App.vue — same
// "don't fire while typing" guard as SearchPalette.vue's Cmd/Ctrl+K and `/`,
// but as its own shared composable since this isn't specific to one
// component. The letters are fixed regardless of UI language (like Gmail's
// own "g i", "g s" — relearning shortcuts per locale would be worse than a
// slightly less mnemonic letter in some languages).
import { onBeforeUnmount, onMounted } from 'vue'
import { useRouter } from 'vue-router'

const ROUTES: Record<string, string> = {
  d: '/', // Dashboard
  m: '/media', // Media (Recordings)
  s: '/series', // Series
  g: '/guide', // Guide
  t: '/timers', // Timers
  r: '/discovery', // Recommendations
  y: '/system', // System
  a: '/admin', // Admin
  c: '/control', // Control (remote)
  ',': '/settings', // Settings
}

// How long after pressing "g" the next key still counts as part of the
// chord — long enough for a deliberate two-key press, short enough that an
// unrelated later "g" (e.g. typing) can't accidentally combine with it.
const CHORD_TIMEOUT_MS = 900

export function useGlobalShortcuts() {
  const router = useRouter()
  let awaitingLetter = false
  let timeoutId: number | undefined

  function reset() {
    awaitingLetter = false
    if (timeoutId !== undefined) window.clearTimeout(timeoutId)
    timeoutId = undefined
  }

  function onKeydown(e: KeyboardEvent) {
    const target = e.target as HTMLElement | null
    const typing = target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA' || target?.isContentEditable
    if (typing || e.metaKey || e.ctrlKey || e.altKey) {
      reset()
      return
    }

    if (awaitingLetter) {
      const path = ROUTES[e.key]
      reset()
      if (path) {
        e.preventDefault()
        void router.push(path)
      }
      return
    }

    if (e.key === 'g') {
      awaitingLetter = true
      timeoutId = window.setTimeout(reset, CHORD_TIMEOUT_MS)
    }
  }

  onMounted(() => document.addEventListener('keydown', onKeydown))
  onBeforeUnmount(() => {
    document.removeEventListener('keydown', onKeydown)
    reset()
  })
}
