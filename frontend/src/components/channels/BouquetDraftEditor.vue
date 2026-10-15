<!--
  Editing one bouquet on a phone (docs/CHANNELHUB_PLAN.md, C2). Every move,
  removal, addition, heading and rename goes into a draft first — shown
  live, nothing sent. "Review" turns the draft into one plan
  (POST /api/v1/channels/bouquet/plan) that lists the changes in words;
  confirming applies it with a backup first and a check afterwards (the
  backend restores the backup if anything differs).

  Moving: a row's own buttons (up, down, to a number) work everywhere; on
  a desktop rows can also be dragged by their handle. Dragging alone would
  be neither accessible nor usable on a touch screen.
-->
<template>
  <div>
    <ol class="flex flex-col">
      <li
        v-for="(row, i) in draft"
        :key="row.key"
        class="border-b border-line-soft last:border-none"
        :class="{ 'opacity-40': dragging === i }"
        @dragover.prevent
        @drop.prevent="dropOn(i)"
      >
        <div class="flex min-h-[52px] items-center gap-2 py-1.5">
          <span
            draggable="true"
            class="hidden h-9 w-6 shrink-0 cursor-grab items-center justify-center text-faint md:flex"
            :aria-hidden="true"
            @dragstart="dragging = i"
            @dragend="dragging = null"
          >
            <Icon name="menu" :size="16" />
          </span>
          <span class="w-6 shrink-0 text-end text-[12px] tabular-nums text-faint">{{ numbers[i] || '' }}</span>
          <span v-if="!row.marker" class="flex w-11 shrink-0 justify-center">
            <ChannelLogo v-if="row.ref" source="channels" :service-reference="row.ref" :service-name="row.name" :height="20" />
          </span>
          <button
            type="button"
            class="min-w-0 flex-1 rounded-sm px-1 py-1 text-start hover:bg-tile-2"
            :aria-expanded="open === row.key"
            @click="toggle(row.key)"
          >
            <span v-if="row.marker" class="block truncate text-[12.5px] font-semibold uppercase tracking-wide text-faint">{{ row.name }}</span>
            <span v-else class="block truncate text-[14.5px] text-text">{{ row.name }}</span>
            <span v-if="row.added" class="text-[11.5px] text-accent">{{ t('channels.edit.added') }}</span>
          </button>
          <button type="button" class="h-9 w-9 shrink-0 rounded-sm text-muted hover:bg-tile-2 hover:text-text disabled:opacity-30" :disabled="i === 0" :aria-label="t('channels.edit.up', { name: row.name })" @click="move(i, i - 1)">
            <Icon name="chevron-up" :size="16" />
          </button>
          <button type="button" class="h-9 w-9 shrink-0 rounded-sm text-muted hover:bg-tile-2 hover:text-text disabled:opacity-30" :disabled="i === draft.length - 1" :aria-label="t('channels.edit.down', { name: row.name })" @click="move(i, i + 1)">
            <Icon name="chevron-down" :size="16" />
          </button>
        </div>

        <!-- The row's other actions, opened by tapping it. -->
        <div v-if="open === row.key" class="mb-2 flex flex-wrap items-end gap-2 rounded-sm bg-tile-2 p-2 text-[13px]">
          <form v-if="!row.marker" class="flex items-end gap-1.5" @submit.prevent="moveToNumber(i)">
            <label class="flex flex-col gap-0.5">
              <span class="text-faint">{{ t('channels.edit.toNumber') }}</span>
              <input v-model.number="targetNumber" type="number" min="1" :max="channelCount" inputmode="numeric" class="w-20 rounded-sm border border-line bg-field px-2 py-1 text-text" />
            </label>
            <button type="submit" class="rounded-sm px-2 py-1 text-accent hover:bg-tile" :disabled="!targetNumber">{{ t('channels.edit.move') }}</button>
          </form>
          <form class="flex items-end gap-1.5" @submit.prevent="rename(i)">
            <label class="flex flex-col gap-0.5">
              <span class="text-faint">{{ t('channels.edit.name') }}</span>
              <input v-model="newName" type="text" maxlength="100" class="w-44 rounded-sm border border-line bg-field px-2 py-1 text-text" />
            </label>
            <button type="submit" class="rounded-sm px-2 py-1 text-accent hover:bg-tile" :disabled="!newName.trim() || newName.trim() === row.name">{{ t('channels.edit.rename') }}</button>
          </form>
          <button type="button" class="ms-auto rounded-sm px-2 py-1 text-onair hover:bg-tile" @click="remove(i)">{{ t('channels.edit.remove') }}</button>
        </div>
      </li>
    </ol>
    <p v-if="draft.length === 0" class="py-3 text-[13.5px] text-muted">{{ t('channels.edit.empty') }}</p>

    <!-- Adding -->
    <div class="mt-3 flex flex-wrap gap-2">
      <button type="button" class="rounded-sm border border-line px-3 py-1.5 text-[13.5px] text-accent hover:bg-tile-2" :aria-expanded="adding" @click="toggleAdding">
        {{ t('channels.edit.addChannels') }}
      </button>
      <form class="flex items-center gap-1.5" @submit.prevent="addMarker">
        <label class="sr-only" for="new-heading">{{ t('channels.edit.headingName') }}</label>
        <input id="new-heading" v-model="headingName" type="text" maxlength="60" :placeholder="t('channels.edit.headingName')" class="w-40 rounded-sm border border-line bg-field px-2 py-1.5 text-[13.5px] text-text" />
        <button type="submit" class="rounded-sm px-2 py-1.5 text-[13.5px] text-accent hover:bg-tile-2" :disabled="!headingName.trim()">{{ t('channels.edit.addHeading') }}</button>
      </form>
    </div>

    <div v-if="adding" class="mt-3 rounded-sm border border-line bg-tile p-3">
      <div class="flex flex-wrap items-center gap-2">
        <label class="min-w-0 flex-1">
          <span class="sr-only">{{ t('channels.find.search') }}</span>
          <input v-model="search" type="search" :placeholder="t('channels.find.search')" class="w-full rounded-sm border border-line bg-field px-3 py-1.5 text-[14px] text-text" />
        </label>
        <button
          type="button"
          class="rounded-full border px-3 py-1 text-[13px]"
          :class="onlyUnused ? 'border-accent bg-tile-2 text-text' : 'border-line text-muted'"
          :aria-pressed="onlyUnused"
          @click="onlyUnused = !onlyUnused"
        >
          {{ t('channels.find.onlyUnused') }}
        </button>
      </div>
      <p v-if="candidatesLoading" class="mt-2 text-[13px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="candidatesFailed" class="mt-2 text-[13px] text-muted"><ErrorText /></p>
      <ul v-else class="mt-2 flex max-h-[50vh] flex-col overflow-y-auto">
        <li v-for="c in candidates" :key="c.ref" class="flex min-h-[48px] items-center gap-2 border-b border-line-soft last:border-none">
          <span class="flex w-11 shrink-0 justify-center"><ChannelLogo source="channels" :service-reference="c.ref" :service-name="c.name" :height="18" /></span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[14px] text-text">{{ c.name }}</span>
            <span class="block truncate text-[12px] text-muted">{{ c.provider }}<template v-if="c.new"> · {{ t('channels.find.new') }}</template></span>
          </span>
          <button type="button" class="shrink-0 rounded-sm px-2 py-1 text-[13px] text-accent hover:bg-tile-2" @click="add(c)">{{ t('channels.edit.add') }}</button>
        </li>
        <li v-if="candidates.length === 0" class="py-2 text-[13px] text-muted">{{ t('channels.edit.noCandidates') }}</li>
      </ul>
    </div>

    <!-- The draft's bar: review or discard. -->
    <div class="sticky bottom-0 z-10 -mx-1 mt-4 border-t border-line bg-bg px-1 py-3">
      <PlanPreview
        v-if="plan"
        :plan="plan"
        :confirm-label="t('channels.edit.apply')"
        :busy="applying"
        :error="applyError"
        @confirm="apply"
        @cancel="plan = null"
      >
        <p class="mb-1">{{ t('channels.edit.reviewIntro', { bouquet: bouquet.name }) }}</p>
        <ul class="flex flex-col gap-0.5">
          <li v-for="(c, n) in plan.changes" :key="n">{{ describeChange(c) }}</li>
        </ul>
        <p class="mt-2 text-[12.5px] text-faint">{{ t('channels.edit.backupNote') }}</p>
      </PlanPreview>
      <div v-else class="flex items-center gap-2">
        <span class="text-[13px]" :class="dirty ? 'text-text' : 'text-faint'" aria-live="polite">
          {{ dirty ? t('channels.edit.unsaved') : t('channels.edit.noChanges') }}
        </span>
        <button type="button" class="ms-auto rounded-sm px-3 py-1.5 text-[13.5px] text-muted hover:bg-tile-2 hover:text-text" @click="emit('cancel')">
          {{ dirty ? t('channels.edit.discard') : t('channels.edit.close') }}
        </button>
        <button type="button" class="rounded-sm bg-accent px-3 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-50" :disabled="!dirty || reviewing" @click="review">
          {{ reviewing ? t('common.loading') : t('channels.edit.review') }}
        </button>
      </div>
      <p v-if="reviewError" class="mt-2 text-[13px] text-warn" role="alert">{{ reviewError }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/Icon.vue'
import ErrorText from '@/components/ErrorText.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import ChannelLogo from '@/components/guide/ChannelLogo.vue'
import { api } from '@/api/client'
import { usePlanError } from '@/composables/usePlanError'
import { useChannelChange } from './useChannelChange'
import type { BouquetEntry, ChannelBouquet, ChannelInfo, Plan } from '@/api/types'

const props = defineProps<{ bouquet: ChannelBouquet; entries: BouquetEntry[] }>()
const emit = defineEmits<{ cancel: []; done: [] }>()
const { t } = useI18n()
const { planErrorMessage } = usePlanError()
const { describeChange } = useChannelChange()

interface Row {
  key: string
  ref: string
  name: string
  marker?: boolean
  added?: boolean
}
let nextKey = 0
const original = props.entries.map((e) => ({ ref: e.ref, name: e.name, marker: e.marker }))
const draft = ref<Row[]>(props.entries.map((e) => ({ key: `k${nextKey++}`, ref: e.ref, name: e.name, marker: e.marker })))

const dirty = computed(
  () => JSON.stringify(draft.value.map((r) => ({ ref: r.ref, name: r.name, marker: r.marker }))) !== JSON.stringify(original),
)
// The remote's numbers: headings aren't counted.
const numbers = computed(() => {
  let n = 0
  return draft.value.map((r) => (r.marker ? 0 : ++n))
})
const channelCount = computed(() => draft.value.filter((r) => !r.marker).length)

// --- moving, renaming, removing ------------------------------------------
const open = ref<string | null>(null)
const targetNumber = ref<number | null>(null)
const newName = ref('')
const dragging = ref<number | null>(null)

function toggle(key: string) {
  open.value = open.value === key ? null : key
  const row = draft.value.find((r) => r.key === key)
  newName.value = row?.name ?? ''
  targetNumber.value = null
}

function move(from: number, to: number) {
  if (to < 0 || to >= draft.value.length || from === to) return
  const list = [...draft.value]
  const [row] = list.splice(from, 1)
  list.splice(to, 0, row)
  draft.value = list
}

function dropOn(i: number) {
  if (dragging.value !== null) move(dragging.value, i)
  dragging.value = null
}

// Moves a channel to the place the remote will call `targetNumber`.
function moveToNumber(i: number) {
  const n = Math.max(1, Math.min(channelCount.value, Math.round(targetNumber.value ?? 0)))
  const row = draft.value[i]
  const rest = draft.value.filter((_, k) => k !== i)
  let seen = 0
  let at = rest.length
  for (let k = 0; k < rest.length; k++) {
    if (!rest[k].marker && ++seen === n) {
      at = k
      break
    }
  }
  draft.value = [...rest.slice(0, at), row, ...rest.slice(at)]
  open.value = null
}

function rename(i: number) {
  const name = newName.value.trim()
  if (!name) return
  draft.value[i] = { ...draft.value[i], name }
  open.value = null
}

function remove(i: number) {
  draft.value = draft.value.filter((_, k) => k !== i)
  open.value = null
}

// --- adding -------------------------------------------------------------------
const adding = ref(false)
const search = ref('')
const onlyUnused = ref(true)
const allChannels = ref<ChannelInfo[]>([])
const candidatesLoading = ref(false)
const candidatesFailed = ref(false)
const headingName = ref('')

async function toggleAdding() {
  adding.value = !adding.value
  if (adding.value && allChannels.value.length === 0) {
    candidatesLoading.value = true
    candidatesFailed.value = false
    try {
      allChannels.value = (await api.channels.all()).channels
    } catch {
      candidatesFailed.value = true
    } finally {
      candidatesLoading.value = false
    }
  }
}

const candidates = computed(() => {
  const inDraft = new Set(draft.value.map((r) => r.ref.split(':').slice(0, 10).join(':').toUpperCase()))
  const q = search.value.trim().toLowerCase()
  return allChannels.value
    .filter(
      (c) =>
        c.kind === props.bouquet.kind &&
        !c.gone &&
        !inDraft.has(c.ref.split(':').slice(0, 10).join(':').toUpperCase()) &&
        (!onlyUnused.value || c.bouquets.length === 0) &&
        (!q || c.name.toLowerCase().includes(q)),
    )
    .slice(0, 50)
})

function add(c: ChannelInfo) {
  draft.value = [...draft.value, { key: `k${nextKey++}`, ref: c.ref, name: c.name, added: true }]
}

function addMarker() {
  const name = headingName.value.trim()
  if (!name) return
  draft.value = [...draft.value, { key: `k${nextKey++}`, ref: '', name, marker: true, added: true }]
  headingName.value = ''
}

// --- review and apply -----------------------------------------------------------
const plan = ref<Plan | null>(null)
const reviewing = ref(false)
const reviewError = ref('')
const applying = ref(false)
const applyError = ref('')

async function review() {
  reviewing.value = true
  reviewError.value = ''
  try {
    plan.value = (
      await api.channels.planBouquet(
        props.bouquet.ref,
        props.bouquet.kind,
        draft.value.map((r) => ({ ref: r.ref, name: r.name, marker: r.marker })),
      )
    ).plan
    applyError.value = ''
  } catch (err) {
    reviewError.value = planErrorMessage(err)
  } finally {
    reviewing.value = false
  }
}

async function apply() {
  if (!plan.value) return
  applying.value = true
  applyError.value = ''
  try {
    await api.plans.apply(plan.value.id)
    emit('done')
  } catch (err) {
    applyError.value = planErrorMessage(err)
  } finally {
    applying.value = false
  }
}
</script>
