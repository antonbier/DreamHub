<!--
  The virtual remote (ControlHub), laid out like a physical one: power and
  mute on top, the number pad, menu/EPG/info, volume and channel rockers
  around the direction pad, back, the four color keys, playback. Used on
  ControlHub's own page and in the side panel every page can open
  (RemotePanel.vue); `compact` folds the number pad away behind a toggle
  to keep the panel short.

  Every key is a real <button> with a translated label, and the whole pad
  also takes the physical keyboard while it has focus (lib/remoteKeys.ts:
  arrows, Enter = OK, Backspace = back, 0–9, PageUp/PageDown, +/-). Sent
  keys go straight to the box, no preview — see ControlHubView.vue's
  comment for why a remote is the exception.
-->
<template>
  <div
    ref="root"
    role="group"
    :aria-label="t('control.pad.title')"
    tabindex="0"
    class="mx-auto flex w-full max-w-[280px] flex-col gap-3 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-accent"
    @keydown="onKeydown"
  >
    <div v-if="remote.inStandby" class="flex items-center justify-between gap-2 rounded border border-line bg-field px-3 py-2">
      <span class="text-[12.5px] text-muted">{{ t('control.standby.banner') }}</span>
      <button type="button" class="shrink-0 rounded bg-accent px-2.5 py-1 text-[12.5px] font-semibold text-bg" @click="press(KEY.POWER)">
        {{ t('control.standby.wake') }}
      </button>
    </div>

    <div class="flex items-center justify-between">
      <button type="button" :class="powerKey" :aria-label="t('control.keys.power')" :title="t('control.keys.power')" @click="press(KEY.POWER)">
        <Icon name="power" :size="18" />
      </button>
      <button
        v-if="compact"
        type="button"
        class="rounded px-2 py-1 text-[12.5px] text-muted hover:bg-tile-2 hover:text-text"
        :aria-expanded="digitsShown"
        @click="digitsShown = !digitsShown"
      >
        {{ t('control.pad.digits') }}
      </button>
      <button type="button" :class="round" :aria-label="t('control.keys.mute')" :title="t('control.keys.mute')" @click="press(KEY.MUTE)">
        <Icon name="mute" :size="18" />
      </button>
    </div>

    <div v-if="digitsShown" class="grid grid-cols-3 gap-2">
      <button
        v-for="n in [1, 2, 3, 4, 5, 6, 7, 8, 9]"
        :key="n"
        type="button"
        :class="pill"
        class="tabular-nums"
        @click="press(KEY.DIGITS[n])"
      >
        {{ formatDigit(n) }}
      </button>
      <span />
      <button type="button" :class="pill" class="tabular-nums" @click="press(KEY.DIGITS[0])">{{ formatDigit(0) }}</button>
      <span />
    </div>

    <div class="grid grid-cols-3 gap-2">
      <button type="button" :class="pill" @click="press(KEY.MENU)">{{ t('control.keys.menu') }}</button>
      <button type="button" :class="pill" @click="press(KEY.EPG)">{{ t('control.keys.epg') }}</button>
      <button type="button" :class="pill" @click="press(KEY.INFO)">{{ t('control.keys.info') }}</button>
    </div>

    <div class="flex items-center justify-between gap-2">
      <div class="flex flex-col overflow-hidden rounded-full border border-line">
        <button type="button" :class="rocker" :aria-label="t('control.keys.volumeUp')" :title="t('control.keys.volumeUp')" @click="press(KEY.VOLUME_UP)">
          <Icon name="volume-up" :size="17" />
        </button>
        <span class="py-0.5 text-center text-[10.5px] text-faint" aria-hidden="true">{{ t('control.pad.volume') }}</span>
        <button type="button" :class="rocker" :aria-label="t('control.keys.volumeDown')" :title="t('control.keys.volumeDown')" @click="press(KEY.VOLUME_DOWN)">
          <Icon name="volume-down" :size="17" />
        </button>
      </div>

      <!-- Direction pad: one ring, OK in the middle. -->
      <div class="grid h-[150px] w-[150px] shrink-0 grid-cols-3 grid-rows-3 rounded-full border border-line bg-field">
        <span />
        <button type="button" :class="arrow" :aria-label="t('control.keys.up')" @click="press(KEY.UP)"><Icon name="chevron-up" :size="20" /></button>
        <span />
        <button type="button" :class="arrow" :aria-label="t('control.keys.left')" @click="press(KEY.LEFT)"><Icon name="chevron-left" :size="20" /></button>
        <button
          type="button"
          class="m-auto flex h-[52px] w-[52px] items-center justify-center rounded-full bg-accent text-[14px] font-bold text-bg transition-transform hover:brightness-110 active:scale-95 motion-reduce:transition-none"
          @click="press(KEY.OK)"
        >
          {{ t('control.keys.ok') }}
        </button>
        <button type="button" :class="arrow" :aria-label="t('control.keys.right')" @click="press(KEY.RIGHT)"><Icon name="chevron-right" :size="20" /></button>
        <span />
        <button type="button" :class="arrow" :aria-label="t('control.keys.down')" @click="press(KEY.DOWN)"><Icon name="chevron-down" :size="20" /></button>
        <span />
      </div>

      <div class="flex flex-col overflow-hidden rounded-full border border-line">
        <button type="button" :class="rocker" :aria-label="t('control.keys.channelUp')" :title="t('control.keys.channelUp')" @click="press(KEY.CHANNEL_UP)">
          <Icon name="chevron-up" :size="17" />
        </button>
        <span class="py-0.5 text-center text-[10.5px] text-faint" aria-hidden="true">{{ t('control.pad.channel') }}</span>
        <button type="button" :class="rocker" :aria-label="t('control.keys.channelDown')" :title="t('control.keys.channelDown')" @click="press(KEY.CHANNEL_DOWN)">
          <Icon name="chevron-down" :size="17" />
        </button>
      </div>
    </div>

    <button type="button" :class="pill" class="flex items-center justify-center gap-1.5" @click="press(KEY.EXIT)">
      <Icon name="back" :size="16" />
      {{ t('control.keys.exit') }}
    </button>

    <!-- Color keys: colored like the physical ones, named for screen readers and on hover. -->
    <div class="grid grid-cols-4 gap-2">
      <button
        v-for="c in colorKeys"
        :key="c.code"
        type="button"
        class="h-7 rounded transition-transform hover:brightness-110 active:scale-95 motion-reduce:transition-none"
        :class="c.bg"
        :aria-label="t(c.label)"
        :title="t(c.label)"
        @click="press(c.code)"
      />
    </div>

    <div class="flex items-center justify-between">
      <button
        v-for="p in playbackKeys"
        :key="p.code"
        type="button"
        :class="round"
        class="h-9 w-9"
        :aria-label="t(p.label)"
        :title="t(p.label)"
        @click="press(p.code)"
      >
        <Icon :name="p.icon" :size="16" />
      </button>
    </div>

    <p class="text-center text-[11.5px] text-faint below-sm:hidden">{{ t('control.pad.keyboardHint') }}</p>
    <p class="min-h-[1em] text-center text-[12.5px] text-warn" aria-live="polite">{{ errorText }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/Icon.vue'
import { KEY, keyForKeyboardEvent } from '@/lib/remoteKeys'
import { useRemoteStore } from '@/stores/remote'
import { usePlanError } from '@/composables/usePlanError'

const props = withDefaults(defineProps<{ compact?: boolean }>(), { compact: false })

const { t, locale } = useI18n()
const remote = useRemoteStore()
const { planErrorMessage } = usePlanError()

const root = ref<HTMLElement | null>(null)
const digitsShown = ref(!props.compact)

const keyBase = 'flex items-center justify-center transition-transform hover:bg-tile-2 active:scale-95 motion-reduce:transition-none'
const base = `${keyBase} text-text`
const round = `${base} h-10 w-10 rounded-full border border-line`
const powerKey = `${keyBase} h-10 w-10 rounded-full border border-line text-onair`
const pill = `${base} h-9 rounded border border-line text-[13px] font-semibold`
const rocker = `${base} h-11 w-11`
const arrow = `${base} rounded-full text-muted hover:text-text`

const colorKeys = [
  { code: KEY.RED, bg: 'bg-onair', label: 'control.keys.red' },
  { code: KEY.GREEN, bg: 'bg-ok', label: 'control.keys.green' },
  { code: KEY.YELLOW, bg: 'bg-warn', label: 'control.keys.yellow' },
  { code: KEY.BLUE, bg: 'bg-accent', label: 'control.keys.blue' },
]
const playbackKeys = [
  { code: KEY.REWIND, icon: 'rewind', label: 'control.keys.rewind' },
  { code: KEY.PLAY, icon: 'play', label: 'control.keys.play' },
  { code: KEY.PAUSE, icon: 'pause', label: 'control.keys.pause' },
  { code: KEY.STOP, icon: 'stop', label: 'control.keys.stop' },
  { code: KEY.FAST_FORWARD, icon: 'fast-forward', label: 'control.keys.fastForward' },
]

const errorText = computed(() => (remote.lastError ? planErrorMessage(remote.lastError) : ''))

function formatDigit(n: number) {
  return new Intl.NumberFormat(locale.value).format(n)
}

function press(code: number) {
  void remote.sendKey(code)
}

function onKeydown(e: KeyboardEvent) {
  // A focused button already turns Enter/Space into its own click.
  if ((e.key === 'Enter' || e.key === ' ') && (e.target as HTMLElement).tagName === 'BUTTON') return
  const code = keyForKeyboardEvent(e)
  if (code === undefined) return
  e.preventDefault()
  press(code)
}

onMounted(() => void remote.loadPower())

// Lets the side panel hand keyboard focus to the pad when it opens.
defineExpose({ focus: () => root.value?.focus() })
</script>
