<!--
  GuideHub's "Now & next": one row per channel — logo and name, what runs
  now with a progress bar and its times, what comes next — from one call
  to the box (GET /api/v1/guide/nownext). "Switch" zaps the box straight
  to the channel (ControlHub's zap — a live input, like the remote, not a
  plan). Made for phones: the grid is too wide there. Reloads by itself
  when a programme ends; the progress bars move every 30 s. With a filter
  on, a channel stays only while its now or next passes.
-->
<template>
  <div>
    <p v-if="loading && !rows.length" class="mt-1 text-[14px] text-muted">{{ t('common.loading') }}</p>
    <p v-else-if="loadFailed" class="mt-1 text-[14px] text-muted">{{ t('common.backendUnreachable') }}</p>
    <p v-else-if="shown.length === 0" class="mt-1 text-[14px] text-muted">{{ filterActive ? t('guide.filteredEmpty') : t('guide.empty') }}</p>
    <ul v-else class="flex flex-col">
      <li v-for="ch in shown" :key="ch.serviceReference" class="flex items-center gap-3 border-b border-line-soft py-2.5 last:border-none below-sm:flex-wrap">
        <div class="flex w-40 shrink-0 items-center gap-2 below-sm:w-full">
          <ChannelLogo :service-reference="ch.serviceReference" :service-name="ch.serviceName" :height="22" />
          <span class="truncate text-[13.5px] font-semibold text-text">{{ ch.serviceName }}</span>
        </div>
        <div class="min-w-0 flex-1">
          <template v-if="ch.now">
            <button type="button" class="block max-w-full truncate text-start text-[14px] text-text hover:text-accent" @click="$emit('select', ch.now)">
              {{ ch.now.title }}
            </button>
            <div class="mt-1 flex items-center gap-2">
              <div class="h-1 flex-1 overflow-hidden rounded-full bg-field" role="progressbar" :aria-valuenow="progress(ch.now)" aria-valuemin="0" aria-valuemax="100" :aria-label="t('guide.progress')">
                <div class="h-full bg-onair" :style="{ width: progress(ch.now) + '%' }" />
              </div>
              <span class="shrink-0 text-[12px] tabular-nums text-faint">{{ range(ch.now) }}</span>
            </div>
          </template>
          <span v-else class="text-[13px] text-faint">{{ t('guide.noEpg') }}</span>
          <button v-if="ch.next" type="button" class="mt-1 block max-w-full truncate text-start text-[12.5px] text-muted hover:text-accent" @click="$emit('select', ch.next)">
            <span class="tabular-nums text-faint">{{ t('guide.then', { time: clock(ch.next.start) }) }}</span> {{ ch.next.title }}
          </button>
        </div>
        <button
          type="button"
          class="shrink-0 rounded border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field disabled:opacity-60"
          :disabled="zapping === ch.serviceReference"
          @click="zap(ch.serviceReference)"
        >
          {{ zapped === ch.serviceReference ? t('guide.switchedNow') : t('guide.switch') }}
        </button>
      </li>
    </ul>
    <p v-if="zapError" class="mt-2 text-[12.5px] text-warn" role="status">{{ zapError }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import ChannelLogo from './ChannelLogo.vue'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { usePlanError } from '@/composables/usePlanError'
import type { EPGEvent, NowNext } from '@/api/types'

const props = defineProps<{ bouquet: string; filterActive: boolean; passes: (title: string) => boolean }>()
defineEmits<{ select: [e: EPGEvent] }>()

const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()
const { planErrorMessage } = usePlanError()

const rows = ref<NowNext[]>([])
const loading = ref(true)
const loadFailed = ref(false)
const nowSec = ref(Date.now() / 1000)

const shown = computed(() =>
  props.filterActive ? rows.value.filter((r) => (r.now && props.passes(r.now.title)) || (r.next && props.passes(r.next.title))) : rows.value,
)

function clock(s: number) {
  return new Intl.DateTimeFormat(locale.value, { timeStyle: 'short', timeZone: timeZone.value }).format(s * 1000)
}
const range = (e: EPGEvent) => `${clock(e.start)} – ${clock(e.start + e.duration)}`
function progress(e: EPGEvent) {
  return Math.round(Math.min(100, Math.max(0, ((nowSec.value - e.start) / e.duration) * 100)))
}

let token = 0
async function load() {
  if (!props.bouquet) return
  const mine = ++token
  loading.value = true
  try {
    const res = await api.guide.nowNext(props.bouquet)
    if (mine !== token) return
    rows.value = res.channels
    loadFailed.value = false
  } catch {
    if (mine === token) loadFailed.value = true
  } finally {
    if (mine === token) loading.value = false
  }
}
watch(() => props.bouquet, load, { immediate: true })

// Tick the bars; reload once any "now" has ended.
const ticker = setInterval(() => {
  nowSec.value = Date.now() / 1000
  if (rows.value.some((r) => r.now && r.now.start + r.now.duration <= nowSec.value)) void load()
}, 30_000)
onBeforeUnmount(() => clearInterval(ticker))

const zapping = ref('')
const zapped = ref('')
const zapError = ref('')
async function zap(ref: string) {
  zapping.value = ref
  zapError.value = ''
  try {
    await api.control.zap(ref)
    zapped.value = ref
  } catch (err) {
    zapError.value = planErrorMessage(err)
  } finally {
    zapping.value = ''
  }
}
</script>
