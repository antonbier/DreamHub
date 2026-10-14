<!--
  The box's current picture (ControlHub): GET /api/v1/control/screenshot,
  fetched with the app's own auth header and shown as an object URL. Beyond
  a manual refresh and an optional 4-second auto refresh, it follows the
  remote: shortly after any key the box accepted (stores/remote.ts,
  keysSent) it refreshes itself, so pressing a key shows its result.
-->
<template>
  <section class="flex flex-col gap-2">
    <div class="flex items-center justify-between gap-3">
      <h2 class="text-[14px] font-semibold text-text">{{ t('control.screenshot.title') }}</h2>
      <div class="flex items-center gap-1">
        <label class="flex cursor-pointer items-center gap-1.5 rounded px-1.5 py-1 text-[12.5px] text-muted hover:text-text">
          <input v-model="autoRefresh" type="checkbox" class="rounded" />
          {{ t('control.screenshot.auto') }}
        </label>
        <button
          type="button"
          class="flex h-7 w-7 items-center justify-center rounded text-muted hover:bg-tile-2 hover:text-text"
          :aria-label="t('control.screenshot.refresh')"
          :title="t('control.screenshot.refresh')"
          @click="refresh"
        >
          <Icon name="refresh" :size="16" :class="{ 'animate-spin motion-reduce:animate-none': loading }" />
        </button>
      </div>
    </div>
    <div class="relative flex aspect-video items-center justify-center overflow-hidden rounded bg-black">
      <img v-if="src" :src="src" :alt="t('control.screenshot.alt')" class="h-full w-full object-contain" />
      <p v-else class="px-4 text-center text-[13px] text-faint" aria-live="polite">
        {{ failed ? t('error.discovery.webcontrol_unavailable') : t('common.loading') }}
      </p>
      <span v-if="src && remote.inStandby" class="absolute inset-x-0 bottom-2 text-center text-[12.5px] text-muted">
        {{ t('control.standby.banner') }}
      </span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import { useRemoteStore } from '@/stores/remote'

const { t } = useI18n()
const remote = useRemoteStore()

const src = ref('')
const failed = ref(false)
const loading = ref(false)
// A refresh asked for while one is still in flight runs right after it,
// so the picture never ends up older than the last key press.
let again = false

async function refresh() {
  if (loading.value) {
    again = true
    return
  }
  loading.value = true
  try {
    const blob = await api.control.screenshot()
    const old = src.value
    src.value = URL.createObjectURL(blob)
    if (old) URL.revokeObjectURL(old)
    failed.value = false
  } catch {
    failed.value = true
    if (src.value) URL.revokeObjectURL(src.value)
    src.value = ''
  } finally {
    loading.value = false
    if (again) {
      again = false
      void refresh()
    }
  }
}

const autoRefresh = ref(false)
let autoTimer: ReturnType<typeof setInterval> | undefined
watch(autoRefresh, (on) => {
  if (autoTimer) clearInterval(autoTimer)
  // Skipped while the tab is hidden: a 4-second poll of a multi-megabyte
  // capture nobody is looking at is wasted work for the box.
  autoTimer = on ? setInterval(() => document.visibilityState === 'visible' && void refresh(), 4000) : undefined
})

// Follow the remote: the box needs a moment to draw the result of a key,
// and a burst of presses should cost one capture, not one each.
let followTimer: ReturnType<typeof setTimeout> | undefined
watch(
  () => remote.keysSent,
  () => {
    if (followTimer) clearTimeout(followTimer)
    followTimer = setTimeout(() => void refresh(), 700)
  },
)

onMounted(() => void refresh())
onUnmounted(() => {
  if (autoTimer) clearInterval(autoTimer)
  if (followTimer) clearTimeout(followTimer)
  if (src.value) URL.revokeObjectURL(src.value)
})
</script>
