<!--
  The "Docker" tab: the curated catalog (backend/internal/appcatalog,
  never AI-generated) with install-as-a-plan (App4, #46-adjacent — no
  issue yet), free search beyond it with an on-click arch check (App3),
  and the installed apps themselves (start/stop direct, remove through
  the same preview-and-confirm flow every built-in hub's own actions use,
  PlanPreview.vue). Only a curated app can be installed in this first
  slice — a free-search result still only offers the compatibility check,
  since installing it needs a web port and config path App4 doesn't know
  for an arbitrary image (docs/APPSHUB_PLAN.md). A curated app may be
  several containers (Nextcloud and its database): one row, its helpers
  listed under it, each with its own logs.
-->
<template>
  <div>
    <TabIntro :text="t('apps.intro.docker')" :gear-label="t('apps.docker.gear')" @gear="settingsOpen = true" />
    <p v-if="loading" class="mt-4 text-[13px] text-faint">{{ t('common.loading') }}</p>
    <p v-else-if="loadError" class="mt-4 text-[13px] text-faint">{{ loadError }}</p>

    <template v-else>
      <p v-if="dockerMissing" class="mt-4 rounded-sm border border-line bg-tile p-4 text-[13px] text-muted">
        {{ t('apps.dockerMissing') }}
        <button type="button" class="ms-1 text-accent hover:underline" @click="settingsOpen = true">{{ t('apps.docker.openSettings') }}</button>
      </p>
      <p v-else-if="installedError" class="mt-4 text-[13px] text-faint">{{ installedError }}</p>
      <!-- Two views, one sentence each. -->
      <div class="mt-4 flex gap-1 rounded-sm border border-line bg-tile p-1" role="tablist" :aria-label="t('apps.views.label')">
        <button
          v-for="v in (['mine', 'catalog'] as const)"
          :key="v"
          type="button"
          role="tab"
          :aria-selected="view === v"
          class="flex-1 rounded-sm px-3 py-1.5 text-[13px] font-semibold"
          :class="view === v ? 'bg-accent text-bg' : 'text-muted hover:bg-tile-2 hover:text-text'"
          @click="setView(v)"
        >
          {{ t('apps.views.' + v) }}<span v-if="v === 'mine' && installed.length" class="ms-1.5 font-normal opacity-80">{{ installed.length }}</span>
        </button>
      </div>
      <p class="mt-2 text-[12.5px] text-muted">{{ t('apps.views.' + view + 'Hint') }}</p>

      <section v-if="view === 'mine'" class="mt-3">
        <div v-if="installed.length > 0" class="flex justify-end">
          <RouterLink to="/device?logUnit=docker" class="text-[12px] text-accent hover:underline">{{ t('apps.dockerServiceLogs') }}</RouterLink>
        </div>
        <p v-if="installed.length === 0 && !dockerMissing" class="rounded-sm border border-line bg-tile p-4 text-[13px] text-muted">
          {{ t('apps.mine.empty') }}
          <button type="button" class="ms-1 text-accent hover:underline" @click="setView('catalog')">{{ t('apps.mine.emptyAction') }}</button>
        </p>
        <div class="mt-2 flex flex-col gap-2">
          <div v-for="a in installed" :key="a.id">
            <div class="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-sm border border-line bg-tile p-3" :class="editing?.id === a.id ? 'rounded-b-none' : ''">
              <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-sm bg-tile-2 text-muted" aria-hidden="true">
                <Icon :name="rowIcon(a)" :size="18" />
              </span>
              <div class="min-w-0 flex-1 basis-40">
                <p class="flex items-center gap-2">
                  <span class="truncate text-[13.5px] font-semibold text-text">{{ a.name }}</span>
                  <span v-if="a.upgradable" class="shrink-0 rounded-full bg-tile-2 px-1.5 py-0.5 text-[11px] text-accent">{{ t('apps.upgradable') }}</span>
                </p>
                <p class="mt-0.5 flex items-center gap-1.5 text-[12px]" :class="a.state === 'running' ? 'text-ok' : a.state === 'restarting' ? 'text-warn' : 'text-muted'">
                  <span class="h-1.5 w-1.5 rounded-full" :class="a.state === 'running' ? 'bg-ok' : a.state === 'restarting' ? 'bg-warn' : 'bg-faint'" />
                  {{ stateText(a.state) }}
                  <span v-if="a.hostPort" class="text-faint">· {{ t('apps.port', { port: a.hostPort }) }}</span>
                </p>
                <p v-for="s in a.services ?? []" :key="s.id" class="mt-0.5 flex flex-wrap items-center gap-x-1.5 text-[12px] text-muted">
                  <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="s.state === 'running' ? 'bg-ok' : 'bg-faint'" />
                  {{ serviceLabel(s.service) }} · {{ stateText(s.state) }}
                  <button type="button" class="ms-1 text-accent hover:underline" @click="openLogs({ ...a, id: s.id, name: a.name + ' · ' + serviceLabel(s.service) })">
                    {{ t('apps.logs') }}
                  </button>
                </p>
              </div>
              <button
                type="button"
                role="switch"
                :aria-checked="a.autostart"
                :aria-label="t('apps.autostart.label', { name: a.name })"
                :title="t('apps.autostart.hint')"
                class="flex shrink-0 items-center gap-2 text-[12px] text-muted"
                @click="toggleAutostart(a)"
              >
                <span class="relative h-5 w-9 rounded-full transition-colors" :class="a.autostart ? 'bg-accent' : 'bg-field border border-line'">
                  <span class="absolute top-0.5 h-4 w-4 rounded-full bg-bg transition-all" :class="a.autostart ? 'inset-s-[1.1rem]' : 'inset-s-0.5 !bg-faint'" />
                </span>
                {{ t('apps.autostart.short') }}
              </button>
              <a
                v-if="a.state === 'running' && a.hostPort"
                :href="openUrl(a.hostPort)"
                target="_blank"
                rel="noopener"
                class="shrink-0 rounded-sm bg-accent px-3 py-1 text-[12.5px] font-semibold text-bg"
              >
                {{ t('apps.open') }}
              </a>
              <button
                v-else
                type="button"
                class="shrink-0 rounded-sm px-3 py-1 text-[12.5px] disabled:opacity-60"
                :class="a.state === 'running' ? 'border border-line text-text hover:bg-field' : 'bg-accent font-semibold text-bg'"
                :disabled="!!busyId"
                @click="toggleRunning(a)"
              >
                {{ a.state === 'running' ? t('apps.stop') : t('apps.start') }}
              </button>
              <DropdownMenu>
                <template #trigger>
                  <button type="button" class="rounded-sm p-1.5 text-muted hover:bg-tile-2 hover:text-text" :aria-label="t('apps.more')">
                    <Icon name="more" :size="16" />
                  </button>
                </template>
                <template #panel="{ close }">
                  <button v-if="a.state === 'running' && a.hostPort" type="button" class="block w-full px-3 py-1.5 text-start text-[13px] text-text hover:bg-tile-2" :disabled="!!busyId" @click="close(); toggleRunning(a)">{{ t('apps.stop') }}</button>
                  <button type="button" class="block w-full px-3 py-1.5 text-start text-[13px] text-text hover:bg-tile-2" @click="close(); openLogs(a)">{{ t('apps.logs') }}</button>
                  <button type="button" class="block w-full px-3 py-1.5 text-start text-[13px] text-text hover:bg-tile-2" @click="close(); openConsole(a)">{{ t('apps.console.open') }}</button>
                  <button type="button" class="block w-full px-3 py-1.5 text-start text-[13px] text-text hover:bg-tile-2" @click="close(); openEdit(a)">{{ t('apps.edit.menu') }}</button>
                  <RouterLink to="/settings/system" class="block w-full px-3 py-1.5 text-start text-[13px] text-text hover:bg-tile-2" @click="close()">{{ t('apps.backupMenu') }}</RouterLink>
                  <button type="button" class="block w-full px-3 py-1.5 text-start text-[13px] text-text hover:bg-tile-2" :disabled="!!busyId" @click="close(); previewUpdate(a)">{{ t('apps.update.menu') }}</button>
                  <button type="button" class="block w-full px-3 py-1.5 text-start text-[13px] text-onair hover:bg-tile-2" :disabled="!!busyId" @click="close(); previewRemove(a)">{{ t('apps.remove') }}</button>
                </template>
              </DropdownMenu>
            </div>
            <div v-if="editing?.id === a.id" class="rounded-b-sm border border-t-0 border-line bg-tile p-3">
              <AppInstallForm :source="editing.source" :edit="{ id: a.id, view: editing.view }" @done="onEdited" @cancel="editing = null" />
            </div>
            <div v-if="removing?.id === a.id" class="rounded-sm border border-line bg-tile p-3" :class="'mt-1'">
              <PlanPreview
                v-if="removing.plan && !removing.done"
                :plan="removing.plan"
                :confirm-label="removing.kind === 'update' ? t('apps.update.confirm') : t('apps.removeConfirm')"
                :busy="removing.applying"
                :error="removing.error"
                :danger="removing.kind !== 'update'"
                @confirm="confirmRemove"
                @cancel="removing = null"
              >
                {{ removing.plan.summary }}
                <InstallPlanDetails v-if="removing.kind === 'update'" :plan="removing.plan" />
              </PlanPreview>
              <p v-else-if="removing.error" class="text-[12.5px] text-warn">{{ removing.error }}</p>
            </div>
          </div>
        </div>
      </section>

      <section v-if="view === 'catalog'" class="mt-4">
        <div class="flex items-center justify-between gap-3">
          <h2 class="text-[13px] font-semibold text-muted">{{ t('apps.catalogTitle') }}</h2>
          <button
            type="button"
            class="rounded-sm border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field disabled:opacity-60"
            :disabled="dockerMissing"
            @click="customOpen = { kind: 'compose', text: '' }"
          >
            {{ t('apps.custom.add') }}
          </button>
        </div>
        <div class="mt-2 flex flex-col gap-3">
          <div v-for="app in catalog" :key="app.id">
          <section class="flex items-center gap-3 rounded-sm border border-line bg-tile p-3">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <h3 class="truncate text-[13.5px] font-semibold text-text">{{ app.name }}</h3>
                <span class="rounded-full bg-tile-2 px-2 py-0.5 text-[11px] text-muted">{{ t('apps.category.' + app.category) }}</span>
              </div>
              <p class="mt-0.5 text-[12.5px] text-muted">{{ app.description }}</p>
              <p class="mt-0.5 text-[12px] text-faint">
                <span v-if="app.services?.length">{{ t('apps.withServices', { services: app.services.map(serviceLabel).join(', ') }) }} · </span>
                <span v-if="app.port">{{ t('apps.port', { port: app.port }) }} · </span>
                {{ t('apps.memory', { mb: app.memoryMB }) }}
              </p>
              <p v-if="app.note" class="mt-1 text-[12px] text-muted">{{ app.note }}</p>
            </div>
            <span v-if="!app.compatible" class="flex items-center gap-1.5 shrink-0 text-[12px] text-faint">
              <span class="h-1.5 w-1.5 rounded-full bg-faint" />
              {{ t('apps.notCompatible') }}
            </span>
            <span v-else-if="app.installed" class="shrink-0 text-[12px] text-muted">{{ t('apps.alreadyInstalled') }}</span>
            <button
              v-else
              type="button"
              class="shrink-0 rounded-sm border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field disabled:opacity-60"
              :disabled="!!installingId || dockerMissing"
              @click="installingId = app.id"
            >
              {{ t('apps.install') }}
            </button>
          </section>
            <div v-if="installingId === app.id" class="-mt-1 rounded-b-sm border border-t-0 border-line bg-tile p-3">
              <AppInstallForm :source="{ kind: 'catalog', app }" @done="onInstalled" @cancel="installingId = ''" />
            </div>
          </div>
        </div>
      </section>

      <details v-if="view === 'catalog'" class="mt-6 border-t border-line-soft pt-4">
        <summary class="cursor-pointer text-[13px] font-semibold text-muted">{{ t('apps.searchTitle') }}</summary>
        <input
          v-model="query"
          type="search"
          class="mt-3 w-full rounded-sm border border-line bg-field px-3 py-2 text-[13.5px] text-text placeholder:text-faint"
          :placeholder="t('apps.searchPlaceholder')"
          @keyup.enter="search"
        />
        <p class="mt-1.5 text-[12px] text-faint">{{ t('apps.notYetInstallable') }}</p>

        <p v-if="searching" class="mt-3 text-[13px] text-faint">{{ t('common.loading') }}</p>
        <p v-else-if="searchError" class="mt-3 text-[13px] text-faint">{{ searchError }}</p>
        <p v-else-if="searched && results.length === 0" class="mt-3 text-[13px] text-faint">{{ t('apps.searchEmpty') }}</p>

        <ul v-else-if="results.length > 0" class="mt-3 flex flex-col gap-2">
          <li v-for="r in results" :key="r.image" class="flex flex-wrap items-center gap-3 rounded-sm border border-line bg-tile p-3">
            <div class="min-w-[10rem] flex-1">
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
              class="shrink-0 rounded-sm border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field"
              @click="checkManifest(r.image)"
            >
              {{ t('apps.checkCompatibility') }}
            </button>
            <button
              type="button"
              class="shrink-0 rounded-sm border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field disabled:opacity-60"
              :disabled="dockerMissing"
              @click="customOpen = { kind: 'image', text: r.image }"
            >
              {{ t('apps.addFromSearch') }}
            </button>
          </li>
        </ul>
      </details>
    </template>

    <!-- Logs modal (App5, "two tiers" — this is the per-container one;
         the daemon's own health reuses DeviceHub's existing viewer via
         the link above). -->
    <div v-if="logsFor" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="logsFor = null">
      <div class="flex max-h-[80vh] w-full max-w-2xl flex-col rounded-sm border border-line bg-bg p-4">
        <div class="flex items-center justify-between">
          <h3 class="text-[14px] font-semibold text-text">{{ t('apps.logsTitle', { name: logsFor.name }) }}</h3>
          <button type="button" class="text-[12.5px] text-muted hover:text-text" @click="logsFor = null">{{ t('apps.logsClose') }}</button>
        </div>
        <p v-if="logsLoading" class="mt-3 text-[13px] text-faint">{{ t('common.loading') }}</p>
        <p v-else-if="logsError" class="mt-3 text-[12.5px] text-warn">{{ logsError }}</p>
        <p v-else-if="!logsText" class="mt-3 text-[13px] text-faint">{{ t('apps.logsEmpty') }}</p>
        <pre v-else class="mt-3 overflow-auto whitespace-pre-wrap rounded-sm border border-line-soft bg-tile p-3 text-[12px] text-muted">{{ logsText }}</pre>
      </div>
    </div>

    <!-- Console modal (App5, Terminal's exact safety posture): off by
         default, needs a fresh password re-entry, a standing warning. -->
    <div v-if="consoleFor" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="closeConsole">
      <div class="flex max-h-[80vh] w-full max-w-2xl flex-col rounded-sm border border-line bg-bg p-4">
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
            <input v-model="consolePassword" type="password" :placeholder="t('apps.console.password')" class="flex-1 rounded-sm border border-line bg-field px-3 py-2 text-[13.5px] text-text" />
            <button type="submit" class="shrink-0 rounded-sm border border-line px-3 py-2 text-[13px] text-text hover:bg-field">{{ t('apps.console.enable') }}</button>
          </form>
          <p v-if="consolePasswordError" class="mt-1.5 text-[12.5px] text-warn">{{ consolePasswordError }}</p>
        </template>

        <template v-else>
          <p class="mt-2 text-[12px] text-warn">{{ t('apps.console.warning') }}</p>
          <pre ref="consoleOutputEl" class="mt-2 h-64 overflow-auto whitespace-pre-wrap rounded-sm border border-line-soft bg-tile p-3 text-[12px] text-muted">{{ consoleOutput }}</pre>
          <p v-if="consoleConnStatus" class="mt-1 text-[12px] text-faint">{{ consoleConnStatus }}</p>
          <form class="mt-2 flex items-center gap-2" @submit.prevent="sendConsoleInput">
            <input v-model="consoleInput" type="text" :placeholder="t('apps.console.inputPlaceholder')" class="flex-1 rounded-sm border border-line bg-field px-3 py-2 text-[13.5px] text-text" />
            <button type="submit" class="shrink-0 rounded-sm border border-line px-3 py-2 text-[13px] text-text hover:bg-field">{{ t('apps.console.send') }}</button>
          </form>
          <button type="button" class="mt-2 self-start text-[12.5px] text-warn hover:underline" @click="disableConsoleNow">{{ t('apps.console.disable') }}</button>
        </template>
      </div>
    </div>

    <p v-if="toast" class="fixed inset-e-4 bottom-4 z-50 flex items-center gap-1.5 rounded-lg border border-line bg-tile px-3 py-2 text-[13px] text-text shadow-lg" role="status">
      <span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ toast }}
    </p>
    <CustomContainerDialog v-if="customOpen" :initial="customOpen" @close="customOpen = null" @done="load" />
    <DockerSettingsDialog v-if="settingsOpen" @close="settingsOpen = false" @changed="load" />
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import DropdownMenu from '@/components/DropdownMenu.vue'
import Icon from '@/components/Icon.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import AppInstallForm from '@/components/apps/AppInstallForm.vue'
import InstallPlanDetails from '@/components/apps/InstallPlanDetails.vue'
import CustomContainerDialog from '@/components/apps/CustomContainerDialog.vue'
import DockerSettingsDialog from '@/components/apps/DockerSettingsDialog.vue'
import TabIntro from '@/components/apps/TabIntro.vue'
import { useFriendlyError } from '@/composables/useFriendlyError'
import { usePlanError } from '@/composables/usePlanError'
import type { AppConsoleStatus, AppEditView, AppSearchResult, CatalogApp, InstallRequest, InstalledApp, ParsedApp, Plan } from '@/api/types'

const { t, te, locale } = useI18n()
const { friendly } = useFriendlyError()
const { planErrorMessage } = usePlanError()

const settingsOpen = ref(false)
const catalog = ref<CatalogApp[]>([])
const installed = ref<InstalledApp[]>([])
const loading = ref(true)
const loadError = ref('')

// Docker itself may be missing on a box (not installed, or the helper
// not running) — the catalog and search still work then, only the
// installed list can't be read, so the two load independently.
const dockerMissing = ref(false)
const installedError = ref('')

async function load() {
  const [c, i] = await Promise.allSettled([api.apps.catalog(locale.value), api.apps.installed()])
  if (c.status === 'fulfilled') {
    catalog.value = c.value.apps
    loadError.value = ''
  } else {
    loadError.value = t('apps.loadError')
  }
  if (i.status === 'fulfilled') {
    installed.value = i.value.apps
    dockerMissing.value = false
    installedError.value = ''
  } else {
    installed.value = []
    dockerMissing.value = i.reason instanceof ApiError && i.reason.code === 'docker_unavailable'
    installedError.value = dockerMissing.value ? '' : t('apps.installedLoadError')
  }
  loading.value = false
}

// A helper container's role ("db"), in the reader's language when known.
function serviceLabel(name: string): string {
  const key = 'apps.service.' + name
  return te(key) ? t(key) : name
}

function openUrl(port: number): string {
  return `${location.protocol}//${location.hostname}:${port}/`
}

// --- install: the form (AppInstallForm), then preview and confirm ----------

const installingId = ref('')
// "Add container" / "Add as container" (an admin's own, App12).
const customOpen = ref<{ kind: 'compose' | 'image' | 'unraid'; text: string } | null>(null)

async function onInstalled() {
  await load()
  window.setTimeout(() => {
    installingId.value = ''
  }, 2500)
}

// The catalog's texts come in the reader's language.
watch(locale, load)

// --- remove: preview, confirm -----------------------------------------------

type RemovePending = { id: string; kind?: 'update'; plan?: Plan; applying?: boolean; done?: boolean; error?: string }
const removing = ref<RemovePending | null>(null)

async function previewRemove(a: InstalledApp) {
  removing.value = { id: a.id }
  editing.value = null
  try {
    const res = await api.apps.removePlan(a.id)
    if (removing.value?.id === a.id) removing.value = { id: a.id, plan: res.plan }
  } catch (err) {
    if (removing.value?.id === a.id) removing.value = { id: a.id, error: planErrorMessage(err) }
  }
}

// Update = the edit plan with the choices as they are: the containers are made
// again from the newest image of the same tag; the data stays.
async function previewUpdate(a: InstalledApp) {
  removing.value = { id: a.id, kind: 'update' }
  editing.value = null
  try {
    const view = await api.apps.editForm(a.id, locale.value)
    const req: InstallRequest = {}
    if (view.kind === 'catalog') req.appId = a.appId
    else req.custom = { compose: (view.app as ParsedApp).compose, name: view.app.name }
    if (Object.keys(view.values).length) req.values = view.values
    if (view.memoryLimitMB) req.memoryLimitMB = view.memoryLimitMB
    const { env, ports, paths } = view.extra
    if (env.length + ports.length + paths.length > 0) req.extra = { env: env.map((e) => ({ key: e.key, value: e.value })), ports, paths }
    // Already confirmed when it was installed; the preview lists it again.
    if ((view.app as ParsedApp).elevated?.length) req.confirmElevated = true
    const res = await api.apps.editPlan(a.id, req)
    if (removing.value?.id === a.id) removing.value = { id: a.id, kind: 'update', plan: res.plan }
  } catch (err) {
    if (removing.value?.id === a.id) removing.value = { id: a.id, kind: 'update', error: planErrorMessage(err) }
  }
}

async function confirmRemove() {
  const p = removing.value
  if (!p?.plan) return
  removing.value = { ...p, applying: true, error: undefined }
  try {
    await api.plans.apply(p.plan.id)
    removing.value = null
    showToast(p.kind === 'update' ? t('apps.update.done') : t('apps.removed'))
    await load()
  } catch (err) {
    removing.value = { id: p.id, kind: p.kind, error: planErrorMessage(err) }
  }
}

// --- edit: the install form again, pre-filled ----------------------------------

const editing = ref<{ id: string; view: AppEditView; source: { kind: 'catalog'; app: CatalogApp } | { kind: 'custom'; app: ParsedApp } } | null>(null)

async function openEdit(a: InstalledApp) {
  removing.value = null
  try {
    const view = await api.apps.editForm(a.id, locale.value)
    const source = view.kind === 'catalog' ? { kind: 'catalog' as const, app: view.app as CatalogApp } : { kind: 'custom' as const, app: view.app as ParsedApp }
    editing.value = { id: a.id, view, source }
  } catch (err) {
    showToast(err instanceof ApiError && err.code === 'not_editable' ? t('apps.edit.notEditable') : friendly(err).text || t('apps.edit.loadError'))
  }
}

async function onEdited() {
  await load()
  window.setTimeout(() => {
    editing.value = null
  }, 2500)
}

// --- the two views, a toast, the row's picture --------------------------------------

type View = 'mine' | 'catalog'
const VIEW_KEY = 'dreamhub.apps.dockerView'
const view = ref<View>('mine')
let viewChosen = false
try {
  const saved = localStorage.getItem(VIEW_KEY)
  if (saved === 'mine' || saved === 'catalog') {
    view.value = saved
    viewChosen = true
  }
} catch {
  // storage may be blocked: the default view is used
}
function setView(v: View) {
  view.value = v
  viewChosen = true
  try {
    localStorage.setItem(VIEW_KEY, v)
  } catch {
    // not remembered
  }
}
// Nothing installed yet and nothing chosen before: open on the catalog.
watch(loading, (l) => {
  if (!l && !viewChosen && installed.value.length === 0) view.value = 'catalog'
})

const toast = ref('')
let toastTimer = 0
function showToast(text: string) {
  toast.value = text
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (toast.value = ''), 3500)
}

const CATEGORY_ICONS: Record<string, string> = { network: 'globe', home: 'control', downloads: 'library', cloud: 'stream' }
const KNOWN_ICONS = ['globe', 'control', 'library', 'stream', 'folder', 'lock', 'radio', 'recordings', 'timers']
function rowIcon(a: InstalledApp): string {
  if (a.icon && KNOWN_ICONS.includes(a.icon)) return a.icon
  return CATEGORY_ICONS[a.category ?? ''] ?? 'extension'
}
function stateText(state: string): string {
  const key = 'apps.state.' + state
  return te(key) ? t(key) : state
}

async function toggleAutostart(a: InstalledApp) {
  const next = !a.autostart
  a.autostart = next
  try {
    await api.apps.setAutostart(a.id, next)
    showToast(t(next ? 'apps.autostart.on' : 'apps.autostart.off', { name: a.name }))
  } catch (err) {
    a.autostart = !next
    showToast(friendly(err).text || t('apps.autostart.error'))
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

onUnmounted(() => {
  disconnectConsole()
  window.clearTimeout(toastTimer)
})
</script>
