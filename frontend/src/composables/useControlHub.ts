// ControlHub's navigation entry and whether its remote can be offered at
// all — shared by its own page, the top bar's remote toggle and the side
// panel, so none of them offers a remote the backend reports as
// unavailable (hub disabled, or WebControl unreachable).
import { computed } from 'vue'
import { useNavigationStore } from '@/stores/navigation'

export function useControlHub() {
  const navigation = useNavigationStore()
  const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/control'))
  const available = computed(() => !!item.value && item.value.status !== 'unavailable')
  return { item, available }
}
