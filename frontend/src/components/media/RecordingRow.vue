<!--
  One entry of a MediaHub folder — DESIGN.md's Row: an optional selection
  box (bulk "move to trash"), title with channel · time · length, size, and
  text actions that appear on hover (always visible below 900 px). A
  non-recording (a manually copied .mkv) is dimmed and labeled, never
  offered file actions.

  Rename expands in place: an input with the current file name, then the
  preview (PlanPreview) listing every file of the recording before → after
  (DESIGN.md: monospace, old struck through, new highlighted). Moving to the
  trash previews the same way; the parent view owns the API calls.
-->
<template>
  <li class="border-b border-line-soft last:border-none" :class="{ 'opacity-60': !movie.isRecording }">
    <div class="group flex items-center gap-3 py-2.5 hover:bg-tile-2">
      <input
        v-if="selectable"
        type="checkbox"
        class="h-4 w-4 shrink-0 accent-[var(--dh-accent)]"
        :checked="selected"
        :aria-label="t('media.select', { title: movie.title })"
        @change="emit('toggle')"
      />
      <span class="min-w-0 flex-1">
        <span class="block truncate text-[14px] text-text">{{ movie.title }}</span>
        <span class="block truncate text-[12.5px] text-muted">
          <template v-if="movie.isRecording">
            <template v-if="movie.serviceName">{{ movie.serviceName }} · </template>
            <template v-if="movie.time > 0">{{ formatTime(movie.time) }} · </template>{{ movie.length }}
          </template>
          <template v-else>{{ t('media.notARecording') }}</template>
        </span>
      </span>
      <span class="shrink-0 whitespace-nowrap font-mono text-[12.5px] tabular-nums text-faint">{{ formatBytes(movie.fileSize, locale) }}</span>
      <template v-if="actions">
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
      </template>
    </div>

    <div v-if="state" class="pb-3">
      <form v-if="state.kind === 'rename' && !state.plan" class="flex flex-wrap items-center gap-2" @submit.prevent="emit('previewRename', draft)">
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
          <p>{{ t('media.renamePreview', { count: state.plan.changes.length }) }}</p>
          <ul class="mt-2 flex flex-col gap-1 font-mono text-[12.5px]">
            <li v-for="(c, i) in state.plan.changes" :key="i" class="break-all">
              <span class="text-faint line-through">{{ fileName(c.before) }}</span>
              <span class="text-faint"> → </span>
              <span class="text-accent">{{ fileName(c.after) }}</span>
            </li>
          </ul>
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
import { formatBytes } from '@/lib/format'
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
}>()
const emit = defineEmits<{
  toggle: []
  rename: []
  trash: []
  previewRename: [newName: string]
  suggest: [source: SuggestionSource]
  confirm: []
  cancel: []
}>()

const { t, locale } = useI18n()

const inputId = `rename-${Math.random().toString(36).slice(2)}`
const nameInput = ref<HTMLInputElement | null>(null)
const baseName = () => fileName(props.movie.filename).replace(/\.ts$/, '')
const draft = ref(baseName())

// Opening the rename form starts from the current name, focused and selected.
watch(
  () => props.state?.kind === 'rename' && !props.state.plan,
  async (open) => {
    if (!open) return
    if (!props.state?.error) draft.value = baseName()
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
