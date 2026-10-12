<!--
  AdminHub's package browser (docs/ADMINHUB_PLAN.md, A2, #47): the box's
  installed and upgradable packages, grouped by apt's own Section field —
  no DreamHub-maintained category lookup, so an unseen Section just becomes
  a new group. Read-only for now: install/remove as a plan is a later
  slice, once the box has confirmed `apt-get -s` simulate output.
-->
<template>
  <section class="mt-5 rounded border border-line bg-tile p-4">
    <div class="flex items-center gap-3">
      <h2 class="text-[14px] font-semibold text-text">{{ t('admin.packages.title') }}</h2>
      <button type="button" class="ms-auto rounded p-1 text-muted hover:bg-tile-2 hover:text-text" :aria-label="t('admin.logs.refresh')" :title="t('admin.logs.refresh')" @click="load">
        <Icon name="refresh" :size="15" />
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
      <select v-model="status" class="rounded border border-line bg-field px-2 py-1.5 text-[13px] text-text" @change="load">
        <option value="">{{ t('admin.packages.allStatus') }}</option>
        <option value="upgradable">{{ t('admin.packages.upgradable') }}</option>
      </select>
    </div>

    <p v-if="loading && !packages.length" class="mt-2 text-[13px] text-faint">{{ t('common.loading') }}</p>
    <p v-else-if="error" class="mt-2 text-[13px] text-faint">{{ error }}</p>
    <p v-else-if="packages.length === 0" class="mt-2 text-[13px] text-faint">{{ t('admin.packages.empty') }}</p>
    <ul v-else class="mt-2 flex max-h-96 flex-col overflow-auto">
      <li v-for="p in packages" :key="p.name" class="border-b border-line-soft py-2 last:border-none">
        <div class="flex items-baseline gap-2">
          <span class="font-mono text-[13px] text-text">{{ p.name }}</span>
          <span class="shrink-0 rounded bg-tile-2 px-1.5 py-0.5 text-[11px] text-faint">{{ p.category }}</span>
        </div>
        <p class="truncate text-[12.5px] text-faint">{{ p.description }}</p>
        <p class="text-[12.5px]" :class="p.upgradable ? 'text-warn' : 'text-muted'">
          {{ p.upgradable ? t('admin.packages.upgradableFrom', { from: p.version, to: p.availableVersion }) : p.version }}
        </p>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { useFriendlyError } from '@/composables/useFriendlyError'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import Icon from '@/components/Icon.vue'
import type { AdminPackage } from '@/api/types'

const { t } = useI18n()
const { friendly } = useFriendlyError()

const packages = ref<AdminPackage[]>([])
const categories = ref<string[]>([])
const loading = ref(true)
const error = ref('')
const q = ref('')
const category = ref('')
const status = ref('')

async function load() {
  loading.value = true
  try {
    const res = await api.admin.packages({ q: q.value, category: category.value, status: status.value })
    packages.value = res.packages
    categories.value = res.categories
    error.value = ''
  } catch (err) {
    error.value = err instanceof ApiError && err.code === 'apt_unavailable' ? t('admin.packages.unavailable') : friendly(err).text
  } finally {
    loading.value = false
  }
}

let debounceTimer: number | undefined
function debouncedLoad() {
  window.clearTimeout(debounceTimer)
  debounceTimer = window.setTimeout(load, 300)
}

onMounted(load)
</script>
