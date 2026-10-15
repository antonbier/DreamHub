<!--
  AdminHub's package browser (docs/ADMINHUB_PLAN.md, A2, #47): the box's
  installed and upgradable packages, grouped by apt's own Section field —
  no DreamHub-maintained category lookup, so an unseen Section just becomes
  a new group. Install, upgrade and remove go through a plan whose
  preview is apt-get's own simulate run — every package it would touch,
  including the ones nobody named (removing one skin component can take
  whole skins with it on the box).
-->
<template>
  <section class="mt-5 rounded border border-line bg-tile p-4">
    <div class="flex items-center gap-3">
      <h2 class="text-[14px] font-semibold text-text">{{ t('admin.packages.title') }}</h2>
      <button type="button" class="ms-auto rounded p-1 text-muted hover:bg-tile-2 hover:text-text" :aria-label="t('admin.logs.refresh')" :title="t('admin.logs.refresh')" @click="load">
        <Icon name="refresh" :size="15" />
      </button>
    </div>

    <p class="mt-1 text-[12.5px] text-muted">{{ t('admin.packages.intro') }}</p>

    <div class="mt-3 flex flex-wrap gap-1 rounded bg-field p-1 text-[12.5px]" role="radiogroup" :aria-label="t('admin.packages.title')">
      <button
        v-for="v in views"
        :key="v.id"
        type="button"
        role="radio"
        :aria-checked="status === v.id"
        class="rounded px-2.5 py-1"
        :class="status === v.id ? 'bg-tile text-text' : 'text-muted hover:text-text'"
        @click="setView(v.id)"
      >
        {{ t('admin.packages.view.' + (v.id || 'installed')) }}<template v-if="v.id === 'upgradable' && upgradableCount > 0"> ({{ upgradableCount }})</template>
      </button>
    </div>

    <div class="mt-2 flex flex-wrap gap-2">
      <input
        v-model="q"
        type="search"
        class="min-w-0 flex-1 rounded border border-line bg-field px-2.5 py-1.5 text-[13px] text-text"
        :placeholder="t('admin.packages.search')"
        @input="debouncedLoad"
      />
      <select v-model="category" class="rounded border border-line bg-field px-2 py-1.5 text-[13px] text-text" @change="load">
        <option value="">{{ t('admin.packages.allCategories') }}</option>
        <option v-for="c in categories" :key="c" :value="c">{{ c }}</option>
      </select>
    </div>

    <div v-if="upgradableCount > 0 && !pending" class="mt-2 flex items-center gap-2 text-[12.5px] text-muted">
      <span>{{ t('admin.packages.upgradableCount', { count: upgradableCount }, upgradableCount) }}</span>
      <button type="button" class="rounded border border-line px-2 py-0.5 text-text hover:bg-field" @click="preview('upgrade', [])">
        {{ t('admin.packages.upgradeAll') }}
      </button>
    </div>

    <div v-if="pending" class="mt-3">
      <p v-if="!pending.plan && !pending.error && !pending.run" class="text-[13px] text-faint">{{ t('admin.packages.simulating') }}</p>
      <PlanPreview
        v-else-if="pending.plan && !pending.run"
        :plan="pending.plan"
        :confirm-label="t('admin.packages.confirm')"
        :busy="pending.applying"
        :error="pending.error"
        :danger="pending.steps.some((s) => s.kind === 'remove')"
        @confirm="apply"
        @cancel="pending = null"
      >
        <p>{{ t('admin.packages.previewIntro', { count: pending.steps.length }, pending.steps.length) }}</p>
        <ul class="mt-1 flex flex-col gap-0.5 font-mono text-[12.5px]">
          <li v-for="st in pending.steps" :key="st.kind + st.name" :class="st.kind === 'remove' ? 'text-onair' : 'text-text'">
            {{ t('admin.packages.step.' + st.kind, { name: st.name, from: st.oldVersion ?? '', to: st.newVersion ?? '' }) }}
          </li>
        </ul>
      </PlanPreview>
      <div v-else-if="pending.run" class="rounded border border-line bg-bg p-3">
        <div class="flex items-center justify-between gap-3">
          <span class="flex items-center gap-1.5 text-[13px]" :class="pending.run.ok ? 'text-ok' : 'text-onair'">
            <span class="h-1.5 w-1.5 rounded-full" :class="pending.run.ok ? 'bg-ok' : 'bg-onair'" />
            {{ pending.run.ok ? t('admin.packages.done') : t('admin.packages.failed') }}
          </span>
          <button type="button" class="text-[12.5px] text-accent hover:underline" @click="pending = null">{{ t('admin.crashLogs.close') }}</button>
        </div>
        <pre v-if="pending.run.output" class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words font-mono text-[12px] text-text">{{ pending.run.output }}</pre>
      </div>
      <div v-else-if="pending.error" class="flex items-start justify-between gap-3">
        <p class="whitespace-pre-wrap text-[12.5px] text-warn">{{ pending.error }}</p>
        <button type="button" class="shrink-0 text-[12.5px] text-accent hover:underline" @click="pending = null">{{ t('admin.crashLogs.close') }}</button>
      </div>
    </div>

    <p v-if="loading && !packages.length" class="mt-2 text-[13px] text-faint">{{ t('common.loading') }}</p>
    <p v-else-if="error" class="mt-2 text-[13px] text-faint">{{ error }}</p>
    <p v-else-if="packages.length === 0" class="mt-2 text-[13px] text-faint">{{ t('admin.packages.empty') }}</p>
    <p v-if="truncated" class="mt-2 text-[12px] text-faint">{{ t('admin.packages.truncated') }}</p>
    <ul v-else class="mt-2 flex max-h-96 flex-col overflow-auto">
      <li v-for="p in packages" :key="p.name" class="border-b border-line-soft py-2 last:border-none">
        <div class="flex items-baseline gap-2">
          <span class="font-mono text-[13px] text-text">{{ p.name }}</span>
          <span v-if="p.category" class="shrink-0 rounded bg-tile-2 px-1.5 py-0.5 text-[11px] text-faint">{{ p.category }}</span>
          <span v-if="p.neededBy?.length" class="shrink-0 rounded border border-accent px-1.5 py-0.5 text-[11px] text-accent">{{ t('admin.packages.neededBy', { hubs: p.neededBy.map(hubName).join(', ') }) }}</span>
          <button
            v-if="aiReady"
            type="button"
            class="tap ms-auto shrink-0 rounded px-2 py-0.5 text-[12px] text-accent hover:bg-tile-2"
            :disabled="explaining === p.name"
            @click="explain(p)"
          >
            {{ t('admin.packages.explain') }}
          </button>
        </div>
        <div v-if="p.category !== 'self'" class="mt-1 flex gap-2">
          <button v-if="catalog || p.missing" type="button" class="rounded border border-line px-2 py-0.5 text-[12px] text-text hover:bg-field disabled:opacity-60" :disabled="!!pending" @click="preview('install', [p.name])">
            {{ t('admin.packages.install') }}
          </button>
          <template v-else>
            <button v-if="p.upgradable" type="button" class="rounded border border-line px-2 py-0.5 text-[12px] text-text hover:bg-field disabled:opacity-60" :disabled="!!pending" @click="preview('upgrade', [p.name])">
              {{ t('admin.packages.upgrade') }}
            </button>
            <button type="button" class="rounded px-2 py-0.5 text-[12px] text-muted hover:bg-field hover:text-text disabled:opacity-60" :disabled="!!pending" @click="preview('remove', [p.name])">
              {{ t('admin.packages.remove') }}
            </button>
          </template>
        </div>
        <p class="truncate text-[12.5px] text-faint">{{ p.description }}</p>
        <p class="text-[12.5px]" :class="p.upgradable || p.missing ? 'text-warn' : 'text-muted'">
          {{ p.missing ? t('admin.packages.missing') : p.upgradable ? t('admin.packages.upgradableFrom', { from: p.version, to: p.availableVersion }) : p.version }}
        </p>
        <p v-if="explained[p.name]" class="mt-1 rounded border border-line bg-bg p-2 text-[12.5px] text-text">{{ explained[p.name] }}</p>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { useFriendlyError } from '@/composables/useFriendlyError'
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import Icon from '@/components/Icon.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import { usePlanError } from '@/composables/usePlanError'
import { useHubLabel } from '@/composables/useHubLabel'
import type { AdminPackage, PackageRun, PackageStep, Plan } from '@/api/types'

const { t } = useI18n()
const { hubName } = useHubLabel()
const { friendly } = useFriendlyError()
const { planErrorMessage } = usePlanError()

const packages = ref<AdminPackage[]>([])
const categories = ref<string[]>([])
const loading = ref(true)
const error = ref('')
const q = ref('')
const category = ref('')
const status = ref('')
const views = [{ id: '' }, { id: 'upgradable' }, { id: 'available' }, { id: 'needed' }]
function setView(id: string) {
  status.value = id
  void load()
}
// The rows on screen are the "not installed" catalog (install buttons)
// rather than installed packages (upgrade/remove).
const catalog = ref(false)
const truncated = ref(false)
const upgradableCount = ref(0)

async function load() {
  loading.value = true
  try {
    const res = await api.admin.packages({ q: q.value, category: category.value, status: status.value })
    packages.value = res.packages
    categories.value = res.categories
    truncated.value = !!res.truncated
    catalog.value = status.value === 'available'
    error.value = ''
    void countUpgradable()
  } catch (err) {
    error.value = err instanceof ApiError && err.code === 'apt_unavailable' ? t('admin.packages.unavailable') : friendly(err).text
  } finally {
    loading.value = false
  }
}

async function countUpgradable() {
  try {
    upgradableCount.value = (await api.admin.packages({ status: 'upgradable' })).packages.length
  } catch {
    upgradableCount.value = 0
  }
}

// --- install, upgrade, remove: preview, confirm, show apt's output -------

type Pending = { plan?: Plan; steps: PackageStep[]; applying?: boolean; error?: string; run?: PackageRun }
const pending = ref<Pending | null>(null)

// Which preview request is current: a ref wraps its object in a proxy, so
// the pending object itself can't be compared by identity.
let previewSeq = 0

async function preview(action: 'install' | 'remove' | 'upgrade', names: string[]) {
  const mine = ++previewSeq
  pending.value = { steps: [] }
  try {
    const res = await api.admin.packagesPlan(action, names)
    if (previewSeq === mine && pending.value) pending.value = { plan: res.plan, steps: res.steps }
  } catch (err) {
    if (previewSeq !== mine || !pending.value) return
    // apt's own explanation (an unknown package, a dependency it can't
    // satisfy) is the most useful thing to show here.
    const aptSays = err instanceof ApiError && err.code === 'apt_refused' ? '\n' + err.message : ''
    pending.value = { steps: [], error: planErrorMessage(err) + aptSays }
  }
}

async function apply() {
  const p = pending.value
  if (!p?.plan) return
  pending.value = { ...p, applying: true, error: undefined }
  let failure = ''
  let failureCode = ''
  try {
    await api.plans.apply(p.plan.id)
  } catch (err) {
    failure = planErrorMessage(err)
    failureCode = err instanceof ApiError ? err.code : ''
  }
  // apt-get ran (successfully or not): its output says what happened.
  if (!failure || failureCode === 'package_apply_failed') {
    const run = await api.admin.packagesLastRun().then((r) => r.run).catch(() => null)
    if (run) {
      pending.value = { steps: p.steps, run }
      void load()
      return
    }
  }
  pending.value = failure ? { ...p, applying: false, error: failure } : null
  void load()
}

let debounceTimer: number | undefined
function debouncedLoad() {
  window.clearTimeout(debounceTimer)
  debounceTimer = window.setTimeout(load, 300)
}

// --- AI explain, on request only ----------------------------------------

const explaining = ref('')
const explained = reactive<Record<string, string>>({})

async function explain(p: AdminPackage) {
  explaining.value = p.name
  try {
    const res = await api.admin.explainPackage(p.name, p.description)
    explained[p.name] = res.explanation
  } catch (err) {
    explained[p.name] = friendly(err).text
  } finally {
    explaining.value = ''
  }
}

// The explanation comes from the AI provider: without a key the button
// would only fail, so it is not offered.
const aiReady = ref(false)
onMounted(() => {
  void load()
  api
    .connectors()
    .then((res) => (aiReady.value = res.connectors.some((c) => c.id === 'openai' && c.state === 'ok')))
    .catch(() => (aiReady.value = false))
})
</script>
