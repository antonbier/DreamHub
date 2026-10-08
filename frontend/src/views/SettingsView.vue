<!--
  The central settings menu (CONCEPT.md, "Settings"): rendered straight from
  GET /api/v1/settings — every section and field the backend registers
  (general, auth, network, each connector, hubs) shows up here with no
  frontend-side list to keep in sync. See src/composables/useSettingsForm.ts.

  Sections are grouped into tabs (DESIGN.md, "Settings"): each section names
  its tab in the schema (backend/internal/settings, Section.Tab); the tab
  order and the panels that aren't schema fields (API tokens, push test,
  install, backup, Plex sign-in) are placed here. Every tab has its own address
  (/settings/services), and /settings/services#tmdb scrolls to a section.
  The old /settings#tmdb links still land on the right tab.

  Drafts survive switching tabs (one form for the whole page); a tab with
  unsaved changes carries a dot, and leaving the page with any asks first.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="mb-4 flex items-center">
      <h1 class="text-[26px] font-bold">{{ t('settings.page.title') }}</h1>
    </div>

    <p v-if="form.loading.value" class="text-[14px] text-muted">{{ t('common.loading') }}</p>
    <div v-else-if="form.loadFailed.value" class="rounded border border-line bg-tile p-5">
      <p class="mb-3 text-[14px] text-muted">{{ t('settings.page.loadFailed') }}</p>
      <button type="button" class="text-[13.5px] text-accent hover:underline" @click="form.load()">
        {{ t('common.retry') }}
      </button>
    </div>
    <template v-else>
      <!-- Scrollable sideways on a phone rather than wrapping or folding
           into a menu. -->
      <nav class="-mx-6 mb-5 flex gap-1 overflow-x-auto border-b border-line-soft px-6 below-md:-mx-4 below-md:px-4" :aria-label="t('settings.page.tabs')">
        <RouterLink
          v-for="tab in tabs"
          :key="tab.id"
          :to="`/settings/${tab.id}`"
          class="-mb-px flex shrink-0 items-center gap-1.5 whitespace-nowrap border-b-2 px-3 py-2 text-[14px]"
          :class="activeTab === tab.id ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text'"
          :aria-current="activeTab === tab.id ? 'page' : undefined"
        >
          {{ t(`settings.tab.${tab.id}`) }}
          <span v-if="tabDirty(tab.id)" class="h-1.5 w-1.5 rounded-full bg-accent" :title="t('settings.page.unsavedChanges')" />
          <span v-if="tabDirty(tab.id)" class="sr-only">{{ t('settings.page.unsavedChanges') }}</span>
        </RouterLink>
      </nav>

      <div class="flex flex-col gap-4">
        <p v-if="intro" class="text-[13.5px] text-muted">{{ intro }}</p>
        <template v-for="section in tabSections" :key="section.id">
          <SettingsSectionCard
            :id="section.id"
            :section="section"
            :form="form"
            :status="connectorState(section.id)"
            class="scroll-mt-4"
          />
          <!-- Not schema fields — placed by hand next to their section. -->
          <ApiTokensPanel v-if="section.id === 'auth' && can('system')" />
          <PushPanel v-if="section.id === 'push'" />
          <InstallPanel v-if="section.id === 'general'" />
          <PlexPanel v-if="section.id === 'plex'" @changed="form.load()" />
        </template>

        <template v-if="activeTab === 'profiles'">
          <MyPinPanel />
          <ProfilesPanel v-if="can('profiles')" />
        </template>

        <template v-if="activeTab === 'system' && can('system')">
          <BackupPanel @restored="form.load()" />
          <section class="rounded border border-line bg-tile p-5">
            <h2 class="text-[15.5px] font-semibold">{{ t('settings.system.diagnostics.title') }}</h2>
            <p class="mt-1 text-[13px] text-muted">{{ t('settings.system.diagnostics.body') }}</p>
            <RouterLink to="/system?tab=diagnostics" class="mt-3 inline-block text-[13.5px] text-accent hover:underline">
              {{ t('settings.system.diagnostics.open') }}
            </RouterLink>
          </section>
          <section class="rounded border border-line bg-tile p-5">
            <h2 class="text-[15.5px] font-semibold">{{ t('settings.system.wizard.title') }}</h2>
            <p class="mt-1 text-[13px] text-muted">{{ t('settings.system.wizard.body') }}</p>
            <RouterLink to="/onboarding" class="mt-3 inline-block text-[13.5px] text-accent hover:underline">
              {{ t('settings.page.runWizard') }}
            </RouterLink>
          </section>
        </template>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import SettingsSectionCard from '@/components/settings/SettingsSectionCard.vue'
import ApiTokensPanel from '@/components/settings/ApiTokensPanel.vue'
import PushPanel from '@/components/settings/PushPanel.vue'
import InstallPanel from '@/components/settings/InstallPanel.vue'
import BackupPanel from '@/components/settings/BackupPanel.vue'
import PlexPanel from '@/components/settings/PlexPanel.vue'
import MyPinPanel from '@/components/profiles/MyPinPanel.vue'
import ProfilesPanel from '@/components/profiles/ProfilesPanel.vue'
import { useAuth } from '@/composables/useAuth'
import { api } from '@/api/client'
import { useSettingsForm } from '@/composables/useSettingsForm'
import { SETTINGS_TABS, type SettingsTab } from '@/lib/settingsTabs'
import type { ConnectorState } from '@/api/types'

const { t, te } = useI18n()
const form = useSettingsForm()
const route = useRoute()
const router = useRouter()

const { can } = useAuth()
// A viewer sees only the tabs with something of theirs: personal settings
// and their PIN (the server sends them only their own fields).
const tabs = computed(() =>
  SETTINGS_TABS.filter((id) => id === 'profiles' || (id === 'system' ? can('system') : form.sections.value.some((s) => s.tab === id))).map(
    (id) => ({ id }),
  ),
)
const activeTab = computed<SettingsTab>(() => {
  const p = route.params.tab
  return typeof p === 'string' && (SETTINGS_TABS as readonly string[]).includes(p) ? (p as SettingsTab) : 'general'
})
// A section whose fields are all edited elsewhere (ChannelHub's logo set)
// has nothing to show here.
const tabSections = computed(() => form.sections.value.filter((s) => s.tab === activeTab.value && s.fields.some((f) => !f.hidden)))
const intro = computed(() => (te(`settings.tabIntro.${activeTab.value}`) ? t(`settings.tabIntro.${activeTab.value}`) : ''))

function tabDirty(tab: SettingsTab): boolean {
  return form.sections.value.some((s) => s.tab === tab && form.sectionDirty(s))
}
const anyDirty = computed(() => form.sections.value.some((s) => form.sectionDirty(s)))

// A connector's state next to its section on the services tab. Section ids
// match connector ids except for the AI provider.
const connectors = ref<ConnectorState[]>([])
const connectorIds: Record<string, string> = { ai: 'openai' }
function connectorState(sectionId: string): ConnectorState['state'] | undefined {
  if (activeTab.value !== 'services') return undefined
  const id = connectorIds[sectionId] ?? sectionId
  return connectors.value.find((c) => c.id === id)?.state
}
async function loadConnectors() {
  if (!can('settings.global')) return
  try {
    connectors.value = (await api.connectors()).connectors
  } catch {
    connectors.value = []
  }
}

// Saving a section can change a connector's state (a key entered).
watch(
  () => form.savingSection.value,
  (now, before) => {
    if (before && !now) void loadConnectors()
  },
)

async function scrollToHash() {
  const id = route.hash.slice(1)
  if (!id) return
  await nextTick()
  document.getElementById(id)?.scrollIntoView({ block: 'start' })
}

watch(() => route.fullPath, scrollToHash)

// The old one-page links (/settings#tmdb): the tab that holds the section.
function redirectOldHash() {
  if (route.params.tab || !route.hash) return
  const section = form.sections.value.find((s) => s.id === route.hash.slice(1))
  if (section) void router.replace({ path: `/settings/${section.tab}`, hash: route.hash })
}

onBeforeRouteLeave(() => {
  if (anyDirty.value && !window.confirm(t('settings.page.leaveUnsaved'))) return false
  return true
})
function onBeforeUnload(e: BeforeUnloadEvent) {
  if (anyDirty.value) e.preventDefault()
}

onMounted(async () => {
  window.addEventListener('beforeunload', onBeforeUnload)
  await Promise.all([form.load(), loadConnectors()])
  redirectOldHash()
  await scrollToHash()
})
onBeforeUnmount(() => window.removeEventListener('beforeunload', onBeforeUnload))
</script>
