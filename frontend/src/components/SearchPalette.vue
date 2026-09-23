<!--
  Global search (CONCEPT.md, "Global search"): opened with `/` or
  Cmd/Ctrl+K (DESIGN.md, Interaction), closed with Escape or a backdrop
  click. Two sources, merged into one list: the sidebar's own pages
  (Dashboard, Settings, every hub) matched locally, and GET /api/v1/search
  (debounced) for hub-contributed content — TimerHub is the first real
  provider (ROADMAP.md, hub contract); more hubs join as they implement
  hub.SearchProvider.
-->
<template>
  <div
    v-if="open"
    class="fixed inset-0 z-40 flex items-start justify-center bg-black/50 pt-[15vh]"
    @click.self="close"
  >
    <div class="w-full max-w-[480px] rounded-lg border border-line bg-tile shadow-lg" @keydown="onKeydown">
      <label class="relative block border-b border-line-soft">
        <Icon name="search" :size="16" class="pointer-events-none absolute start-3 top-1/2 -translate-y-1/2 text-faint" />
        <input
          ref="inputEl"
          v-model="query"
          type="text"
          :placeholder="t('topbar.searchPlaceholder')"
          class="w-full bg-transparent py-3 ps-10 pe-3 text-[14.5px] text-text outline-none"
        />
      </label>
      <ul v-if="results.length > 0" class="max-h-[320px] overflow-y-auto py-1.5">
        <li v-for="(item, i) in results" :key="item.key">
          <button
            type="button"
            class="flex w-full items-center gap-2.5 px-3 py-2 text-start text-[14px]"
            :class="i === active ? 'bg-tile-2 text-text' : 'text-muted'"
            @mouseenter="active = i"
            @click="go(item)"
          >
            <Icon :name="item.icon" :size="16" />
            <span class="min-w-0 flex-1">
              <span class="block truncate">{{ item.label }}</span>
              <span v-if="item.subtitle" class="block truncate text-[12px] text-faint">{{ item.subtitle }}</span>
            </span>
          </button>
        </li>
      </ul>
      <p v-else class="px-3 py-4 text-[13.5px] text-faint">{{ t('search.noResults') }}</p>
      <div class="border-t border-line-soft px-3 py-1.5 text-[11.5px] text-faint">{{ t('search.hint') }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import Icon from './Icon.vue'
import { api } from '@/api/client'
import { useNavigationStore } from '@/stores/navigation'

const { t } = useI18n()
const router = useRouter()
const navigation = useNavigationStore()

const open = ref(false)
const query = ref('')
const active = ref(0)
const inputEl = ref<HTMLInputElement | null>(null)

interface ResultItem {
  key: string
  path: string
  label: string
  subtitle?: string
  icon: string
}

const allItems = computed<ResultItem[]>(
  () =>
    navigation.navigation?.sections
      .flatMap((s) => s.items)
      .map((i) => ({ key: `page:${i.path}`, path: i.path, label: t(i.label), icon: i.icon })) ?? [],
)

// Hub-contributed content (GET /api/v1/search) — debounced, and only while
// the palette is open and there's something to search for.
const hubResults = ref<ResultItem[]>([])
let debounce: ReturnType<typeof setTimeout> | undefined
watch(query, (q) => {
  clearTimeout(debounce)
  const trimmed = q.trim()
  if (!open.value || trimmed === '') {
    hubResults.value = []
    return
  }
  debounce = setTimeout(async () => {
    try {
      const { results } = await api.search(trimmed)
      hubResults.value = results.map((r) => ({
        key: `hub:${r.hubId}:${r.path}:${r.title}`,
        path: r.path,
        label: r.title,
        subtitle: [t(r.hubLabel), r.subtitle].filter(Boolean).join(' · '),
        icon: navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.id === r.hubId)?.icon ?? 'search',
      }))
    } catch {
      hubResults.value = []
    }
  }, 150)
})

const results = computed(() => {
  const q = query.value.trim().toLowerCase()
  const pages = q === '' ? allItems.value : allItems.value.filter((i) => i.label.toLowerCase().includes(q))
  return [...pages, ...hubResults.value].slice(0, 8)
})

function show() {
  open.value = true
  query.value = ''
  hubResults.value = []
  active.value = 0
  void nextTick(() => inputEl.value?.focus())
}
function close() {
  open.value = false
}
function go(item: ResultItem) {
  close()
  void router.push(item.path)
}
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    close()
  } else if (e.key === 'ArrowDown') {
    e.preventDefault()
    active.value = Math.min(active.value + 1, results.value.length - 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    active.value = Math.max(active.value - 1, 0)
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const item = results.value[active.value]
    if (item) go(item)
  }
}

// Global shortcuts: Cmd/Ctrl+K always; `/` only when not already typing
// somewhere else (DESIGN.md, Interaction).
function onGlobalKeydown(e: KeyboardEvent) {
  const target = e.target as HTMLElement | null
  const typing = target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA' || target?.isContentEditable
  if ((e.key === 'k' && (e.metaKey || e.ctrlKey)) || (e.key === '/' && !typing)) {
    e.preventDefault()
    show()
  }
}

onMounted(() => document.addEventListener('keydown', onGlobalKeydown))
onBeforeUnmount(() => document.removeEventListener('keydown', onGlobalKeydown))

defineExpose({ show })
</script>
