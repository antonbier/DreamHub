// The installable app (manifest.webmanifest, public/sw.js). Two parts:
//
// - registerServiceWorker(): only in a production build and a secure
//   context — browsers refuse service workers on plain http (except
//   localhost), and DreamHub on the box usually is plain http. Without it
//   the app still works, it just isn't installable through the browser's
//   own prompt (iOS "Add to Home Screen" works regardless).
// - usePwaInstall(): Chrome/Edge/Android fire `beforeinstallprompt` once the
//   app qualifies; we keep the event so Settings can offer an Install
//   button instead of relying on the browser's easily-missed menu entry.
import { ref } from 'vue'

interface InstallPromptEvent extends Event {
  prompt(): Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

const deferred = ref<InstallPromptEvent | null>(null)
const installed = ref(false)

export function registerServiceWorker() {
  if (typeof window === 'undefined') return
  window.addEventListener('beforeinstallprompt', (e) => {
    e.preventDefault()
    deferred.value = e as InstallPromptEvent
  })
  window.addEventListener('appinstalled', () => {
    deferred.value = null
    installed.value = true
  })
  if (!import.meta.env.PROD || !('serviceWorker' in navigator) || !window.isSecureContext) return
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {
      // Not fatal: the app works the same without it, only not installable.
    })
  })
}

export type InstallPlatform = 'standalone' | 'prompt' | 'ios' | 'needsHttps' | 'browserMenu'

export function usePwaInstall() {
  function platform(): InstallPlatform {
    const nav = navigator as Navigator & { standalone?: boolean }
    if (installed.value || window.matchMedia('(display-mode: standalone)').matches || nav.standalone) return 'standalone'
    if (deferred.value) return 'prompt'
    // iPadOS reports itself as a Mac; touch points tell them apart.
    const ios = /iPhone|iPad|iPod/.test(navigator.userAgent) || (navigator.userAgent.includes('Macintosh') && navigator.maxTouchPoints > 1)
    if (ios) return 'ios'
    if (!window.isSecureContext) return 'needsHttps'
    return 'browserMenu'
  }

  async function install() {
    const e = deferred.value
    if (!e) return
    await e.prompt()
    const { outcome } = await e.userChoice
    deferred.value = null
    if (outcome === 'accepted') installed.value = true
  }

  return { deferred, installed, platform, install }
}
