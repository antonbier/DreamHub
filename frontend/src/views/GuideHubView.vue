<!--
  GuideHub's real screen (ROADMAP.md, "v1 — hubs"): now/next per channel for
  a chosen bouquet (GET /api/v1/guide/bouquets, GET /api/v1/guide/now). A
  full timeline grid EPG is still open — this is the real now/next data any
  such grid would build on. Same status-banner-plus-content pattern as
  MediaHubView.vue; see its comment for why they're independent.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="guide" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ t(item ? item.label : 'hub.guide') }}</h1>
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

    <template v-if="!item || item.status !== 'unavailable'">
      <div v-if="bouquets.length > 1" class="mt-5">
        <select
          v-model="selectedBouquet"
          class="rounded border border-line bg-field px-3 py-1.5 text-[13.5px] text-text"
        >
          <option v-for="b in bouquets" :key="b.serviceReference" :value="b.serviceReference">{{ b.serviceName }}</option>
        </select>
      </div>

      <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted">{{ t('common.backendUnreachable') }}</p>
      <p v-else-if="channels.length === 0" class="mt-5 text-[14px] text-muted">{{ t('guide.empty') }}</p>
      <ul v-else class="mt-5 flex flex-col">
        <li v-for="ch in channels" :key="ch.serviceReference" class="flex items-start gap-4 border-b border-line-soft py-2.5 last:border-none">
          <span class="w-40 shrink-0 truncate text-[14px] font-semibold text-text">{{ ch.serviceName }}</span>
          <span class="min-w-0 flex-1">
            <span v-if="ch.now" class="block truncate text-[14px] text-text">
              {{ ch.now.title }}
              <span class="text-faint">{{ ' — ' + t('guide.until', { time: formatTime(ch.now.start + ch.now.duration) }) }}</span>
            </span>
            <span v-else class="block text-[14px] text-faint">{{ t('guide.noEvent') }}</span>
            <span v-if="ch.next" class="block truncate text-[12.5px] text-muted">
              {{ t('guide.next', { time: formatTime(ch.next.start), title: ch.next.title }) }}
            </span>
          </span>
        </li>
      </ul>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import type { Bouquet, ChannelEvents } from '@/api/types'

const { t, locale } = useI18n()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/guide'))

const bouquets = ref<Bouquet[]>([])
const selectedBouquet = ref('')
const channels = ref<ChannelEvents[]>([])
const loading = ref(true)
const loadFailed = ref(false)

function formatTime(unixSeconds: number): string {
  return new Intl.DateTimeFormat(locale.value, { hour: '2-digit', minute: '2-digit' }).format(unixSeconds * 1000)
}

async function loadNow() {
  if (!selectedBouquet.value) return
  loading.value = true
  try {
    const res = await api.guide.now(selectedBouquet.value)
    channels.value = res.channels
    loadFailed.value = false
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
}

watch(selectedBouquet, loadNow)

onMounted(async () => {
  try {
    const res = await api.guide.bouquets()
    bouquets.value = res.bouquets
    if (res.bouquets.length > 0) {
      selectedBouquet.value = res.bouquets[0].serviceReference
    } else {
      loading.value = false
    }
  } catch {
    loadFailed.value = true
    loading.value = false
  }
})
</script>
