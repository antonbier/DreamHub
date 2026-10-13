// Notifications shown by the browser itself (the Notification API), for
// when DreamHub is open in a background tab or the installed app. It needs
// a secure context — HTTPS, or localhost — and the person's permission,
// asked for only when they switch it on. Nothing is sent from a server:
// the page shows what the live event stream delivers, so with DreamHub
// closed there is nothing to show (that would need Web Push).
import { computed, ref } from 'vue'

const KEY = 'dreamhub.browserNotifications'

function stored(): boolean {
  try {
    return localStorage.getItem(KEY) === '1'
  } catch {
    return false
  }
}

const wanted = ref(stored())
const permission = ref<NotificationPermission>(typeof Notification === 'undefined' ? 'denied' : Notification.permission)

export function useBrowserNotifications() {
  // Notification exists, and the page may use it.
  const supported = typeof Notification !== 'undefined' && window.isSecureContext
  const active = computed(() => supported && wanted.value && permission.value === 'granted')

  async function enable(): Promise<void> {
    if (!supported) return
    permission.value = await Notification.requestPermission()
    wanted.value = permission.value === 'granted'
    try {
      localStorage.setItem(KEY, wanted.value ? '1' : '0')
    } catch {
      // Not remembered across visits; works for this one.
    }
  }

  function disable(): void {
    wanted.value = false
    try {
      localStorage.setItem(KEY, '0')
    } catch {
      // Same as above.
    }
  }

  // Shows one, but only while the page isn't in front — then the bell
  // already says it.
  function show(title: string, body: string): void {
    if (!active.value || !document.hidden) return
    try {
      new Notification(title, { body, icon: '/icons/icon-192.png', tag: 'dreamhub' })
    } catch {
      // Some browsers only allow it through a service worker; skip quietly.
    }
  }

  return { supported, active, permission, enable, disable, show }
}
