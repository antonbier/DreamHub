<!--
  ChannelHub (/channels, docs/CHANNELHUB_PLAN.md): the box's channel list
  on a phone. Four tabs, each its own address (?tab=): the bouquets and
  what's in them, finding channels (search, new since DreamHub's first
  look, in no bouquet, TV/radio, HD, provider), the channel check, and the
  picon sets. Reading only for now — editing goes through the
  BouquetEditor plugin and comes next; without the plugin, the hub says so.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="list" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ hubTitle('channels', 'hub.channels') }}</h1>
    </div>

    <nav class="-mx-6 mt-4 flex gap-1 overflow-x-auto border-b border-line-soft px-6 below-md:-mx-4 below-md:px-4" :aria-label="t('channels.tabs.label')">
      <RouterLink
        v-for="id in TABS"
        :key="id"
        :to="{ query: { tab: id } }"
        class="-mb-px shrink-0 whitespace-nowrap border-b-2 px-3 py-2 text-[14px]"
        :class="tab === id ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
        :aria-current="tab === id ? 'page' : undefined"
      >
        {{ t(`channels.tabs.${id}`) }}
        <span v-if="id === 'check' && issues.length" class="ms-1 text-[12px] text-faint">{{ issues.length }}</span>
      </RouterLink>
    </nav>

    <!-- Bouquets -->
    <section v-if="tab === 'bouquets'" class="mt-4">
      <p v-if="bouquetsLoading" class="text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="bouquetsFailed" class="text-[14px] text-muted"><ErrorText /></p>
      <template v-else>
        <p v-if="!editor" class="mb-3 rounded border border-line bg-tile px-4 py-3 text-[13.5px] text-muted">{{ t('channels.noEditor') }}</p>
        <p v-if="saved" class="mb-3 rounded border border-ok bg-tile px-4 py-3 text-[13.5px] text-text" role="status">{{ saved }}</p>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="b in bouquets"
            :key="b.ref"
            type="button"
            class="rounded border px-3 py-2 text-start text-[14px] disabled:opacity-50"
            :class="openRef === b.ref ? 'border-accent bg-tile-2 text-text' : 'border-line bg-tile text-muted hover:text-text'"
            :aria-pressed="openRef === b.ref"
            :disabled="editing"
            @click="openBouquet(b.ref)"
          >
            <span class="block font-semibold">{{ b.name }}</span>
            <span class="block text-[12.5px] text-faint">{{ t(`channels.kind.${b.kind}`) }} · {{ t('channels.count', { n: b.count }, b.count) }}</span>
          </button>
          <button
            v-if="editor && !editing"
            type="button"
            class="rounded border border-dashed border-line px-3 py-2 text-[14px] text-accent hover:bg-tile-2"
            :aria-expanded="bouquetForm?.action === 'create'"
            @click="startBouquetAction('create')"
          >
            {{ t('channels.bouquet.create') }}
          </button>
        </div>

        <!-- Creating, renaming or deleting a bouquet: a name, then the preview. -->
        <div v-if="bouquetForm" class="mt-3 rounded border border-line bg-tile p-3">
          <PlanPreview
            v-if="bouquetForm.plan"
            :plan="bouquetForm.plan"
            :confirm-label="t(`channels.bouquet.confirm.${bouquetForm.action}`)"
            :danger="bouquetForm.action === 'delete'"
            :busy="bouquetForm.busy"
            :error="bouquetForm.error"
            @confirm="applyBouquetAction"
            @cancel="bouquetForm = null"
          >
            <p v-for="(c, n) in bouquetForm.plan.changes" :key="n">{{ describeChange(c) }}</p>
            <p class="mt-2 text-[12.5px] text-faint">{{ t('channels.edit.backupNote') }}</p>
          </PlanPreview>
          <form v-else class="flex flex-wrap items-end gap-2" @submit.prevent="reviewBouquetAction">
            <label v-if="bouquetForm.action !== 'delete'" class="flex flex-col gap-0.5 text-[13px]">
              <span class="text-faint">{{ t('channels.bouquet.name') }}</span>
              <input v-model="bouquetForm.name" type="text" maxlength="60" class="w-56 rounded border border-line bg-field px-2 py-1.5 text-[14px] text-text" />
            </label>
            <label v-if="bouquetForm.action === 'create'" class="flex flex-col gap-0.5 text-[13px]">
              <span class="text-faint">{{ t('channels.find.kind') }}</span>
              <select v-model="bouquetForm.kind" class="rounded border border-line bg-field px-2 py-1.5 text-[14px] text-text">
                <option value="tv">{{ t('channels.kind.tv') }}</option>
                <option value="radio">{{ t('channels.kind.radio') }}</option>
              </select>
            </label>
            <p v-if="bouquetForm.action === 'delete'" class="text-[13.5px] text-text">{{ t('channels.bouquet.deleteAsk', { name: currentBouquet?.name ?? '' }) }}</p>
            <button type="submit" class="rounded bg-accent px-3 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-50" :disabled="bouquetForm.busy || (bouquetForm.action !== 'delete' && !bouquetForm.name.trim())">
              {{ t('channels.edit.review') }}
            </button>
            <button type="button" class="rounded px-3 py-1.5 text-[13.5px] text-muted hover:bg-tile-2" @click="bouquetForm = null">{{ t('plan.cancel') }}</button>
            <p v-if="bouquetForm.error" class="w-full text-[13px] text-warn" role="alert">{{ bouquetForm.error }}</p>
          </form>
        </div>

        <div v-if="currentBouquet && editor && !editing && !bouquetForm" class="mt-4 flex flex-wrap gap-2">
          <button type="button" class="rounded bg-accent px-3 py-1.5 text-[13.5px] font-semibold text-bg" @click="startEditing">{{ t('channels.edit.start') }}</button>
          <button type="button" class="rounded px-3 py-1.5 text-[13.5px] text-accent hover:bg-tile-2" @click="startBouquetAction('rename')">{{ t('channels.bouquet.rename') }}</button>
          <button type="button" class="rounded px-3 py-1.5 text-[13.5px] text-onair hover:bg-tile-2" @click="startBouquetAction('delete')">{{ t('channels.bouquet.delete') }}</button>
        </div>

        <p v-if="entriesLoading" class="mt-4 text-[14px] text-muted">{{ t('common.loading') }}</p>
        <p v-else-if="entriesFailed" class="mt-4 text-[14px] text-muted"><ErrorText /></p>
        <BouquetDraftEditor
          v-else-if="editing && currentBouquet"
          class="mt-4"
          :bouquet="currentBouquet"
          :entries="entries"
          @cancel="editing = false"
          @done="editDone"
        />
        <ol v-else-if="entries.length" class="mt-4 flex flex-col">
          <li v-for="(e, i) in entries" :key="i" class="border-b border-line-soft last:border-none">
            <h3 v-if="e.marker" class="pb-1 pt-4 text-[12.5px] font-semibold uppercase tracking-wide text-faint">{{ e.name }}</h3>
            <div v-else class="flex min-h-[52px] items-center gap-3 py-2">
              <span class="w-6 shrink-0 text-end text-[12px] tabular-nums text-faint">{{ number(i) }}</span>
              <span class="flex w-12 shrink-0 justify-center"><ChannelLogo source="channels" :service-reference="e.ref" :service-name="e.name" :height="22" /></span>
              <span class="min-w-0 flex-1">
                <span class="flex items-center gap-1.5">
                  <span class="truncate text-[14.5px] text-text">{{ e.name }}</span>
                  <QualityBadge :quality="e.quality" />
                </span>
                <span v-if="e.now" class="block truncate text-[12.5px] text-muted">{{ e.now.title }}</span>
              </span>
            </div>
          </li>
        </ol>

        <!-- Backups (C3) -->
        <details v-if="editor && !editing" class="mt-6 rounded border border-line bg-tile p-3" @toggle="onBackupsToggle">
          <summary class="cursor-pointer text-[14px] font-semibold">{{ t('channels.backups.title') }}</summary>
          <p class="mt-1 text-[13px] text-muted">{{ t('channels.backups.explain') }}</p>
          <p v-if="backupsLoading" class="mt-2 text-[13px] text-muted">{{ t('common.loading') }}</p>
          <p v-else-if="backups.length === 0" class="mt-2 text-[13px] text-muted">{{ t('channels.backups.none') }}</p>
          <ul v-else class="mt-2 flex flex-col">
            <li v-for="b in backups" :key="b.filename" class="border-b border-line-soft py-2 last:border-none">
              <div class="flex items-center gap-3">
                <span class="min-w-0 flex-1">
                  <span class="block text-[13.5px] text-text">{{ dateTime(b.createdAt) }}</span>
                  <span class="block truncate text-[12.5px] text-muted">{{ t('channels.backups.before', { what: b.summary }) }}</span>
                </span>
                <button type="button" class="shrink-0 rounded px-2 py-1 text-[13px] text-accent hover:bg-tile-2" @click="reviewRestore(b.filename)">{{ t('channels.backups.restore') }}</button>
              </div>
              <PlanPreview
                v-if="restore?.filename === b.filename && restore.plan"
                class="mt-2"
                :plan="restore.plan"
                :confirm-label="t('channels.backups.restore')"
                :busy="restore.busy"
                :error="restore.error"
                @confirm="applyRestore"
                @cancel="restore = null"
              >
                {{ t('channels.backups.restoreAsk', { date: dateTime(b.createdAt) }) }}
                <p class="mt-2 text-[12.5px] text-faint">{{ t('channels.edit.backupNote') }}</p>
              </PlanPreview>
              <p v-else-if="restore?.filename === b.filename && restore.error" class="mt-1 text-[13px] text-warn">{{ restore.error }}</p>
            </li>
          </ul>
        </details>
      </template>
    </section>

    <!-- Find channels -->
    <section v-else-if="tab === 'find'" class="mt-4">
      <div class="flex flex-col gap-3">
        <label class="block">
          <span class="sr-only">{{ t('channels.find.search') }}</span>
          <input
            v-model="query"
            type="search"
            :placeholder="t('channels.find.search')"
            class="w-full rounded border border-line bg-field px-3 py-2 text-[15px] text-text"
          />
        </label>
        <div class="flex flex-wrap items-center gap-2 text-[13.5px]">
          <div class="flex gap-1 rounded bg-field p-1" role="group" :aria-label="t('channels.find.kind')">
            <button
              v-for="k in ['tv', 'radio'] as const"
              :key="k"
              type="button"
              class="rounded px-3 py-1"
              :class="kind === k ? 'bg-tile text-text' : 'text-muted hover:text-text'"
              :aria-pressed="kind === k"
              @click="kind = k"
            >
              {{ t(`channels.kind.${k}`) }}
            </button>
          </div>
          <FilterChip v-model="onlyNew">{{ t('channels.find.onlyNew') }}</FilterChip>
          <FilterChip v-model="onlyUnused">{{ t('channels.find.onlyUnused') }}</FilterChip>
          <FilterChip v-if="kind === 'tv'" v-model="onlyHD">{{ t('channels.find.onlyHD') }}</FilterChip>
          <label class="flex items-center gap-1.5">
            <span class="text-muted">{{ t('channels.find.provider') }}</span>
            <select v-model="provider" class="rounded border border-line bg-field px-2 py-1 text-text">
              <option value="">{{ t('channels.find.anyProvider') }}</option>
              <option v-for="p in providers" :key="p" :value="p">{{ p }}</option>
            </select>
          </label>
        </div>
      </div>

      <p v-if="allLoading" class="mt-4 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="allFailed" class="mt-4 text-[14px] text-muted"><ErrorText /></p>
      <template v-else>
        <p class="mt-3 flex flex-wrap items-center gap-x-3 text-[12.5px] text-faint" aria-live="polite">
          <span>{{ t('channels.find.matches', { n: matches.length }, matches.length) }}</span>
          <span v-if="since">{{ t('channels.find.newSince', { date: date(since) }) }}</span>
          <button type="button" class="text-accent hover:underline" :disabled="allLoading" @click="loadAll(true)">{{ t('channels.find.reload') }}</button>
        </p>
        <ul class="mt-2 flex flex-col">
          <li v-for="c in shownMatches" :key="c.ref" class="border-b border-line-soft last:border-none">
            <div class="flex min-h-[52px] items-center gap-3 py-2" :class="{ 'opacity-60': c.gone }">
              <span class="flex w-12 shrink-0 justify-center"><ChannelLogo source="channels" :service-reference="c.ref" :service-name="c.name" :height="22" /></span>
              <span class="min-w-0 flex-1">
                <span class="flex flex-wrap items-center gap-1.5">
                  <span class="truncate text-[14.5px] text-text">{{ c.name }}</span>
                  <QualityBadge :quality="c.quality" />
                  <span v-if="c.new" class="rounded bg-accent px-1.5 text-[11px] font-semibold text-bg">{{ t('channels.find.new') }}</span>
                  <span v-if="c.gone" class="text-[12px] text-warn">{{ t('channels.find.gone') }}</span>
                </span>
                <span class="block truncate text-[12.5px] text-muted">
                  {{ c.provider || t('channels.find.noProvider') }} ·
                  {{ c.bouquets.length ? t('channels.find.inBouquets', { names: c.bouquets.join(', ') }) : t('channels.find.inNone') }}
                </span>
              </span>
            </div>
          </li>
        </ul>
        <p v-if="matches.length > shownMatches.length" class="mt-2 text-[13px] text-faint">
          {{ t('channels.find.more', { n: matches.length - shownMatches.length }) }}
        </p>
      </template>
    </section>

    <!-- Check -->
    <section v-else-if="tab === 'check'" class="mt-4">
      <p v-if="checkLoading" class="text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="checkFailed" class="text-[14px] text-muted"><ErrorText /></p>
      <p v-else-if="issues.length === 0" class="text-[14px] text-muted">{{ t('channels.check.allGood') }}</p>
      <template v-else>
        <section v-for="group in issueGroups" :key="group.kind" class="mb-5">
          <h2 class="text-[15.5px] font-semibold">{{ t(`channels.check.${group.kind}.title`) }} <span class="text-faint">{{ group.items.length }}</span></h2>
          <p class="mb-1 text-[13px] text-muted">{{ t(`channels.check.${group.kind}.explain`) }}</p>
          <ul class="flex flex-col">
            <li v-for="(i, n) in group.items" :key="n" class="flex items-center gap-3 border-b border-line-soft py-2 last:border-none">
              <span class="flex w-12 shrink-0 justify-center"><ChannelLogo source="channels" :service-reference="i.ref" :service-name="i.name" :height="20" /></span>
              <span class="min-w-0 flex-1 truncate text-[14px] text-text">{{ i.name }}</span>
              <span class="shrink-0 truncate text-[12.5px] text-faint">{{ i.bouquet }}</span>
            </li>
          </ul>
        </section>
      </template>
    </section>

    <!-- Picons -->
    <section v-else class="mt-4">
      <p class="mb-3 text-[13.5px] text-muted">{{ t('channels.picons.explain') }}</p>
      <p v-if="piconsLoading" class="text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="piconsFailed" class="text-[14px] text-muted"><ErrorText /></p>
      <p v-else-if="sets.length === 0" class="text-[14px] text-muted">{{ t('channels.picons.none') }}</p>
      <fieldset v-else :disabled="choosing">
        <legend class="sr-only">{{ t('channels.picons.choose') }}</legend>
        <label class="flex cursor-pointer items-center gap-3 border-b border-line-soft py-3">
          <input v-model="chosen" type="radio" name="picon-set" value="" @change="choose('')" />
          <span class="min-w-0 flex-1">
            <span class="block text-[14.5px] text-text">{{ t('channels.picons.auto') }}</span>
            <span class="block text-[12.5px] text-muted">{{ t('channels.picons.autoHint') }}</span>
          </span>
        </label>
        <label v-for="s in sets" :key="s.path" class="flex cursor-pointer items-center gap-3 border-b border-line-soft py-3 last:border-none">
          <input v-model="chosen" type="radio" name="picon-set" :value="s.name" @change="choose(s.name)" />
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[14.5px] text-text">{{ s.name }}</span>
            <span class="mt-1 block h-[4px] w-full max-w-[240px] rounded bg-field">
              <span class="block h-full rounded bg-accent" :style="{ width: `${piconTotal ? (s.covered / piconTotal) * 100 : 0}%` }" />
            </span>
          </span>
          <span class="shrink-0 text-[12.5px] tabular-nums text-muted">{{ t('channels.picons.coverage', { covered: s.covered, total: piconTotal }) }}</span>
        </label>
      </fieldset>
      <p v-if="chooseMessage" class="mt-2 text-[13px]" :class="chooseFailed ? 'text-warn' : 'text-ok'" role="status">{{ chooseMessage }}</p>
    </section>
  </div>
</template>

<script setup lang="ts">
import { useHubLabel } from '@/composables/useHubLabel'
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/Icon.vue'
import ErrorText from '@/components/ErrorText.vue'
import ChannelLogo from '@/components/guide/ChannelLogo.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import BouquetDraftEditor from '@/components/channels/BouquetDraftEditor.vue'
import { useChannelChange } from '@/components/channels/useChannelChange'
import { api } from '@/api/client'
import { useFriendlyError } from '@/composables/useFriendlyError'
import { usePlanError } from '@/composables/usePlanError'
import type { BouquetEntry, ChannelBackup, ChannelBouquet, ChannelInfo, ChannelIssue, PiconSet, Plan } from '@/api/types'

const TABS = ['bouquets', 'find', 'check', 'picons'] as const
type Tab = (typeof TABS)[number]

const { t, locale } = useI18n()
const { hubTitle } = useHubLabel()
const { friendly } = useFriendlyError()
const { planErrorMessage } = usePlanError()
const { describeChange } = useChannelChange()
const route = useRoute()
const tab = computed<Tab>(() => (TABS as readonly string[]).includes(route.query.tab as string) ? (route.query.tab as Tab) : 'bouquets')

// "HD"/"UHD" next to a channel's name; nothing for SD and radio.
const QualityBadge = defineComponent({
  props: { quality: { type: String, default: '' } },
  setup(props) {
    return () =>
      props.quality === 'hd' || props.quality === 'uhd'
        ? h('span', { class: 'shrink-0 rounded border border-line px-1 text-[10.5px] font-semibold text-muted' }, props.quality.toUpperCase())
        : null
  },
})

// A toggle chip for a filter.
const FilterChip = defineComponent({
  props: { modelValue: { type: Boolean, required: true } },
  emits: ['update:modelValue'],
  setup(props, { emit, slots }) {
    return () =>
      h(
        'button',
        {
          type: 'button',
          'aria-pressed': props.modelValue,
          class: ['rounded-full border px-3 py-1', props.modelValue ? 'border-accent bg-tile-2 text-text' : 'border-line text-muted hover:text-text'],
          onClick: () => emit('update:modelValue', !props.modelValue),
        },
        slots.default?.(),
      )
  },
})

function date(unix: number) {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium' }).format(unix * 1000)
}

// --- bouquets ---------------------------------------------------------------
const bouquets = ref<ChannelBouquet[]>([])
const editor = ref(true)
const bouquetsLoading = ref(true)
const bouquetsFailed = ref(false)
const openRef = ref('')
const entries = ref<BouquetEntry[]>([])
const entriesLoading = ref(false)
const entriesFailed = ref(false)

async function loadBouquets() {
  bouquetsLoading.value = true
  bouquetsFailed.value = false
  try {
    const res = await api.channels.bouquets()
    bouquets.value = res.bouquets
    editor.value = res.editor
    if (!openRef.value && res.bouquets.length) await openBouquet(res.bouquets[0].ref)
  } catch {
    bouquetsFailed.value = true
  } finally {
    bouquetsLoading.value = false
  }
}

async function openBouquet(ref: string) {
  openRef.value = ref
  entriesLoading.value = true
  entriesFailed.value = false
  try {
    entries.value = (await api.channels.bouquet(ref)).entries
  } catch {
    entriesFailed.value = true
  } finally {
    entriesLoading.value = false
  }
}

// A channel's place in its bouquet, headings not counted — the number the
// remote uses.
const numbers = computed(() => {
  let n = 0
  return entries.value.map((e) => (e.marker ? 0 : ++n))
})
const number = (i: number) => numbers.value[i] || ''
const currentBouquet = computed(() => bouquets.value.find((b) => b.ref === openRef.value))

function dateTime(iso: string) {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(iso))
}

// --- editing (C2) -----------------------------------------------------------
const editing = ref(false)
const saved = ref('')

function startEditing() {
  saved.value = ''
  editing.value = true
}

async function afterChange(message: string) {
  editing.value = false
  saved.value = message
  checkLoaded.value = false
  allLoaded.value = false
  backupsLoaded.value = false
  await loadBouquets()
  if (openRef.value && bouquets.value.some((b) => b.ref === openRef.value)) await openBouquet(openRef.value)
  else if (bouquets.value.length) await openBouquet(bouquets.value[0].ref)
  void loadCheck()
}

function editDone() {
  void afterChange(t('channels.edit.saved'))
}

// Creating, renaming, deleting a bouquet.
interface BouquetForm {
  action: 'create' | 'rename' | 'delete'
  name: string
  kind: 'tv' | 'radio'
  plan?: Plan
  busy: boolean
  error: string
}
const bouquetForm = ref<BouquetForm | null>(null)

function startBouquetAction(action: BouquetForm['action']) {
  saved.value = ''
  const b = currentBouquet.value
  bouquetForm.value = { action, name: action === 'rename' ? (b?.name ?? '') : '', kind: action === 'create' ? 'tv' : (b?.kind ?? 'tv'), busy: false, error: '' }
}

async function reviewBouquetAction() {
  const f = bouquetForm.value
  if (!f) return
  f.busy = true
  f.error = ''
  try {
    f.plan = (await api.channels.planBouquets(f.action, f.kind, f.action === 'create' ? '' : openRef.value, f.name.trim())).plan
  } catch (err) {
    f.error = planErrorMessage(err)
  } finally {
    f.busy = false
  }
}

async function applyBouquetAction() {
  const f = bouquetForm.value
  if (!f?.plan) return
  f.busy = true
  f.error = ''
  try {
    await api.plans.apply(f.plan.id)
    if (f.action === 'delete') openRef.value = ''
    bouquetForm.value = null
    await afterChange(t('channels.edit.saved'))
  } catch (err) {
    f.error = planErrorMessage(err)
    f.busy = false
  }
}

// --- backups (C3) -------------------------------------------------------------
const backups = ref<ChannelBackup[]>([])
const backupsLoading = ref(false)
const backupsLoaded = ref(false)
const restore = ref<{ filename: string; plan?: Plan; busy: boolean; error: string } | null>(null)

async function onBackupsToggle(e: Event) {
  if (!(e.target as HTMLDetailsElement).open || backupsLoaded.value) return
  backupsLoading.value = true
  try {
    backups.value = (await api.channels.backups()).backups
    backupsLoaded.value = true
  } catch {
    backups.value = []
  } finally {
    backupsLoading.value = false
  }
}

async function reviewRestore(filename: string) {
  restore.value = { filename, busy: true, error: '' }
  try {
    restore.value.plan = (await api.channels.planRestore(filename)).plan
  } catch (err) {
    restore.value.error = planErrorMessage(err)
  } finally {
    if (restore.value) restore.value.busy = false
  }
}

async function applyRestore() {
  const r = restore.value
  if (!r?.plan) return
  r.busy = true
  r.error = ''
  try {
    await api.plans.apply(r.plan.id)
    restore.value = null
    await afterChange(t('channels.backups.restored'))
  } catch (err) {
    r.error = planErrorMessage(err)
    r.busy = false
  }
}

// --- find -------------------------------------------------------------------
const all = ref<ChannelInfo[]>([])
const since = ref(0)
const allLoading = ref(false)
const allFailed = ref(false)
const allLoaded = ref(false)
const query = ref('')
const kind = ref<'tv' | 'radio'>('tv')
const onlyNew = ref(false)
const onlyUnused = ref(false)
const onlyHD = ref(false)
const provider = ref('')

async function loadAll(refresh = false) {
  allLoading.value = true
  allFailed.value = false
  try {
    const res = await api.channels.all(refresh)
    all.value = res.channels
    since.value = res.since
    allLoaded.value = true
  } catch {
    allFailed.value = true
  } finally {
    allLoading.value = false
  }
}

const providers = computed(() =>
  [...new Set(all.value.filter((c) => c.kind === kind.value && c.provider).map((c) => c.provider as string))].sort((a, b) =>
    a.localeCompare(b, locale.value),
  ),
)
const matches = computed(() => {
  const q = query.value.trim().toLocaleLowerCase(locale.value)
  return all.value.filter(
    (c) =>
      c.kind === kind.value &&
      (!q || c.name.toLocaleLowerCase(locale.value).includes(q)) &&
      (!onlyNew.value || c.new) &&
      (!onlyUnused.value || c.bouquets.length === 0) &&
      (!onlyHD.value || c.quality === 'hd' || c.quality === 'uhd') &&
      (!provider.value || c.provider === provider.value),
  )
})
// A long list on a phone: the first 200, the rest by narrowing the search.
const shownMatches = computed(() => matches.value.slice(0, 200))
watch(kind, () => (provider.value = ''))

// --- check ------------------------------------------------------------------
const issues = ref<ChannelIssue[]>([])
const checkLoading = ref(false)
const checkFailed = ref(false)
const checkLoaded = ref(false)

async function loadCheck() {
  checkLoading.value = true
  checkFailed.value = false
  try {
    issues.value = (await api.channels.check()).issues
    checkLoaded.value = true
  } catch {
    checkFailed.value = true
  } finally {
    checkLoading.value = false
  }
}

const ISSUE_ORDER = ['gone', 'duplicate', 'noEpg', 'noPicon'] as const
const issueGroups = computed(() =>
  ISSUE_ORDER.map((kind) => ({ kind, items: issues.value.filter((i) => i.kind === kind) })).filter((g) => g.items.length > 0),
)

// --- picons -----------------------------------------------------------------
const sets = ref<PiconSet[]>([])
const piconTotal = ref(0)
const chosen = ref('')
const piconsLoading = ref(false)
const piconsFailed = ref(false)
const piconsLoaded = ref(false)
const choosing = ref(false)
const chooseMessage = ref('')
const chooseFailed = ref(false)

async function loadPicons() {
  piconsLoading.value = true
  piconsFailed.value = false
  try {
    const res = await api.channels.picons()
    sets.value = res.sets
    piconTotal.value = res.total
    chosen.value = res.chosen
    piconsLoaded.value = true
  } catch {
    piconsFailed.value = true
  } finally {
    piconsLoading.value = false
  }
}

async function choose(name: string) {
  choosing.value = true
  chooseMessage.value = ''
  try {
    await api.channels.choosePicons(name)
    chooseFailed.value = false
    chooseMessage.value = t('channels.picons.saved')
  } catch (err) {
    chooseFailed.value = true
    chooseMessage.value = friendly(err).text
  } finally {
    choosing.value = false
  }
}

// Each tab loads when first shown.
function loadTab(t: Tab) {
  if (t === 'find' && !allLoaded.value) void loadAll()
  if (t === 'picons' && !piconsLoaded.value) void loadPicons()
  if (t === 'check' && !checkLoaded.value) void loadCheck()
}
watch(tab, loadTab)
onMounted(() => {
  void loadBouquets()
  // The check's count shows on its tab from the start.
  void loadCheck()
  loadTab(tab.value)
})
</script>
