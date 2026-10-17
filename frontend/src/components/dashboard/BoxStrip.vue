<!--
  A slim strip of the other boxes above the dashboard (docs/MULTIDEVICE_PLAN.md,
  M3): one line per box — what it shows, whether it is on standby, offline
  or recording. Only with two or more boxes; a tap switches to that box.
  There is no second dashboard: the tiles are always the selected box's.
-->
<template>
  <nav v-if="boxes.multi && others.length > 0" :aria-label="t('boxes.strip.label')" class="mb-4 flex flex-wrap items-center gap-2">
    <span class="basis-full text-[12px] text-faint" aria-hidden="true">{{ t('boxes.strip.label') }}</span>
    <div v-for="o in others" :key="o.id" class="flex min-w-0 max-w-full items-center gap-2 rounded-sm border border-line bg-tile pe-2 hover:bg-tile-2">
    <button
      type="button"
      class="flex min-w-0 flex-1 items-center gap-2 px-3 py-1.5 text-start text-[13px]"
      :class="o.online ? 'text-muted' : 'text-faint'"
      @click="boxes.select(o.id)"
    >
      <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ background: `var(--dh-profile-${o.color || 'slate'})` }" :class="{ 'opacity-40': !o.online }" aria-hidden="true" />
      <span class="shrink-0 font-semibold text-text">{{ o.name }}</span>
      <span class="truncate">{{ line(o) }}</span>
      <span v-if="o.recording" class="shrink-0 text-onair" :title="t('boxes.strip.recording')">
        <span aria-hidden="true">●</span><span class="sr-only">{{ t('boxes.strip.recording') }}</span>
      </span>
    </button>
    <BoxWake :box-id="o.id" />
    </div>
  </nav>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import BoxWake from '@/components/BoxWake.vue'
import { useBoxesStore } from '@/stores/boxes'
import type { BoxInfo, BoxStatus } from '@/api/types'

const { t } = useI18n()
const boxes = useBoxesStore()
const status = ref<Record<string, BoxStatus>>({})

const others = computed(() =>
  boxes.others.map((b: BoxInfo) => ({ ...b, online: status.value[b.id]?.online ?? b.online, recording: status.value[b.id]?.recording })),
)

function line(o: { id: string; online: boolean }): string {
  const s = status.value[o.id]
  if (!o.online) return t('boxes.selector.offline')
  if (!s) return ''
  if (s.standby) return t('boxes.strip.standby')
  return s.service ?? ''
}

async function load() {
  if (!boxes.multi) return
  try {
    const res = await api.boxes.overview()
    status.value = Object.fromEntries(res.boxes.map((b) => [b.id, b]))
  } catch {
    // Keep what is shown; the strip is a convenience.
  }
}

let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  void load()
  timer = setInterval(() => void load(), 30_000)
})
onBeforeUnmount(() => clearInterval(timer))
</script>
