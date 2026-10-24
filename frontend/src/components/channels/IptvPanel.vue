<!--
  Internet channels from an M3U list into a bouquet (docs/NETCAST_PLAN.md,
  N1): give the address of a list or a file, pick channels, choose a new or an
  existing bouquet, see the preview, confirm. The box reads the list; a stream
  address — which may hold a login — never reaches this page, only "with
  login" does. Nothing changes before the preview is confirmed.
-->
<template>
  <section class="mt-4">
    <p class="text-[13.5px] text-muted">{{ t('channels.iptv.explain') }}</p>
    <p v-if="!editor" class="mt-3 rounded-sm border border-line bg-tile px-4 py-3 text-[13.5px] text-muted">{{ t('channels.noEditor') }}</p>
    <p v-if="saved" class="mt-3 rounded-sm border border-ok bg-tile px-4 py-3 text-[13.5px] text-text" role="status">{{ saved }}</p>

    <!-- 1. Where the list is -->
    <form v-if="!list" class="mt-4 flex flex-wrap items-end gap-3" @submit.prevent="readUrl">
      <label class="flex min-w-0 grow flex-col gap-0.5 text-[13px]">
        <span class="text-faint">{{ t('channels.iptv.address') }}</span>
        <input
          v-model="url"
          type="url"
          inputmode="url"
          autocomplete="off"
          spellcheck="false"
          placeholder="https://"
          class="w-full max-w-2xl rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text"
        />
        <span class="text-[12px] text-faint">{{ t('channels.iptv.addressHint') }}</span>
      </label>
      <button type="submit" class="rounded-sm bg-accent px-3 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-50" :disabled="reading || !url.trim()">
        {{ reading ? t('channels.iptv.reading') : t('channels.iptv.read') }}
      </button>
      <span class="text-[13px] text-faint">{{ t('channels.iptv.orFile') }}</span>
      <input ref="fileInput" type="file" class="sr-only" accept=".m3u,.m3u8,.gz,text/plain,audio/x-mpegurl,application/vnd.apple.mpegurl" @change="readFile" />
      <button type="button" class="rounded-sm border border-line px-3 py-1.5 text-[13.5px] text-text hover:bg-tile-2 disabled:opacity-50" :disabled="reading" @click="fileInput?.click()">
        {{ t('channels.iptv.chooseFile') }}
      </button>
      <p v-if="error" class="w-full text-[13px] text-warn" role="alert">{{ error }}</p>
    </form>

    <template v-else>
      <!-- 2. The channels of the list -->
      <div class="mt-4 flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h2 class="text-[16px] font-semibold">{{ list.fromFile ? t('channels.iptv.fromFile') + (list.name ? ': ' + list.name : '') : list.name }}</h2>
        <span class="text-[13px] text-muted">{{ t('channels.iptv.summary', { n: list.channels.length }, list.channels.length) }}</span>
        <button type="button" class="rounded-sm px-2 py-1 text-[13px] text-accent hover:bg-tile-2" :disabled="busy" @click="reset">{{ t('channels.iptv.readAnother') }}</button>
      </div>
      <p v-for="s in list.skipped" :key="s.reason" class="text-[12.5px] text-faint">
        {{ t(s.reason === 'scheme' ? 'channels.iptv.skippedScheme' : 'channels.iptv.skippedInvalid', { n: s.count }, s.count) }}
      </p>
      <p v-if="list.truncated" class="text-[12.5px] text-warn">{{ t('channels.iptv.truncated', { limit: number(list.limit) }) }}</p>

      <div v-if="!result" class="mt-3 flex flex-wrap items-end gap-2">
        <label class="flex flex-col gap-0.5 text-[13px]">
          <span class="text-faint">{{ t('channels.iptv.search') }}</span>
          <input v-model="query" type="search" class="w-56 rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text" />
        </label>
        <label v-if="list.groups.length > 1" class="flex flex-col gap-0.5 text-[13px]">
          <span class="text-faint">{{ t('channels.iptv.group') }}</span>
          <select v-model="group" class="max-w-[16rem] rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text">
            <option :value="ALL">{{ t('channels.iptv.allGroups') }}</option>
            <option v-for="g in list.groups" :key="g.name" :value="g.name">{{ g.name || '—' }} ({{ number(g.count) }})</option>
          </select>
        </label>
        <button type="button" class="rounded-sm px-2 py-1.5 text-[13px] text-accent hover:bg-tile-2" @click="selectShown">{{ t('channels.iptv.selectShown') }}</button>
        <button type="button" class="rounded-sm px-2 py-1.5 text-[13px] text-muted hover:bg-tile-2" :disabled="selected.size === 0" @click="selected = new Set()">{{ t('channels.iptv.selectNone') }}</button>
        <span class="ms-auto text-[13px] text-text" aria-live="polite">{{ t('channels.iptv.selected', { n: number(selected.size) }, selected.size) }}</span>
      </div>

      <ul v-if="!result" class="mt-2 divide-y divide-line-soft rounded-sm border border-line bg-tile">
        <li v-for="c in visible" :key="c.id">
          <label class="flex cursor-pointer items-center gap-3 px-3 py-2 text-[14px] hover:bg-tile-2">
            <input type="checkbox" class="h-4 w-4 shrink-0" :checked="selected.has(c.id)" @change="toggle(c.id)" />
            <span class="min-w-0 grow">
              <span class="block truncate text-text">{{ c.name }}</span>
              <span class="block truncate text-[12.5px] text-faint">{{ [c.group, c.host].filter(Boolean).join(' · ') }}</span>
            </span>
            <span v-if="c.radio" class="shrink-0 rounded-sm border border-line px-1 text-[11px] text-muted">{{ t('channels.iptv.radio') }}</span>
            <span v-if="c.hasLogin" class="shrink-0 rounded-sm border border-line px-1 text-[11px] text-muted">{{ t('channels.iptv.login') }}</span>
            <span v-if="c.duplicate" class="shrink-0 rounded-sm border border-line px-1 text-[11px] text-muted">{{ t('channels.iptv.duplicate') }}</span>
            <span v-if="c.needsHeaders" class="shrink-0 rounded-sm border border-warn px-1 text-[11px] text-warn" :title="t('channels.iptv.headersTip')">{{ t('channels.iptv.headers') }}</span>
          </label>
        </li>
        <li v-if="filtered.length === 0" class="px-3 py-3 text-[13.5px] text-muted">{{ t('channels.iptv.noMatch') }}</li>
      </ul>
      <div v-if="!result && filtered.length > 0" class="mt-2 flex flex-wrap items-center gap-3 text-[13px] text-muted">
        <span>{{ t('channels.iptv.showing', { shown: number(visible.length), total: number(filtered.length) }) }}</span>
        <button v-if="visible.length < filtered.length" type="button" class="rounded-sm px-2 py-1 text-accent hover:bg-tile-2" @click="limit += PAGE">{{ t('channels.iptv.more') }}</button>
      </div>

      <!-- 3. Where to put them -->
      <form v-if="!result" class="mt-4 flex flex-wrap items-end gap-3 rounded-sm border border-line bg-tile p-3" @submit.prevent="review">
        <fieldset class="flex flex-col gap-1 text-[13.5px]">
          <legend class="mb-1 text-[13px] text-faint">{{ t('channels.iptv.target') }}</legend>
          <label class="flex items-center gap-2"><input v-model="mode" type="radio" value="new" /> {{ t('channels.iptv.newBouquet') }}</label>
          <label class="flex items-center gap-2"><input v-model="mode" type="radio" value="existing" :disabled="bouquets.length === 0" /> {{ t('channels.iptv.existingBouquet') }}</label>
        </fieldset>
        <template v-if="mode === 'new'">
          <label class="flex flex-col gap-0.5 text-[13px]">
            <span class="text-faint">{{ t('channels.iptv.name') }}</span>
            <input v-model="newName" type="text" maxlength="60" class="w-56 rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text" />
          </label>
          <label class="flex flex-col gap-0.5 text-[13px]">
            <span class="text-faint">{{ t('channels.iptv.kind') }}</span>
            <select v-model="kind" class="rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text">
              <option value="tv">{{ t('channels.kind.tv') }}</option>
              <option value="radio">{{ t('channels.kind.radio') }}</option>
            </select>
          </label>
        </template>
        <label v-else class="flex flex-col gap-0.5 text-[13px]">
          <span class="text-faint">{{ t('channels.iptv.bouquet') }}</span>
          <select v-model="existing" class="max-w-[18rem] rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text">
            <option v-for="b in bouquets" :key="b.ref" :value="b.ref">{{ b.name }} ({{ t(`channels.kind.${b.kind}`) }})</option>
          </select>
        </label>
        <label v-if="!list.fromFile" class="flex w-full items-start gap-2 text-[13.5px]">
          <input v-model="remember" type="checkbox" class="mt-0.5 h-4 w-4 shrink-0" />
          <span>
            {{ t('channels.iptv.sources.remember') }}
            <span class="block text-[12px] text-faint">{{ t('channels.iptv.sources.rememberHint') }}</span>
          </span>
        </label>
        <button type="submit" class="rounded-sm bg-accent px-3 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-50" :disabled="busy || !editor || selected.size === 0 || (mode === 'new' ? !newName.trim() : !existing)">
          {{ t('channels.iptv.review') }}
        </button>
        <p v-if="error" class="w-full text-[13px] text-warn" role="alert">{{ error }}</p>
      </form>

      <!-- 4. The preview -->
      <PlanPreview
        v-if="result"
        class="mt-4"
        :plan="result.plan"
        :confirm-label="t('channels.iptv.confirm')"
        :busy="busy"
        :error="error"
        @confirm="apply"
        @cancel="cancelReview"
      >
        <p v-for="(c, n) in shownChanges" :key="n">{{ describeChange(c) }}</p>
        <p v-if="hiddenChanges > 0" class="text-muted">{{ t('channels.iptv.moreChanges', { n: number(hiddenChanges) }) }}</p>
        <p v-if="result.skipped.length > 0" class="mt-2 text-[12.5px] text-muted">{{ t('channels.iptv.leftOut', { n: number(result.skipped.length) }, result.skipped.length) }}</p>
        <p class="mt-2 text-[12.5px] text-faint">{{ t('channels.iptv.backupNote') }}</p>
      </PlanPreview>
    </template>

    <IptvSources v-if="!list" :editor="editor" @done="emit('done')" />
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import PlanPreview from '@/components/PlanPreview.vue'
import IptvSources from '@/components/channels/IptvSources.vue'
import { useChannelChange } from '@/components/channels/useChannelChange'
import { usePlanError } from '@/composables/usePlanError'
import type { ChannelBouquet, IptvList, IptvPlanResult } from '@/api/types'

const props = defineProps<{ bouquets: ChannelBouquet[]; editor: boolean }>()
const emit = defineEmits<{ done: [] }>()

const { t, n: formatNumber } = useI18n()
const { planErrorMessage } = usePlanError()
const { describeChange } = useChannelChange()

const ALL = '\u0000all'
const PAGE = 150
const PREVIEW_LINES = 12

const number = (v: number) => formatNumber(v)

const url = ref('')
const fileInput = ref<HTMLInputElement | null>(null)
const reading = ref(false)
const busy = ref(false)
const error = ref('')
const saved = ref('')

const list = ref<IptvList | null>(null)
const query = ref('')
const group = ref(ALL)
const limit = ref(PAGE)
const selected = ref(new Set<number>())

const mode = ref<'new' | 'existing'>('new')
const newName = ref('')
const kind = ref<'tv' | 'radio'>('tv')
const existing = ref('')
const result = ref<IptvPlanResult | null>(null)
const remember = ref(true)

const filtered = computed(() => {
  const l = list.value
  if (!l) return []
  const q = query.value.trim().toLowerCase()
  return l.channels.filter(
    (c) =>
      (group.value === ALL || (c.group ?? '') === group.value) &&
      (q === '' || c.name.toLowerCase().includes(q) || (c.group ?? '').toLowerCase().includes(q) || c.host.toLowerCase().includes(q)),
  )
})
const visible = computed(() => filtered.value.slice(0, limit.value))

// A new search or group starts at the top again.
watch([query, group], () => (limit.value = PAGE))

// A kind for a new bouquet follows what the chosen channels are.
watch(
  () => [selected.value.size, mode.value],
  () => {
    const l = list.value
    if (!l || mode.value !== 'new' || selected.value.size === 0) return
    const radio = l.channels.filter((c) => selected.value.has(c.id) && c.radio).length
    kind.value = radio * 2 > selected.value.size ? 'radio' : 'tv'
  },
)

watch(
  () => props.bouquets,
  (b) => {
    if (!b.some((x) => x.ref === existing.value)) existing.value = b[0]?.ref ?? ''
    if (b.length === 0) mode.value = 'new'
  },
  { immediate: true },
)

const shownChanges = computed(() => result.value?.plan.changes.slice(0, PREVIEW_LINES) ?? [])
const hiddenChanges = computed(() => Math.max(0, (result.value?.plan.changes.length ?? 0) - PREVIEW_LINES))

function toggle(id: number) {
  const s = new Set(selected.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  selected.value = s
}

function selectShown() {
  const s = new Set(selected.value)
  for (const c of filtered.value) s.add(c.id)
  selected.value = s
}

function take(l: IptvList) {
  list.value = l
  selected.value = new Set()
  query.value = ''
  group.value = ALL
  limit.value = PAGE
  result.value = null
  saved.value = ''
  newName.value = l.fromFile ? l.name.replace(/\.[^.]*$/, '') : l.name
}

async function readUrl() {
  reading.value = true
  error.value = ''
  try {
    take(await api.channels.readIptv(url.value.trim()))
    url.value = ''
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    reading.value = false
  }
}

async function readFile() {
  const f = fileInput.value?.files?.[0]
  if (!f) return
  reading.value = true
  error.value = ''
  try {
    take(await api.channels.uploadIptv(f, f.name))
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    reading.value = false
    if (fileInput.value) fileInput.value.value = ''
  }
}

function reset() {
  list.value = null
  result.value = null
  selected.value = new Set()
  error.value = ''
}

function cancelReview() {
  result.value = null
  error.value = ''
}

async function review() {
  const l = list.value
  if (!l) return
  busy.value = true
  error.value = ''
  try {
    const ids = [...selected.value].sort((a, b) => a - b)
    result.value = await api.channels.planIptv(
      mode.value === 'new' ? { token: l.token, ids, newName: newName.value.trim(), kind: kind.value, remember: remember.value && !l.fromFile }
        : { token: l.token, ids, bouquet: existing.value, remember: remember.value && !l.fromFile },
    )
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}

async function apply() {
  const r = result.value
  if (!r) return
  busy.value = true
  error.value = ''
  try {
    await api.plans.apply(r.plan.id)
    const added = r.plan.changes.filter((c) => c.type === 'channel.add').length
    const name = mode.value === 'new' ? newName.value.trim() : (props.bouquets.find((b) => b.ref === existing.value)?.name ?? '')
    saved.value = t('channels.iptv.saved', { n: number(added), bouquet: name }, added)
    result.value = null
    selected.value = new Set()
    emit('done')
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}
</script>
