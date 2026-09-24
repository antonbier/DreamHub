<!--
  Diagnostics (ROADMAP.md, Foundation: "a dedicated page with reason and fix
  instructions across all hubs at once"). Reuses GET /api/v1/hubs (already
  built for exactly this — its own doc comment calls it "the diagnostics
  view") and the same translated problem/fix pattern as the sidebar,
  NotificationsBell and every hub placeholder. Not a hub itself — a core
  route, like /settings (backend/internal/hub/hub.go's Navigation()).
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <h1 class="text-[26px] font-bold">{{ t('diagnostics.title') }}</h1>

    <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
    <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted">{{ t('common.backendUnreachable') }}</p>
    <template v-else>
      <section v-for="group in groups" :key="group.key" class="mt-6 first:mt-5">
        <h2 class="text-[15px] font-semibold text-text">{{ t(`diagnostics.group.${group.key}`) }} ({{ group.hubs.length }})</h2>
        <ul v-if="group.hubs.length > 0" class="mt-2 flex flex-col">
          <li v-for="hub in group.hubs" :key="hub.id" class="flex items-start gap-3 border-b border-line-soft py-2.5 last:border-none">
            <Icon :name="hub.icon" :size="18" class="mt-0.5 shrink-0 text-muted" />
            <span class="min-w-0 flex-1">
              <span class="flex items-center gap-2">
                <span class="truncate text-[14px] text-text">{{ t(hub.label) }}</span>
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import { useProblem } from '@/composables/useProblem'
import type { HubStatus } from '@/api/types'

const { t } = useI18n()
const { describe, describeFix } = useProblem()

const hubs = ref<HubStatus[]>([])
const loading = ref(true)
const loadFailed = ref(false)

// Most urgent first: hubs that don't work, then ones that partly work,
// then ones the user turned off on purpose, then everything that's fine.
const groups = computed(() => [
  { key: 'unavailable', hubs: hubs.value.filter((h) => h.enabled && h.status === 'unavailable') },
  { key: 'degraded', hubs: hubs.value.filter((h) => h.enabled && h.status === 'degraded') },
  { key: 'disabled', hubs: hubs.value.filter((h) => !h.enabled) },
  { key: 'available', hubs: hubs.value.filter((h) => h.enabled && h.status === 'available') },
])

onMounted(async () => {
  try {
    const res = await api.hubs()
    hubs.value = res.hubs
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
})
</script>
