<!--
  StreamHub (/stream): live TV and recordings in the browser. The box sends
  its stream unchanged; the browser decodes it (src/streaming/tsPlayer.ts).

  Live: a bouquet's channels with what's on, the one the box shows on top
  ("same as the TV" needs no tuner of its own). ?ref= starts a channel.
  Another channel needs a free tuner — with one tuner, a channel on another
  transponder can't be watched while the TV or a recording uses it; the
  box then sends nothing, and the screen says why.

  Recordings: ?recording=<path>&title=&length=MM:SS (from MediaHub). The
  bar seeks by restarting at that point of the file.

  Keys while the player has focus: Space pause, M mute, F full screen.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="stream" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ t(item ? item.label : 'hub.stream') }}</h1>
    </div>
    <p class="mt-1 text-[13.5px] text-muted">{{ t('stream.intro') }}</p>

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

    <p v-if="!supported" class="mt-5 rounded border border-line bg-tile p-4 text-[14px] text-muted">{{ t('stream.unsupported') }}</p>

    <div v-else-if="!item || item.status !== 'unavailable'" class="mt-5 grid grid-cols-[minmax(0,1fr)_320px] gap-6 below-lg:grid-cols-1">
      <!-- The player -->
      <section :aria-label="t('stream.player')">
        <div
          ref="frame"
          tabindex="0"
          class="group relative aspect-video overflow-hidden rounded bg-black focus-visible:outline focus-visible:outline-2 focus-visible:outline-accent"
          :class="{ 'rounded-none': fullscreen }"
          @keydown="onKey"
          @dblclick="toggleFullscreen"
        >
          <canvas ref="canvas" class="h-full w-full object-contain" />
          <div v-if="state !== 'playing' && state !== 'paused'" class="absolute inset-0 flex items-center justify-center p-6 text-center" aria-live="polite">
            <p v-if="state === 'idle'" class="text-[14px] text-white/70">{{ t('stream.pick') }}</p>
            <p v-else-if="state === 'loading'" class="flex items-center gap-2 text-[14px] text-white/80">
              <span class="h-2 w-2 animate-pulse rounded-full bg-onair" />{{ t('stream.loading') }}
            </p>
            <p v-else-if="state === 'ended'" class="text-[14px] text-white/80">{{ t('stream.ended') }}</p>
            <div v-else-if="state === 'error'" class="max-w-md">
              <p class="text-[14.5px] font-semibold text-white">{{ errorText }}</p>
              <p v-if="errorHint" class="mt-1.5 text-[13px] text-white/70">{{ errorHint }}</p>
              <button type="button" class="mt-3 rounded border border-white/40 px-3 py-1 text-[13px] text-white hover:bg-white/10" @click="restart()">
                {{ t('stream.retry') }}
              </button>
            </div>
          </div>
          <span
            v-if="source?.kind === 'live' && state === 'playing'"
            class="pointer-events-none absolute start-3 top-3 flex items-center gap-1.5 rounded bg-black/60 px-2 py-0.5 text-[11.5px] font-semibold uppercase tracking-wide text-white"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-onair" />{{ t('stream.live') }}
          </span>
        </div>

        <!-- What's playing -->
        <div class="mt-3 flex items-center gap-3">
          <ChannelLogo v-if="current" source="stream" :service-reference="current.serviceReference" :service-name="current.serviceName" :height="28" />
          <div class="min-w-0 flex-1">
            <div class="truncate text-[15.5px] font-semibold text-text">{{ heading }}</div>
            <div v-if="current?.now" class="truncate text-[13px] text-muted">
              {{ current.now.title }} <span class="tabular-nums text-faint">· {{ range(current.now) }}</span>
            </div>
            <div v-else-if="source?.kind === 'recording' && recordingSubtitle" class="truncate text-[13px] text-muted">{{ recordingSubtitle }}</div>
          </div>
        </div>

        <!-- Recording: position, seekable -->
        <div v-if="source?.kind === 'recording'" class="mt-3 flex items-center gap-3">
          <span class="w-14 shrink-0 text-end font-mono text-[12px] tabular-nums text-faint">{{ positionText }}</span>
          <input
            type="range"
            min="0"
            max="1000"
            :value="Math.round(position * 1000)"
            class="h-1 flex-1 accent-[var(--dh-accent)]"
            :aria-label="t('stream.position')"
            :aria-valuetext="positionText"
            @change="seek(($event.target as HTMLInputElement).valueAsNumber / 1000)"
          />
          <span class="w-14 shrink-0 font-mono text-[12px] tabular-nums text-faint">{{ lengthText }}</span>
        </div>

        <!-- Controls -->
        <div class="mt-3 flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="flex h-9 w-9 items-center justify-center rounded border border-line text-text hover:bg-field disabled:opacity-50"
            :disabled="state !== 'playing' && state !== 'paused'"
            :aria-label="state === 'paused' ? t('stream.resume') : t('stream.pause')"
            @click="togglePause"
          >
            <Icon :name="state === 'paused' ? 'play' : 'pause'" :size="16" />
          </button>
          <button
            type="button"
            class="flex h-9 w-9 items-center justify-center rounded border border-line text-text hover:bg-field disabled:opacity-50"
            :disabled="!source"
            :aria-label="t('stream.stop')"
            @click="stop"
          >
            <Icon name="stop" :size="15" />
          </button>
          <button
            type="button"
            class="flex h-9 w-9 items-center justify-center rounded border border-line text-text hover:bg-field"
            :aria-label="muted ? t('stream.unmute') : t('stream.mute')"
            :aria-pressed="muted"
            @click="setMuted(!muted)"
          >
            <Icon :name="muted ? 'mute' : 'volume-up'" :size="16" />
          </button>
          <input
            type="range"
            min="0"
            max="100"
            :value="Math.round(volume * 100)"
            class="h-1 w-28 accent-[var(--dh-accent)]"
            :aria-label="t('stream.volume')"
            @input="setVolume(($event.target as HTMLInputElement).valueAsNumber / 100)"
          />
          <select
            v-if="tracks.length > 1"
            :value="track"
            class="rounded border border-line bg-field px-2 py-1.5 text-[13px] text-text"
            :aria-label="t('stream.audioTrack')"
            @change="selectTrack(Number(($event.target as HTMLSelectElement).value))"
          >
            <option v-for="(tr, i) in tracks" :key="tr.index" :value="tr.index">{{ trackLabel(tr, i) }}</option>
          </select>
          <span class="flex-1" />
          <button
            type="button"
            class="rounded px-2 py-1 text-[12.5px] text-muted hover:text-text"
            :aria-pressed="showInfo"
            @click="showInfo = !showInfo"
          >
            {{ t('stream.info') }}
          </button>
          <button
            type="button"
            class="flex h-9 w-9 items-center justify-center rounded border border-line text-text hover:bg-field"
            :aria-label="t('stream.fullscreen')"
            @click="toggleFullscreen"
          >
            <Icon name="fullscreen" :size="16" />
          </button>
        </div>
        <dl v-if="showInfo && source" class="mt-3 grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-4 gap-y-0.5 break-words rounded border border-line-soft bg-tile p-3 font-mono text-[12px] text-muted below-sm:grid-cols-1">
          <dt class="text-faint">{{ t('stream.stats.video') }}</dt><dd>{{ stats.path || stats.video }} · {{ stats.width }}×{{ stats.height }}</dd>
          <dt class="text-faint">{{ t('stream.stats.audio') }}</dt><dd>{{ stats.audio }}</dd>
          <dt class="text-faint">{{ t('stream.stats.data') }}</dt><dd>{{ stats.kbps }} kbit/s</dd>
          <dt class="text-faint">{{ t('stream.stats.frames') }}</dt><dd>{{ stats.shown }} / {{ stats.dropped }} · {{ stats.buffered }}</dd>
          <dt class="text-faint">{{ t('stream.stats.sync') }}</dt><dd>{{ stats.avOffsetMs }} ms · {{ stats.audioUnderruns }}</dd>
        </dl>
        <p v-if="source?.kind === 'live'" class="mt-3 text-[12.5px] text-faint">{{ t('stream.tunerHint') }}</p>
      </section>

      <!-- Channels -->
      <aside v-if="source?.kind !== 'recording'" :aria-label="t('stream.channels')">
        <p v-if="bouquetsFailed" class="text-[14px] text-muted"><ErrorText /></p>
        <template v-else>
          <div class="flex flex-wrap items-center gap-2">
            <select
              v-if="bouquets.length > 1"
              v-model="selectedBouquet"
              class="min-w-0 flex-1 rounded border border-line bg-field px-3 py-1.5 text-[13.5px] text-text"
              :aria-label="t('stream.bouquet')"
            >
              <option v-for="b in bouquets" :key="b.serviceReference" :value="b.serviceReference">{{ b.serviceName }}</option>
            </select>
            <input
              v-if="channels.length > 8"
              v-model="query"
              type="search"
              :placeholder="t('stream.find')"
              :aria-label="t('stream.find')"
              class="w-full rounded border border-line bg-field px-2.5 py-1.5 text-[13.5px] text-text"
            />
          </div>
          <button
            v-if="playingChannel"
            type="button"
            class="mt-3 flex w-full items-center gap-2.5 rounded border border-line bg-tile px-3 py-2 text-start hover:bg-tile-2"
            :class="{ 'border-accent': source?.kind === 'live' && source.ref === playingChannel.serviceReference }"
            @click="watch_(playingChannel.serviceReference)"
          >
            <Icon name="control" :size="16" class="shrink-0 text-muted" />
            <span class="min-w-0 flex-1">
              <span class="block text-[12px] text-faint">{{ t('stream.sameAsTv') }}</span>
              <span class="block truncate text-[13.5px] font-semibold text-text">{{ playingChannel.serviceName }}</span>
            </span>
          </button>
          <p v-if="channelsLoading && !channels.length" class="mt-3 text-[14px] text-muted">{{ t('common.loading') }}</p>
          <p v-else-if="channelsFailed" class="mt-3 text-[14px] text-muted"><ErrorText /></p>
          <ul v-else class="mt-2 flex max-h-[70vh] flex-col overflow-y-auto below-lg:max-h-none">
            <li v-for="ch in shownChannels" :key="ch.serviceReference">
              <button
                type="button"
                class="flex w-full items-center gap-2.5 rounded px-2 py-2 text-start hover:bg-tile-2"
                :class="{ 'bg-tile-2': source?.kind === 'live' && source.ref === ch.serviceReference }"
                :aria-current="source?.kind === 'live' && source.ref === ch.serviceReference ? 'true' : undefined"
                @click="watch_(ch.serviceReference)"
              >
                <span class="flex w-10 shrink-0 justify-center"><ChannelLogo source="stream" :service-reference="ch.serviceReference" :service-name="ch.serviceName" :height="20" /></span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-[13.5px] font-semibold text-text">{{ ch.serviceName }}</span>
                  <span v-if="ch.now" class="block truncate text-[12.5px] text-muted">{{ ch.now.title }}</span>
                </span>
                <span v-if="playing === ch.serviceReference" class="h-1.5 w-1.5 shrink-0 rounded-full bg-onair" :title="t('stream.onTv')" />
              </button>
            </li>
          </ul>
        </template>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import ErrorText from '@/components/ErrorText.vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { api, ApiError } from '@/api/client'
import Icon from '@/components/Icon.vue'
import ChannelLogo from '@/components/guide/ChannelLogo.vue'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { TsPlayer, type AudioTrack, type PlayerStats, type Source } from '@/streaming/tsPlayer'
import type { Bouquet, EPGEvent, NowNext } from '@/api/types'

const BOUQUET_KEY = 'dreamhub.streamBouquet'
const VOLUME_KEY = 'dreamhub.streamVolume'

const { t, locale } = useI18n()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()
const { timeZone } = useDisplayTimeZone()
const route = useRoute()
const router = useRouter()

const navItems = computed(() => navigation.navigation?.sections.flatMap((s) => s.items) ?? [])
const item = computed(() => navItems.value.find((i) => i.path === '/stream'))
const supported = typeof window !== 'undefined' && 'VideoFrame' in window && 'AudioContext' in window && typeof WebAssembly === 'object'

// --- time -------------------------------------------------------------------

const nowSec = ref(Date.now() / 1000)
function clock(s: number) {
  return new Intl.DateTimeFormat(locale.value, { timeStyle: 'short', timeZone: timeZone.value }).format(s * 1000)
}
const range = (e: EPGEvent) => `${clock(e.start)} – ${clock(e.start + e.duration)}`

// --- channels ---------------------------------------------------------------

const bouquets = ref<Bouquet[]>([])
const bouquetsFailed = ref(false)
const selectedBouquet = ref('')
const channels = ref<NowNext[]>([])
const channelsLoading = ref(false)
const channelsFailed = ref(false)
const playing = ref('')
const query = ref('')

const shownChannels = computed(() => {
  const q = query.value.trim().toLowerCase()
  return q ? channels.value.filter((c) => c.serviceName.toLowerCase().includes(q)) : channels.value
})
const playingChannel = computed(() => channels.value.find((c) => c.serviceReference === playing.value) ?? null)

function store(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* not remembered */
  }
}
function recall(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

async function loadBouquets() {
  try {
    bouquets.value = (await api.stream.bouquets()).bouquets
    bouquetsFailed.value = false
    const remembered = recall(BOUQUET_KEY)
    selectedBouquet.value = bouquets.value.find((b) => b.serviceReference === remembered)?.serviceReference ?? bouquets.value[0]?.serviceReference ?? ''
  } catch {
    bouquetsFailed.value = true
  }
}

let token = 0
async function loadChannels() {
  if (!selectedBouquet.value) return
  const mine = ++token
  channelsLoading.value = true
  try {
    const res = await api.stream.channels(selectedBouquet.value)
    if (mine !== token) return
    channels.value = res.channels
    playing.value = res.playing
    channelsFailed.value = false
  } catch {
    if (mine === token) channelsFailed.value = true
  } finally {
    if (mine === token) channelsLoading.value = false
  }
}
watch(selectedBouquet, (b) => {
  if (b) store(BOUQUET_KEY, b)
  void loadChannels()
})

// --- the player ---------------------------------------------------------------

const frame = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
type State = 'idle' | 'loading' | 'playing' | 'paused' | 'ended' | 'error'
const state = ref<State>('idle')
const error = ref<Error | null>(null)
const source = ref<Source | null>(null)
const tracks = ref<AudioTrack[]>([])
const track = ref(-1)
const position = ref(0)
const showInfo = ref(false)
const fullscreen = ref(false)
const stats = reactive<PlayerStats>({ video: '', audio: '', path: '', width: 0, height: 0, shown: 0, dropped: 0, audioUnderruns: 0, avOffsetMs: 0, buffered: 0, kbps: 0 })
const savedVolume = Number(recall(VOLUME_KEY) ?? '1')
const volume = ref(Number.isFinite(savedVolume) ? Math.min(1, Math.max(0, savedVolume)) : 1)
const muted = ref(false)
let player: TsPlayer | null = null

const current = computed(() => {
  const s = source.value
  return s?.kind === 'live' ? (channels.value.find((c) => c.serviceReference === s.ref) ?? null) : null
})
const heading = computed(() => {
  const s = source.value
  if (s?.kind === 'recording') return (route.query.title as string) || s.path.split('/').pop() || ''
  return current.value?.serviceName ?? (s ? '' : t('stream.nothing'))
})
const recordingSubtitle = computed(() => (route.query.channel as string) ?? '')

// The recording's length (MM:SS from WebControl) for the time labels.
const lengthSeconds = computed(() => {
  const m = /^(\d+):(\d{2})$/.exec(String(route.query.length ?? ''))
  return m ? Number(m[1]) * 60 + Number(m[2]) : 0
})
function mmss(s: number) {
  const m = Math.floor(s / 60)
  return `${m}:${String(Math.floor(s % 60)).padStart(2, '0')}`
}
const positionText = computed(() => (lengthSeconds.value ? mmss(position.value * lengthSeconds.value) : `${Math.round(position.value * 100)} %`))
const lengthText = computed(() => (lengthSeconds.value ? mmss(lengthSeconds.value) : ''))

const errorText = computed(() => {
  const e = error.value
  if (e instanceof ApiError) {
    if (e.code === 'stream_no_tuner') return t('stream.error.noTuner')
    if (e.code === 'stream_busy') return t('stream.error.busy')
    if (e.code === 'not_found') return t('stream.error.notFound')
    if (e.status === 0) return t('stream.error.network')
  }
  return t('stream.error.generic')
})
const errorHint = computed(() => {
  const e = error.value
  return e instanceof ApiError && e.code === 'stream_no_tuner' ? t('stream.error.noTunerHint') : ''
})

let statsTimer = 0
function start(src: Source, audioIndex?: number) {
  player?.stop()
  if (!canvas.value) return
  source.value = src
  error.value = null
  tracks.value = []
  if (src.kind === 'recording') position.value = src.offset ?? 0
  const p = new TsPlayer(canvas.value, {
    state: (s, err) => {
      if (player !== p) return
      state.value = s
      if (err) error.value = err
    },
    tracks: (list, cur) => {
      if (player !== p) return
      tracks.value = list
      track.value = cur
    },
    progress: (f, seconds) => {
      if (player !== p) return
      // With the length known, by time played (the bytes read run ahead
      // by the buffer); else by bytes.
      const start = src.kind === 'recording' ? (src.offset ?? 0) : 0
      position.value = lengthSeconds.value ? Math.min(1, start + seconds / lengthSeconds.value) : f
    },
  })
  player = p
  p.setVolume(volume.value)
  p.setMuted(muted.value)
  void p.play(src, audioIndex)
}

function stop() {
  player?.stop()
  player = null
  source.value = null
  state.value = 'idle'
  const ctx = canvas.value?.getContext('2d')
  if (canvas.value && ctx) ctx.clearRect(0, 0, canvas.value.width, canvas.value.height)
  void router.replace({ query: {} })
}

function restart() {
  if (source.value) start(source.value, track.value >= 0 ? track.value : undefined)
}

function watch_(ref_: string) {
  void router.replace({ query: { ref: ref_ } })
}

function seek(f: number) {
  const s = source.value
  if (s?.kind === 'recording') start({ ...s, offset: f }, track.value >= 0 ? track.value : undefined)
}

function togglePause() {
  if (state.value === 'paused') player?.resume()
  else if (state.value === 'playing') player?.pause()
}
function setMuted(m: boolean) {
  muted.value = m
  player?.setMuted(m)
}
function setVolume(v: number) {
  volume.value = v
  if (v > 0 && muted.value) setMuted(false)
  player?.setVolume(v)
  store(VOLUME_KEY, String(v))
}
function selectTrack(index: number) {
  track.value = index
  player?.selectAudio(index)
}
function trackLabel(tr: AudioTrack, i: number) {
  const lang = tr.language ? languageName(tr.language) : t('stream.track', { n: i + 1 })
  const kind = tr.codec === 'ac3' || tr.codec === 'eac3' ? 'Dolby' : tr.codec.toUpperCase().replace('_LATM', '')
  return `${lang} (${kind})`
}
function languageName(code: string) {
  try {
    return new Intl.DisplayNames([locale.value], { type: 'language' }).of(code) ?? code
  } catch {
    return code
  }
}

async function toggleFullscreen() {
  try {
    if (document.fullscreenElement) await document.exitFullscreen()
    else await frame.value?.requestFullscreen()
  } catch {
    /* not allowed here */
  }
}
function onFullscreen() {
  fullscreen.value = document.fullscreenElement === frame.value
}

function onKey(e: KeyboardEvent) {
  if (e.altKey || e.ctrlKey || e.metaKey) return
  if (e.key === ' ' || e.key === 'k') {
    e.preventDefault()
    togglePause()
  } else if (e.key === 'm') {
    setMuted(!muted.value)
  } else if (e.key === 'f') {
    void toggleFullscreen()
  }
}

// ?at= starts a recording part-way in (a share of it, 0–1) — MediaHub's ad
// breaks link here to check a boundary.
function startAt(): number | undefined {
  const at = Number(route.query.at)
  return Number.isFinite(at) && at > 0 && at < 1 ? at : undefined
}

// The route says what plays: ?ref= or ?recording=.
watch(
  () => [route.query.ref, route.query.recording],
  ([refQ, rec]) => {
    if (!supported) return
    if (typeof rec === 'string' && rec) {
      if (source.value?.kind !== 'recording' || source.value.path !== rec) start({ kind: 'recording', path: rec, offset: startAt() })
    } else if (typeof refQ === 'string' && refQ) {
      if (source.value?.kind !== 'live' || source.value.ref !== refQ) start({ kind: 'live', ref: refQ })
    }
  },
)

let tick = 0
onMounted(async () => {
  document.addEventListener('fullscreenchange', onFullscreen)
  tick = window.setInterval(() => {
    nowSec.value = Date.now() / 1000
    // Now/next moves on: refresh when the current programme ended.
    const ended = channels.value.some((c) => c.now && c.now.start + c.now.duration < nowSec.value)
    if (ended && !channelsLoading.value) void loadChannels()
  }, 30_000)
  statsTimer = window.setInterval(() => {
    if (player) Object.assign(stats, player.stats)
  }, 1000)
  if (!supported) return
  const rec = route.query.recording
  const refQ = route.query.ref
  if (typeof rec === 'string' && rec) start({ kind: 'recording', path: rec, offset: startAt() })
  else {
    await loadBouquets()
    if (typeof refQ === 'string' && refQ) start({ kind: 'live', ref: refQ })
  }
})
onBeforeUnmount(() => {
  player?.stop()
  player = null
  document.removeEventListener('fullscreenchange', onFullscreen)
  clearInterval(tick)
  clearInterval(statsTimer)
})
</script>
