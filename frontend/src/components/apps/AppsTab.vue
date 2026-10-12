<!--
  AppsHub's "Apps" tab: the curated catalog (backend/internal/appcatalog,
  never AI-generated) with install-as-a-plan (App4, #46-adjacent — no
  issue yet), free search beyond it with an on-click arch check (App3),
  and the installed apps themselves (start/stop direct, remove through
  the same preview-and-confirm flow every built-in hub's own actions use,
  PlanPreview.vue). Only a curated app can be installed in this first
  slice — a free-search result still only offers the compatibility check,
  since installing it needs a web port and config path App4 doesn't know
  for an arbitrary image (docs/APPSHUB_PLAN.md).
-->
<template>
  <div>
    <p v-if="loading" class="mt-4 text-[13px] text-faint">{{ t('common.loading') }}</p>
    <p v-else-if="loadError" class="mt-4 text-[13px] text-faint">{{ loadError }}</p>

    <template v-else>
      <section v-if="installed.length > 0" class="mt-4">
        <h2 class="text-[13px] font-semibold text-muted">{{ t('apps.installedTitle') }}</h2>
        <div class="mt-2 flex flex-col gap-2">
          <div v-for="a in installed" :key="a.id" class="flex items-center gap-3 rounded border border-line bg-tile p-3">
            <div class="min-w-0 flex-1">
              <p class="truncate text-[13.5px] font-semibold text-text">{{ a.name }}</p>
              <p class="mt-0.5 flex items-center gap-1.5 text-[12px]" :class="a.state === 'running' ? 'text-ok' : 'text-muted'">
                <span class="h-1.5 w-1.5 rounded-full" :class="a.state === 'running' ? 'bg-ok' : 'bg-faint'" />
                {{ a.state }}
              </p>
            </div>
            <a v-if="a.hostPort" :href="openUrl(a.hostPort)" target="_blank" rel="noopener" class="shrink-0 text-[12.5px] text-accent hover:underline">
              {{ t('apps.open') }}
            </a>
            <button
              type="button"
              class="shrink-0 rounded border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field disabled:opacity-60"
              :disabled="!!busyId"
              @click="toggleRunning(a)"
            >
              {{ a.state === 'running' ? t('apps.stop') : t('apps.start') }}
            </button>
            <button
              type="button"
              class="shrink-0 rounded border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field disabled:opacity-60"
              :disabled="!!busyId"
              @click="previewRemove(a)"
            >
              {{ t('apps.remove') }}
            </button>
          </div>
          <div v-if="removing" class="rounded border border-line bg-tile p-3">
            <PlanPreview
              v-if="removing.plan && !removing.done"
              :plan="removing.plan"
              :confirm-label="t('apps.removeConfirm')"
              :busy="removing.applying"
              :error="removing.error"
              danger
              @confirm="confirmRemove"
              @cancel="removing = null"
            >
              {{ removing.plan.summary }}
            </PlanPreview>
            <p v-else-if="removing.error" class="text-[12.5px] text-warn">{{ removing.error }}</p>
          </div>
        </div>
      </section>

      <section class="mt-5">
        <h2 class="text-[13px] font-semibold text-muted">{{ t('apps.catalogTitle') }}</h2>
        <div class="mt-2 flex flex-col gap-3">
          <section v-for="app in catalog" :key="app.id" class="flex items-center gap-3 rounded border border-line bg-tile p-3">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <h3 class="truncate text-[13.5px] font-semibold text-text">{{ app.name }}</h3>
                <span class="rounded-full bg-tile-2 px-2 py-0.5 text-[11px] text-muted">{{ t('apps.category.' + app.category) }}</span>
              </div>
              <p class="mt-0.5 truncate text-[12.5px] text-muted">{{ app.description }}</p>
            </div>
            <span v-if="!app.compatible" class="flex items-center gap-1.5 shrink-0 text-[12px] text-faint">
              <span class="h-1.5 w-1.5 rounded-full bg-faint" />
              {{ t('apps.notCompatible') }}
            </span>
            <span v-else-if="app.installed" class="shrink-0 text-[12px] text-muted">{{ t('apps.alreadyInstalled') }}</span>
            <button
              v-else
              type="button"
              class="shrink-0 rounded border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field disabled:opacity-60"
              :disabled="!!installing"
              @click="previewInstall(app)"
            >
              {{ t('apps.install') }}
            </button>
          </section>
        </div>
        <div v-if="installing" class="mt-3 rounded border border-line bg-tile p-3">
          <p v-if="installing.loading" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
          <PlanPreview
            v-else-if="installing.plan && !installing.done"
            :plan="installing.plan"
            :confirm-label="t('apps.installConfirm')"
            :busy="installing.applying"
            :error="installing.error"
            @confirm="confirmInstall"
            @cancel="installing = null"
          >
            {{ installing.plan.summary }}
          </PlanPreview>
          <p v-else-if="installing.done" class="flex items-center gap-1.5 text-[13px] text-ok" role="status">
            <span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('apps.installed') }}
          </p>
          <p v-else-if="installing.error" class="text-[12.5px] text-warn">{{ installing.error }}</p>
        </div>
      </section>

      <div class="mt-6 border-t border-line-soft pt-4">
        <input
          v-model="query"
          type="search"
          class="w-full rounded border border-line bg-field px-3 py-2 text-[13.5px] text-text placeholder:text-faint"
          :placeholder="t('apps.searchPlaceholder')"
          @keyup.enter="search"
        />
        <p class="mt-1.5 text-[12px] text-faint">{{ t('apps.notYetInstallable') }}</p>

        <p v-if="searching" class="mt-3 text-[13px] text-faint">{{ t('common.loading') }}</p>
        <p v-else-if="searchError" class="mt-3 text-[13px] text-faint">{{ searchError }}</p>
        <p v-else-if="searched && results.length === 0" class="mt-3 text-[13px] text-faint">{{ t('apps.searchEmpty') }}</p>

        <ul v-else-if="results.length > 0" class="mt-3 flex flex-col gap-2">
          <li v-for="r in results" :key="r.image" class="flex items-center gap-3 rounded border border-line bg-tile p-3">
            <div class="min-w-0 flex-1">
              <p class="truncate text-[13.5px] font-semibold text-text">{{ r.image }}</p>
              <p v-if="r.description" class="mt-0.5 truncate text-[12.5px] text-muted">{{ r.description }}</p>
            </div>
            <template v-if="manifests[r.image] === 'loading'">
              <span class="shrink-0 text-[12px] text-faint">{{ t('apps.checking') }}</span>
            </template>
            <template v-else-if="manifests[r.image]">
              <span class="flex items-center gap-1.5 shrink-0 text-[12px]" :class="manifests[r.image] === 'yes' ? 'text-ok' : 'text-faint'">
                <span class="h-1.5 w-1.5 rounded-full" :class="manifests[r.image] === 'yes' ? 'bg-ok' : 'bg-faint'" />
                {{ manifests[r.image] === 'yes' ? t('apps.compatible') : t('apps.notCompatible') }}
              </span>
            </template>
            <button
              v-else
              type="button"
              class="shrink-0 rounded border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field"
              @click="checkManifest(r.image)"
            >
              {{ t('apps.checkCompatibility') }}
            </button>
          </li>
        </ul>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import PlanPreview from '@/components/PlanPreview.vue'
import { useFriendlyError } from '@/composables/useFriendlyError'
import { usePlanError } from '@/composables/usePlanError'
import type { AppSearchResult, CatalogApp, InstalledApp, Plan } from '@/api/types'

const { t } = useI18n()
const { friendly } = useFriendlyError()
const { planErrorMessage } = usePlanError()

const catalog = ref<CatalogApp[]>([])
const installed = ref<InstalledApp[]>([])
const loading = ref(true)
const loadError = ref('')

async function load() {
  try {
    const [c, i] = await Promise.all([api.apps.catalog(), api.apps.installed()])
    catalog.value = c.apps
    installed.value = i.apps
    loadError.value = ''
  } catch {
    loadError.value = t('apps.loadError')
  } finally {
    loading.value = false
  }
}

function openUrl(port: number): string {
  return `${location.protocol}//${location.hostname}:${port}/`
}

// --- install: preview, confirm --------------------------------------------

type InstallPending = { appId: string; loading?: boolean; plan?: Plan; applying?: boolean; done?: boolean; error?: string }
const installing = ref<InstallPending | null>(null)

async function previewInstall(app: CatalogApp) {
  installing.value = { appId: app.id, loading: true }
  try {
    const res = await api.apps.installPlan(app.id)
    if (installing.value?.appId === app.id) installing.value = { appId: app.id, plan: res.plan }
  } catch (err) {
    if (installing.value?.appId === app.id) installing.value = { appId: app.id, error: planErrorMessage(err) }
  }
}

async function confirmInstall() {
  const p = installing.value
  if (!p?.plan) return
  installing.value = { ...p, applying: true, error: undefined }
  try {
    await api.plans.apply(p.plan.id)
    installing.value = { appId: p.appId, done: true }
    await load()
    window.setTimeout(() => {
      if (installing.value?.done) installing.value = null
    }, 3000)
  } catch (err) {
    installing.value = { appId: p.appId, error: planErrorMessage(err) }
  }
}

// --- remove: preview, confirm -----------------------------------------------

type RemovePending = { id: string; plan?: Plan; applying?: boolean; done?: boolean; error?: string }
const removing = ref<RemovePending | null>(null)

async function previewRemove(a: InstalledApp) {
  removing.value = { id: a.id }
  try {
    const res = await api.apps.removePlan(a.id)
    if (removing.value?.id === a.id) removing.value = { id: a.id, plan: res.plan }
  } catch (err) {
    if (removing.value?.id === a.id) removing.value = { id: a.id, error: planErrorMessage(err) }
  }
}

async function confirmRemove() {
  const p = removing.value
  if (!p?.plan) return
  removing.value = { ...p, applying: true, error: undefined }
  try {
    await api.plans.apply(p.plan.id)
    removing.value = null
    await load()
  } catch (err) {
    removing.value = { id: p.id, error: planErrorMessage(err) }
  }
}

// --- start/stop: direct, no preview (docs/APPSHUB_PLAN.md) ------------------

const busyId = ref('')

async function toggleRunning(a: InstalledApp) {
  busyId.value = a.id
  try {
    if (a.state === 'running') await api.apps.stop(a.id)
    else await api.apps.start(a.id)
    await load()
  } finally {
    busyId.value = ''
  }
}

// --- free search + on-click arch check (App3) -------------------------------

const query = ref('')
const results = ref<AppSearchResult[]>([])
const searching = ref(false)
const searched = ref(false)
const searchError = ref('')

async function search() {
  const q = query.value.trim()
  if (!q) return
  searching.value = true
  searchError.value = ''
  try {
    results.value = (await api.apps.search(q)).results
    searched.value = true
  } catch (err) {
    searchError.value = friendly(err).text || t('apps.searchError')
  } finally {
    searching.value = false
  }
}

const manifests = ref<Record<string, 'loading' | 'yes' | 'no'>>({})

async function checkManifest(image: string) {
  manifests.value = { ...manifests.value, [image]: 'loading' }
  try {
    const res = await api.apps.manifest(image)
    manifests.value = { ...manifests.value, [image]: res.compatible ? 'yes' : 'no' }
  } catch {
    const rest = { ...manifests.value }
    delete rest[image]
    manifests.value = rest
  }
}

onMounted(load)
</script>
