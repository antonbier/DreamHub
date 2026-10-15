<!--
  Built-in dashboard tile "Now on the box" (GET /api/v1/control/current):
  the channel, radio station or recording the box shows, its programme
  with a progress bar and what comes next, and a few remote buttons —
  channel up/down, mute, and the full remote panel (RemotePanel.vue). In
  standby it says so and offers to switch on. Keys are ControlHub's live
  inputs, not plans. Refreshes every 30 s and shortly after a key press.
-->
<template>
  <p v-if="loading" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
  <p v-else-if="unavailable" class="text-[13px] text-faint">{{ t('dashboard.nowPlaying.unavailable') }}</p>
  <div v-else-if="cur" class="flex h-full flex-col">
    <div class="flex items-center gap-3">
      <ChannelLogo
        v-if="cur.kind !== 'recording'"
        :source="cur.kind === 'radio' ? 'radio' : 'guide'"
        :service-reference="cur.serviceReference"
        :service-name="cur.serviceName"
        :height="30"
      />
      <Icon v-else name="recordings" :size="22" class="shrink-0 text-muted" />
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-1.5 text-[12px]" :class="cur.inStandby ? 'text-faint' : 'text-onair'">
          <span class="h-1.5 w-1.5 rounded-full" :class="cur.inStandby ? 'bg-faint' : 'bg-onair'" />
          {{ cur.inStandby ? t('dashboard.nowPlaying.standby') : t(`dashboard.nowPlaying.kind.${cur.kind}`) }}
        </div>
        <div class="truncate text-[15px] font-semibold text-text" :title="cur.serviceName">{{ cur.serviceName || '—' }}</div>
      </div>
    </div>

    <template v-if="!cur.inStandby">
      <div v-if="cur.now" class="mt-3">
        <div class="truncate text-[14px] text-text" :title="cur.now.title">{{ cur.now.title }}</div>
        <div class="mt-1 flex items-center gap-2">
          <div
            class="h-1 flex-1 overflow-hidden rounded-full bg-field"
            role="progressbar"
            :aria-valuenow="progress(cur.now)"
            aria-valuemin="0"
            aria-valuemax="100"
            :aria-label="t('guide.progress')"
          >
            <div class="h-full bg-onair" :style="{ width: progress(cur.now) + '%' }" />
          </div>
          <span class="shrink-0 text-[12px] tabular-nums text-faint">{{ clock(cur.now.start) }} – {{ clock(cur.now.start + cur.now.duration) }}</span>
        </div>
        <div v-if="cur.next" class="mt-1 truncate text-[12.5px] text-muted">
          <span class="tabular-nums text-faint">{{ t('guide.then', { time: clock(cur.next.start) }) }}</span> {{ cur.next.title }}
        </div>
      </div>
      <p v-else-if="cur.kind !== 'recording'" class="mt-3 text-[13px] text-faint">{{ t('radio.noEpg') }}</p>
    </template>

    <div class="mt-auto flex flex-wrap items-center gap-1.5 pt-3">
      <template v-if="cur.inStandby">
        <button type="button" class="flex items-center gap-1.5 rounded border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field" @click="press(KEY.POWER)">
          <Icon name="power" :size="14" />{{ t('dashboard.nowPlaying.wake') }}
        </button>
      </template>
      <template v-else>
        <button type="button" :class="iconButton" :aria-label="t('dashboard.nowPlaying.channelDown')" :title="t('dashboard.nowPlaying.channelDown')" @click="press(KEY.CHANNEL_DOWN)">
          <Icon name="chevron-down" :size="16" />
        </button>
        <button type="button" :class="iconButton" :aria-label="t('dashboard.nowPlaying.channelUp')" :title="t('dashboard.nowPlaying.channelUp')" @click="press(KEY.CHANNEL_UP)">
          <Icon name="chevron-up" :size="16" />
        </button>
        <button
          type="button"
          :class="[iconButton, cur.muted ? 'text-warn' : '']"
          :aria-label="t('dashboard.nowPlaying.mute')"
          :aria-pressed="cur.muted"
          :title="t('dashboard.nowPlaying.mute')"
          @click="press(KEY.MUTE)"
        >
          <Icon name="mute" :size="16" />
        </button>
      </template>
      <button type="button" class="tap ms-auto flex items-center gap-1.5 rounded px-2 py-1 text-[12.5px] text-accent hover:bg-tile-2" @click="remote.setPanelOpen(true)">
        <Icon name="control" :size="14" />{{ t('dashboard.nowPlaying.remote') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import ChannelLogo from '@/components/guide/ChannelLogo.vue'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { KEY } from '@/lib/remoteKeys'
import { useRemoteStore } from '@/stores/remote'
import type { CurrentService, EPGEvent } from '@/api/types'

const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()
const remote = useRemoteStore()

const iconButton = 'tap flex h-7 w-7 items-center justify-center rounded border border-line text-text hover:bg-field'

const cur = ref<CurrentService | null>(null)
const loading = ref(true)
const unavailable = ref(false)
const nowSec = ref(Date.now() / 1000)

function clock(s: number) {
  return new Intl.DateTimeFormat(locale.value, { timeStyle: 'short', timeZone: timeZone.value }).format(s * 1000)
}
function progress(e: EPGEvent) {
  return Math.round(Math.min(100, Math.max(0, ((nowSec.value - e.start) / e.duration) * 100)))
}

async function load() {
  try {
    cur.value = await api.control.current()
    unavailable.value = false
  } catch {
    unavailable.value = true
  } finally {
    loading.value = false
  }
}

// The box needs a moment to switch before getcurrent reports the new channel.
let followUp: number | undefined
async function press(code: number) {
  await remote.sendKey(code)
  window.clearTimeout(followUp)
  followUp = window.setTimeout(load, code === KEY.POWER ? 3000 : 1200)
}

let ticker: number | undefined
onMounted(() => {
  void load()
  ticker = window.setInterval(() => {
    nowSec.value = Date.now() / 1000
    void load()
  }, 30_000)
})
onBeforeUnmount(() => {
  window.clearInterval(ticker)
  window.clearTimeout(followUp)
})
</script>
