<!--
  The custom tile builder (DESIGN mockup: "Eigene Kachel · Ansicht mit
  Kriterien"): name, source, display, the conditions the chosen source
  supports (GET /api/v1/dashboard/sources lists them per source, so nothing
  offered here would silently do nothing), sort and limit. A live match
  count under the form shows what the tile would find before it's saved.
  Saving is direct, like the rest of the dashboard layout — DreamHub's own
  state, not a device change. A dialog, so it traps nothing but focus: the
  name field gets focus on open, Escape and the backdrop cancel, focus
  returns to whatever opened it.
-->
<template>
  <div class="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/50 px-4 py-[8vh]" @click.self="$emit('close')">
    <form
      ref="dialogEl"
      role="dialog"
      aria-modal="true"
      aria-labelledby="builder-title"
      class="w-full max-w-[520px] rounded-lg border border-line bg-tile p-5 shadow-lg"
      @submit.prevent="save"
    >
      <p class="text-[12px] text-faint">{{ t('dashboard.builder.eyebrow') }}</p>
      <h2 id="builder-title" class="mb-4 text-[21px] font-bold">{{ t('dashboard.builder.title') }}</h2>

      <p v-if="sourcesFailed" class="text-[13.5px] text-muted">{{ t('dashboard.builder.sourcesFailed') }}</p>
      <p v-else-if="sources.length === 0" class="text-[13.5px] text-muted">{{ loadingSources ? t('common.loading') : t('dashboard.builder.noSources') }}</p>
      <div v-else class="flex flex-col gap-3.5">
        <label class="flex flex-col gap-1">
          <span :class="labelClass">{{ t('dashboard.builder.name') }}</span>
          <input ref="nameInput" v-model="title" type="text" maxlength="60" required :class="field" />
        </label>

        <div class="grid grid-cols-2 gap-3 below-sm:grid-cols-1">
          <label class="flex flex-col gap-1">
            <span :class="labelClass">{{ t('dashboard.builder.source') }}</span>
            <select v-model="sourceId" :class="field">
              <option v-for="s in sources" :key="s.id" :value="s.id">{{ s.name ? `${t(s.label)}: ${s.name}` : t(s.label) }}</option>
            </select>
          </label>
          <label class="flex flex-col gap-1">
            <span :class="labelClass">{{ t('dashboard.builder.display') }}</span>
            <select v-model="display" :class="field">
              <option value="list">{{ t('dashboard.builder.displayList') }}</option>
              <option value="count">{{ t('dashboard.builder.displayCount') }}</option>
              <option v-if="hasPictures" value="posters">{{ t('dashboard.builder.displayPosters') }}</option>
            </select>
          </label>
        </div>

        <fieldset class="flex flex-col gap-2.5 rounded-sm border border-line-soft p-3">
          <legend class="px-1 text-[13px] text-muted">{{ t('dashboard.builder.conditions') }}</legend>

          <label v-if="has('bouquet')" :class="row">
            <span :class="rowLabel">{{ t('dashboard.criteriaField.bouquet') }}</span>
            <select v-model="q.bouquet" :class="field">
              <option value="">{{ t('dashboard.builder.firstBouquet') }}</option>
              <option v-for="b in bouquets" :key="b.serviceReference" :value="b.serviceReference">{{ b.serviceName }}</option>
            </select>
          </label>
          <label v-if="has('text')" :class="row">
            <span :class="rowLabel">{{ t('dashboard.criteriaField.text') }}</span>
            <input v-model.trim="q.text" type="text" maxlength="100" :class="field" />
          </label>
          <label v-if="has('olderThanDays')" :class="row">
            <span :class="rowLabel">{{ t('dashboard.criteriaField.olderThanDays') }}</span>
            <input v-model.number="q.olderThanDays" type="number" min="0" max="3650" :class="[field, 'w-24']" />
          </label>
          <label v-if="has('newerThanDays')" :class="row">
            <span :class="rowLabel">{{ t('dashboard.criteriaField.newerThanDays') }}</span>
            <input v-model.number="q.newerThanDays" type="number" min="0" max="3650" :class="[field, 'w-24']" />
          </label>
          <label v-if="has('minSizeGB')" :class="row">
            <span :class="rowLabel">{{ t('dashboard.criteriaField.minSizeGB') }}</span>
            <input v-model.number="q.minSizeGB" type="number" min="0" step="0.5" :class="[field, 'w-24']" />
          </label>
          <label v-if="has('withinDays')" :class="row">
            <span :class="rowLabel">{{ t('dashboard.criteriaField.withinDays') }}</span>
            <select v-model.number="q.withinDays" :class="[field, 'w-32']">
              <option :value="0">{{ t('dashboard.builder.any') }}</option>
              <option v-for="n in withinOptions" :key="n" :value="n">{{ t('dashboard.builder.days', { n }, n) }}</option>
            </select>
          </label>
          <label v-if="has('fromTime')" :class="row">
            <span :class="rowLabel">{{ t('dashboard.criteriaField.fromTime') }}</span>
            <input v-model="q.fromTime" type="time" :class="[field, 'w-32']" />
          </label>
          <label v-if="has('minDuration')" :class="row">
            <span :class="rowLabel">{{ t('dashboard.criteriaField.minDuration') }}</span>
            <input v-model.number="q.minDuration" type="number" min="0" max="600" step="5" :class="[field, 'w-24']" />
          </label>
          <p v-if="has('minDuration')" class="text-[12px] text-faint">{{ t('dashboard.builder.minDurationHint') }}</p>
          <label v-if="has('warnOnly')" class="flex items-center gap-2 text-[13.5px] text-text">
            <input v-model="q.warnOnly" type="checkbox" class="rounded-sm" />
            {{ t(sourceId === 'discovery.picks' ? 'dashboard.criteriaField.withoutTimer' : 'dashboard.criteriaField.warnOnly') }}
          </label>
          <label v-if="has('unwatched')" class="flex items-center gap-2 text-[13.5px] text-text">
            <input v-model="q.unwatched" type="checkbox" class="rounded-sm" />
            {{ t('dashboard.criteriaField.unwatched') }}
          </label>
          <p v-if="sourceId === 'media.recordings'" class="text-[12px] text-faint">{{ t('dashboard.builder.topFolderOnly') }}</p>
        </fieldset>

        <div class="grid grid-cols-2 gap-3">
          <label class="flex flex-col gap-1">
            <span :class="labelClass">{{ t('dashboard.builder.sort') }}</span>
            <select v-model="q.sort" :class="field">
              <option v-for="s in source?.sorts ?? []" :key="s" :value="s">{{ t(`dashboard.sort.${s}`) }}</option>
            </select>
          </label>
          <label class="flex flex-col gap-1">
            <span :class="labelClass">{{ t('dashboard.builder.limit') }}</span>
            <select v-model.number="q.limit" :class="field">
              <option v-for="n in [3, 5, 10, 20]" :key="n" :value="n">{{ n }}</option>
            </select>
          </label>
        </div>

        <p class="text-[13px] text-muted" aria-live="polite">
          <template v-if="preview">{{ t('dashboard.builder.preview', { n: preview.total }, preview.total) }}</template>
          <template v-else-if="previewFailed">{{ t('dashboard.custom.failed') }}</template>
        </p>
        <p class="text-[12.5px] text-faint">{{ t('dashboard.builder.note') }}</p>
      </div>

      <p v-if="saveError" class="mt-3 text-[12.5px] text-warn">{{ saveError }}</p>
      <div class="mt-4 flex justify-end gap-2">
        <button type="button" class="rounded-sm px-3 py-1.5 text-[13.5px] text-text hover:bg-field" @click="$emit('close')">
          {{ t('plan.cancel') }}
        </button>
        <button
          type="submit"
          class="rounded-sm bg-accent px-3 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-60"
          :disabled="saving || !source || !title.trim()"
        >
          {{ initial ? t('dashboard.builder.save') : t('dashboard.builder.create') }}
        </button>
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { useViewText } from '@/composables/useViewText'
import { useFocusTrap } from '@/composables/useFocusTrap'
import type { Bouquet, CustomTileView, ViewQuery, ViewResult, ViewSource } from '@/api/types'

const props = defineProps<{ initial?: CustomTileView }>()
const emit = defineEmits<{ close: []; save: [CustomTileView] }>()

const { t } = useI18n()
const { zoneName } = useViewText()

const labelClass = 'text-[13px] text-muted'
const field = 'rounded-sm border border-line bg-field px-2.5 py-1.5 text-[14px] text-text'
const row = 'grid grid-cols-[180px_1fr] items-center gap-3 below-sm:grid-cols-1 below-sm:gap-1'
const rowLabel = 'text-[13.5px] text-text'

const sources = ref<ViewSource[]>([])
const loadingSources = ref(true)
const sourcesFailed = ref(false)
const bouquets = ref<Bouquet[]>([])

const title = ref(props.initial?.title ?? '')
const sourceId = ref(props.initial?.source ?? '')
const display = ref<'list' | 'count' | 'posters'>(props.initial?.display ?? 'list')
const q = reactive<ViewQuery>({ limit: 5, withinDays: 0, ...(props.initial?.query ?? {}) })

const source = computed(() => sources.value.find((s) => s.id === sourceId.value))
const has = (f: string) => source.value?.filters.includes(f) ?? false
// Plex libraries have pictures: they can be a row of posters, and start as one.
const hasPictures = computed(() => sourceId.value.startsWith('plex.'))
const withinOptions = computed(() => (sourceId.value === 'guide.broadcasts' ? [1, 2, 3] : [1, 2, 3, 7, 14]))

// A source switch keeps the criteria it shares with the old one; the sort
// falls back to the new source's default when the old one isn't offered.
watch(source, (s) => {
  if (s && !s.sorts.includes(q.sort ?? '')) q.sort = s.sorts[0]
  if (hasPictures.value && !props.initial) display.value = 'posters'
  else if (!hasPictures.value && display.value === 'posters') display.value = 'list'
  if (s?.filters.includes('bouquet') && bouquets.value.length === 0) void loadBouquets()
})

async function loadBouquets() {
  try {
    bouquets.value = (await api.guide.bouquets()).bouquets
  } catch {
    bouquets.value = []
  }
}

// Only the criteria the source offers go into the saved tile.
function cleanQuery(): ViewQuery {
  const out: ViewQuery = { sort: q.sort, limit: q.limit }
  const keep = <K extends keyof ViewQuery>(k: K, filter: string) => {
    const v = q[k]
    if (has(filter) && v !== undefined && v !== '' && v !== 0 && v !== false) out[k] = v
  }
  keep('text', 'text')
  keep('olderThanDays', 'olderThanDays')
  keep('newerThanDays', 'newerThanDays')
  keep('minSizeGB', 'minSizeGB')
  keep('withinDays', 'withinDays')
  keep('fromTime', 'fromTime')
  keep('minDuration', 'minDuration')
  keep('warnOnly', 'warnOnly')
  keep('unwatched', 'unwatched')
  keep('bouquet', 'bouquet')
  return out
}

const preview = ref<ViewResult | null>(null)
const previewFailed = ref(false)
let previewTimer: ReturnType<typeof setTimeout> | undefined
watch(
  () => [sourceId.value, JSON.stringify(q)],
  () => {
    if (previewTimer) clearTimeout(previewTimer)
    if (!source.value) return
    previewTimer = setTimeout(async () => {
      try {
        preview.value = await api.dashboard.view(sourceId.value, cleanQuery(), zoneName())
        previewFailed.value = false
      } catch {
        preview.value = null
        previewFailed.value = true
      }
    }, 400)
  },
)

const saving = ref(false)
const saveError = ref('')
async function save() {
  if (!source.value || !title.value.trim()) return
  saving.value = true
  saveError.value = ''
  emit('save', { title: title.value.trim(), source: sourceId.value, display: display.value, query: cleanQuery() })
}
// The parent reports back whether the save went through.
function saveFailed() {
  saving.value = false
  saveError.value = t('dashboard.builder.saveFailed')
}
defineExpose({ saveFailed })

const nameInput = ref<HTMLInputElement | null>(null)
const dialogEl = ref<HTMLElement | null>(null)
// Tab stays in the builder; Escape closes it; focus returns to the button
// that opened it.
useFocusTrap(dialogEl, { onEscape: () => emit('close') })
const opener = document.activeElement as HTMLElement | null
onBeforeUnmount(() => opener?.focus())
onMounted(async () => {
  try {
    sources.value = (await api.dashboard.sources()).sources
    if (!sourceId.value && sources.value.length > 0) sourceId.value = sources.value[0].id
  } catch {
    sourcesFailed.value = true
  } finally {
    loadingSources.value = false
  }
  await nextTick()
  nameInput.value?.focus()
})
</script>
