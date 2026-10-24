<!--
  A guide for internet channels (docs/NETCAST_PLAN.md, "Linking a guide"):
  each chosen channel can show the programme of a channel the box already
  has. Certain matches are filled in, similar ones are offered, anything
  else is searched for by hand, with what each candidate shows now so a wrong
  station shows at once. Nothing is applied here; the choices go into the
  plan and its preview.
-->
<template>
  <details class="rounded-sm border border-line bg-tile p-3" @toggle="onToggle">
    <summary class="cursor-pointer text-[14px] font-semibold">
      {{ t('channels.iptv.guide.title') }}
      <span v-if="picked > 0" class="ms-2 text-[12.5px] font-normal text-muted">{{ t('channels.iptv.guide.count', { n: picked }, picked) }}</span>
    </summary>
    <p class="mt-1 text-[12.5px] text-muted">{{ t('channels.iptv.guide.explain') }}</p>
    <p v-if="loading" class="mt-2 text-[13.5px] text-muted">{{ t('common.loading') }}</p>
    <p v-else-if="failed" class="mt-2 text-[13.5px] text-muted"><ErrorText /></p>
    <ul v-else-if="opened" class="mt-2 divide-y divide-line-soft">
      <li v-for="row in rows" :key="row.id" class="py-2">
        <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span class="min-w-0 grow basis-40 truncate text-[14px] text-text">{{ row.name }}</span>
          <select
            class="max-w-full rounded-sm border border-line bg-field px-2 py-1.5 text-[13.5px] text-text"
            :value="modelValue[row.id]?.ref ?? ''"
            :aria-label="t('channels.iptv.guide.title') + ': ' + row.name"
            @change="onChange(row.id, $event)"
          >
            <option value="">{{ t('channels.iptv.guide.none') }}</option>
            <option v-for="o in options(row.id)" :key="o.ref" :value="o.ref">{{ o.label }}</option>
            <option :value="SEARCH">{{ t('channels.iptv.guide.search') }}</option>
          </select>
        </div>
        <div v-if="searching === row.id" class="mt-2 rounded-sm border border-line-soft bg-bg p-2">
          <label class="flex flex-col gap-0.5 text-[13px]">
            <span class="text-faint">{{ t('channels.iptv.guide.searchLabel') }} · {{ t('channels.iptv.guide.searchHint') }}</span>
            <input v-model="query" type="search" class="w-64 max-w-full rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text" @input="searchSoon" />
          </label>
          <p v-if="searched && hits.length === 0" class="mt-1 text-[13px] text-muted">{{ t('channels.iptv.guide.noResults') }}</p>
          <ul v-if="hits.length > 0" class="mt-1 divide-y divide-line-soft">
            <li v-for="h in hits" :key="h.ref" class="flex flex-wrap items-center gap-x-3 gap-y-1 py-1.5">
              <span class="min-w-0 grow basis-40">
                <span class="block truncate text-[14px] text-text">{{ h.name }}</span>
                <span class="block truncate text-[12.5px] text-faint">{{ h.now ? t('channels.iptv.guide.now', { title: h.now.title }) : t('channels.iptv.guide.noNow') }}</span>
              </span>
              <button type="button" class="rounded-sm px-2 py-1 text-[13px] text-accent hover:bg-tile-2" @click="use(row.id, h)">{{ t('channels.iptv.guide.use') }}</button>
            </li>
          </ul>
          <button type="button" class="mt-1 rounded-sm px-2 py-1 text-[13px] text-muted hover:bg-tile-2" @click="stopSearch">{{ t('channels.iptv.guide.close') }}</button>
        </div>
      </li>
    </ul>
    <button v-if="opened && !loading && shown < ids.length" type="button" class="mt-2 rounded-sm px-2 py-1 text-[13px] text-accent hover:bg-tile-2" @click="shown += PAGE">
      {{ t('channels.iptv.guide.more') }}
    </button>
  </details>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import ErrorText from '@/components/ErrorText.vue'
import type { IptvChannel, IptvTwin, IptvTwinHit, IptvTwinSuggestion } from '@/api/types'

const props = defineProps<{
  token: string
  channels: IptvChannel[]
  // The chosen channels, by number.
  ids: number[]
  modelValue: Record<number, IptvTwin>
}>()
const emit = defineEmits<{ 'update:modelValue': [Record<number, IptvTwin>] }>()

const { t } = useI18n()

const SEARCH = '\u0000search'
const PAGE = 50

const opened = ref(false)
const loading = ref(false)
const failed = ref(false)
const shown = ref(PAGE)
const suggestions = ref(new Map<number, IptvTwinSuggestion>())
// Channels the person has decided about: a later suggestion never overrides them.
const touched = new Set<number>()

const searching = ref<number | null>(null)
const query = ref('')
const hits = ref<IptvTwinHit[]>([])
const searched = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
let searchSeq = 0

const nameOf = (id: number) => props.channels[id]?.name ?? ''
const rows = computed(() => props.ids.slice(0, shown.value).map((id) => ({ id, name: nameOf(id) })))
const picked = computed(() => props.ids.filter((id) => props.modelValue[id]).length)

function options(id: number): { ref: string; label: string }[] {
  const s = suggestions.value.get(id)
  const out: { ref: string; label: string }[] = []
  const seen = new Set<string>()
  const add = (tw: IptvTwin | undefined, label?: string) => {
    if (tw && !seen.has(tw.ref)) {
      seen.add(tw.ref)
      out.push({ ref: tw.ref, label: label ?? tw.name })
    }
  }
  if (s?.remembered) add(s.remembered, t('channels.iptv.guide.remembered', { name: s.remembered.name }))
  add(s?.sure)
  for (const c of s?.candidates ?? []) add(c)
  add(props.modelValue[id]) // one found by search
  return out
}

function set(next: Record<number, IptvTwin>) {
  emit('update:modelValue', next)
}

function onChange(id: number, e: Event) {
  const el = e.target as HTMLSelectElement
  if (el.value === SEARCH) {
    // "Search" opens the search; the select itself stays on what it was
    el.value = props.modelValue[id]?.ref ?? ''
    searching.value = id
    query.value = ''
    hits.value = []
    searched.value = false
    return
  }
  choose(id, el.value)
}

function choose(id: number, ref: string) {
  touched.add(id)
  const next = { ...props.modelValue }
  if (ref === '') delete next[id]
  else {
    const o = options(id).find((x) => x.ref === ref)
    const tw = suggestionTwin(id, ref) ?? props.modelValue[id]
    if (o && tw) next[id] = { ref, name: tw.name, hd: tw.hd }
  }
  set(next)
}

function suggestionTwin(id: number, ref: string): IptvTwin | undefined {
  const s = suggestions.value.get(id)
  return [s?.remembered, s?.sure, ...(s?.candidates ?? [])].find((x) => x?.ref === ref)
}

function use(id: number, h: IptvTwinHit) {
  touched.add(id)
  set({ ...props.modelValue, [id]: { ref: h.ref, name: h.name, hd: h.hd } })
  stopSearch()
}

function stopSearch() {
  searching.value = null
  hits.value = []
  query.value = ''
}

function searchSoon() {
  clearTimeout(timer)
  timer = setTimeout(runSearch, 350)
}

async function runSearch() {
  const q = query.value.trim()
  if ([...q].length < 2) {
    hits.value = []
    searched.value = false
    return
  }
  const seq = ++searchSeq
  try {
    const res = await api.channels.searchIptvTwins(q)
    if (seq !== searchSeq) return
    hits.value = res.results
    searched.value = true
  } catch {
    if (seq === searchSeq) hits.value = []
  }
}

// Ask for the suggestions of the channels not asked about yet, and fill in
// the certain ones.
async function load() {
  const missing = props.ids.filter((id) => !suggestions.value.has(id))
  if (missing.length === 0) return
  loading.value = suggestions.value.size === 0
  failed.value = false
  try {
    const res = await api.channels.suggestIptvTwins(props.token, missing)
    const next = new Map(suggestions.value)
    const fill = { ...props.modelValue }
    for (const s of res.suggestions) {
      next.set(s.id, s)
      if (!touched.has(s.id) && !fill[s.id]) {
        const tw = s.remembered ?? s.sure
        if (tw) fill[s.id] = tw
      }
    }
    suggestions.value = next
    set(fill)
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
}

function onToggle(e: Event) {
  opened.value = (e.target as HTMLDetailsElement).open
  if (opened.value) void load()
}

watch(
  () => props.ids,
  () => {
    if (opened.value) void load()
  },
)
// A new list starts over.
watch(
  () => props.token,
  () => {
    suggestions.value = new Map()
    touched.clear()
    stopSearch()
    shown.value = PAGE
    if (opened.value) void load()
  },
)
onBeforeUnmount(() => clearTimeout(timer))
</script>
