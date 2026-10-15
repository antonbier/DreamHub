<!--
  The Packages tab's list (docs/APPSHUB_PLAN.md, App11): search, the filters
  Installed / Available / Updates plus category chips, a bar "N updates →
  Update all" (one plan, one preview), and one action per row. A row's
  description is the curated text, else the package's own, else nothing
  (internal/pkginfo) — an AI is only an optional last resort behind "⋯".
  Install, update and remove go through a plan whose preview is apt-get's
  own simulate run — every package it would touch, including the ones
  nobody named (removing one skin component can take whole skins with it
  on the box).
-->
<template>
  <section class="mt-5 rounded-sm border border-line bg-tile p-4">
    <div class="flex items-center gap-2">
      <input
        v-model="q"
        type="search"
        class="min-w-0 flex-1 rounded-sm border border-line bg-field px-2.5 py-1.5 text-[13px] text-text"
        :placeholder="t('admin.packages.search')"
        :aria-label="t('admin.packages.search')"
        @input="debouncedLoad"
      />
      <button type="button" class="tap shrink-0 rounded-sm p-1 text-muted hover:bg-tile-2 hover:text-text" :aria-label="t('admin.logs.refresh')" :title="t('admin.logs.refresh')" @click="load">
        <Icon name="refresh" :size="15" />
      </button>
    </div>

    <div class="mt-3 flex flex-wrap gap-1.5" role="radiogroup" :aria-label="t('admin.packages.title')">
      <button
        v-for="v in views"
        :key="v"
        type="button"
        role="radio"
        :aria-checked="status === v"
        class="tap rounded-full border px-3 py-1 text-[12.5px]"
        :class="status === v ? 'border-accent bg-tile-2 text-text' : 'border-line text-muted hover:text-text'"
        @click="setView(v)"
      >
        {{ t('admin.packages.view.' + (v || 'installed')) }}<template v-if="v === 'upgradable' && upgradableCount > 0"> ({{ upgradableCount }})</template>
      </button>
    </div>

    <div v-if="categories.length > 1 || category" class="mt-2 flex flex-wrap gap-1.5" role="radiogroup" :aria-label="t('admin.packages.categories')">
      <button
        type="button"
        role="radio"
        :aria-checked="category === ''"
        class="tap rounded-full border px-2.5 py-0.5 text-[12px]"
        :class="category === '' ? 'border-accent bg-tile-2 text-text' : 'border-line text-muted hover:text-text'"
        @click="setCategory('')"
      >
        {{ t('admin.packages.allCategories') }}
      </button>
      <button
        v-for="c in categories"
        :key="c.id"
        type="button"
        role="radio"
        :aria-checked="category === c.id"
        class="tap rounded-full border px-2.5 py-0.5 text-[12px]"
        :class="category === c.id ? 'border-accent bg-tile-2 text-text' : 'border-line text-muted hover:text-text'"
        @click="setCategory(c.id)"
      >
        {{ c.label }}
      </button>
    </div>

    <div v-if="upgradableCount > 0 && !pending" class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-sm border border-accent bg-tile-2 px-3 py-2 text-[13px] text-text">
      <span>{{ t('admin.packages.upgradableCount', { count: upgradableCount }, upgradableCount) }}</span>
      <button type="button" class="tap ms-auto rounded-sm bg-accent px-3 py-1 text-[12.5px] font-semibold text-bg hover:opacity-90" @click="preview('upgrade', [])">
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
      <div v-else-if="pending.run" class="rounded-sm border border-line bg-bg p-3">
        <div class="flex items-center justify-between gap-3">
          <span class="flex items-center gap-1.5 text-[13px]" :class="pending.run.ok ? 'text-ok' : 'text-onair'">
            <span class="h-1.5 w-1.5 rounded-full" :class="pending.run.ok ? 'bg-ok' : 'bg-onair'" />
            {{ pending.run.ok ? t('admin.packages.done') : t('admin.packages.failed') }}
          </span>
          <button type="button" class="text-[12.5px] text-accent hover:underline" @click="pending = null">{{ t('admin.crashLogs.close') }}</button>
        </div>
        <pre v-if="pending.run.output" class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap wrap-break-word font-mono text-[12px] text-text">{{ pending.run.output }}</pre>
      </div>
      <div v-else-if="pending.error" class="flex items-start justify-between gap-3">
        <p class="whitespace-pre-wrap text-[12.5px] text-warn">{{ pending.error }}</p>
        <button type="button" class="shrink-0 text-[12.5px] text-accent hover:underline" @click="pending = null">{{ t('admin.crashLogs.close') }}</button>
      </div>
    </div>

    <p v-if="loading && !packages.length" class="mt-3 text-[13px] text-faint">{{ t('common.loading') }}</p>
    <p v-else-if="error" class="mt-3 text-[13px] text-faint">{{ error }}</p>
    <p v-else-if="packages.length === 0" class="mt-3 text-[13px] text-faint">{{ t('admin.packages.empty') }}</p>
    <ul v-else class="mt-2 flex flex-col">
      <li v-for="p in packages" :key="p.name" class="border-b border-line-soft py-2.5 last:border-none">
        <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span class="min-w-0 break-all font-mono text-[13px] text-text">{{ p.name }}</span>
          <span v-if="p.info.categoryLabel" class="shrink-0 rounded-sm bg-tile-2 px-1.5 py-0.5 text-[11px] text-faint">{{ p.info.categoryLabel }}</span>
          <div v-if="p.category !== 'self'" class="ms-auto flex shrink-0 items-center gap-1">
            <button
              type="button"
              class="tap rounded-sm border px-2.5 py-0.5 text-[12.5px] disabled:opacity-60"
              :class="primary(p).action === 'remove' ? 'border-line text-muted hover:bg-field hover:text-text' : 'border-accent text-accent hover:bg-tile-2'"
              :disabled="!!pending"
              @click="preview(primary(p).action, [p.name])"
            >
              {{ t('admin.packages.' + primary(p).label) }}
            </button>
            <DropdownMenu>
              <template #trigger>
                <button type="button" class="tap rounded-sm p-1.5 text-muted hover:bg-tile-2 hover:text-text" :aria-label="t('apps.more')">
                  <Icon name="more" :size="16" />
                </button>
              </template>
              <template #panel="{ close }">
                <button type="button" class="block w-full px-3 py-1.5 text-start text-[13px] text-text hover:bg-tile-2" @click="close(); toggleDetails(p.name)">
                  {{ t('admin.packages.details') }}
                </button>
                <button v-if="aiReady && p.info.source !== 'curated'" type="button" class="block w-full px-3 py-1.5 text-start text-[13px] text-text hover:bg-tile-2" :disabled="explaining === p.name" @click="close(); explain(p)">
                  {{ t('admin.packages.explain') }}
                </button>
                <button v-if="p.upgradable && !catalog" type="button" class="block w-full px-3 py-1.5 text-start text-[13px] text-onair hover:bg-tile-2" :disabled="!!pending" @click="close(); preview('remove', [p.name])">
                  {{ t('admin.packages.remove') }}
                </button>
              </template>
            </DropdownMenu>
          </div>
        </div>
        <p v-if="p.info.description" class="mt-0.5 text-[13px] text-muted">{{ p.info.description }}</p>
        <p class="mt-0.5 text-[12.5px]" :class="p.upgradable ? 'text-warn' : 'text-faint'">
          <template v-if="p.upgradable">{{ t('admin.packages.upgradableFrom', { from: p.version, to: p.availableVersion }) }}</template>
          <template v-else>{{ p.version }}</template>
          <template v-if="p.neededBy?.length"> · <span class="text-accent">{{ t('admin.packages.neededBy', { hubs: p.neededBy.map(hubName).join(', ') }) }}</span></template>
        </p>
        <dl v-if="open.has(p.name)" class="mt-2 grid grid-cols-[max-content_1fr] gap-x-3 gap-y-0.5 rounded-sm border border-line bg-bg p-2.5 text-[12.5px]">
          <dt class="text-faint">{{ t('admin.packages.detail.version') }}</dt>
          <dd class="font-mono text-text">{{ p.version }}<template v-if="p.upgradable"> → {{ p.availableVersion }}</template></dd>
          <template v-if="p.category">
            <dt class="text-faint">{{ t('admin.packages.detail.section') }}</dt>
            <dd class="font-mono text-text">{{ p.category }}</dd>
          </template>
          <dt class="text-faint">{{ t('admin.packages.detail.source') }}</dt>
          <dd class="text-text">{{ t('admin.packages.source.' + p.info.source) }}</dd>
          <template v-if="p.description && p.description !== p.info.description">
            <dt class="text-faint">{{ t('admin.packages.detail.own') }}</dt>
            <dd class="text-text">{{ p.description }}</dd>
          </template>
        </dl>
        <p v-if="explained[p.name]" class="mt-1 rounded-sm border border-line bg-bg p-2 text-[12.5px] text-text">{{ explained[p.name] }}</p>
      </li>
    </ul>
    <p v-if="truncated" class="mt-2 text-[12px] text-faint">{{ t('admin.packages.truncated') }}</p>
  </section>
</template>

<script setup lang="ts">
import { useFriendlyError } from '@/composables/useFriendlyError'
import { onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import DropdownMenu from '@/components/DropdownMenu.vue'
import Icon from '@/components/Icon.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import { usePlanError } from '@/composables/usePlanError'
import { useHubLabel } from '@/composables/useHubLabel'
import type { AdminPackage, PackageCategory, PackageRun, PackageStep, Plan } from '@/api/types'

const { t, locale } = useI18n()
const { hubName } = useHubLabel()
const { friendly } = useFriendlyError()
const { planErrorMessage } = usePlanError()

const packages = ref<AdminPackage[]>([])
const categories = ref<PackageCategory[]>([])
const loading = ref(true)
const error = ref('')
const q = ref('')
const category = ref('')
const status = ref('')
const views = ['', 'available', 'upgradable']
function setView(id: string) {
  status.value = id
  void load()
}
function setCategory(id: string) {
  category.value = id
  void load()
}

// The one action a row offers up front; the rest is behind "⋯".
function primary(p: AdminPackage): { action: 'install' | 'upgrade' | 'remove'; label: string } {
  if (catalog.value || p.missing) return { action: 'install', label: 'install' }
  if (p.upgradable) return { action: 'upgrade', label: 'upgrade' }
  return { action: 'remove', label: 'remove' }
}

const open = reactive(new Set<string>())
function toggleDetails(name: string) {
  if (!open.delete(name)) open.add(name)
}
// The rows on screen are the "not installed" catalog (install buttons)
// rather than installed packages (upgrade/remove).
const catalog = ref(false)
const truncated = ref(false)
const upgradableCount = ref(0)

async function load() {
  loading.value = true
  let again = false
  try {
    const res = await api.admin.packages({ q: q.value, category: category.value, status: status.value, lang: locale.value })
    packages.value = res.packages
    categories.value = res.categories
    // A category that this view doesn't hold (switched from Installed to
    // Available, say) would show an empty list with no chip to undo it.
    if (category.value && !res.categories.some((c) => c.id === category.value)) {
      category.value = ''
      again = true
      return
    }
    truncated.value = !!res.truncated
    catalog.value = status.value === 'available'
    error.value = ''
    void countUpgradable()
  } catch (err) {
    error.value = err instanceof ApiError && err.code === 'apt_unavailable' ? t('admin.packages.unavailable') : friendly(err).text
  } finally {
    loading.value = false
  }
  if (again) await load()
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
watch(locale, () => void load())
onMounted(() => {
  void load()
  api
    .connectors()
    .then((res) => (aiReady.value = res.connectors.some((c) => c.id === 'openai' && c.state === 'ok')))
    .catch(() => (aiReady.value = false))
})
</script>
