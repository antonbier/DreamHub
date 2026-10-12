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
        <div class="flex items-center justify-between">
          <h2 class="text-[13px] font-semibold text-muted">{{ t('apps.installedTitle') }}</h2>
          <RouterLink to="/device?logUnit=docker" class="text-[12px] text-accent hover:underline">{{ t('apps.dockerServiceLogs') }}</RouterLink>
        </div>
        <div class="mt-2 flex flex-col gap-2">
          <div v-for="a in installed" :key="a.id" class="flex items-center gap-3 rounded border border-line bg-tile p-3">
            <div class="min-w-0 flex-1">
              <p class="truncate text-[13.5px] font-semibold text-text">{{ a.name }}</p>
              <p class="mt-0.5 flex items-center gap-1.5 text-[12px]" :class="a.state === 'running' ? 'text-ok' : 'text-muted'">
                <span class="h-1.5 w-1.5 rounded-full" :class="a.state === 'running' ? 'bg-ok' : 'bg-faint'" />
                {{ a.state }}
                <span v-if="a.upgradable" class="ml-1 rounded-full bg-tile-2 px-1.5 py-0.5 text-[11px] text-accent">{{ t('apps.upgradable') }}</span>
              </p>
            </div>
            <a v-if="a.hostPort" :href="openUrl(a.hostPort)" target="_blank" rel="noopener" class="shrink-0 text-[12.5px] text-accent hover:underline">
              {{ t('apps.open') }}
            </a>
            <button type="button" class="shrink-0 text-[12.5px] text-accent hover:underline" @click="openLogs(a)">
              {{ t('apps.logs') }}
            </button>
            <button type="button" class="shrink-0 text-[12.5px] text-accent hover:underline" @click="openConsole(a)">
              {{ t('apps.console.open') }}
            </button>
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

    <!-- Logs modal (App5, "two tiers" — this is the per-container one;
         the daemon's own health reuses DeviceHub's existing viewer via
         the link above). -->
    <div v-if="logsFor" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="logsFor = null">
      <div class="flex max-h-[80vh] w-full max-w-2xl flex-col rounded border border-line bg-bg p-4">
        <div class="flex items-center justify-between">
          <h3 class="text-[14px] font-semibold text-text">{{ t('apps.logsTitle', { name: logsFor.name }) }}</h3>
          <button type="button" class="text-[12.5px] text-muted hover:text-text" @click="logsFor = null">{{ t('apps.logsClose') }}</button>
        </div>
        <p v-if="logsLoading" class="mt-3 text-[13px] text-faint">{{ t('common.loading') }}</p>
        <p v-else-if="logsError" class="mt-3 text-[12.5px] text-warn">{{ logsError }}</p>
        <p v-else-if="!logsText" class="mt-3 text-[13px] text-faint">{{ t('apps.logsEmpty') }}</p>
        <pre v-else class="mt-3 overflow-auto whitespace-pre-wrap rounded border border-line-soft bg-tile p-3 text-[12px] text-muted">{{ logsText }}</pre>
      </div>
    </div>

    <!-- Console modal (App5, Terminal's exact safety posture): off by
         default, needs a fresh password re-entry, a standing warning. -->
    <div v-if="consoleFor" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="closeConsole">
      <div class="flex max-h-[80vh] w-full max-w-2xl flex-col rounded border border-line bg-bg p-4">
        <div class="flex items-center justify-between">
          <h3 class="text-[14px] font-semibold text-text">{{ t('apps.console.title', { name: consoleFor.name }) }}</h3>
          <button type="button" class="text-[12.5px] text-muted hover:text-text" @click="closeConsole">{{ t('apps.console.close') }}</button>
        </div>

        <p v-if="!consoleStatusInfo" class="mt-3 text-[13px] text-faint">{{ t('common.loading') }}</p>
        <p v-else-if="!consoleStatusInfo.available" class="mt-3 text-[12.5px] text-faint">{{ t('apps.console.unavailable') }}</p>
        <p v-else-if="!consoleStatusInfo.authOn" class="mt-3 text-[12.5px] text-warn">{{ t('apps.console.authRequired') }}</p>

        <template v-else-if="!consoleStatusInfo.enabled">
          <p class="mt-3 text-[12.5px] text-warn">{{ t('apps.console.warning') }}</p>
          <form class="mt-3 flex items-center gap-2" @submit.prevent="enableConsoleNow">
            <input v-model="consolePassword" type="password" :placeholder="t('apps.console.password')" class="flex-1 rounded border border-line bg-field px-3 py-2 text-[13.5px] text-text" />
            <button type="submit" class="shrink-0 rounded border border-line px-3 py-2 text-[13px] text-text hover:bg-field">{{ t('apps.console.enable') }}</button>
          </form>
          <p v-if="consolePasswordError" class="mt-1.5 text-[12.5px] text-warn">{{ consolePasswordError }}</p>
        </template>

        <template v-else>
          <p class="mt-2 text-[12px] text-warn">{{ t('apps.console.warning') }}</p>
          <pre ref="consoleOutputEl" class="mt-2 h-64 overflow-auto whitespace-pre-wrap rounded border border-line-soft bg-tile p-3 text-[12px] text-muted">{{ consoleOutput }}</pre>
          <p v-if="consoleConnStatus" class="mt-1 text-[12px] text-faint">{{ consoleConnStatus }}</p>
          <form class="mt-2 flex items-center gap-2" @submit.prevent="sendConsoleInput">
            <input v-model="consoleInput" type="text" :placeholder="t('apps.console.inputPlaceholder')" class="flex-1 rounded border border-line bg-field px-3 py-2 text-[13.5px] text-text" />
            <button type="submit" class="shrink-0 rounded border border-line px-3 py-2 text-[13px] text-text hover:bg-field">{{ t('apps.console.send') }}</button>
          </form>
          <button type="button" class="mt-2 self-start text-[12.5px] text-warn hover:underline" @click="disableConsoleNow">{{ t('apps.console.disable') }}</button>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import PlanPreview from '@/components/PlanPreview.vue'
import { useFriendlyError } from '@/composables/useFriendlyError'
import { usePlanError } from '@/composables/usePlanError'
import type { AppConsoleStatus, AppSearchResult, CatalogApp, InstalledApp, Plan } from '@/api/types'

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

// --- per-container logs (App5) -----------------------------------------

const logsFor = ref<InstalledApp | null>(null)
const logsLoading = ref(false)
const logsError = ref('')
const logsText = ref('')

async function openLogs(a: InstalledApp) {
  logsFor.value = a
  logsLoading.value = true
  logsError.value = ''
  logsText.value = ''
  try {
    logsText.value = (await api.apps.logs(a.id)).text
  } catch {
    logsError.value = t('apps.logsError')
  } finally {
    logsLoading.value = false
  }
}

// --- per-container console (App5, Terminal's exact safety posture) ----------

const consoleFor = ref<InstalledApp | null>(null)
const consoleStatusInfo = ref<AppConsoleStatus | null>(null)
const consolePassword = ref('')
const consolePasswordError = ref('')
const consoleOutput = ref('')
const consoleConnStatus = ref('')
const consoleInput = ref('')
const consoleOutputEl = ref<HTMLElement | null>(null)
let consoleSocket: WebSocket | null = null

async function openConsole(a: InstalledApp) {
  consoleFor.value = a
  consolePassword.value = ''
  consolePasswordError.value = ''
  consoleOutput.value = ''
  consoleConnStatus.value = ''
  consoleStatusInfo.value = await api.apps.consoleStatus()
  if (consoleStatusInfo.value.enabled) connectConsole(a.id)
}

function closeConsole() {
  disconnectConsole()
  consoleFor.value = null
}

async function enableConsoleNow() {
  if (!consoleFor.value) return
  consolePasswordError.value = ''
  try {
    await api.apps.enableConsole(consolePassword.value)
    consolePassword.value = ''
    consoleStatusInfo.value = await api.apps.consoleStatus()
    connectConsole(consoleFor.value.id)
  } catch (err) {
    consolePasswordError.value = friendly(err).text || t('apps.console.wrongPassword')
  }
}

async function disableConsoleNow() {
  disconnectConsole()
  await api.apps.disableConsole()
  consoleStatusInfo.value = await api.apps.consoleStatus()
}

function connectConsole(id: string) {
  disconnectConsole()
  const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const url = `${wsProtocol}//${location.host}${api.apps.consolePath(id)}`
  const socket = new WebSocket(url)
  socket.binaryType = 'arraybuffer'
  consoleConnStatus.value = t('apps.console.connecting')
  socket.onopen = () => {
    consoleConnStatus.value = ''
  }
  socket.onmessage = (ev) => {
    const text = typeof ev.data === 'string' ? ev.data : new TextDecoder().decode(ev.data as ArrayBuffer)
    consoleOutput.value += text
    void nextTick(() => {
      consoleOutputEl.value?.scrollTo(0, consoleOutputEl.value.scrollHeight)
    })
  }
  socket.onclose = () => {
    consoleConnStatus.value = t('apps.console.disconnected')
  }
  consoleSocket = socket
}

function disconnectConsole() {
  consoleSocket?.close()
  consoleSocket = null
}

function sendConsoleInput() {
  if (!consoleSocket || consoleSocket.readyState !== WebSocket.OPEN) return
  consoleSocket.send(new TextEncoder().encode(consoleInput.value + '\n'))
  consoleInput.value = ''
}

onUnmounted(disconnectConsole)
</script>
