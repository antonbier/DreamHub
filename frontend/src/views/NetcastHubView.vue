<!--
  Netcast (/netcast, docs/NETCAST_PLAN.md, "A page of its own"): the internet
  channels of the Gemini Netcast plugin on the box, laid out like a streaming
  service rather than a list — a channel is a tile with its logo and what runs
  on it now and next, grouped in shelves, and a click plays it on the TV (no
  bouquet is touched). Sources are tabs at the top ("All" searches them all);
  one that Netcast has not downloaded yet is loaded by DreamHub from its own
  address. What runs now comes from the box's guide or from the XMLTV files set
  up in Netcast. "Put into a bouquet…" hands a source's list to the channel
  list's IPTV tab. A stream address — which may hold a login — never reaches
  this page.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="stream" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ hubTitle('netcast', 'hub.netcast') }}</h1>
    </div>
    <p class="mt-1 text-[13.5px] text-muted">{{ t('netcast.intro') }}</p>

    <HubStatusBanner :item="item" />

    <template v-if="!item || item.status !== 'unavailable'">
      <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="failed" class="mt-5 text-[14px] text-muted"><ErrorText /></p>
      <p v-else-if="!sources.available" class="mt-5 rounded-sm border border-line bg-tile px-4 py-3 text-[13.5px] text-muted">{{ t('netcast.none') }}</p>
      <p v-else-if="openable.length === 0" class="mt-5 text-[14px] text-muted">{{ t('netcast.noSources') }}</p>

      <template v-else>
        <!-- the sources, as tabs -->
        <nav class="mt-4 flex gap-1 overflow-x-auto border-b border-line-soft pb-0" :aria-label="t('netcast.group')">
          <button
            v-for="tab in tabs"
            :key="tab.id"
            type="button"
            class="shrink-0 border-b-2 px-3 py-2 text-[14px] font-semibold"
            :class="selected === tab.id ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
            :aria-current="selected === tab.id ? 'page' : undefined"
            @click="select(tab.id)"
          >{{ tab.label }}</button>
        </nav>

        <div class="mt-3 flex flex-wrap items-end gap-2">
          <label class="flex flex-col gap-0.5 text-[13px]">
            <span class="text-faint">{{ t('netcast.search') }}</span>
            <input v-model="query" type="search" class="w-56 rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text" />
          </label>
          <label v-if="groups.length > 1" class="flex flex-col gap-0.5 text-[13px]">
            <span class="text-faint">{{ t('netcast.group') }}</span>
            <select v-model="group" class="max-w-[16rem] rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text">
              <option :value="ALL">{{ t('netcast.allGroups') }}</option>
              <option v-for="g in groups" :key="g.name" :value="g.name">{{ g.name || '—' }} ({{ number(g.count) }})</option>
            </select>
          </label>
          <span class="text-[13px] text-muted" aria-live="polite">{{ t('netcast.summary', { n: number(rows.length) }, rows.length) }}</span>
          <button v-if="boxList" type="button" class="ms-auto rounded-sm px-2 py-1.5 text-[13px] text-accent hover:bg-tile-2" :title="t('netcast.toBouquetHint')" @click="toBouquet">{{ t('netcast.toBouquet') }}</button>
        </div>

        <p v-if="fetchedNote" class="mt-2 text-[12.5px] text-faint">{{ fetchedNote }}</p>
        <p v-if="!canControl" class="mt-2 text-[12.5px] text-faint">{{ t('netcast.readOnlyHint') }}</p>
        <p v-if="error" class="mt-2 text-[13px] text-warn" role="alert">{{ error }}</p>
        <p v-if="opening.size > 0" class="mt-3 text-[13.5px] text-muted">{{ t('netcast.opening') }}</p>
        <p v-else-if="guideBusy" class="mt-2 text-[12.5px] text-faint">{{ t('netcast.guideLoading') }}</p>

        <!-- shelves: one row per group, until a search or a group is chosen -->
        <template v-if="shelfMode">
          <section v-for="s in shelves" :key="s.name" class="mt-6">
            <div class="flex items-baseline gap-3">
              <h2 class="text-[16px] font-semibold">{{ s.name || '—' }}</h2>
              <span class="text-[12.5px] text-faint">{{ number(s.rows.length) }}</span>
              <button v-if="s.rows.length > SHELF" type="button" class="rounded-sm px-2 py-0.5 text-[13px] text-accent hover:bg-tile-2" @click="group = s.name">{{ t('netcast.showAll') }}</button>
            </div>
            <div class="mt-2 flex gap-3 overflow-x-auto pb-2">
              <NetcastTile v-for="r in s.rows.slice(0, SHELF)" :key="r.key" class="w-44 shrink-0" :row="r" :guide="guide[r.key]" :now-sec="nowSec" :playing="current?.key === r.key" :busy="starting === r.key" :can-play="canControl" :clock="clock" :variant="variantOf(r)" @play="play(r)" @variant="nextVariant(r)" />
            </div>
          </section>
          <button v-if="hiddenShelves > 0" type="button" class="mt-4 rounded-sm px-3 py-1.5 text-[13px] text-accent hover:bg-tile-2" @click="shelfLimit += SHELVES">{{ t('netcast.more') }}</button>
        </template>

        <!-- a search or one group: a grid -->
        <template v-else>
          <button v-if="group !== ALL && query.trim() === ''" type="button" class="mt-4 rounded-sm px-2 py-1 text-[13px] text-accent hover:bg-tile-2" @click="group = ALL">‹ {{ t('netcast.backToShelves') }}</button>
          <p v-if="filtered.length === 0 && opening.size === 0" class="mt-5 text-[14px] text-muted">{{ t('netcast.noMatch') }}</p>
          <div class="mt-3 grid grid-cols-5 gap-3 below-lg:grid-cols-4 below-md:grid-cols-3 below-sm:grid-cols-2">
            <NetcastTile v-for="r in gridRows" :key="r.key" :row="r" :guide="guide[r.key]" :now-sec="nowSec" :playing="current?.key === r.key" :busy="starting === r.key" :can-play="canControl" :clock="clock" :variant="variantOf(r)" @play="play(r)" @variant="nextVariant(r)" />
          </div>
          <button v-if="filtered.length > gridRows.length" type="button" class="mt-3 rounded-sm px-3 py-1.5 text-[13px] text-accent hover:bg-tile-2" @click="limit += PAGE">{{ t('netcast.more') }}</button>
        </template>

        <!-- sources that cannot be opened, and why -->
        <details v-if="unavailable.length > 0" class="mt-8 text-[13px] text-muted">
          <summary class="cursor-pointer">{{ t('netcast.unavailable', { n: unavailable.length }) }}</summary>
          <ul class="mt-1 space-y-0.5">
            <li v-for="s in unavailable" :key="s.id"><span class="text-text">{{ s.name }}</span> — {{ t(s.state === 'waiting' ? 'netcast.reasonWaiting' : 'netcast.reasonUnsupported') }}</li>
          </ul>
        </details>
      </template>
    </template>

    <!-- what plays on the TV now -->
    <div v-if="current" class="sticky bottom-0 z-20 -mx-6 mt-6 border-t border-line bg-side px-6 py-2 below-md:-mx-4 below-md:px-4" role="region" :aria-label="t('netcast.nowPlaying')">
      <div class="flex items-center gap-3">
        <div class="h-10 w-16 shrink-0 overflow-hidden rounded-sm bg-tile-2"><NetcastLogo :token="current.token" :id="current.c.id" :name="current.c.name" :has-logo="true" /></div>
        <div class="min-w-0 grow">
          <p class="truncate text-[11.5px] text-onair">{{ t('netcast.nowPlaying') }}</p>
          <p class="truncate text-[14px] font-semibold text-text">{{ current.c.name }}<span v-if="current.c.variants && current.c.variants.length > 1" class="ms-2 text-[12px] font-normal text-muted">{{ current.c.variants[variantOf(current)] }}</span></p>
          <p v-if="guide[current.key]?.now" class="truncate text-[12.5px] text-muted">
            {{ clock(guide[current.key]!.now!.start) }} {{ guide[current.key]!.now!.title }}
          </p>
        </div>
        <button type="button" class="shrink-0 rounded-sm border border-line px-3 py-1.5 text-[14px] text-text hover:bg-tile-2" :aria-label="t('netcast.previous')" @click="step(-1)">‹</button>
        <button type="button" class="shrink-0 rounded-sm border border-line px-3 py-1.5 text-[14px] text-text hover:bg-tile-2" :aria-label="t('netcast.next')" @click="step(1)">›</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import ErrorText from '@/components/ErrorText.vue'
import HubStatusBanner from '@/components/HubStatusBanner.vue'
import Icon from '@/components/Icon.vue'
import NetcastLogo from '@/components/channels/NetcastLogo.vue'
import NetcastTile from '@/components/channels/NetcastTile.vue'
import type { NetcastRow } from '@/components/channels/NetcastTile.vue'
import { useAuth } from '@/composables/useAuth'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { useHubLabel } from '@/composables/useHubLabel'
import { useIptvHandoff } from '@/composables/useIptvHandoff'
import { usePlanError } from '@/composables/usePlanError'
import { useNavigationStore } from '@/stores/navigation'
import type { IptvList, NetcastHubSource, NetcastHubSources, NetcastNowNext } from '@/api/types'

const ALL = '\u0000all'
const EVERY = 'all' // the tab that shows every source
const SHELF = 12
const SHELVES = 12
const PAGE = 120
const GUIDE_RETRY_MS = 4000
const GUIDE_REFRESH_MS = 60000

const { t, n: formatNumber, locale } = useI18n()
const { hubTitle } = useHubLabel()
const { planErrorMessage } = usePlanError()
const { can } = useAuth()
const { offer } = useIptvHandoff()
const { timeZone } = useDisplayTimeZone()
const route = useRoute()
const router = useRouter()
const navigation = useNavigationStore()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/netcast'))
const canControl = computed(() => can('control'))
const number = (v: number) => formatNumber(v)
const nowSec = ref(Date.now() / 1000)
function clock(s: number) {
  return new Intl.DateTimeFormat(locale.value, { timeStyle: 'short', timeZone: timeZone.value }).format(s * 1000)
}

const sources = ref<NetcastHubSources>({ available: false, sources: [] })
const loading = ref(true)
const failed = ref(false)
const error = ref('')

const openable = computed(() => sources.value.sources.filter((s) => s.state === 'ready' || s.state === 'fetch'))
const unavailable = computed(() => sources.value.sources.filter((s) => s.state === 'waiting' || s.state === 'unsupported'))
const tabs = computed(() => {
  const list = openable.value.map((s) => ({ id: s.id, label: s.name }))
  return list.length > 1 ? [{ id: EVERY, label: t('netcast.all') }, ...list] : list
})

// --- lists, per source ---------------------------------------------------------

const lists = reactive<Record<string, IptvList & { fetched?: boolean }>>({})
const opening = reactive(new Set<string>())
const selected = ref('')

const wanted = computed<NetcastHubSource[]>(() =>
  selected.value === EVERY ? openable.value.filter((s) => s.state === 'ready') : openable.value.filter((s) => s.id === selected.value),
)

async function ensureLoaded() {
  error.value = ''
  await Promise.all(
    wanted.value.map(async (s) => {
      if (lists[s.id] || opening.has(s.id)) return
      opening.add(s.id)
      try {
        lists[s.id] = await api.netcast.read(s.id)
      } catch (err) {
        error.value = `${s.name}: ${planErrorMessage(err)}`
      } finally {
        opening.delete(s.id)
      }
    }),
  )
}

function select(id: string) {
  if (selected.value === id) return
  selected.value = id
  void router.replace({ query: { source: id } })
}
watch(selected, () => {
  query.value = ''
  group.value = ALL
  limit.value = PAGE
  shelfLimit.value = SHELVES
  void ensureLoaded()
})

const fetchedNote = computed(() => {
  const s = wanted.value.find((w) => lists[w.id]?.fetched)
  return s ? t('netcast.fetchedNote', { host: s.host ?? s.name }) : ''
})

// The list the IPTV tab can take: only with one source open.
const boxList = computed(() => (wanted.value.length === 1 && canManage.value && !wanted.value[0].noBouquet ? (lists[wanted.value[0].id] ?? null) : null))
const canManage = computed(() => can('channels.manage'))

// --- the rows ------------------------------------------------------------------------

const query = ref('')
const group = ref(ALL)
const limit = ref(PAGE)
const shelfLimit = ref(SHELVES)

const rows = computed<NetcastRow[]>(() =>
  wanted.value.flatMap((s) => {
    const l = lists[s.id]
    return l ? l.channels.map((c) => ({ key: `${s.id}:${c.id}`, sid: s.id, source: s.name, token: l.token, c })) : []
  }),
)
const groups = computed(() => {
  const m = new Map<string, number>()
  for (const r of rows.value) m.set(r.c.group ?? '', (m.get(r.c.group ?? '') ?? 0) + 1)
  return [...m].map(([name, count]) => ({ name, count })).sort((a, b) => a.name.toLowerCase().localeCompare(b.name.toLowerCase()))
})
const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  return rows.value.filter((r) => (group.value === ALL || (r.c.group ?? '') === group.value) && (q === '' || r.c.name.toLowerCase().includes(q) || (r.c.group ?? '').toLowerCase().includes(q)))
})
const shelfMode = computed(() => query.value.trim() === '' && group.value === ALL)
const allShelves = computed(() => {
  const m = new Map<string, NetcastRow[]>()
  for (const r of rows.value) {
    const k = r.c.group ?? ''
    if (!m.has(k)) m.set(k, [])
    m.get(k)!.push(r)
  }
  return [...m].map(([name, rs]) => ({ name, rows: rs })).sort((a, b) => b.rows.length - a.rows.length || a.name.localeCompare(b.name))
})
const shelves = computed(() => allShelves.value.slice(0, shelfLimit.value))
const hiddenShelves = computed(() => Math.max(0, allShelves.value.length - shelfLimit.value))
const gridRows = computed(() => filtered.value.slice(0, limit.value))
watch([query, group], () => (limit.value = PAGE))

// The rows on screen, for the guide.
const shown = computed(() => (shelfMode.value ? shelves.value.flatMap((s) => s.rows.slice(0, SHELF)) : gridRows.value))

// --- the guide: now and next ----------------------------------------------------------

const guide = reactive<Record<string, NetcastNowNext>>({})
const guideBusy = ref(false)
let guideTimer: ReturnType<typeof setTimeout> | undefined
let guideRun = 0

async function refreshGuide(retries = 0) {
  clearTimeout(guideTimer)
  const run = ++guideRun
  const bySource = new Map<string, NetcastRow[]>()
  for (const r of shown.value) {
    if (!bySource.has(r.sid)) bySource.set(r.sid, [])
    bySource.get(r.sid)!.push(r)
  }
  let again = false
  await Promise.all(
    [...bySource.values()].map(async (rs) => {
      for (let i = 0; i < rs.length; i += 400) {
        const part = rs.slice(i, i + 400)
        try {
          const res = await api.netcast.guide(part[0].token, part.map((r) => r.c.id))
          if (run !== guideRun) return
          for (const r of part) {
            const g = res.programmes[String(r.c.id)]
            if (g) guide[r.key] = g
          }
          if (res.loading) again = true
        } catch {
          /* the guide is a bonus: a failed call only leaves tiles without it */
        }
      }
    }),
  )
  if (run !== guideRun) return
  guideBusy.value = again
  const wait = again && retries < 30 ? GUIDE_RETRY_MS : GUIDE_REFRESH_MS
  guideTimer = setTimeout(() => void refreshGuide(again ? retries + 1 : 0), wait)
}

let debounce: ReturnType<typeof setTimeout> | undefined
watch(
  () => shown.value.map((r) => r.key).join('|'),
  () => {
    clearTimeout(debounce)
    debounce = setTimeout(() => void refreshGuide(), 300)
  },
)

// --- playing -------------------------------------------------------------------------

const current = ref<NetcastRow | null>(null)
const starting = ref('')

// Which of a channel's streams (Vavoo keeps several, and not every one plays on
// the box) was picked: remembered in this browser per source and channel.
const VARIANT_KEY = 'dreamhub.netcast.variants'
const variants = reactive<Record<string, number>>(readVariants())
function readVariants(): Record<string, number> {
  try {
    return JSON.parse(localStorage.getItem(VARIANT_KEY) ?? '{}') as Record<string, number>
  } catch {
    return {}
  }
}
const variantKey = (r: NetcastRow) => `${r.sid}:${r.c.name}`
const variantOf = (r: NetcastRow) => variants[variantKey(r)] ?? 0
function nextVariant(r: NetcastRow) {
  const n = r.c.variants?.length ?? 0
  if (n < 2) return
  variants[variantKey(r)] = (variantOf(r) + 1) % n
  try {
    localStorage.setItem(VARIANT_KEY, JSON.stringify(variants))
  } catch {
    /* private mode: the choice just is not remembered */
  }
  void play(r)
}

async function play(r: NetcastRow) {
  if (!canControl.value || starting.value) return
  starting.value = r.key
  error.value = ''
  try {
    await api.netcast.play(r.token, r.c.id, r.c.variants && r.c.variants.length > 1 ? variantOf(r) : undefined)
    current.value = r
  } catch (err) {
    error.value = err instanceof ApiError && err.code === 'iptv_session_gone' ? t('netcast.gone') : planErrorMessage(err)
  } finally {
    starting.value = ''
  }
}

// The channel before or after the playing one, in the list it was picked from.
function step(by: number) {
  const cur = current.value
  if (!cur) return
  const list = shelfMode.value ? rows.value.filter((r) => r.sid === cur.sid && (r.c.group ?? '') === (cur.c.group ?? '')) : filtered.value
  const i = list.findIndex((r) => r.key === cur.key)
  const base = i < 0 ? rows.value.filter((r) => r.sid === cur.sid) : list
  const at = i < 0 ? base.findIndex((r) => r.key === cur.key) : i
  if (base.length === 0) return
  void play(base[(at + by + base.length) % base.length])
}

// The channel list's IPTV tab picks channels and plans the bouquet.
async function toBouquet() {
  if (!boxList.value) return
  offer(boxList.value)
  await router.push({ path: '/channels', query: { tab: 'iptv' } })
}

// --- start ---------------------------------------------------------------------------

let tick: ReturnType<typeof setInterval> | undefined
onMounted(async () => {
  tick = setInterval(() => (nowSec.value = Date.now() / 1000), 30000)
  try {
    sources.value = await api.netcast.sources()
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
  const asked = typeof route.query.source === 'string' ? route.query.source : ''
  const first = openable.value.find((s) => s.state === 'ready') ?? openable.value[0]
  const pick = tabs.value.some((tab) => tab.id === asked) ? asked : (first?.id ?? '')
  if (pick) selected.value = pick
})
onBeforeUnmount(() => {
  clearInterval(tick)
  clearTimeout(guideTimer)
  clearTimeout(debounce)
  guideRun++
})
</script>
