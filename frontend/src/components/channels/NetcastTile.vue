<!--
  A channel of the Netcast page as a tile: logo, name, what runs now (with its
  progress) and what comes next. A click plays it on the TV.
-->
<template>
  <component
    :is="canPlay ? 'button' : 'div'"
    :type="canPlay ? 'button' : undefined"
    class="group block rounded-sm border bg-tile text-start"
    :class="[playing ? 'border-onair' : 'border-line', canPlay ? 'hover:bg-tile-2' : '']"
    :aria-label="canPlay ? t('netcast.playNamed', { name: row.c.name }) : undefined"
    :aria-busy="busy || undefined"
    @click="canPlay && emit('play')"
  >
    <span class="block aspect-video overflow-hidden rounded-t-sm bg-tile-2">
      <NetcastLogo :token="row.token" :id="row.c.id" :name="row.c.name" :has-logo="!!row.c.hasLogo" />
    </span>
    <span class="block px-2 pb-2 pt-1.5">
      <span class="flex items-center gap-1">
        <span class="min-w-0 grow truncate text-[13.5px] font-semibold text-text">{{ row.c.name }}</span>
        <span v-if="playing" class="shrink-0 rounded-sm bg-onair px-1 text-[10.5px] font-semibold text-bg">{{ t('netcast.nowPlaying') }}</span>
        <span v-else-if="busy" class="shrink-0 text-[11px] text-muted">…</span>
      </span>
      <template v-if="guide?.now">
        <span class="mt-0.5 block truncate text-[12.5px] text-text">{{ guide.now.title }}</span>
        <span class="mt-1 block h-1 overflow-hidden rounded-sm bg-line" role="progressbar" :aria-valuenow="progress" aria-valuemin="0" aria-valuemax="100" :aria-label="t('guide.progress')">
          <span class="block h-full bg-onair" :style="{ width: progress + '%' }" />
        </span>
      </template>
      <span v-if="guide?.next" class="mt-1 block truncate text-[11.5px] text-faint"><span class="tabular-nums">{{ t('guide.then', { time: clock(guide.next.start) }) }}</span> {{ guide.next.title }}</span>
      <span v-else-if="!guide?.now" class="mt-0.5 block truncate text-[11.5px] text-faint">{{ [row.c.group, row.c.radio ? t('channels.iptv.radio') : ''].filter(Boolean).join(' · ') }}</span>
    </span>
  </component>
</template>

<script lang="ts">
import type { IptvChannel } from '@/api/types'
// One channel of an opened source; `key` is "<source>:<id>", `token` the
// kept list it came from.
export interface NetcastRow {
  key: string
  sid: string
  source: string
  token: string
  c: IptvChannel
}
</script>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import NetcastLogo from '@/components/channels/NetcastLogo.vue'
import type { NetcastNowNext } from '@/api/types'

const props = defineProps<{ row: NetcastRow; guide?: NetcastNowNext; nowSec: number; playing: boolean; busy: boolean; canPlay: boolean; clock: (s: number) => string }>()
const emit = defineEmits<{ play: [] }>()
const { t } = useI18n()

const progress = computed(() => {
  const n = props.guide?.now
  if (!n || n.end <= n.start) return 0
  return Math.round(Math.min(100, Math.max(0, ((props.nowSec - n.start) / (n.end - n.start)) * 100)))
})
</script>
