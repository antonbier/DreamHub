<!--
  AppsHub's "Apps" tab (docs/APPSHUB_PLAN.md, App3 — "Curated list +
  search"): the hand-maintained catalog (backend/internal/appcatalog,
  never AI-generated) plus free search beyond it, each image's
  compatibility with this box checked server-side
  (dockerhub.Supports/BoxArch) rather than guessed here. Installing,
  running or removing an app is App4, not built yet — this tab is
  read-only, a preview of what AppsHub will eventually be able to run.
-->
<template>
  <div>
    <p v-if="loading" class="mt-4 text-[13px] text-faint">{{ t('common.loading') }}</p>
    <p v-else-if="loadError" class="mt-4 text-[13px] text-faint">{{ loadError }}</p>

    <template v-else>
      <p class="mt-3 text-[12.5px] text-faint">{{ t('apps.notYetInstallable') }}</p>

      <div class="mt-4 flex flex-col gap-3">
        <section v-for="app in catalog" :key="app.id" class="flex items-center gap-3 rounded border border-line bg-tile p-3">
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <h3 class="truncate text-[13.5px] font-semibold text-text">{{ app.name }}</h3>
              <span class="rounded-full bg-tile-2 px-2 py-0.5 text-[11px] text-muted">{{ t('apps.category.' + app.category) }}</span>
            </div>
            <p class="mt-0.5 truncate text-[12.5px] text-muted">{{ app.description }}</p>
          </div>
          <span class="flex items-center gap-1.5 shrink-0 text-[12px]" :class="app.compatible ? 'text-ok' : 'text-faint'">
            <span class="h-1.5 w-1.5 rounded-full" :class="app.compatible ? 'bg-ok' : 'bg-faint'" />
            {{ app.compatible ? t('apps.compatible') : t('apps.notCompatible') }}
          </span>
        </section>
      </div>

      <div class="mt-6 border-t border-line-soft pt-4">
        <input
          v-model="query"
          type="search"
          class="w-full rounded border border-line bg-field px-3 py-2 text-[13.5px] text-text placeholder:text-faint"
          :placeholder="t('apps.searchPlaceholder')"
          @keyup.enter="search"
        />

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
import { useFriendlyError } from '@/composables/useFriendlyError'
import type { AppSearchResult, CatalogApp } from '@/api/types'

const { t } = useI18n()
const { friendly } = useFriendlyError()

const catalog = ref<CatalogApp[]>([])
const loading = ref(true)
const loadError = ref('')

async function load() {
  try {
    catalog.value = (await api.apps.catalog()).apps
    loadError.value = ''
  } catch {
    loadError.value = t('apps.loadError')
  } finally {
    loading.value = false
  }
}

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
