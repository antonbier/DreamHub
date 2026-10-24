<!--
  DeviceHub → Box settings (docs/BOXSETTINGS_PLAN.md, S1): the receiver's
  own setup menus on a phone, read only for now. Sections on the left (a
  select below md), a search across every section, the box's own labels
  with the current value, and a "?" explaining what a setting does where
  that is not obvious. A box that can't show its menus yet says so.
-->
<template>
  <div class="mt-4">
    <p v-if="loading" class="text-[14px] text-muted">{{ t('common.loading') }}</p>
    <p v-else-if="loadFailed" class="text-[14px] text-muted"><ErrorText /></p>
    <p v-else-if="!overview?.available" class="text-[14px] text-muted">
      {{ overview?.reason === 'plugin' ? t('boxsettings.needsPlugin') : t('boxsettings.unavailable') }}
    </p>
    <template v-else>
      <p class="text-[13px] text-muted">{{ t('boxsettings.intro') }}</p>
      <p class="mt-1 text-[12.5px] text-faint">
        <template v-if="overview.boxLevelName">
          {{ t(groups.length > 1 ? 'boxsettings.levelAll' : 'boxsettings.level', { level: overview.boxLevelName }) }}
        </template>
        {{ ' ' }}
        <button type="button" class="text-accent hover:underline disabled:opacity-60" :disabled="reloading" @click="reload">
          {{ reloading ? t('common.loading') : t('boxsettings.reload') }}
        </button>
      </p>

      <div class="mt-3 flex items-center gap-2">
        <label class="relative min-w-0 flex-1">
          <span class="sr-only">{{ t('boxsettings.search') }}</span>
          <Icon name="search" :size="16" class="pointer-events-none absolute inset-y-0 start-2.5 my-auto text-faint" />
          <input
            v-model="query"
            type="search"
            :placeholder="t('boxsettings.searchPlaceholder')"
            class="w-full rounded-sm border border-line bg-field py-2 pe-2.5 ps-8 text-[13.5px] text-text"
          />
        </label>
      </div>

      <!-- Search results -->
      <section v-if="searching" class="mt-4 rounded-sm border border-line bg-tile p-4">
        <p v-if="searchBusy && hits.length === 0" class="text-[13px] text-muted">{{ t('common.loading') }}</p>
        <p v-else-if="hits.length === 0" class="text-[13px] text-muted">{{ t('boxsettings.searchEmpty', { query: query.trim() }) }}</p>
        <template v-else>
          <p class="text-[12.5px] text-faint">{{ t('boxsettings.searchCount', { count: hits.length }, hits.length) }}</p>
          <ul class="mt-1">
            <template v-for="(h, i) in hits" :key="h.sectionKey + h.item.path + i">
              <p v-if="i === 0 || hits[i - 1].sectionKey !== h.sectionKey" class="mt-3 text-[12.5px] font-semibold text-muted first:mt-1">
                <button type="button" class="hover:text-text hover:underline" @click="openSection(h.sectionKey)">{{ h.sectionTitle }}</button>
              </p>
              <BoxSettingRow :item="h.item" />
            </template>
          </ul>
        </template>
      </section>

      <!-- Sections -->
      <div v-else class="mt-4 grid grid-cols-[14rem_1fr] gap-5 below-md:grid-cols-1 below-md:gap-3">
        <nav :aria-label="t('boxsettings.sections')" class="below-md:hidden">
          <template v-for="g in groups" :key="g.id">
            <p v-if="groups.length > 1" class="px-2.5 pb-1 pt-3 text-[12.5px] font-semibold text-muted first:pt-0">{{ g.label }}</p>
            <ul class="flex flex-col gap-0.5" :class="g.id === 'tree' ? 'max-h-[28rem] overflow-y-auto' : ''">
              <li v-for="s in g.sections" :key="s.key">
                <button
                  type="button"
                  class="flex w-full items-baseline justify-between gap-2 rounded-sm px-2.5 py-1.5 text-start text-[13.5px]"
                  :class="s.key === current ? 'bg-tile-2 font-semibold text-text' : 'text-muted hover:bg-tile hover:text-text'"
                  :aria-current="s.key === current ? 'true' : undefined"
                  @click="openSection(s.key)"
                >
                  <span class="min-w-0 truncate">{{ s.title }}</span>
                  <span class="shrink-0 text-[12px] tabular-nums text-faint">{{ s.count }}</span>
                </button>
              </li>
            </ul>
          </template>
        </nav>
        <label class="hidden below-md:block">
          <span class="sr-only">{{ t('boxsettings.sections') }}</span>
          <select
            :value="current"
            class="w-full rounded-sm border border-line bg-field px-2.5 py-2 text-[13.5px] text-text"
            @change="openSection(($event.target as HTMLSelectElement).value)"
          >
            <optgroup v-for="g in groups" :key="g.id" :label="groups.length > 1 ? g.label : ''">
              <option v-for="s in g.sections" :key="s.key" :value="s.key">{{ s.title }} ({{ s.count }})</option>
            </optgroup>
          </select>
        </label>

        <section class="min-w-0 rounded-sm border border-line bg-tile p-4">
          <p v-if="sectionBusy" class="text-[13px] text-muted">{{ t('common.loading') }}</p>
          <p v-else-if="sectionFailed" class="text-[13px] text-muted"><ErrorText /></p>
          <template v-else-if="section">
            <h2 class="text-[15px] font-semibold text-text">{{ section.section.title }}</h2>
            <p v-if="section.section.hiddenByLevel > 0" class="mt-1 text-[12.5px] text-faint">
              {{ t('boxsettings.hidden', { count: section.section.hiddenByLevel }, section.section.hiddenByLevel) }}
            </p>
            <p v-if="section.items.length === 0" class="mt-2 text-[13px] text-muted">{{ t('boxsettings.sectionEmpty') }}</p>
            <ul v-else class="mt-2">
              <template v-for="(it, i) in section.items" :key="it.path + i">
                <li v-if="it.kind === 'heading'" class="pb-1 pt-4 text-[13px] font-semibold text-muted first:pt-1">{{ it.label }}</li>
                <BoxSettingRow v-else :item="it" />
              </template>
            </ul>
          </template>
        </section>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import ErrorText from '@/components/ErrorText.vue'
import Icon from '@/components/Icon.vue'
import BoxSettingRow from '@/components/boxsettings/BoxSettingRow.vue'
import type { BoxSetting, BoxSettingHit, BoxSettingSection, BoxSettingsOverview } from '@/api/types'

const { t } = useI18n()

const loading = ref(true)
const loadFailed = ref(false)
const overview = ref<BoxSettingsOverview | null>(null)

const current = ref('')
const section = ref<{ section: BoxSettingSection; items: BoxSetting[] } | null>(null)
const sectionBusy = ref(false)
const sectionFailed = ref(false)

// The sections in two groups: the box's own setup menus, and — on a Dreambox —
// the rest of its config tree by key name.
const groups = computed(() => {
  const all = overview.value?.sections ?? []
  const menus = all.filter((s) => s.group !== 'tree')
  const tree = all.filter((s) => s.group === 'tree')
  const out = [{ id: 'menus', label: t('boxsettings.menusGroup'), sections: menus }]
  if (tree.length > 0) out.push({ id: 'tree', label: t('boxsettings.treeGroup'), sections: tree })
  return out.filter((g) => g.sections.length > 0)
})

const query = ref('')
const hits = ref<BoxSettingHit[]>([])
const searchBusy = ref(false)
const searching = computed(() => query.value.trim().length >= 2)

async function openSection(key: string) {
  current.value = key
  query.value = ''
  sectionBusy.value = true
  sectionFailed.value = false
  try {
    const res = await api.boxSettings.section(key)
    if (current.value === key) section.value = res
  } catch {
    if (current.value === key) sectionFailed.value = true
  } finally {
    if (current.value === key) sectionBusy.value = false
  }
}

// Search as the person types, but not on every key and never out of order.
let timer: ReturnType<typeof setTimeout> | undefined
let seq = 0
watch(query, (q) => {
  clearTimeout(timer)
  const text = q.trim()
  if (text.length < 2) {
    hits.value = []
    searchBusy.value = false
    return
  }
  searchBusy.value = true
  const mine = ++seq
  timer = setTimeout(async () => {
    try {
      const res = await api.boxSettings.search(text)
      if (mine === seq) hits.value = res.hits
    } catch {
      if (mine === seq) hits.value = []
    } finally {
      if (mine === seq) searchBusy.value = false
    }
  }, 250)
})

// The box's menus are read once and kept on the server for a while; this
// reads them again (slow on a VU+: a few seconds).
// The first menu that has settings: some menus have none (the One's remote
// control menu), and opening on an empty page looks broken.
const firstWithSettings = <T extends { count: number }>(sections: T[]): T | undefined => sections.find((s) => s.count > 0) ?? sections[0]

const reloading = ref(false)
async function reload() {
  reloading.value = true
  try {
    overview.value = await api.boxSettings.overview(true)
    const keep = overview.value.sections.find((s) => s.key === current.value) ?? firstWithSettings(overview.value.sections)
    if (keep) await openSection(keep.key)
  } catch {
    loadFailed.value = true
  } finally {
    reloading.value = false
  }
}

onMounted(async () => {
  try {
    overview.value = await api.boxSettings.overview()
    const first = firstWithSettings(overview.value.sections)
    if (overview.value.available && first) await openSection(first.key)
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
})
</script>
