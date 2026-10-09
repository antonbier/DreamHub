<!--
  MediaHub's real screen (ROADMAP.md, "v1 — hubs"; docs/V1_PLAN.md, WP4).

  Recordings are browsed folder by folder (GET /api/v1/media/movies?dir=) —
  the box keeps most of them in subfolders (SerienRecorder's series folders,
  hand-made "Film/…" trees), and walking the whole tree through WebControl
  takes tens of seconds while blocking enigma2 (ARCHITECTURE.md).

  File operations — rename, move to trash, restore, delete for good — each
  go through a preview (PlanPreview) and are refused while a recording is
  being written or played. They only exist when DreamHub runs on the box
  (`fileOps.available`); elsewhere browsing still works and one quiet line
  says why the actions are missing. The trash follows AdvancedMovieSelection's
  convention, so it's the same trash the TV's wastebasket shows.

  The status banner and the content are independent (see HubView): a
  degraded MediaHub (optional TMDB missing) still browses fine.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="recordings" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ t(item ? item.label : 'hub.media') }}</h1>
    </div>

    <div v-if="item && item.status && item.status !== 'available'" class="mt-4 rounded border border-line bg-tile p-4">
      <div class="mb-1.5 flex items-center gap-2 text-[13.5px] font-semibold">
        <span class="h-1.5 w-1.5 rounded-full" :class="item.status === 'degraded' ? 'bg-warn' : 'bg-faint'" />
        {{ t(`status.${item.status}`) }}
      </div>
      <ul class="flex flex-col gap-1 text-[13px] text-muted">
        <li v-for="(p, i) in item.problems ?? []" :key="i">
          {{ describe(p) }}
          <span v-if="describeFix(p)" class="text-faint">{{ ' — ' + describeFix(p) }}</span>
        </li>
      </ul>
    </div>

    <template v-if="!item || item.status !== 'unavailable'">
      <section v-if="disks.length > 0" class="mt-5 flex flex-col gap-3 rounded border border-line bg-tile p-4">
        <div v-for="(disk, i) in disks" :key="i">
          <div class="mb-1 flex items-baseline justify-between gap-2 text-[13px]">
            <span class="text-text">{{ disk.model }}</span>
            <span class="text-muted">{{ t('media.diskFree', { free: disk.freeText, capacity: disk.capacityText }) }}</span>
          </div>
          <div class="h-1.5 overflow-hidden rounded-full bg-line-soft">
            <div
              class="h-full rounded-full"
              :class="disk.usedPercent != null && disk.usedPercent >= 90 ? 'bg-warn' : 'bg-accent'"
              :style="{ width: (disk.usedPercent ?? 0) + '%' }"
            />
          </div>
        </div>
      </section>

      <div class="mt-5 flex items-center gap-1 border-b border-line-soft" role="tablist">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          type="button"
          role="tab"
          :aria-selected="activeTab === tab.id"
          class="-mb-px border-b-2 px-3 py-2 text-[14px]"
          :class="activeTab === tab.id ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
          @click="switchTab(tab.id)"
        >
          {{ tab.label }}
        </button>
      </div>

      <p v-if="notice" class="mt-4 text-[13.5px] text-ok" role="status">{{ notice }}</p>

      <!-- Recordings, one folder at a time -->
      <template v-if="activeTab === 'recordings'">
        <nav v-if="folder && breadcrumbs.length > 1" class="mt-4 flex flex-wrap items-center gap-1 text-[13.5px]" :aria-label="t('media.folderPath')">
          <template v-for="(crumb, i) in breadcrumbs" :key="crumb.path">
            <span v-if="i > 0" class="text-faint">/</span>
            <button
              v-if="i < breadcrumbs.length - 1"
              type="button"
              class="rounded px-1 text-accent hover:bg-tile-2"
              @click="open(crumb.path)"
            >
              {{ crumb.name }}
            </button>
            <span v-else class="px-1 font-semibold text-text">{{ crumb.name }}</span>
          </template>
        </nav>

        <p v-if="folder && !folder.fileOps.available && can('recordings.manage')" class="mt-2 text-[13px] text-faint">{{ t(`media.fileOps.${folder.fileOps.reason ?? 'notOnBox'}`) }}</p>

        <div v-if="selection.size > 0" class="mt-3 flex flex-wrap items-center gap-3 rounded border border-line bg-tile px-3 py-2 text-[13.5px]">
          <span class="text-text">{{ t('media.selected', { count: selection.size }, selection.size) }}</span>
          <button type="button" class="rounded px-2 py-1 text-accent hover:bg-tile-2" @click="selection.clear()">{{ t('media.clearSelection') }}</button>
          <button
            v-if="selection.size < selectableCount"
            type="button"
            class="rounded px-2 py-1 text-accent hover:bg-tile-2"
            @click="selectAll"
          >
            {{ t('media.selectAll') }}
          </button>
          <button
            type="button"
            class="ms-auto rounded px-3 py-1.5 text-accent hover:bg-tile-2 disabled:opacity-60"
            :disabled="bulk?.busy || batchOpen || coversOpen"
            @click="openBatch"
          >
            {{ t('media.batch.open') }}
          </button>
          <button
            v-if="suggestSources.tmdb"
            type="button"
            class="rounded px-3 py-1.5 text-accent hover:bg-tile-2 disabled:opacity-60"
            :disabled="bulk?.busy || batchOpen || coversOpen"
            @click="openCovers"
          >
            {{ t('media.covers.open') }}
          </button>
          <button
            v-if="adScanOn"
            type="button"
            class="rounded px-3 py-1.5 text-accent hover:bg-tile-2 disabled:opacity-60"
            :disabled="bulk?.busy || batchOpen || coversOpen || adsStarting"
            @click="startBatchAds"
          >
            {{ t('media.ads.detect') }}
          </button>
          <button type="button" class="rounded bg-accent px-3 py-1.5 font-semibold text-bg disabled:opacity-60" :disabled="bulk?.busy || batchOpen || coversOpen" @click="previewBulkTrash">
            {{ t('media.moveSelectedToTrash') }}
          </button>
        </div>
        <!-- Ad detection over a selection runs in the background: its
             progress stays here, even after the selection is cleared. -->
        <div v-if="adsBatch" class="mt-3 flex flex-wrap items-center gap-3 rounded border border-line-soft px-3 py-2 text-[13.5px]" aria-live="polite">
          <span class="text-text">{{ adsBatchText }}</span>
          <button v-if="adsBatchProgress.active > 0" type="button" class="ms-auto rounded px-2 py-1 text-accent hover:bg-tile-2" @click="stopBatchAds">
            {{ t('media.ads.stop') }}
          </button>
          <button v-else type="button" class="ms-auto rounded px-2 py-1 text-accent hover:bg-tile-2" @click="adsBatch = null">
            {{ t('media.ads.dismiss') }}
          </button>
        </div>
        <BatchRenamePreview
          v-if="batchOpen"
          class="mt-3"
          :paths="[...selection]"
          :tmdb="suggestSources.tmdb"
          @cancel="batchOpen = false"
          @done="onBatchDone"
        />
        <SaveCoversPreview v-if="coversOpen" class="mt-3" :paths="[...selection]" @cancel="coversOpen = false" @done="onCoversDone" />
        <div v-if="bulk" class="mt-3">
          <p v-if="bulk.busy && !bulk.plan" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
          <PlanPreview
            v-else-if="bulk.plan"
            :plan="bulk.plan"
            :confirm-label="t('media.confirmTrash')"
            :busy="bulk.busy"
            :error="bulk.error"
            @confirm="confirmBulk"
            @cancel="bulk = null"
          >
            {{ t('media.bulkTrashPreview', { count: bulk.plan.changes.length }, bulk.plan.changes.length) }}
          </PlanPreview>
          <p v-else-if="bulk.error" class="text-[12.5px] text-warn">{{ bulk.error }}</p>
        </div>

        <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
        <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted"><ErrorText /></p>
        <template v-else-if="folder">
          <ul v-if="folder.subfolders.length > 0" class="mt-4 flex flex-col">
            <li v-for="sub in folder.subfolders" :key="sub.path" class="border-b border-line-soft last:border-none">
              <button type="button" class="flex w-full items-center gap-3 py-2 text-start hover:bg-tile-2" @click="open(sub.path)">
                <Icon name="folder" :size="18" class="shrink-0 text-muted" />
                <span class="min-w-0 flex-1 truncate text-[14px] text-text">{{ sub.name }}</span>
              </button>
            </li>
          </ul>
          <p v-if="folder.movies.length === 0 && folder.subfolders.length === 0" class="mt-5 text-[14px] text-muted">{{ t('media.empty') }}</p>
          <ul v-if="folder.movies.length > 0" class="mt-4 flex flex-col">
            <RecordingRow
              v-for="movie in folder.movies"
              :key="movie.filename"
              :movie="movie"
              :time-zone="timeZone"
              :actions="fileOpsAvailable && movie.isRecording && !movie.inUse"
              :selectable="fileOpsAvailable && movie.isRecording && !movie.inUse"
              :selected="selection.has(movie.filename)"
              :state="rowStates[movie.filename] ?? null"
              :ads-running="adsRunning(movie.filename)"
              :sources="suggestSources"
              @toggle="toggle(movie.filename)"
              @open="openMovie = movie"
              @rename="rowStates[movie.filename] = { kind: 'rename', busy: false }"
              @trash="previewTrash(movie)"
              @preview-rename="(name: string, title?: string) => previewRename(movie, name, title)"
              @suggest="(source: SuggestionSource) => suggest(movie, source)"
              @confirm="confirmRow(movie)"
              @cancel="delete rowStates[movie.filename]"
            />
          </ul>
        </template>
      </template>

      <!-- Trash -->
      <template v-else>
        <p class="mt-4 text-[13px] text-muted">{{ t('media.trashExplain') }}</p>
        <p v-if="trashLoading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
        <p v-else-if="trashError" class="mt-5 text-[14px] text-muted">{{ trashError }}</p>
        <p v-else-if="trash.length === 0" class="mt-5 text-[14px] text-muted">{{ t('media.trashEmpty') }}</p>
        <template v-else>
          <div class="mt-3 flex items-center gap-3 text-[13px]">
            <span class="text-muted">{{ t('media.trashTotal', { count: trash.length, size: formatBytes(trashTotal, locale) }, trash.length) }}</span>
            <button type="button" class="ms-auto rounded px-2 py-1 text-accent hover:bg-tile-2" @click="previewTrashAction('empty', trash.map((x) => x.path))">
              {{ t('media.emptyTrash') }}
            </button>
          </div>
          <div v-if="trashAction && trashAction.all" class="mt-3">
            <TrashPreview :state="trashAction" @confirm="confirmTrashAction" @cancel="trashAction = null" />
          </div>
          <ul class="mt-3 flex flex-col">
            <li v-for="entry in trash" :key="entry.path" class="border-b border-line-soft last:border-none">
              <div class="group flex items-center gap-3 py-2.5 hover:bg-tile-2">
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-[14px] text-text">{{ entry.title }}</span>
                  <span class="block truncate text-[12.5px] text-muted">{{ folderOf(entry.originalPath) }}</span>
                </span>
                <span class="shrink-0 whitespace-nowrap font-mono text-[12.5px] tabular-nums text-faint">{{ formatBytes(entry.size, locale) }}</span>
                <button
                  type="button"
                  class="shrink-0 rounded px-2 py-1 text-[13px] text-accent hover:bg-tile"
                  :class="{ invisible: trashAction }"
                  @click="previewTrashAction('restore', [entry.path])"
                >
                  {{ t('media.restore') }}
                </button>
                <button
                  type="button"
                  class="shrink-0 rounded px-2 py-1 text-[13px] text-onair hover:bg-tile"
                  :class="{ invisible: trashAction }"
                  @click="previewTrashAction('empty', [entry.path])"
                >
                  {{ t('media.deleteForGood') }}
                </button>
              </div>
              <div v-if="trashAction && !trashAction.all && trashAction.paths[0] === entry.path" class="pb-3">
                <TrashPreview :state="trashAction" @confirm="confirmTrashAction" @cancel="trashAction = null" />
              </div>
            </li>
          </ul>
        </template>
      </template>
    </template>
    <ProgrammeDialog
      v-if="openMovie"
      :title="titleFromFileName(openMovie.title)"
      :service-name="openMovie.serviceName"
      :start="openMovie.time"
      :duration="0"
      when=""
      :now="0"
      :subtitle="movieSubtitle(openMovie)"
      :info="enrich.info(openMovie.title)"
      :recording="openMovie.cover ? openMovie.filename : undefined"
      @close="openMovie = null"
    >
      <section v-if="openMovie.description || openMovie.descriptionExtended" class="mt-4 rounded border border-line-soft bg-bg p-3">
        <h3 class="text-[12.5px] text-faint">{{ t('media.detail.fromRecording') }}</h3>
        <p v-if="openMovie.description" class="mt-1 text-[14px] text-text">{{ openMovie.description }}</p>
        <p v-if="openMovie.descriptionExtended" class="mt-1 whitespace-pre-line text-[13.5px] text-muted">{{ openMovie.descriptionExtended }}</p>
      </section>
      <AdScanPanel
        v-if="openMovie.isRecording && (folder?.adScan || openMovie.ads)"
        :path="openMovie.filename"
        :title="openMovie.title"
        :channel="openMovie.serviceName"
        :length="openMovie.length"
        :can-start="adScanOn && openMovie.inUse !== 'recording'"
        :can-play="streamHubOn"
        @analysed="(sum: AdSummary) => (openMovie!.ads = sum)"
      />
      <template #actions>
        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="rounded bg-accent px-3.5 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-60"
            :disabled="playing"
            @click="playOnTv(openMovie)"
          >
            {{ played ? t('media.detail.playing') : t('media.detail.play') }}
          </button>
          <RouterLink
            v-if="streamHubOn && openMovie.filename.endsWith('.ts')"
            :to="{ path: '/stream', query: { recording: openMovie.filename, title: openMovie.title, length: openMovie.length, channel: openMovie.serviceName } }"
            class="rounded border border-line px-3.5 py-1.5 text-[13.5px] text-text hover:bg-field"
          >
            {{ t('media.detail.watchInBrowser') }}
          </RouterLink>
        </div>
        <p v-if="playError" class="mt-2 text-[12.5px] text-warn">{{ playError }}</p>
      </template>
    </ProgrammeDialog>
  </div>
</template>

<script setup lang="ts">
import ErrorText from '@/components/ErrorText.vue'
import { useAuth } from '@/composables/useAuth'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import AdScanPanel from '@/components/media/AdScanPanel.vue'
import RecordingRow from '@/components/media/RecordingRow.vue'
import TrashPreview from '@/components/media/TrashPreview.vue'
import BatchRenamePreview from '@/components/media/BatchRenamePreview.vue'
import SaveCoversPreview from '@/components/media/SaveCoversPreview.vue'
import ProgrammeDialog from '@/components/programme/ProgrammeDialog.vue'
import { titleFromFileName } from '@/lib/titles'
import { useEnrichStore } from '@/stores/enrich'
import type { RowState, SuggestSources, TrashActionState } from '@/components/media/types'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { usePlanError } from '@/composables/usePlanError'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { formatBytes } from '@/lib/format'
import type { AdSummary, Disk, MediaFolder, Movie, Plan, SuggestionSource, TrashItem } from '@/api/types'
import { ACTIVE, cleanPath, useJobsStore } from '@/stores/jobs'

const { t, locale } = useI18n()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()
const { planErrorMessage } = usePlanError()
const { timeZone } = useDisplayTimeZone()
const enrich = useEnrichStore()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/media'))
// "Watch in the browser" is StreamHub's; offered while it's there.
const streamHubOn = computed(() => {
  const s = navigation.navigation?.sections.flatMap((x) => x.items).find((i) => i.id === 'stream')
  return !!s && s.status !== 'unavailable'
})

const disks = ref<Disk[]>([])
const folder = ref<MediaFolder | null>(null)
const loading = ref(true)
const loadFailed = ref(false)
const notice = ref('')
// Renaming and the trash need the box's files and the profile's permission
// (a viewer just watches; docs/PROFILES_PLAN.md).
const { can } = useAuth()
const fileOpsAvailable = computed(() => can('recordings.manage') && (folder.value?.fileOps.available ?? false))

// --- ad detection (docs/ADSCAN_PLAN.md) --------------------------------------

const jobs = useJobsStore()
// "Detect ads" is offered where it can run and the profile may start it.
const adScanOn = computed(() => can('recordings.manage') && (folder.value?.adScan ?? false))
const adsStarting = ref(false)
// The last batch started from the selection: its jobs' progress.
const adsBatch = ref<{ jobIds: number[]; skipped: number } | null>(null)

function adsRunning(path: string) {
  const j = jobs.latest('media.adscan', path)
  return !!j && ACTIVE.includes(j.state)
}

const adsBatchProgress = computed(() => {
  const ids = adsBatch.value?.jobIds ?? []
  let done = 0
  let active = 0
  let failed = 0
  for (const id of ids) {
    const state = jobs.byId[id]?.state ?? 'queued'
    if (state === 'done') done++
    else if (ACTIVE.includes(state)) active++
    else failed++
  }
  return { done, active, failed, total: ids.length }
})
const adsBatchText = computed(() => {
  const p = adsBatchProgress.value
  const skipped = adsBatch.value?.skipped ?? 0
  const parts: string[] = []
  if (p.active > 0) {
    parts.push(t('media.ads.batchRunning', { done: p.done, total: p.total }))
    if (jobs.blockedBy) parts.push(t(`media.ads.waiting.${jobs.blockedBy}`))
  } else {
    parts.push(t('media.ads.batchDone', { count: p.done }, p.done))
  }
  if (p.failed > 0) parts.push(t('media.ads.batchFailed', { count: p.failed }, p.failed))
  if (skipped > 0) parts.push(t('media.ads.batchSkipped', { count: skipped }, skipped))
  return parts.join(' · ')
})

async function startBatchAds() {
  adsStarting.value = true
  try {
    const res = await api.media.adScanStart([...selection])
    const jobIds: number[] = []
    let skipped = 0
    for (const item of res.items) {
      if (!item.jobId) {
        skipped++
        continue
      }
      jobIds.push(item.jobId)
      if (!jobs.byId[item.jobId]) {
        jobs.apply({ id: item.jobId, kind: 'media.adscan', key: item.path, ownerId: 0, state: 'queued', done: 0, total: 0, createdAt: new Date().toISOString() })
      }
    }
    adsBatch.value = { jobIds, skipped }
    selection.clear()
  } catch {
    notice.value = t('media.ads.error.generic')
  } finally {
    adsStarting.value = false
  }
}

function stopBatchAds() {
  for (const id of adsBatch.value?.jobIds ?? []) {
    const state = jobs.byId[id]?.state
    if (state && ACTIVE.includes(state)) void jobs.cancel(id)
  }
}

// A recording in this folder finished its analysis: show its summary in
// the row without reloading the folder (the box's movie list is slow).
watch(
  () =>
    Object.values(jobs.byId)
      .filter((j) => j.kind === 'media.adscan' && j.state === 'done')
      .map((j) => j.id),
  async (doneIds, before) => {
    const fresh = doneIds.filter((id) => !before?.includes(id))
    for (const id of fresh) {
      const path = jobs.byId[id]?.key
      const movie = folder.value?.movies.find((m) => cleanPath(m.filename) === path)
      if (!movie || !path) continue
      try {
        const res = await api.media.adScanResult(path)
        const blocks = res.result.blocks
        movie.ads = {
          blocks: blocks.length,
          seconds: blocks.reduce((s, b) => s + (b.end - b.start), 0),
          uncertain: blocks.filter((b) => b.uncertain).length || undefined,
        }
      } catch {
        // The row keeps what it had.
      }
    }
  },
)

// --- tabs -------------------------------------------------------------------

type TabId = 'recordings' | 'trash'
const activeTab = ref<TabId>('recordings')
const tabs = computed(() => {
  const list: { id: TabId; label: string }[] = [{ id: 'recordings', label: t('media.tabRecordings') }]
  // The trash is a file-system view: only where file operations work.
  if (fileOpsAvailable.value) list.push({ id: 'trash', label: t('media.tabTrash') })
  return list
})
function switchTab(id: TabId) {
  activeTab.value = id
  notice.value = ''
  if (id === 'trash') void loadTrash()
}

// --- folders ----------------------------------------------------------------

// Breadcrumbs from the root folder down to the open one; the root is shown
// as "Recordings" rather than its path.
const breadcrumbs = computed(() => {
  const f = folder.value
  if (!f) return []
  const crumbs = [{ path: f.root, name: t('media.rootFolder') }]
  let path = f.root
  for (const part of f.dir.slice(f.root.length).split('/').filter(Boolean)) {
    path += part + '/'
    crumbs.push({ path, name: part })
  }
  return crumbs
})

let loadToken = 0
async function open(dir?: string) {
  const token = ++loadToken
  loading.value = true
  selection.clear()
  bulk.value = null
  batchOpen.value = false
  coversOpen.value = false
  Object.keys(rowStates).forEach((k) => delete rowStates[k])
  try {
    const res = await api.media.folder(dir)
    if (token !== loadToken) return
    folder.value = res
    loadFailed.value = false
  } catch {
    if (token === loadToken) loadFailed.value = true
  } finally {
    if (token === loadToken) loading.value = false
  }
}

function folderOf(p: string): string {
  const root = folder.value?.root ?? ''
  const dir = p.slice(0, p.lastIndexOf('/') + 1)
  const rel = root && dir.startsWith(root) ? dir.slice(root.length) : dir
  return t('media.inFolder', { folder: rel === '' ? t('media.rootFolder') : rel.replace(/\/$/, '') })
}

// --- row actions: rename, trash ------------------------------------------------

const rowStates = reactive<Record<string, RowState>>({})

async function previewRename(movie: Movie, newName: string, title?: string) {
  const key = movie.filename
  // Keep the suggestions: if the preview is refused, they're still there.
  const suggestions = rowStates[key]?.suggestions
  rowStates[key] = { kind: 'rename', busy: true, suggestions }
  try {
    const res = await api.media.renamePlan(movie.filename, newName, title)
    rowStates[key] = { kind: 'rename', plan: res.plan, busy: false }
  } catch (err) {
    rowStates[key] = { kind: 'rename', busy: false, error: planErrorMessage(err), suggestions }
  }
}

// --- name suggestions (decision D2) ---------------------------------------------

// TMDB and the AI provider need their keys; until then their buttons are
// disabled with a hint (checked once, from the connector states).
const suggestSources = ref<SuggestSources>({ tmdb: false, ai: false })

async function suggest(movie: Movie, source: SuggestionSource) {
  const key = movie.filename
  const state = rowStates[key]
  if (!state) return
  rowStates[key] = { ...state, suggesting: source, suggestError: undefined }
  try {
    const res = await api.media.suggestName(movie.filename, source)
    rowStates[key] = { ...rowStates[key], suggesting: undefined, suggestions: res.suggestions }
  } catch (err) {
    rowStates[key] = { ...rowStates[key], suggesting: undefined, suggestions: undefined, suggestError: planErrorMessage(err) }
  }
}

async function previewTrash(movie: Movie) {
  const key = movie.filename
  rowStates[key] = { kind: 'trash', busy: true }
  try {
    const res = await api.media.trashPlan([movie.filename])
    rowStates[key] = { kind: 'trash', plan: res.plan, busy: false }
  } catch (err) {
    rowStates[key] = { kind: 'trash', busy: false, error: planErrorMessage(err) }
  }
}

async function confirmRow(movie: Movie) {
  const key = movie.filename
  const state = rowStates[key]
  if (!state?.plan) return
  rowStates[key] = { ...state, busy: true, error: undefined }
  try {
    await api.plans.apply(state.plan.id)
    notice.value = state.kind === 'rename' ? t('media.renamed', { title: movie.title }) : t('media.trashed', { title: movie.title })
    await open(folder.value?.dir)
  } catch (err) {
    // A failed or stale plan is used up; the user starts over.
    rowStates[key] = { kind: state.kind, busy: false, error: planErrorMessage(err) }
  }
}

// --- one recording's details, and playing it on the TV ----------------------

const openMovie = ref<Movie | null>(null)
const playing = ref(false)
const played = ref(false)
const playError = ref('')
watch(openMovie, () => {
  played.value = false
  playError.value = ''
})
// channel · date and time · length, leaving out what's unknown.
function movieSubtitle(m: Movie): string {
  const when = m.time > 0 ? new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short', timeZone: timeZone.value }).format(m.time * 1000) : ''
  return [m.serviceName, when, m.length ? t('media.detail.length', { length: m.length }) : ''].filter(Boolean).join(' · ')
}
// Playing is a zap to the recording's own reference — a live input like
// the remote (ControlHub), not a change to anything, so no preview.
async function playOnTv(m: Movie) {
  playing.value = true
  playError.value = ''
  try {
    await api.control.zap(m.serviceReference)
    played.value = true
  } catch (err) {
    playError.value = planErrorMessage(err)
  } finally {
    playing.value = false
  }
}

// --- bulk: move the selection to the trash ------------------------------------

const selection = reactive(new Set<string>())
function toggle(filename: string) {
  if (selection.has(filename)) selection.delete(filename)
  else selection.add(filename)
}

// "Select all": every recording in this folder that file actions apply to.
const selectableCount = computed(() => folder.value?.movies.filter((m) => m.isRecording && !m.inUse).length ?? 0)
function selectAll() {
  for (const m of folder.value?.movies ?? []) if (m.isRecording && !m.inUse) selection.add(m.filename)
}

// --- bulk: rename the selection by the naming scheme ---------------------------
const batchOpen = ref(false)
function openBatch() {
  bulk.value = null
  coversOpen.value = false
  batchOpen.value = true
}
async function onBatchDone(count: number) {
  batchOpen.value = false
  notice.value = t('media.batch.done', { n: count }, count)
  await open(folder.value?.dir)
}

// --- bulk: save covers next to the selection ---------------------------------
const coversOpen = ref(false)
function openCovers() {
  bulk.value = null
  batchOpen.value = false
  coversOpen.value = true
}
async function onCoversDone(count: number) {
  coversOpen.value = false
  notice.value = t('media.covers.done', { n: count }, count)
  await open(folder.value?.dir)
}

const bulk = ref<{ plan?: Plan; busy: boolean; error?: string } | null>(null)
async function previewBulkTrash() {
  bulk.value = { busy: true }
  try {
    bulk.value = { plan: (await api.media.trashPlan([...selection])).plan, busy: false }
  } catch (err) {
    bulk.value = { busy: false, error: planErrorMessage(err) }
  }
}
async function confirmBulk() {
  const plan = bulk.value?.plan
  if (!plan) return
  bulk.value = { plan, busy: true }
  try {
    await api.plans.apply(plan.id)
    notice.value = t('media.bulkTrashed', { count: plan.changes.length }, plan.changes.length)
    await open(folder.value?.dir)
  } catch (err) {
    bulk.value = { busy: false, error: planErrorMessage(err) }
  }
}

// --- trash tab --------------------------------------------------------------------

const trash = ref<TrashItem[]>([])
const trashLoading = ref(false)
const trashError = ref('')
const trashTotal = computed(() => trash.value.reduce((sum, x) => sum + x.size, 0))
const trashAction = ref<TrashActionState | null>(null)

async function loadTrash() {
  trashLoading.value = true
  trashError.value = ''
  try {
    trash.value = (await api.media.trash()).items
  } catch (err) {
    trashError.value = planErrorMessage(err)
  } finally {
    trashLoading.value = false
  }
}

async function previewTrashAction(kind: 'restore' | 'empty', paths: string[]) {
  const all = paths.length > 1
  trashAction.value = { kind, paths, all, busy: true }
  try {
    if (kind === 'restore') {
      const res = await api.media.restorePlan(paths)
      trashAction.value = { kind, paths, all, busy: false, plan: res.plan }
    } else {
      const res = await api.media.emptyPlan(paths)
      trashAction.value = { kind, paths, all, busy: false, plan: res.plan, bytes: res.bytes }
    }
  } catch (err) {
    trashAction.value = { kind, paths, all, busy: false, error: planErrorMessage(err) }
  }
}

async function confirmTrashAction() {
  const state = trashAction.value
  if (!state?.plan) return
  trashAction.value = { ...state, busy: true, error: undefined }
  try {
    await api.plans.apply(state.plan.id)
    notice.value = state.kind === 'restore' ? t('media.restored') : t('media.deleted', { count: state.paths.length }, state.paths.length)
    trashAction.value = null
    await loadTrash()
  } catch (err) {
    trashAction.value = { ...state, busy: false, plan: undefined, error: planErrorMessage(err) }
  }
}

// ?dir= opens a folder directly — search results and custom tiles link a
// recording's folder this way.
const route = useRoute()
const queryDir = () => (typeof route.query.dir === 'string' && route.query.dir ? route.query.dir : undefined)
watch(queryDir, (dir) => {
  if (dir) void open(dir)
})

onMounted(async () => {
  void open(queryDir())
  api
    .connectors()
    .then((res) => {
      const ok = (id: string) => res.connectors.some((c) => c.id === id && c.state === 'ok')
      suggestSources.value = { tmdb: ok('tmdb'), ai: ok('openai') }
    })
    .catch(() => {
      // Unknown: leave TMDB/AI off; the metadata source always works.
    })
  try {
    disks.value = (await api.media.disk()).disks
  } catch {
    // Disk usage is a bonus; the folder listing reports reachability.
  }
})
</script>
