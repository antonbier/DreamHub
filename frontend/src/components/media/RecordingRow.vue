<!--
  One entry of a MediaHub folder — DESIGN.md's Row: an optional selection
  box (bulk "move to trash"), a poster thumbnail (the recording's own cover
  art on the box if there is one, else TMDB's when enrichment is on),
  title with channel · time · length · genres, rating, size, and
  text actions that appear on hover (always visible below 900 px). A
  non-recording (a manually copied .mkv) is shown in `muted` (not by opacity, which
  would drop it below AA contrast) and labeled, never
  offered file actions. A recording the box is writing or playing right
  now (`inUse`) says so with a dot, like TimerHub's state; the parent
  offers it no file actions until it's done.

  The title opens the recording's details (MediaHubView's ProgrammeDialog).
  Rename expands in place: an input with the current file name, then the
  preview (PlanPreview) listing every file of the recording before → after
  (DESIGN.md: monospace, old struck through, new highlighted). Moving to the
  trash previews the same way; the parent view owns the API calls.
-->
<template>
  <li class="border-b border-line-soft last:border-none">
    <div class="group flex items-center gap-3 py-2.5 hover:bg-tile-2 below-sm:flex-wrap below-sm:gap-y-1">
      <input
        v-if="selectable"
        type="checkbox"
        class="h-4 w-4 shrink-0 accent-[var(--dh-accent)]"
        :checked="selected"
        :aria-label="t('media.select', { title: movie.title })"
        @change="emit('toggle')"
      />
      <PosterImage
        v-if="movie.isRecording && (movie.cover || enrich.enabled)"
        :recording="movie.cover ? movie.filename : undefined"
        :path="movie.cover ? undefined : info?.poster"
        :alt="movie.title"
      />
      <span class="min-w-0 flex-1">
        <button
          v-if="movie.isRecording"
          type="button"
          class="block max-w-full truncate text-start text-[14px] text-text hover:text-accent"
          :title="movie.title"
          @click="emit('open')"
        >
          {{ shownTitle }}
        </button>
        <span v-else class="block truncate text-[14px] text-muted" :title="movie.title">{{ shownTitle }}</span>
        <span class="block truncate text-[12.5px] text-muted">
          <template v-if="movie.isRecording">{{ details }}</template>
          <template v-else>{{ t('media.notARecording') }}</template>
        </span>
      </span>
      <span v-if="movie.inUse" class="flex shrink-0 items-center gap-1.5 text-[12.5px]" :class="movie.inUse === 'recording' ? 'text-onair' : 'text-muted'">
        <span class="h-1.5 w-1.5 rounded-full" :class="movie.inUse === 'recording' ? 'bg-onair' : 'bg-accent'" />
        {{ t(`media.inUse.${movie.inUse}`) }}
      </span>
      <RatingBadge v-if="info" class="shrink-0 text-[12.5px] below-sm:hidden" :rating="info.rating" :votes="info.votes" />
      <span class="shrink-0 whitespace-nowrap font-mono text-[12.5px] tabular-nums text-faint">{{ formatBytes(movie.fileSize, locale) }}</span>
      <!-- On phones the actions move to their own line under the title, so
           the title isn't cut down to a couple of letters. -->
      <span v-if="actions" class="flex shrink-0 below-sm:basis-full below-sm:justify-end">
        <button
          type="button"
          class="shrink-0 rounded px-2 py-1 text-[13px] text-accent opacity-0 hover:bg-tile focus:opacity-100 group-hover:opacity-100 below-md:opacity-100"
          :class="{ invisible: state }"
          :disabled="!!state"
          @click="emit('rename')"
        >
          {{ t('media.rename') }}
        </button>
        <button
          type="button"
          class="shrink-0 rounded px-2 py-1 text-[13px] text-accent opacity-0 hover:bg-tile focus:opacity-100 group-hover:opacity-100 below-md:opacity-100"
          :class="{ invisible: state }"
          :disabled="!!state"
          @click="emit('trash')"
        >
          {{ t('media.moveToTrash') }}
        </button>
      </span>
    </div>

    <div v-if="state" class="pb-3">
      <form v-if="state.kind === 'rename' && !state.plan" class="flex flex-wrap items-center gap-2" @submit.prevent="emit('previewRename', draft, changeTitle ? titleDraft : undefined)">
        <label class="sr-only" :for="inputId">{{ t('media.newName') }}</label>
        <input
          :id="inputId"
          ref="nameInput"
          v-model="draft"
          type="text"
          class="min-w-0 flex-1 rounded border border-line bg-field px-2.5 py-1.5 font-mono text-[13px] text-text"
          spellcheck="false"
        />
        <span class="font-mono text-[13px] text-faint">.ts</span>
        <button type="button" class="rounded px-3 py-1.5 text-[13px] text-text hover:bg-field" @click="emit('cancel')">{{ t('plan.cancel') }}</button>
        <button type="submit" class="rounded bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60" :disabled="state.busy || draft.trim() === ''">
          {{ t('media.previewRename') }}
        </button>
        <p v-if="state.error" class="basis-full text-[12.5px] text-warn">{{ state.error }}</p>

        <!-- The title the box's own movie list shows (.ts.meta), optional. -->
        <div class="flex basis-full flex-wrap items-center gap-2">
          <label class="flex items-center gap-2 text-[13px] text-text">
            <input v-model="changeTitle" type="checkbox" class="accent-[var(--dh-accent)]" />
            {{ t('media.changeTitle') }}
          </label>
          <template v-if="changeTitle">
            <label class="sr-only" :for="inputId + '-title'">{{ t('media.newTitle') }}</label>
            <input
              :id="inputId + '-title'"
              v-model="titleDraft"
              type="text"
              maxlength="200"
              class="min-w-0 flex-1 rounded border border-line bg-field px-2.5 py-1.5 text-[13px] text-text"
            />
          </template>
        </div>

        <!-- Name suggestions: each fills the box; the rename still goes
             through the preview. -->
        <div class="flex basis-full flex-wrap items-center gap-1 text-[13px]">
          <span class="me-1 text-faint">{{ t('media.suggest.label') }}</span>
          <button
            v-for="src in sourceList"
            :key="src.id"
            type="button"
            class="rounded px-2 py-1 text-accent hover:bg-tile-2 disabled:text-faint disabled:hover:bg-transparent"
            :disabled="!src.enabled || !!state.suggesting"
            :title="src.enabled ? undefined : t(`media.suggest.needsKey.${src.id}`)"
            @click="emit('suggest', src.id)"
          >
            {{ state.suggesting === src.id ? t('media.suggest.asking') : t(`media.suggest.source.${src.id}`) }}
          </button>
        </div>
        <p v-if="state.suggestError" class="basis-full text-[12.5px] text-warn">{{ state.suggestError }}</p>
        <p v-else-if="state.suggestions && state.suggestions.length === 0" class="basis-full text-[12.5px] text-faint">{{ t('media.suggest.none') }}</p>
        <ul v-else-if="state.suggestions" class="flex basis-full flex-col">
          <li v-for="(sg, i) in state.suggestions" :key="i">
            <button
              type="button"
              class="flex w-full items-baseline gap-2 rounded px-2 py-1 text-start hover:bg-tile-2"
              @click="useSuggestion(sg.name)"
            >
              <span class="min-w-0 flex-1 truncate font-mono text-[13px] text-text">{{ sg.name }}</span>
              <span class="shrink-0 text-[12px] text-faint">
                {{ t(`media.suggest.source.${sg.source}`) }}<template v-if="sg.kind"> · {{ t(`media.suggest.kind.${sg.kind}`) }}</template>
              </span>
            </button>
          </li>
        </ul>
      </form>

      <p v-else-if="state.busy && !state.plan" class="text-[13px] text-faint">{{ t('common.loading') }}</p>

      <PlanPreview
        v-else-if="state.plan"
        :plan="state.plan"
        :confirm-label="state.kind === 'rename' ? t('media.confirmRename') : t('media.confirmTrash')"
        :busy="state.busy"
        :error="state.error"
        @confirm="emit('confirm')"
        @cancel="emit('cancel')"
      >
        <template v-if="state.kind === 'rename'">
          <template v-if="fileChanges.length">
            <p>{{ t('media.renamePreview', { count: fileChanges.length }) }}</p>
            <ul class="mt-2 flex flex-col gap-1 font-mono text-[12.5px]">
              <li v-for="(c, i) in fileChanges" :key="i" class="break-all">
                <span class="text-faint line-through">{{ fileName(c.before) }}</span>
                <span class="text-faint"> → </span>
                <span class="text-accent">{{ fileName(c.after) }}</span>
              </li>
            </ul>
          </template>
          <p v-if="titleChange" :class="{ 'mt-2': fileChanges.length }">
            {{ t('media.titlePreview') }}
            <span class="text-faint line-through">{{ titleChange.before }}</span>
            <span class="text-faint"> → </span>
            <span class="text-accent">{{ titleChange.after }}</span>
          </p>
        </template>
        <p v-else>{{ t('media.trashPreview', { title: movie.title }) }}</p>
      </PlanPreview>

      <div v-else-if="state.error" class="flex items-center gap-3 text-[12.5px] text-warn">
        {{ state.error }}
        <button type="button" class="text-accent hover:underline" @click="emit('cancel')">{{ t('plan.close') }}</button>
      </div>
    </div>
  </li>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import PlanPreview from '@/components/PlanPreview.vue'
import PosterImage from '@/components/enrich/PosterImage.vue'
import RatingBadge from '@/components/enrich/RatingBadge.vue'
import { useEnrichStore } from '@/stores/enrich'
import { formatBytes, formatMinutes } from '@/lib/format'
import { titleFromFileName } from '@/lib/titles'
import type { Movie } from '@/api/types'
import type { RowState, SuggestSources } from './types'
import type { SuggestionSource } from '@/api/types'

const props = defineProps<{
  movie: Movie
  timeZone: string | undefined
  // File actions are offered (DreamHub runs on the box, and it's a recording).
  actions: boolean
  selectable: boolean
  selected: boolean
  state: RowState | null
  // Which name-suggestion sources are set up.
  sources: SuggestSources
  // Ad detection runs for it right now (the background queue).
  adsRunning?: boolean
}>()
const emit = defineEmits<{
  toggle: []
  rename: []
  trash: []
  previewRename: [newName: string, title?: string]
  suggest: [source: SuggestionSource]
  confirm: []
  cancel: []
  // The title was clicked: show the recording's details.
  open: []
}>()

const { t, locale } = useI18n()

// Poster, genres and rating from enrichment (backend/internal/enrich),
// only for real recordings — a copied .mkv's file name isn't a title.
const enrich = useEnrichStore()
const info = computed(() => (props.movie.isRecording ? enrich.info(props.movie.title) : null))
// Without a .ts.meta title the box reports the file name; show its title part.
const shownTitle = computed(() => (props.movie.isRecording ? titleFromFileName(props.movie.title) : props.movie.title))

// channel · time · length · genres · ads, leaving out whatever is unknown.
// Ads only as information here; acting on them is in the detail dialog.
const adsText = computed(() => {
  if (props.adsRunning) return t('media.ads.rowRunning')
  const a = props.movie.ads
  if (!a) return ''
  if (a.trimmed) return t('media.ads.rowTrimmed')
  if (a.marked) return t('media.ads.rowMarked')
  return a.blocks === 0 ? t('media.ads.rowNone') : t('media.ads.rowMinutes', { minutes: formatMinutes(a.seconds, locale.value) })
})
const details = computed(() =>
  [
    props.movie.serviceName,
    props.movie.time > 0 ? formatTime(props.movie.time) : '',
    props.movie.length,
    info.value?.genres?.slice(0, 2).join(', '),
    adsText.value,
  ]
    .filter(Boolean)
    .join(' · '),
)

const inputId = `rename-${Math.random().toString(36).slice(2)}`
const nameInput = ref<HTMLInputElement | null>(null)
const baseName = () => fileName(props.movie.filename).replace(/\.ts$/, '')
const draft = ref(baseName())
const changeTitle = ref(false)
const titleDraft = ref(props.movie.title)

// Opening the rename form starts from the current name, focused and selected.
watch(
  () => props.state?.kind === 'rename' && !props.state.plan,
  async (open) => {
    if (!open) return
    if (!props.state?.error) {
      draft.value = baseName()
      changeTitle.value = false
      titleDraft.value = props.movie.title
    }
    await nextTick()
    nameInput.value?.focus()
    nameInput.value?.select()
  },
)

const sourceList = computed<{ id: SuggestionSource; enabled: boolean }[]>(() => [
  { id: 'metadata', enabled: true },
  { id: 'tmdb', enabled: props.sources.tmdb },
  { id: 'ai', enabled: props.sources.ai },
])

// A rename preview lists every file first, then the title change if any.
const fileChanges = computed(() => props.state?.plan?.changes.filter((c) => c.type === 'file.rename') ?? [])
const titleChange = computed(() => props.state?.plan?.changes.find((c) => c.type === 'meta.title'))

function useSuggestion(name: string) {
  draft.value = name
  nameInput.value?.focus()
}

function fileName(p: unknown): string {
  return typeof p === 'string' ? p.slice(p.lastIndexOf('/') + 1) : ''
}

function formatTime(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short', timeZone: props.timeZone }).format(unixSeconds * 1000)
}
</script>
