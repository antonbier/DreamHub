// For a row that scrolls sideways (tab bars on a phone): whether there is
// more hidden past its trailing edge. The row gets the `scroll-fade` class
// (style.css) while that is so, so the cut-off reads as "scroll for more"
// instead of as a clipped label. Wire `measure` to the row's @scroll, and call
// it when the row's content changes.
import { nextTick, onBeforeUnmount, onMounted, ref, type Ref } from 'vue'

export function useScrollMore(el: Ref<HTMLElement | null>) {
  const more = ref(false)
  function measure() {
    const e = el.value
    more.value = !!e && e.scrollWidth - Math.abs(e.scrollLeft) - e.clientWidth > 4
  }
  onMounted(() => {
    window.addEventListener('resize', measure)
    void nextTick(measure)
  })
  onBeforeUnmount(() => window.removeEventListener('resize', measure))
  return { more, measure }
}
