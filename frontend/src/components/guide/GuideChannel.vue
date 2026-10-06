<!--
  GuideHub's "Channel": one channel's whole TV day (05:00 to 05:00 in the
  display time zone) as a list — time, title, the EPG's short text, and
  what enrichment knows (film/series, rating). What runs now is marked
  `onair`, what's over is dimmed. A channel picker (with its logo) and a
  day picker; the grid endpoint narrowed to one channel (?service=).
-->
<template>
  <div>
    <div class="flex flex-wrap items-center gap-3">
      <div class="flex items-center gap-2">
        <ChannelLogo v-if="channel" :service-reference="channel" :service-name="channelName" :height="22" />
        <select v-model="channel" class="rounded border border-line bg-field px-3 py-1.5 text-[13.5px] text-text" :aria-label="t('guide.channel')">
          <option v-for="c in channelList" :key="c.serviceReference" :value="c.serviceReference">{{ c.serviceName }}</option>
        </select>
      </div>
      <DayPicker v-model="day" />
    </div>

    <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
    <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted">{{ t('common.backendUnreachable') }}</p>
    <p v-else-if="shown.length === 0" class="mt-5 text-[14px] text-muted">{{ filterActive ? t('guide.filteredEmpty') : t('guide.empty') }}</p>
    <ul v-else class="mt-4 flex flex-col">
      <li v-for="e in shown" :key="e.start" :ref="(el) => isNow(e) && (nowRow = el as HTMLElement)">
        <button
          type="button"
          class="flex w-full items-start gap-3 border-b border-line-soft px-1 py-2 text-start hover:bg-tile"
          :class="{ 'opacity-50': ended(e), 'bg-tile': isNow(e) }"
          @click="$emit('select', e)"
        >
          <span class="w-14 shrink-0 pt-px text-[13px] tabular-nums" :class="isNow(e) ? 'text-onair' : 'text-muted'">{{ clock(e.start) }}</span>
          <span class="min-w-0 flex-1">
            <span class="block text-[14px] text-text">{{ e.title }}</span>
            <span v-if="e.description" class="block truncate text-[12.5px] text-muted">{{ e.description }}</span>
          </span>
          <span v-if="info(e) || e.inPlex" class="flex shrink-0 items-center gap-2 text-[12px] text-muted below-sm:hidden">
            <span v-if="info(e)">{{ t(`discovery.kind.${info(e)!.kind}`) }}</span>
            <RatingBadge v-if="info(e)" :rating="info(e)!.rating" :votes="info(e)!.votes" />
            <span v-if="e.inPlex">{{ t('discovery.inPlex') }}</span>
          </span>
        </button>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import ChannelLogo from './ChannelLogo.vue'
import DayPicker from './DayPicker.vue'
import RatingBadge from '@/components/enrich/RatingBadge.vue'
import { useEnrichStore } from '@/stores/enrich'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { zonedUnix } from '@/lib/zonedTime'
import type { EPGEvent, NowNext } from '@/api/types'

// A TV day runs from 05:00 to 05:00 the next morning.
const DAY_START_HOUR = 5
const CHANNEL_KEY = 'dreamhub.guideChannel'

const props = defineProps<{ bouquet: string; filterActive: boolean; passes: (title: string) => boolean }>()
defineEmits<{ select: [e: EPGEvent] }>()

const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()
const enrich = useEnrichStore()

const channelList = ref<NowNext[]>([])
const channel = ref('')
const channelName = computed(() => channelList.value.find((c) => c.serviceReference === channel.value)?.serviceName ?? '')
const day = ref(0)
const events = ref<EPGEvent[]>([])
const loading = ref(true)
const loadFailed = ref(false)
const nowSec = ref(Date.now() / 1000)
const nowRow = ref<HTMLElement | null>(null)

const info = (e: EPGEvent) => enrich.info(e.title)
const isNow = (e: EPGEvent) => e.start <= nowSec.value && nowSec.value < e.start + e.duration
const ended = (e: EPGEvent) => e.start + e.duration <= nowSec.value
const shown = computed(() => events.value.filter((e) => props.passes(e.title)))

function clock(s: number) {
  return new Intl.DateTimeFormat(locale.value, { timeStyle: 'short', timeZone: timeZone.value }).format(s * 1000)
}

// The channel list comes from now/next (one cheap call); the last channel
// looked at is remembered.
async function loadChannels() {
  if (!props.bouquet) return
  try {
    channelList.value = (await api.guide.nowNext(props.bouquet)).channels
  } catch {
    channelList.value = []
    loadFailed.value = true
    loading.value = false
    return
  }
  let saved = ''
  try {
    saved = localStorage.getItem(CHANNEL_KEY) ?? ''
  } catch {
    // no memory
  }
  channel.value = channelList.value.find((c) => c.serviceReference === saved)?.serviceReference ?? channelList.value[0]?.serviceReference ?? ''
  if (!channel.value) loading.value = false
}
watch(() => props.bouquet, loadChannels, { immediate: true })
watch(channel, (c) => {
  try {
    if (c) localStorage.setItem(CHANNEL_KEY, c)
  } catch {
    // no memory
  }
})

let token = 0
async function load() {
  if (!props.bouquet || !channel.value) return
  const mine = ++token
  loading.value = true
  nowSec.value = Date.now() / 1000
  // Today's TV day started yesterday when it's before 05:00 now.
  const before5 = Number(new Intl.DateTimeFormat('en-GB', { hour: '2-digit', hourCycle: 'h23', timeZone: timeZone.value }).format(Date.now())) < DAY_START_HOUR
  const from = zonedUnix(timeZone.value, day.value - (before5 ? 1 : 0), DAY_START_HOUR, 0)
  try {
    const res = await api.guide.grid(props.bouquet, from, from + 24 * 3600, channel.value)
    if (mine !== token) return
    events.value = res.channels[0]?.events ?? []
    loadFailed.value = false
  } catch {
    if (mine === token) loadFailed.value = true
  } finally {
    if (mine === token) loading.value = false
  }
  // Today: bring what runs now into view.
  if (day.value === 0) {
    await nextTick()
    nowRow.value?.scrollIntoView({ block: 'center' })
  }
}
watch([channel, day], load)
</script>
