<!--
  Built-in dashboard tile: disk usage per storage device
  (GET /api/v1/media/disk) — the reference device's disk being close to
  full is a real, recurring pain point (ROADMAP.md, "MediaHub").
-->
<template>
  <p v-if="loading" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
  <p v-else-if="unavailable" class="text-[13px] text-faint">{{ t('dashboard.mediaDiskTile.unavailable') }}</p>
  <p v-else-if="disks.length === 0" class="text-[13px] text-faint">{{ t('dashboard.mediaDiskTile.empty') }}</p>
  <ul v-else class="flex flex-col gap-3">
    <li v-for="(disk, i) in disks" :key="i">
      <div class="mb-1 flex items-baseline justify-between gap-2 text-[13px]">
        <span class="truncate text-text">{{ disk.model }}</span>
        <span class="shrink-0 whitespace-nowrap text-muted">{{ disk.freeText }} / {{ disk.capacityText }}</span>
      </div>
      <div class="h-1.5 overflow-hidden rounded-full bg-line-soft">
        <div
          class="h-full rounded-full"
          :class="disk.usedPercent != null && disk.usedPercent >= 90 ? 'bg-warn' : 'bg-accent'"
          :style="{ width: (disk.usedPercent ?? 0) + '%' }"
        />
      </div>
    </li>
  </ul>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import type { Disk } from '@/api/types'

const { t } = useI18n()
const disks = ref<Disk[]>([])
const loading = ref(true)
const unavailable = ref(false)

onMounted(async () => {
  try {
    const res = await api.media.disk()
    disks.value = res.disks
  } catch {
    unavailable.value = true
  } finally {
    loading.value = false
  }
})
</script>
