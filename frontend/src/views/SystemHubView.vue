<!--
  SystemHub's real screen (docs/archive/V1_PLAN.md, WP5/WP6): device info
  (GET /api/v1/deviceinfo), the change log
  (GET /api/v1/changes) with a client-side filter, and — as a second tab —
  diagnostics (GET /api/v1/hubs), which used to be its own core route.
  CONCEPT.md already describes diagnostics as "a diagnostics page in
  SystemHub"; this folds the two together instead of keeping near-duplicate
  hub-status UI in two places. Same status-banner-plus-content pattern as
  SeriesHubView.vue.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="system" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ hubTitle('system', 'hub.system') }}</h1>
    </div>

    <div v-if="item && item.status && item.status !== 'available'" class="mt-4 rounded border border-line bg-tile p-4">
      <div class="mb-1.5 flex items-center gap-2 text-[13.5px] font-semibold">
        <span class="h-1.5 w-1.5 rounded-full" :class="item.status === 'degraded' ? 'bg-warn' : 'bg-faint'" />
        {{ t(`status.${item.status}`) }}
      </div>
      <ul class="flex flex-col gap-1 text-[13px] text-muted">
        <li v-for="(p, i) in item.problems ?? []" :key="i">
          {{ describe(p) }}
          <span v-if="describeFix(p)" class="text-faint">{{ ' — ' + describeFix(p) }}</span>
        </li>
      </ul>
    </div>

    <div class="mt-4 flex gap-1 border-b border-line-soft" role="tablist">
      <button
        type="button"
        role="tab"
        :aria-selected="tab === 'overview'"
        class="border-b-2 px-3 py-2 text-[13.5px] font-semibold"
        :class="tab === 'overview' ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
        @click="tab = 'overview'"
      >
        {{ t('system.tabs.overview') }}
      </button>
      <button
        type="button"
        role="tab"
        :aria-selected="tab === 'diagnostics'"
        class="border-b-2 px-3 py-2 text-[13.5px] font-semibold"
        :class="tab === 'diagnostics' ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
        @click="tab = 'diagnostics'"
      >
        {{ t('diagnostics.title') }}
      </button>
    </div>

    <template v-if="tab === 'overview'">
      <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted"><ErrorText /></p>
      <template v-else>
        <!-- Device info -->
        <section class="mt-5 rounded border border-line bg-tile p-4">
          <h2 class="text-[14px] font-semibold text-text">{{ t('system.device.title') }}</h2>
          <!-- Label and value side by side; one under the other on phones. -->
          <dl class="mt-2 grid grid-cols-[max-content_1fr] gap-x-6 gap-y-1.5 text-[13px] below-sm:grid-cols-1 below-sm:gap-y-0">
            <template v-for="row in deviceRows" :key="row.label">
              <dt class="text-muted below-sm:mt-1.5">{{ row.label }}</dt>
              <dd class="min-w-0 break-words text-text">{{ row.value }}</dd>
            </template>
          </dl>
        </section>

        <!-- Backup & restore, and About, moved to Settings → System: both
             are app-level, administrative information, not live device
             status (the same reasoning that moved backup/restore there
             on 2026-10-05). -->
        <p class="mt-4 text-[13px] text-muted">
          {{ t('system.backup.movedHint') }}
          <RouterLink to="/settings/system" class="text-accent hover:underline">{{ t('system.backup.openSettings') }}</RouterLink>
        </p>

        <!-- Change log -->
        <section class="mt-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <h2 class="text-[15px] font-semibold text-text">{{ t('system.changes.title') }}</h2>
            <div class="flex flex-wrap items-center gap-2">
              <label v-if="profiles.length > 1" class="flex items-center gap-1.5 text-[13px]">
                <span class="text-muted">{{ t('system.changes.by') }}</span>
                <select v-model="byUser" class="rounded border border-line bg-field px-2 py-1.5 text-text" @change="loadChanges">
                  <option :value="-1">{{ t('system.changes.everyone') }}</option>
                  <option v-for="p in profiles" :key="p.id" :value="p.id">{{ p.name }}</option>
                  <option :value="0">{{ t('system.changes.system') }}</option>
                </select>
              </label>
              <input
                v-model="filter"
                type="search"
                :placeholder="t('system.changes.filterPlaceholder')"
                class="w-64 max-w-full rounded border border-line bg-field px-2.5 py-1.5 text-[13px] text-text"
              />
            </div>
          </div>
          <p v-if="filteredChanges.length === 0" class="mt-2 text-[13px] text-faint">{{ t('system.changes.empty') }}</p>
          <ul v-else class="mt-2 flex flex-col">
            <li v-for="c in filteredChanges" :key="c.id" class="flex items-start gap-3 border-b border-line-soft py-2 last:border-none">
              <span class="w-36 shrink-0 whitespace-nowrap text-[12px] font-mono text-faint">{{ formatDate(c.at) }}</span>
              <span class="min-w-0 flex-1">
                <span class="block truncate text-[13.5px] text-text">{{ c.summary }}</span>
                <span class="block truncate text-[12px] text-faint">
                  <template v-if="profiles.length > 1">{{ c.userName || (c.userId ? t('system.changes.deletedProfile') : t('system.changes.system')) }} · </template>{{ c.action }}
                </span>
              </span>
            </li>
          </ul>
          <button
            v-if="changes.length >= changesLimit"
            type="button"
            class="mt-3 rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field"
            @click="loadMoreChanges"
          >
            {{ t('system.changes.loadMore') }}
          </button>
        </section>
      </template>
    </template>

    <template v-else>
      <SelfTestPanel />
      <p v-if="diagLoading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="diagLoadFailed" class="mt-5 text-[14px] text-muted"><ErrorText /></p>
      <template v-else>
        <section v-for="group in diagGroups" :key="group.key" class="mt-5 first:mt-5">
          <h2 class="text-[15px] font-semibold text-text">{{ t(`diagnostics.group.${group.key}`) }} ({{ group.hubs.length }})</h2>
          <ul v-if="group.hubs.length > 0" class="mt-2 flex flex-col">
            <li v-for="hub in group.hubs" :key="hub.id" class="flex items-start gap-3 border-b border-line-soft py-2.5 last:border-none">
              <Icon :name="hub.icon" :size="18" class="mt-0.5 shrink-0 text-muted" />
              <span class="min-w-0 flex-1">
                <span class="flex items-center gap-2">
                  <span class="truncate text-[14px] text-text">{{ hubLabel(hub) }}</span>
                  <span v-if="!hub.enabled" class="shrink-0 text-[12px] text-faint">{{ t('diagnostics.disabled') }}</span>
                </span>
                <ul v-if="hub.problems.length > 0" class="mt-0.5 flex flex-col gap-0.5 text-[12.5px] text-muted">
                  <li v-for="(p, i) in hub.problems" :key="i">
                    {{ describe(p) }}
                    <span v-if="describeFix(p)" class="text-faint">{{ ' — ' + describeFix(p) }}</span>
                  </li>
                </ul>
              </span>
            </li>
          </ul>
          <p v-else class="mt-2 text-[13px] text-faint">{{ t('diagnostics.groupEmpty') }}</p>
        </section>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { useHubLabel } from '@/composables/useHubLabel'
import ErrorText from '@/components/ErrorText.vue'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import SelfTestPanel from '@/components/system/SelfTestPanel.vue'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { ChangeEntry, DeviceInfo, HubStatus, Profile } from '@/api/types'

const { t, locale } = useI18n()
const { hubLabel, hubTitle } = useHubLabel()
const route = useRoute()
const router = useRouter()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()
const { timeZone } = useDisplayTimeZone()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/system'))

type Tab = 'overview' | 'diagnostics'
const tab = ref<Tab>(route.query.tab === 'diagnostics' ? 'diagnostics' : 'overview')
watch(tab, (t) => router.replace({ query: { ...route.query, tab: t === 'overview' ? undefined : t } }))

const device = ref<DeviceInfo | null>(null)
const deviceRows = computed(() => {
  const d = device.value
  if (!d) return []
  const tuners = d.frontends.map((f) => f.model.trim()).filter(Boolean)
  return [
    { label: t('system.device.model'), value: `Dreambox ${d.deviceName}` },
    { label: t('system.device.imageLabel'), value: d.imageVersion },
    { label: 'Enigma2', value: d.enigmaVersion },
    { label: 'WebControl', value: d.webifVersion },
    { label: t('system.device.tuners', { count: d.frontends.length }, d.frontends.length), value: tuners.join(', ') || '—' },
  ]
})
const changes = ref<ChangeEntry[]>([])
const changesLimit = ref(50)
// A search result (?changes=) opens the change log already filtered.
const filter = ref(typeof route.query.changes === 'string' ? route.query.changes : '')
watch(
  () => route.query.changes,
  (q) => {
    if (typeof q === 'string') {
      filter.value = q
      tab.value = 'overview'
    }
  },
)
const loading = ref(true)
const loadFailed = ref(false)

const filteredChanges = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q) return changes.value
  return changes.value.filter((c) => c.summary.toLowerCase().includes(q) || c.action.toLowerCase().includes(q))
})

function formatDate(iso: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'short', timeZone: timeZone.value }).format(new Date(iso))
}

// Whose changes: everyone's (-1), a profile's, or the system's own (0) —
// the profile choice only shows once there's more than one profile.
const profiles = ref<Profile[]>([])
const byUser = ref(-1)
void api.profiles
  .list()
  .then((r) => (profiles.value = r.profiles))
  .catch(() => {})

async function loadChanges() {
  const res = await api.changes(changesLimit.value, byUser.value)
  changes.value = res.changes
}

async function loadMoreChanges() {
  changesLimit.value += 50
  await loadChanges()
}

onMounted(async () => {
  try {
    const [deviceRes] = await Promise.all([api.deviceInfo(), loadChanges()])
    device.value = deviceRes
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
})

// Diagnostics tab: GET /api/v1/hubs, the same data and grouping the old
// standalone /diagnostics route used. Loaded once, on first visit to the
// tab, not eagerly — most visits to SystemHub are for the overview.
const diagHubs = ref<HubStatus[]>([])
const diagLoading = ref(true)
const diagLoadFailed = ref(false)
const diagLoaded = ref(false)

const diagGroups = computed(() => [
  { key: 'unavailable', hubs: diagHubs.value.filter((h) => h.enabled && h.status === 'unavailable') },
  { key: 'degraded', hubs: diagHubs.value.filter((h) => h.enabled && h.status === 'degraded') },
  { key: 'disabled', hubs: diagHubs.value.filter((h) => !h.enabled) },
  { key: 'available', hubs: diagHubs.value.filter((h) => h.enabled && h.status === 'available') },
])

async function loadDiagnostics() {
  if (diagLoaded.value) return
  diagLoaded.value = true
  try {
    const res = await api.hubs()
    diagHubs.value = res.hubs
  } catch {
    diagLoadFailed.value = true
  } finally {
    diagLoading.value = false
  }
}

watch(tab, (t) => { if (t === 'diagnostics') loadDiagnostics() }, { immediate: true })
</script>
