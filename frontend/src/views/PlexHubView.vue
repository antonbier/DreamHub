<!--
  PlexHub, "Plex Mediathek" (backend/internal/hubs/plex.go): a Plex
  server's films and series.

  - The status line says which server and whether it answers; offline, the
    library comes from DreamHub's cache ("Plex offline · last synced …")
    and nothing can be played or marked.
  - One library at a time (underline tabs), filtered (text, seen state),
    sorted, as tiles (posters) or as a list — the choice is remembered in
    this browser. Tiles are only here, inside the hub: the dashboard's
    cover row is an optional tile (DESIGN.md).
  - A film opens its details (PlexItemDialog); a series opens its page
    with season tabs and the episode list.
  - Seen state lives in Plex: one film or episode is marked directly, with
    "Undo" in a short notice; a season or series goes through the preview
    (PlanPreview), listing every episode.
  - "Play on the box" hands the file to the box's own player
    through the TV plugin's movie player (see the backend).
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex flex-wrap items-center gap-3">
      <Icon name="library" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ hubTitle('plex', 'hub.plex') }}</h1>
      <div v-if="status?.configured" class="ms-auto flex items-center gap-3 text-[13px]">
        <span class="flex items-center gap-1.5" :class="online ? 'text-muted' : 'text-warn'">
          <span class="h-1.5 w-1.5 rounded-full" :class="online ? 'bg-ok' : 'bg-warn'" />
          <template v-if="online">{{ status.server?.name || t('plex.server') }}</template>
          <template v-else-if="status.error === 'unauthorized'">{{ t('plex.state.unauthorized') }}</template>
          <template v-else>{{ t('plex.state.offline') }}</template>
          <span v-if="syncedAt" class="text-faint">· {{ t('plex.syncedAt', { when: formatSynced(syncedAt) }) }}</span>
        </span>
        <button type="button" class="text-accent hover:underline disabled:text-faint" :disabled="syncing" @click="syncNow">
          {{ syncing ? t('plex.syncing') : t('plex.syncNow') }}
        </button>
      </div>
    </div>

    <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>

    <!-- Not set up: say what it is and where to set it up. -->
    <section v-else-if="status && !status.configured" class="mt-5 rounded border border-line bg-tile p-5">
      <h2 class="text-[15.5px] font-semibold">{{ t('plex.setup.title') }}</h2>
      <p class="mt-1 text-[13.5px] text-muted">{{ t('plex.setup.body') }}</p>
      <RouterLink to="/settings/services#plex" class="mt-3 inline-block rounded bg-accent px-3.5 py-1.5 text-[13.5px] font-semibold text-bg">
        {{ t('plex.setup.action') }}
      </RouterLink>
    </section>

    <p v-else-if="loadError" class="mt-5 text-[14px] text-warn">{{ loadError }}</p>

    <!-- A series: its page. -->
    <template v-else-if="show">
      <button type="button" class="mt-4 flex items-center gap-1 text-[13.5px] text-accent hover:underline" @click="closeShow">
        <Icon name="chevron-left" :size="16" />{{ currentLibrary?.title ?? t('plex.back') }}
      </button>
      <section class="mt-3 flex gap-5 below-sm:flex-col">
        <PosterImage :plex="show.thumb" :plex-width="360" :width="140" :height="210" alt="" />
        <div class="min-w-0 flex-1">
          <PosterImage v-if="show.logo" :plex="show.logo" :plex-width="360" logo :width="220" :height="36" :alt="show.title" class="mb-2" />
          <h2 class="text-[22px] font-bold leading-tight">{{ show.title }}</h2>
          <p class="mt-1 text-[13.5px] text-muted">
            {{ text.details(show) }}
            <span v-if="show.rating" class="ms-1 tabular-nums"><span class="text-warn" aria-hidden="true">★</span> <span class="text-text">{{ text.rating(show.rating) }}</span></span>
          </p>
          <p v-if="show.genres?.length" class="mt-0.5 text-[13px] text-muted">{{ show.genres.join(', ') }}</p>
          <p class="mt-3 line-clamp-5 text-[13.5px] text-text">{{ show.summary }}</p>
          <button
            type="button"
            class="mt-3 rounded px-2 py-1 text-[13.5px] text-accent hover:bg-tile-2 disabled:text-faint"
            :disabled="!online || !!planFor"
            @click="previewWatched(show, !show.watched)"
          >
            {{ show.watched ? t('plex.showUnwatched') : t('plex.showWatched') }}
          </button>
          <span v-if="!online" class="ms-2 text-[12.5px] text-faint">{{ t('plex.offlineAction') }}</span>
        </div>
      </section>
      <div v-if="planFor === show.ratingKey && plan" class="mt-4">
        <PlanPreview :plan="plan" :confirm-label="t('plex.confirmWatched', { count: plan.changes.length }, plan.changes.length)" :busy="applying" :error="planError" @confirm="applyPlan" @cancel="cancelPlan">
          <PlexWatchedPreview :plan="plan" />
        </PlanPreview>
      </div>
      <p v-else-if="planFor === show.ratingKey && planError" class="mt-3 text-[13px] text-warn">{{ planError }}</p>

      <div v-if="seasons.length" class="mt-6 flex items-center gap-1 overflow-x-auto border-b border-line-soft" role="tablist">
        <button
          v-for="s in seasons"
          :key="s.ratingKey"
          type="button"
          role="tab"
          :aria-selected="season?.ratingKey === s.ratingKey"
          class="-mb-px flex shrink-0 items-center gap-1.5 whitespace-nowrap border-b-2 px-3 py-2 text-[14px]"
          :class="season?.ratingKey === s.ratingKey ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
          @click="openSeason(s)"
        >
          {{ s.title }}
          <Icon v-if="s.watched" name="check" :size="14" class="text-ok" />
        </button>
      </div>
      <template v-if="season">
        <div class="mt-3 flex flex-wrap items-center gap-3 text-[13px]">
          <span class="text-muted">{{ t('plex.episodesSeen', { seen: season.viewedLeafCount ?? 0, total: season.leafCount ?? episodes.length }) }}</span>
          <button
            type="button"
            class="rounded px-2 py-1 text-accent hover:bg-tile-2 disabled:text-faint"
            :disabled="!online || !!planFor"
            @click="previewWatched(season, !season.watched)"
          >
            {{ season.watched ? t('plex.seasonUnwatched') : t('plex.seasonWatched') }}
          </button>
        </div>
        <div v-if="planFor === season.ratingKey && plan" class="mt-3">
          <PlanPreview :plan="plan" :confirm-label="t('plex.confirmWatched', { count: plan.changes.length }, plan.changes.length)" :busy="applying" :error="planError" @confirm="applyPlan" @cancel="cancelPlan">
            <PlexWatchedPreview :plan="plan" />
          </PlanPreview>
        </div>
        <p v-else-if="planFor === season.ratingKey && planError" class="mt-3 text-[13px] text-warn">{{ planError }}</p>
        <p v-if="episodesLoading" class="mt-3 text-[13.5px] text-muted">{{ t('common.loading') }}</p>
        <ul v-else class="mt-2 flex flex-col">
          <PlexItemRow
            v-for="ep in episodes"
            :key="ep.ratingKey"
            :item="ep"
            :online="online"
            :can-play="canPlay"
            numbered
            :busy="marking === ep.ratingKey"
            @open="openItem(ep)"
            @toggle-watched="toggleWatched(ep)"
            @play="play(ep)"
          />
        </ul>
      </template>
    </template>

    <!-- A library. -->
    <template v-else-if="status?.configured">
      <div v-if="libraries.length > 1" class="mt-5 flex items-center gap-1 overflow-x-auto border-b border-line-soft" role="tablist">
        <button
          v-for="lib in libraries"
          :key="lib.id"
          type="button"
          role="tab"
          :aria-selected="libraryId === lib.id"
          class="-mb-px shrink-0 whitespace-nowrap border-b-2 px-3 py-2 text-[14px]"
          :class="libraryId === lib.id ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
          @click="selectLibrary(lib.id)"
        >
          {{ lib.title }} <span v-if="lib.count" class="text-faint tabular-nums">{{ lib.count }}</span>
        </button>
      </div>
      <p v-else-if="libraries.length === 0" class="mt-5 text-[14px] text-muted">{{ t('plex.noLibraries') }}</p>

      <div v-if="libraries.length" class="mt-4 flex flex-wrap items-center gap-2">
        <input
          v-model="query"
          type="search"
          :placeholder="t('plex.filterPlaceholder')"
          :aria-label="t('plex.filterPlaceholder')"
          class="w-56 max-w-full rounded border border-line bg-field px-2.5 py-1.5 text-[13px] text-text below-sm:w-full"
        />
        <select v-model="seenFilter" class="rounded border border-line bg-field px-2 py-1.5 text-[13px] text-text" :aria-label="t('plex.filter.label')">
          <option value="all">{{ t('plex.filter.all') }}</option>
          <option value="unwatched">{{ t('plex.filter.unwatched') }}</option>
          <option value="started">{{ t('plex.filter.started') }}</option>
          <option value="watched">{{ t('plex.filter.watched') }}</option>
        </select>
        <select v-model="sort" class="rounded border border-line bg-field px-2 py-1.5 text-[13px] text-text" :aria-label="t('plex.sort.label')">
          <option value="title">{{ t('plex.sort.title') }}</option>
          <option value="added">{{ t('plex.sort.added') }}</option>
          <option value="year">{{ t('plex.sort.year') }}</option>
          <option value="rating">{{ t('plex.sort.rating') }}</option>
        </select>
        <span class="ms-auto text-[12.5px] text-faint tabular-nums">{{ t('plex.count', { n: shownItems.length }, shownItems.length) }}</span>
        <!-- Tiles or list: a small segmented switch. -->
        <div class="flex rounded bg-field p-0.5" role="group" :aria-label="t('plex.view.label')">
          <button
            v-for="v in (['grid', 'list'] as const)"
            :key="v"
            type="button"
            class="flex items-center gap-1.5 rounded px-2 py-1 text-[12.5px]"
            :class="view === v ? 'bg-tile text-text' : 'text-muted hover:text-text'"
            :aria-pressed="view === v"
            @click="view = v"
          >
            <Icon :name="v" :size="15" />{{ t(`plex.view.${v}`) }}
          </button>
        </div>
      </div>

      <p v-if="itemsLoading" class="mt-5 text-[13.5px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="libraries.length && shownItems.length === 0" class="mt-5 text-[13.5px] text-muted">{{ t('plex.empty') }}</p>
      <div v-else-if="view === 'grid'" class="mt-4 grid grid-cols-[repeat(auto-fill,minmax(138px,1fr))] gap-x-4 gap-y-5 below-sm:grid-cols-[repeat(auto-fill,minmax(104px,1fr))] below-sm:gap-x-3">
        <PlexItemCard v-for="it in shownItems" :key="it.ratingKey" :item="it" @open="openItem(it)" />
      </div>
      <ul v-else class="mt-3 flex flex-col">
        <PlexItemRow
          v-for="it in shownItems"
          :key="it.ratingKey"
          :item="it"
          :online="online"
          :can-play="canPlay"
          :busy="marking === it.ratingKey"
          @open="openItem(it)"
          @toggle-watched="it.type === 'show' ? openItem(it) : toggleWatched(it)"
          @play="play(it)"
        />
      </ul>
    </template>

    <PlexItemDialog
      v-if="dialogItem"
      :item="dialogItem"
      :online="online"
      :can-play="canPlay"
      :busy="marking === dialogItem.ratingKey"
      :playing="playingKey === dialogItem.ratingKey"
      :error="dialogError"
      @close="closeDialog"
      @play="play(dialogItem, $event)"
      @toggle-watched="toggleWatched(dialogItem)"
    />

    <!-- A short notice at the bottom, with Undo after marking. -->
    <div
      v-if="notice"
      class="fixed inset-x-0 bottom-5 z-50 mx-auto flex w-fit max-w-[calc(100%-2rem)] items-center gap-3 rounded border border-line bg-tile-2 px-4 py-2.5 text-[13.5px] text-text shadow-lg"
      role="status"
    >
      <span class="min-w-0">{{ notice.text }}</span>
      <button v-if="notice.undo" type="button" class="shrink-0 font-semibold text-accent hover:underline" @click="undo">{{ t('plex.undo') }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useHubLabel } from '@/composables/useHubLabel'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import PosterImage from '@/components/enrich/PosterImage.vue'
import PlexItemCard from '@/components/plex/PlexItemCard.vue'
import PlexItemRow from '@/components/plex/PlexItemRow.vue'
import PlexItemDialog from '@/components/plex/PlexItemDialog.vue'
import PlexWatchedPreview from '@/components/plex/PlexWatchedPreview.vue'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { usePlanError } from '@/composables/usePlanError'
import { usePlexText } from '@/composables/usePlexText'
import { formatWhen } from '@/lib/format'
import type { Plan, PlexItem, PlexLibrary, PlexStatus } from '@/api/types'

const { t, locale } = useI18n()
const { hubTitle } = useHubLabel()
const route = useRoute()
const router = useRouter()
const { timeZone } = useDisplayTimeZone()
const { planErrorMessage } = usePlanError()
const text = usePlexText()

const status = ref<PlexStatus | null>(null)
const loading = ref(true)
const loadError = ref('')
const online = ref(false)
const syncedAt = ref<number | undefined>()
const syncing = ref(false)
const canPlay = computed(() => !!status.value?.canPlay)

function formatSynced(unix: number): string {
  return formatWhen(unix, locale.value, timeZone.value)
}
function noteMeta(meta: { online: boolean; syncedAt?: number }) {
  online.value = meta.online
  if (meta.syncedAt) syncedAt.value = meta.syncedAt
}

// --- per-browser view choices ---
function stored<T extends string>(key: string, fallback: T, allowed: readonly T[]): T {
  try {
    const v = localStorage.getItem(key) as T | null
    return v && allowed.includes(v) ? v : fallback
  } catch {
    return fallback
  }
}
function remember(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    // private window: the choice just isn't kept
  }
}
const view = ref(stored('dreamhub.plex.view', 'grid', ['grid', 'list'] as const))
const sort = ref(stored('dreamhub.plex.sort', 'title', ['title', 'added', 'year', 'rating'] as const))
watch(view, (v) => remember('dreamhub.plex.view', v))
watch(sort, (v) => remember('dreamhub.plex.sort', v))
const seenFilter = ref<'all' | 'unwatched' | 'started' | 'watched'>('all')
const query = ref('')

// --- libraries and items ---
const libraries = ref<PlexLibrary[]>([])
const libraryId = ref('')
const currentLibrary = computed(() => libraries.value.find((l) => l.id === libraryId.value))
const items = ref<PlexItem[]>([])
const itemsLoading = ref(false)

const collator = computed(() => new Intl.Collator(locale.value, { sensitivity: 'base', numeric: true }))
const shownItems = computed(() => {
  const q = query.value.trim().toLowerCase()
  let list = items.value.filter((it) => !q || it.title.toLowerCase().includes(q))
  switch (seenFilter.value) {
    case 'unwatched':
      list = list.filter((it) => !it.watched)
      break
    case 'watched':
      list = list.filter((it) => it.watched)
      break
    case 'started':
      list = list.filter((it) => !it.watched && ((it.viewOffset ?? 0) > 0 || (it.type === 'show' && (it.viewedLeafCount ?? 0) > 0)))
  }
  const by = {
    title: (a: PlexItem, b: PlexItem) => collator.value.compare(a.title, b.title),
    added: (a: PlexItem, b: PlexItem) => (b.addedAt ?? 0) - (a.addedAt ?? 0),
    year: (a: PlexItem, b: PlexItem) => (b.year ?? 0) - (a.year ?? 0) || collator.value.compare(a.title, b.title),
    rating: (a: PlexItem, b: PlexItem) => (b.rating ?? 0) - (a.rating ?? 0) || collator.value.compare(a.title, b.title),
  }[sort.value]
  return [...list].sort(by)
})

async function loadStatus() {
  status.value = await api.plex.status()
  online.value = !!status.value.online
  syncedAt.value = status.value.syncedAt || undefined
}

async function loadLibraries() {
  const res = await api.plex.libraries()
  libraries.value = res.libraries.filter((l) => l.selected)
  noteMeta(res.meta)
  const wanted = typeof route.query.library === 'string' ? route.query.library : ''
  libraryId.value = libraries.value.find((l) => l.id === wanted)?.id ?? libraries.value[0]?.id ?? ''
}

async function loadItems() {
  if (!libraryId.value) {
    items.value = []
    return
  }
  itemsLoading.value = true
  try {
    const res = await api.plex.items(libraryId.value)
    items.value = res.items
    noteMeta(res.meta)
  } catch (err) {
    items.value = []
    loadError.value = planErrorMessage(err)
  } finally {
    itemsLoading.value = false
  }
}

function selectLibrary(id: string) {
  void router.replace({ query: { library: id } })
}

// --- a series' page ---
const show = ref<PlexItem | null>(null)
const seasons = ref<PlexItem[]>([])
const season = ref<PlexItem | null>(null)
const episodes = ref<PlexItem[]>([])
const episodesLoading = ref(false)

async function loadShow(key: string) {
  const res = await api.plex.item(key)
  noteMeta(res.meta)
  if (res.item.type === 'season') {
    // A season link: its series' page on that season.
    await loadShow(res.item.showKey ?? '')
    const s = seasons.value.find((x) => x.ratingKey === key)
    if (s) await openSeason(s, false)
    return
  }
  show.value = res.item
  seasons.value = res.children ?? []
  const wanted = typeof route.query.season === 'string' ? route.query.season : ''
  const first = seasons.value.find((s) => s.ratingKey === wanted) ?? seasons.value.find((s) => !s.watched) ?? seasons.value[0]
  if (first) await openSeason(first, false)
}

async function openSeason(s: PlexItem, updateUrl = true) {
  season.value = s
  if (updateUrl) void router.replace({ query: { ...route.query, season: s.ratingKey } })
  episodesLoading.value = true
  try {
    const res = await api.plex.item(s.ratingKey)
    noteMeta(res.meta)
    season.value = res.item
    episodes.value = res.children ?? []
  } finally {
    episodesLoading.value = false
  }
}

function closeShow() {
  void router.push({ query: { library: libraryId.value || undefined } })
}

// --- details dialog ---
const dialogItem = ref<PlexItem | null>(null)
const dialogError = ref('')

async function openItem(it: PlexItem) {
  if (it.type === 'show' || it.type === 'season') {
    void router.push({ query: { library: libraryId.value || undefined, item: it.ratingKey } })
    return
  }
  dialogError.value = ''
  dialogItem.value = it
  // The full item carries its file's formats.
  try {
    const res = await api.plex.item(it.ratingKey)
    if (dialogItem.value?.ratingKey === it.ratingKey) dialogItem.value = res.item
  } catch {
    // the list's copy is enough to show
  }
}
function closeDialog() {
  dialogItem.value = null
  if (route.query.item && !show.value) void router.replace({ query: { ...route.query, item: undefined } })
}

// --- seen state ---
const marking = ref('')
interface Notice {
  text: string
  undo?: { key: string; watched: boolean }
}
const notice = ref<Notice | null>(null)
let noticeTimer: ReturnType<typeof setTimeout> | undefined
function showNotice(n: Notice) {
  notice.value = n
  clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => (notice.value = null), 8000)
}
onBeforeUnmount(() => clearTimeout(noticeTimer))

function replaceItem(updated: PlexItem) {
  const swap = (list: PlexItem[]) => list.map((x) => (x.ratingKey === updated.ratingKey ? { ...x, ...updated } : x))
  items.value = swap(items.value)
  episodes.value = swap(episodes.value)
  if (dialogItem.value?.ratingKey === updated.ratingKey) dialogItem.value = { ...dialogItem.value, ...updated }
}

async function setWatched(it: PlexItem, watched: boolean, withUndo: boolean) {
  marking.value = it.ratingKey
  dialogError.value = ''
  try {
    const res = await api.plex.setWatched(it.ratingKey, watched)
    replaceItem(res.item)
    showNotice({
      text: t(watched ? 'plex.markedWatched' : 'plex.markedUnwatched', { title: text.title(res.item) }),
      undo: withUndo ? { key: it.ratingKey, watched: !watched } : undefined,
    })
    // The season's and series' counts changed too.
    if (it.type === 'episode') await refreshShow()
  } catch (err) {
    const message = planErrorMessage(err)
    if (dialogItem.value) dialogError.value = message
    else showNotice({ text: message })
  } finally {
    marking.value = ''
  }
}

function toggleWatched(it: PlexItem) {
  void setWatched(it, !it.watched, true)
}

function undo() {
  const u = notice.value?.undo
  if (!u) return
  const it = [...items.value, ...episodes.value].find((x) => x.ratingKey === u.key) ?? dialogItem.value
  if (it) void setWatched(it, u.watched, false)
}

async function refreshShow() {
  if (!show.value) return
  const res = await api.plex.item(show.value.ratingKey)
  show.value = res.item
  seasons.value = res.children ?? []
  if (season.value) {
    const s = await api.plex.item(season.value.ratingKey)
    season.value = s.item
    episodes.value = s.children ?? []
  }
}

// A series or season: preview first.
const plan = ref<Plan | null>(null)
const planFor = ref('')
const planError = ref('')
const applying = ref(false)

async function previewWatched(it: PlexItem, watched: boolean) {
  planFor.value = it.ratingKey
  planError.value = ''
  plan.value = null
  try {
    plan.value = (await api.plex.watchedPlan(it.ratingKey, watched)).plan
  } catch (err) {
    planError.value = planErrorMessage(err)
  }
}
function cancelPlan() {
  plan.value = null
  planFor.value = ''
  planError.value = ''
}
async function applyPlan() {
  if (!plan.value) return
  applying.value = true
  planError.value = ''
  try {
    await api.plans.apply(plan.value.id)
    const count = plan.value.changes.length
    cancelPlan()
    await refreshShow()
    showNotice({ text: t('plex.planApplied', { count }, count) })
  } catch (err) {
    planError.value = planErrorMessage(err)
  } finally {
    applying.value = false
  }
}

// --- play on the box ---
const playingKey = ref('')
async function play(it: PlexItem, choice?: { media: number; part: number }) {
  playingKey.value = it.ratingKey
  dialogError.value = ''
  try {
    const res = await api.plex.play(it.ratingKey, choice)
    showNotice({ text: t('plex.nowPlaying', { title: res.playing }) })
  } catch (err) {
    const message = planErrorMessage(err)
    if (dialogItem.value) dialogError.value = message
    else showNotice({ text: message })
  } finally {
    playingKey.value = ''
  }
}

// --- sync ---
async function syncNow() {
  syncing.value = true
  try {
    status.value = await api.plex.sync()
    online.value = !!status.value.online
    syncedAt.value = status.value.syncedAt || syncedAt.value
    await loadLibraries()
    if (show.value) await refreshShow()
    else await loadItems()
  } catch (err) {
    online.value = false
    showNotice({ text: planErrorMessage(err) })
  } finally {
    syncing.value = false
  }
}

// --- the URL drives what's shown ---
async function applyRoute() {
  loadError.value = ''
  const lib = typeof route.query.library === 'string' ? route.query.library : ''
  if (lib && lib !== libraryId.value && libraries.value.some((l) => l.id === lib)) {
    libraryId.value = lib
    await loadItems()
  }
  const key = typeof route.query.item === 'string' ? route.query.item : ''
  if (!key) {
    show.value = null
    season.value = null
    episodes.value = []
    if (items.value.length === 0) await loadItems()
    return
  }
  if (show.value?.ratingKey === key) return
  try {
    const res = await api.plex.item(key)
    noteMeta(res.meta)
    if (res.item.type === 'movie' || res.item.type === 'episode') {
      show.value = null
      if (items.value.length === 0) await loadItems()
      dialogItem.value = res.item
    } else {
      await loadShow(key)
    }
  } catch (err) {
    loadError.value = planErrorMessage(err)
  }
}

watch(
  () => route.fullPath,
  () => {
    if (route.path === '/plex' && !loading.value && status.value?.configured) void applyRoute()
  },
)

onMounted(async () => {
  try {
    await loadStatus()
    if (status.value?.configured) {
      await loadLibraries()
      await loadItems()
      await applyRoute()
    }
  } catch (err) {
    loadError.value = planErrorMessage(err)
  } finally {
    loading.value = false
  }
})
</script>
