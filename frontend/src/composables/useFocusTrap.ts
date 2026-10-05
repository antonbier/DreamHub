// Keeps the keyboard inside a modal (DESIGN.md, Interaction; the
// accessibility pass of 2026-10-05): Tab and Shift+Tab cycle through the
// container's focusable elements instead of wandering into the page behind
// it, and Escape closes it wherever the focus is — a listener on the
// element itself misses Escape once focus has slipped out. Listens on the
// document in the capture phase while `active()` is true.
import { onBeforeUnmount, onMounted, type Ref } from 'vue'

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

function focusables(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => el.offsetParent !== null || el === document.activeElement)
}

export function useFocusTrap(container: Ref<HTMLElement | null>, options: { onEscape?: () => void; active?: () => boolean } = {}) {
  function onKeydown(e: KeyboardEvent) {
    const root = container.value
    if (!root || (options.active && !options.active())) return
    if (e.key === 'Escape' && options.onEscape) {
      e.preventDefault()
      e.stopPropagation()
      options.onEscape()
      return
    }
    if (e.key !== 'Tab') return
    const items = focusables(root)
    if (items.length === 0) {
      e.preventDefault()
      return
    }
    const first = items[0]
    const last = items[items.length - 1]
    const current = document.activeElement as HTMLElement | null
    if (!current || !root.contains(current)) {
      e.preventDefault()
      ;(e.shiftKey ? last : first).focus()
    } else if (e.shiftKey && current === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && current === last) {
      e.preventDefault()
      first.focus()
    }
  }
  onMounted(() => document.addEventListener('keydown', onKeydown, true))
  onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown, true))
}
