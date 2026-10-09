<!--
  Built-in dashboard tile: every hub that isn't fully available
  (GET /api/v1/hubs), with the same translated problem/fix explanation as
  the sidebar (PRINCIPLES.md, "Hub status: always explain what's missing").
  Disabled hubs are left out — they're an intentional choice, not a problem.
-->
<template>
  <p v-if="loading" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
  <p v-else-if="failed" class="text-[13px] text-faint"><ErrorText /></p>
  <p v-else-if="attention.length === 0" class="text-[13px] text-faint">{{ t('dashboard.hubsTile.allOk') }}</p>
  <ul v-else class="flex flex-col">
    <li v-for="hub in attention" :key="hub.id" class="flex items-start gap-2.5 border-b border-line-soft py-2 last:border-none">
      <span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full" :class="hub.status === 'degraded' ? 'bg-warn' : 'bg-faint'" />
      <span class="min-w-0 flex-1">
        <span class="block truncate text-[14px] text-text">{{ hubLabel(hub) }}</span>
        <span class="block text-[12.5px] text-muted">
          {{ hub.problems.map(describe).join(' · ') }}
        </span>
      </span>
    </li>
  </ul>
</template>

<script setup lang="ts">
import { useHubLabel } from '@/composables/useHubLabel'
import ErrorText from '@/components/ErrorText.vue'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { useProblem } from '@/composables/useProblem'
import type { HubStatus } from '@/api/types'

const { t } = useI18n()
const { hubLabel } = useHubLabel()
const { describe } = useProblem()
const hubs = ref<HubStatus[]>([])
const loading = ref(true)
const failed = ref(false)

const attention = computed(() => hubs.value.filter((h) => h.enabled && h.status !== 'available'))

onMounted(async () => {
  try {
    const res = await api.hubs()
    hubs.value = res.hubs
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
})
</script>
