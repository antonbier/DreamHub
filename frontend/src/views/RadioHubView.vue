<!--
  RadioHub (/radio): the box's radio stations with what's on now and next,
  the station the box is playing on top, and the radio recordings coming
  up. "Play on the box" is ControlHub's zap — a live input like the remote,
  not a plan. Listening in the browser uses StreamHub's player (the box's
  stream, decoded here — radio is mostly MP2, which browsers don't play
  themselves), offered while StreamHub is there. A programme opens the shared ProgrammeDialog (no film
  details to look up: plain) with the EPG text and Record, through
  TimerHub's plan/apply like everywhere else.

  The bouquet is remembered per browser; a search result
  (?bouquet=&station=) opens its bouquet and highlights the station. With
  many stations a filter field narrows the list by name. Progress bars
  move every 30 s, and the list reloads once a programme ends.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="radio" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ hubTitle('radio', 'hub.radio') }}</h1>
    </div>
    <p class="mt-1 text-[13.5px] text-muted">{{ t('radio.intro') }}</p>

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
      <p v-if="bouquetsLoading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
      <p v-else-if="bouquetsFailed" class="mt-5 text-[14px] text-muted"><ErrorText /></p>
      <p v-else-if="bouquets.length === 0" class="mt-5 text-[14px] text-muted">{{ t('radio.noBouquets') }}</p>
      <template v-else>
        <!-- What the box plays right now, when it's one of these stations. -->
        <section v-if="playingStation" class="mt-5 flex items-center gap-4 rounded border border-line bg-tile p-4 below-sm:flex-wrap" aria-live="polite">
          <ChannelLogo source="radio" :service-reference="playingStation.serviceReference" :service-name="playingStation.serviceName" :height="36" />
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-1.5 text-[12.5px] text-onair">
              <span class="h-1.5 w-1.5 animate-pulse rounded-full bg-onair" />{{ t('radio.nowPlaying') }}
            </div>
            <div class="truncate text-[17px] font-semibold text-text">{{ playingStation.serviceName }}</div>
            <button
              v-if="playingStation.now"
              type="button"
              class="block max-w-full truncate text-start text-[14px] text-muted hover:text-accent"
              @click="select(playingStation.now)"
            >
              {{ playingStation.now.title }}
            </button>
          </div>
          <div v-if="playingStation.now" class="w-48 shrink-0 below-sm:w-full">
            <div class="h-1 overflow-hidden rounded-full bg-field">
              <div class="h-full bg-onair" :style="{ width: progress(playingStation.now) + '%' }" />
            </div>
            <div class="mt-1 text-end text-[12px] tabular-nums text-faint">{{ range(playingStation.now) }}</div>
          </div>
        </section>

        <!-- Listening in the browser (StreamHub's player, sound only). -->
        <section v-if="listening" class="mt-5 flex flex-wrap items-center gap-3 rounded border border-accent bg-tile p-3" :aria-label="t('radio.inBrowser')">
          <ChannelLogo source="radio" :service-reference="listening" :service-name="listeningStation?.serviceName ?? ''" :height="28" />
          <!-- On a phone the controls wrap below, so the name has the width. -->
          <div class="min-w-0 flex-1 below-sm:basis-[calc(100%-5rem)]" aria-live="polite">
            <div class="text-[12px] text-faint">{{ t('radio.inBrowser') }}</div>
            <div class="truncate text-[14.5px] font-semibold text-text">{{ listeningStation?.serviceName ?? listening }}</div>
            <div v-if="listenState === 'loading'" class="text-[12.5px] text-muted">{{ t('stream.loading') }}</div>
            <div v-else-if="listenState === 'error'" class="text-[12.5px] text-warn">{{ listenErrorText }}</div>
            <div v-else-if="listeningStation?.now" class="truncate text-[12.5px] text-muted">{{ listeningStation.now.title }}</div>
          </div>
          <input
            type="range"
            min="0"
            max="100"
            :value="Math.round(listenVolume * 100)"
            class="h-1 w-28 accent-[var(--dh-accent)]"
            :aria-label="t('stream.volume')"
            @input="setListenVolume(($event.target as HTMLInputElement).valueAsNumber / 100)"
          />
          <button
            v-if="listenState === 'playing' || listenState === 'paused'"
            type="button"
            class="rounded border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field"
            @click="togglePause"
          >
            {{ listenState === 'paused' ? t('radio.resume') : t('radio.pause') }}
          </button>
          <button type="button" class="rounded border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field" @click="stopListening">
            {{ t('stream.stop') }}
          </button>
          <RadioVisualizer v-if="listenState === 'playing' || listenState === 'paused'" class="w-full basis-full" :analyser="listenAnalyser" />
        </section>
        <!-- The sound's way out: a media element keeps playing with the
             phone's screen locked and carries the lock-screen controls. -->
        <audio ref="audioEl" class="hidden" />

        <div class="mt-5 flex flex-wrap items-center gap-3">
          <select
            v-if="bouquets.length > 1"
            v-model="selectedBouquet"
            class="rounded border border-line bg-field px-3 py-1.5 text-[13.5px] text-text"
            :aria-label="t('radio.bouquet')"
            @change="autoPicked = false"
          >
            <option v-for="b in bouquets" :key="b.serviceReference" :value="b.serviceReference">{{ b.serviceName || t('radio.allStations') }}</option>
          </select>
          <input
            v-if="stations.length > 8"
            v-model="query"
            type="search"
            :placeholder="t('radio.find')"
            :aria-label="t('radio.find')"
            class="w-60 max-w-full rounded border border-line bg-field px-2.5 py-1.5 text-[13.5px] text-text"
          />
        </div>

        <p v-if="stationsLoading && !stations.length" class="mt-4 text-[14px] text-muted">{{ t('common.loading') }}</p>
        <p v-else-if="stationsFailed" class="mt-4 text-[14px] text-muted"><ErrorText /></p>
        <p v-else-if="stations.length === 0" class="mt-4 text-[14px] text-muted">{{ t('radio.empty') }}</p>
        <p v-else-if="shown.length === 0" class="mt-4 text-[14px] text-muted">{{ t('radio.noMatch', { query }) }}</p>
        <ul v-else class="mt-3 flex flex-col">
          <li
            v-for="st in shown"
            :id="rowId(st.serviceReference)"
            :key="st.serviceReference"
            class="flex scroll-mt-20 items-center gap-3 rounded border-b border-line-soft px-1 py-2.5 transition-colors last:border-none below-sm:flex-wrap"
            :class="{ 'bg-tile-2': highlighted === st.serviceReference }"
          >
            <div class="flex w-44 shrink-0 items-center gap-2 below-sm:w-auto below-sm:min-w-0 below-sm:flex-1">
              <ChannelLogo source="radio" :service-reference="st.serviceReference" :service-name="st.serviceName" :height="22" />
              <span class="truncate text-[13.5px] font-semibold text-text">{{ st.serviceName }}</span>
            </div>
            <div class="min-w-0 flex-1 below-sm:order-last below-sm:basis-full">
              <template v-if="st.now">
                <button type="button" class="block max-w-full truncate text-start text-[14px] text-text hover:text-accent" @click="select(st.now)">
                  {{ st.now.title }}
                </button>
                <div class="mt-1 flex items-center gap-2">
                  <div
                    class="h-1 flex-1 overflow-hidden rounded-full bg-field"
                    role="progressbar"
                    :aria-valuenow="progress(st.now)"
                    aria-valuemin="0"
                    aria-valuemax="100"
                    :aria-label="t('guide.progress')"
                  >
                    <div class="h-full bg-onair" :style="{ width: progress(st.now) + '%' }" />
                  </div>
                  <span class="shrink-0 text-[12px] tabular-nums text-faint">{{ range(st.now) }}</span>
                </div>
              </template>
              <span v-else class="text-[13px] text-faint">{{ t('radio.noEpg') }}</span>
              <button v-if="st.next" type="button" class="mt-1 block max-w-full truncate text-start text-[12.5px] text-muted hover:text-accent" @click="select(st.next)">
                <span class="tabular-nums text-faint">{{ t('guide.then', { time: clock(st.next.start) }) }}</span> {{ st.next.title }}
              </button>
            </div>
            <span v-if="playing === st.serviceReference" class="flex shrink-0 items-center gap-1.5 px-2.5 py-1 text-[12.5px] text-onair">
              <span class="h-1.5 w-1.5 rounded-full bg-onair" />{{ t('radio.playing') }}
            </span>
            <button
              v-else
              type="button"
              class="shrink-0 rounded border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field disabled:opacity-60"
              :disabled="zapping === st.serviceReference"
              @click="play(st.serviceReference)"
            >
              {{ t('radio.play') }}
            </button>
            <button
              v-if="streamHubOn"
              type="button"
              class="flex shrink-0 items-center gap-1.5 rounded border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field"
              :class="{ 'border-accent text-accent': listening === st.serviceReference }"
              :aria-label="t('radio.listen', { station: st.serviceName })"
              :aria-pressed="listening === st.serviceReference"
              @click="listening === st.serviceReference ? stopListening() : listen(st.serviceReference)"
            >
              <Icon :name="listening === st.serviceReference ? 'stop' : 'volume-up'" :size="14" />{{ t('radio.listenHere') }}
            </button>
          </li>
        </ul>
        <p v-if="zapError" class="mt-2 text-[12.5px] text-warn" role="status">{{ zapError }}</p>
      </template>

      <!-- Radio recordings coming up (TimerHub manages all of them). -->
      <section class="mt-8 border-t border-line-soft pt-5">
        <div class="flex items-center gap-3">
          <h2 class="text-[15px] font-semibold text-text">{{ t('radio.timers.title') }}</h2>
          <RouterLink v-if="timerHubShown" to="/timers" class="text-[13px] text-accent hover:underline">{{ t('radio.timers.manage') }}</RouterLink>
        </div>
        <p v-if="timersFailed" class="mt-2 text-[13px] text-faint"><ErrorText /></p>
        <p v-else-if="timers.length === 0" class="mt-2 text-[13px] text-faint">{{ t('radio.timers.empty') }}</p>
        <ul v-else class="mt-2 flex flex-col">
          <li v-for="tm in timers" :key="tm.serviceReference + tm.begin" class="flex items-center gap-3 border-b border-line-soft py-2 last:border-none">
            <span class="w-40 shrink-0 whitespace-nowrap font-mono text-[12.5px] tabular-nums text-faint below-sm:w-28">{{ formatWhen(tm.begin, locale, timeZone) }}</span>
            <span class="min-w-0 flex-1">
              <span class="block truncate text-[14px] text-text">{{ tm.name }}</span>
              <span class="block truncate text-[12.5px] text-muted">
                {{ tm.serviceName }}<template v-if="tm.begin <= nowSec && nowSec < tm.end"> · <span class="text-onair">{{ t('radio.timers.running') }}</span></template>
              </span>
            </span>
          </li>
        </ul>
      </section>
    </template>

    <ProgrammeDialog
      v-if="selected"
      plain
      :title="selected.title"
      :service-name="selected.serviceName"
      :start="selected.start"
      :duration="selected.duration"
      :when="formatRange(selected.start, selected.duration)"
      :now="Date.now() / 1000"
      @close="closeSelected"
    >
      <section v-if="selected.description" class="mt-4 rounded border border-line-soft bg-bg p-3">
        <h3 class="text-[12.5px] text-faint">{{ t('guide.epgText') }}</h3>
        <p class="mt-1 text-[14px] text-text">{{ selected.description }}</p>
      </section>

      <template #actions>
        <p v-if="record.status === 'created'" class="flex items-center gap-1.5 text-[13.5px] text-ok" role="status">
          <span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('guide.timerCreated') }}
        </p>
        <PlanPreview
          v-else-if="record.plan"
          :plan="record.plan"
          :confirm-label="t('guide.confirmRecord')"
          :busy="record.status === 'applying'"
          :error="record.error"
          @confirm="confirmRecord"
          @cancel="record = { status: 'idle' }"
        >
          {{ t('guide.recordPreview', { name: selected.title, channel: selected.serviceName, when: recordWindow }) }}
        </PlanPreview>
        <template v-else>
          <div class="flex flex-wrap items-center gap-2">
            <button
              v-if="!selectedEnded"
              type="button"
              class="rounded bg-accent px-3.5 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-60"
              :disabled="record.status === 'loading' || !canRecord"
              :title="canRecord ? undefined : t('guide.timerHubUnavailable')"
              @click="previewRecord"
            >
              {{ t('guide.record') }}
            </button>
            <button
              v-if="selectedLive && playing !== selected.serviceReference"
              type="button"
              class="rounded border border-line px-3.5 py-1.5 text-[13.5px] text-text hover:bg-field disabled:opacity-60"
              :disabled="zapping === selected.serviceReference"
              @click="play(selected.serviceReference)"
            >
              {{ t('radio.play') }}
            </button>
          </div>
          <p v-if="record.error" class="mt-2 text-[12.5px] text-warn">{{ record.error }}</p>
          <p v-if="zapError" class="mt-2 text-[12.5px] text-warn">{{ zapError }}</p>
        </template>
      </template>
    </ProgrammeDialog>
  </div>
</template>

<script setup lang="ts">
import { useHubLabel } from '@/composables/useHubLabel'
import ErrorText from '@/components/ErrorText.vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRoute } from 'vue-router'
import { api, ApiError } from '@/api/client'
import Icon from '@/components/Icon.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import ProgrammeDialog from '@/components/programme/ProgrammeDialog.vue'
import ChannelLogo from '@/components/guide/ChannelLogo.vue'
import { usePlanError } from '@/composables/usePlanError'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { formatWhen } from '@/lib/format'
import { TsPlayer, type PlayerState } from '@/streaming/tsPlayer'
import RadioVisualizer from '@/components/radio/RadioVisualizer.vue'
import type { Bouquet, EPGEvent, NowNext, Plan, Timer } from '@/api/types'

const BOUQUET_KEY = 'dreamhub.radioBouquet'

const { t, locale } = useI18n()
const { hubTitle } = useHubLabel()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()
const { timeZone } = useDisplayTimeZone()
const { planErrorMessage } = usePlanError()
const route = useRoute()

const navItems = computed(() => navigation.navigation?.sections.flatMap((s) => s.items) ?? [])
const item = computed(() => navItems.value.find((i) => i.path === '/radio'))
// Recording needs TimerHub; RadioHub itself works without it.
const timerHub = computed(() => navItems.value.find((i) => i.id === 'timer'))
const canRecord = computed(() => timerHub.value?.status !== 'unavailable')
const timerHubShown = computed(() => !!timerHub.value && timerHub.value.status !== 'unavailable')

// --- time -------------------------------------------------------------------

const nowSec = ref(Date.now() / 1000)
function clock(s: number) {
  return new Intl.DateTimeFormat(locale.value, { timeStyle: 'short', timeZone: timeZone.value }).format(s * 1000)
}
const range = (e: EPGEvent) => `${clock(e.start)} – ${clock(e.start + e.duration)}`
function progress(e: EPGEvent) {
  return Math.round(Math.min(100, Math.max(0, ((nowSec.value - e.start) / e.duration) * 100)))
}
function formatRange(start: number, duration: number): string {
  const day = new Intl.DateTimeFormat(locale.value, { weekday: 'short', day: 'numeric', month: 'numeric', timeZone: timeZone.value }).format(start * 1000)
  return `${day}, ${clock(start)} – ${clock(start + duration)}`
}

// --- bouquets ---------------------------------------------------------------

const bouquets = ref<Bouquet[]>([])
const selectedBouquet = ref('')
// The list of every radio service (the backend's last, unnamed entry).
const allRef = ref('')
// True while the bouquet was chosen for the user, not by them: an empty
// one (radio favourites nobody filled — the reference box's) then gives
// way to all radio stations.
const autoPicked = ref(false)
const bouquetsLoading = ref(true)
const bouquetsFailed = ref(false)

function rememberedBouquet(): string | null {
  try {
    return localStorage.getItem(BOUQUET_KEY)
  } catch {
    return null
  }
}
watch(selectedBouquet, (b) => {
  try {
    if (b) localStorage.setItem(BOUQUET_KEY, b)
  } catch {
    // Not remembered; still switched.
  }
})

// --- stations ---------------------------------------------------------------

const stations = ref<NowNext[]>([])
const playing = ref('')
const stationsLoading = ref(false)
const stationsFailed = ref(false)
const query = ref('')

const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  return q ? stations.value.filter((s) => s.serviceName.toLowerCase().includes(q)) : stations.value
})
const playingStation = computed(() => stations.value.find((s) => s.serviceReference === playing.value) ?? null)

let token = 0
async function loadStations() {
  if (!selectedBouquet.value) return
  const mine = ++token
  stationsLoading.value = true
  try {
    const res = await api.radio.stations(selectedBouquet.value)
    if (mine !== token) return
    stations.value = res.stations
    playing.value = res.playing
    stationsFailed.value = false
    if (autoPicked.value && res.stations.length === 0 && allRef.value && selectedBouquet.value !== allRef.value) {
      autoPicked.value = false
      selectedBouquet.value = allRef.value
    }
  } catch {
    if (mine === token) stationsFailed.value = true
  } finally {
    if (mine === token) stationsLoading.value = false
  }
}
watch(selectedBouquet, () => {
  stations.value = []
  query.value = ''
  void loadStations()
})

// A search result: open its bouquet, then scroll to and mark the station.
const highlighted = ref('')
const rowId = (ref: string) => 'station-' + ref.replace(/[^0-9A-Za-z]/g, '_')
async function showStation(ref: string) {
  highlighted.value = ref
  await nextTick()
  document.getElementById(rowId(ref))?.scrollIntoView({ block: 'center' })
  window.setTimeout(() => {
    if (highlighted.value === ref) highlighted.value = ''
  }, 2500)
}
watch(
  () => [route.query.bouquet, route.query.station],
  async ([b, s]) => {
    if (typeof b === 'string' && bouquets.value.some((x) => x.serviceReference === b) && b !== selectedBouquet.value) {
      selectedBouquet.value = b
      await loadStations()
    }
    if (typeof s === 'string') void showStation(s)
  },
)

// --- radio timers -----------------------------------------------------------

const timers = ref<Timer[]>([])
const timersFailed = ref(false)
async function loadTimers() {
  try {
    timers.value = (await api.radio.timers()).timers
    timersFailed.value = false
  } catch {
    timersFailed.value = true
  }
}

onMounted(async () => {
  void loadTimers()
  try {
    const res = await api.radio.bouquets()
    bouquets.value = res.bouquets
    allRef.value = res.all
    const wanted = [route.query.bouquet, rememberedBouquet()].find((b) => typeof b === 'string' && res.bouquets.some((x) => x.serviceReference === b))
    autoPicked.value = !wanted
    selectedBouquet.value = (wanted as string | undefined) ?? res.bouquets[0]?.serviceReference ?? ''
  } catch {
    bouquetsFailed.value = true
  } finally {
    bouquetsLoading.value = false
  }
  if (typeof route.query.station === 'string') {
    await loadStations()
    void showStation(route.query.station)
  }
})

// Tick the bars; reload once any "now" has ended.
const ticker = setInterval(() => {
  nowSec.value = Date.now() / 1000
  if (stations.value.some((s) => s.now && s.now.start + s.now.duration <= nowSec.value)) void loadStations()
}, 30_000)
onBeforeUnmount(() => clearInterval(ticker))

// --- listen in the browser (StreamHub's player) ---------------------------
// The box's radio stream, unchanged, decoded here (MP2 mostly); offered
// while StreamHub is there, whose endpoint it uses. Needs a tuner like
// any stream: "same as the box" always works.

const streamHubOn = computed(() => {
  const s = navItems.value.find((i) => i.id === 'stream')
  return !!s && s.status !== 'unavailable'
})
const listening = ref('')
const listenState = ref<PlayerState | 'idle'>('idle')
const listenError = ref<Error | null>(null)
const listeningStation = computed(() => stations.value.find((s) => s.serviceReference === listening.value) ?? null)
const VOLUME_KEY = 'dreamhub.streamVolume'
const listenVolume = ref(readVolume())
function readVolume() {
  try {
    const v = Number(localStorage.getItem(VOLUME_KEY) ?? '1')
    return Number.isFinite(v) ? Math.min(1, Math.max(0, v)) : 1
  } catch {
    return 1
  }
}
let radioPlayer: TsPlayer | null = null
const audioEl = ref<HTMLAudioElement | null>(null)
const listenAnalyser = shallowRef<AnalyserNode | null>(null)
function listen(ref_: string) {
  radioPlayer?.stop()
  listening.value = ref_
  listenError.value = null
  listenAnalyser.value = null
  // Sound only: the player draws nowhere visible.
  const p = new TsPlayer(
    document.createElement('canvas'),
    {
      state: (st, err) => {
        if (radioPlayer !== p) return
        listenState.value = st
        if (err) listenError.value = err
        if (st === 'playing') listenAnalyser.value = p.analyser
      },
    },
    { mediaElement: audioEl.value ?? undefined },
  )
  radioPlayer = p
  p.setVolume(listenVolume.value)
  void p.play({ kind: 'live', ref: ref_ })
}
function stopListening() {
  radioPlayer?.stop()
  radioPlayer = null
  listening.value = ''
  listenState.value = 'idle'
  listenAnalyser.value = null
}
function togglePause() {
  if (listenState.value === 'paused') radioPlayer?.resume()
  else radioPlayer?.pause()
}

// --- the phone's lock screen and notification (Media Session) ----------------
// What's playing, its logo, and play/pause/stop/previous/next station —
// shown by the phone while the sound runs through audioEl.
const artwork = ref('')
watch(listening, async (ref_) => {
  artwork.value = ''
  const st = listeningStation.value
  if (!ref_ || !st) return
  try {
    const url = URL.createObjectURL(await api.radio.picon(ref_, st.serviceName))
    if (listening.value === ref_) artwork.value = url
  } catch {
    /* no logo: the phone shows its own */
  }
})
function neighbour(step: number) {
  const list = stations.value.filter((s) => !s.serviceReference.startsWith('1:64:')) // not the headings
  const i = list.findIndex((s) => s.serviceReference === listening.value)
  if (i < 0 || list.length < 2) return
  listen(list[(i + step + list.length) % list.length].serviceReference)
}
watchEffect(() => {
  if (!('mediaSession' in navigator)) return
  const ms = navigator.mediaSession
  const st = listeningStation.value
  if (!listening.value) {
    ms.metadata = null
    ms.playbackState = 'none'
    return
  }
  ms.metadata = new MediaMetadata({
    title: st?.now?.title || st?.serviceName || '',
    artist: st?.serviceName ?? '',
    album: t('hub.radio'),
    artwork: artwork.value ? [{ src: artwork.value, type: 'image/png' }] : [],
  })
  ms.playbackState = listenState.value === 'paused' ? 'paused' : 'playing'
})
onMounted(() => {
  if (!('mediaSession' in navigator)) return
  const ms = navigator.mediaSession
  const set = (action: MediaSessionAction, fn: () => void) => {
    try {
      ms.setActionHandler(action, fn)
    } catch {
      /* this browser doesn't offer it */
    }
  }
  set('play', () => radioPlayer?.resume())
  set('pause', () => radioPlayer?.pause())
  set('stop', stopListening)
  set('previoustrack', () => neighbour(-1))
  set('nexttrack', () => neighbour(1))
})
onBeforeUnmount(() => {
  if (!('mediaSession' in navigator)) return
  for (const a of ['play', 'pause', 'stop', 'previoustrack', 'nexttrack'] as MediaSessionAction[]) {
    try {
      navigator.mediaSession.setActionHandler(a, null)
    } catch {
      /* not offered */
    }
  }
  navigator.mediaSession.metadata = null
})
function setListenVolume(v: number) {
  listenVolume.value = v
  radioPlayer?.setVolume(v)
  try {
    localStorage.setItem(VOLUME_KEY, String(v))
  } catch {
    /* not remembered */
  }
}
const listenErrorText = computed(() => {
  const e = listenError.value
  if (e instanceof ApiError && e.code === 'stream_no_tuner') return t('radio.noTuner')
  if (e instanceof ApiError && e.code === 'stream_busy') return t('stream.error.busy')
  return t('stream.error.generic')
})
onBeforeUnmount(() => radioPlayer?.stop())

// --- play on the box (ControlHub's zap) ------------------------------------

const zapping = ref('')
const zapError = ref('')
async function play(ref: string) {
  zapping.value = ref
  zapError.value = ''
  try {
    await api.control.zap(ref)
    playing.value = ref
  } catch (err) {
    zapError.value = planErrorMessage(err)
  } finally {
    zapping.value = ''
  }
}

// --- the selected programme: details, record --------------------------------

const selected = ref<EPGEvent | null>(null)
type RecordState = { status: 'idle' | 'loading' | 'previewing' | 'applying' | 'created'; plan?: Plan; error?: string }
const record = ref<RecordState>({ status: 'idle' })

const selectedEnded = computed(() => (selected.value ? selected.value.start + selected.value.duration <= nowSec.value : false))
const selectedLive = computed(() => {
  const e = selected.value
  return !!e && e.start <= nowSec.value && nowSec.value < e.start + e.duration
})

// The recording window as planned (margins included), from the plan itself.
const recordWindow = computed(() => {
  const after = record.value.plan?.changes[0]?.after as { recordBegin?: number; recordEnd?: number } | undefined
  return after?.recordBegin && after?.recordEnd ? `${clock(after.recordBegin)} – ${clock(after.recordEnd)}` : ''
})

function select(e: EPGEvent) {
  selected.value = e
  record.value = { status: 'idle' }
  zapError.value = ''
}
function closeSelected() {
  selected.value = null
  record.value = { status: 'idle' }
}

async function previewRecord() {
  const e = selected.value
  if (!e) return
  record.value = { status: 'loading' }
  try {
    const res = await api.timerPlan({
      serviceReference: e.serviceReference,
      serviceName: e.serviceName,
      name: e.title,
      description: e.description,
      begin: e.start,
      end: e.start + e.duration,
      eventId: e.id,
    })
    if (selected.value === e) record.value = { status: 'previewing', plan: res.plan }
  } catch (err) {
    if (selected.value === e) record.value = { status: 'idle', error: planErrorMessage(err) }
  }
}

async function confirmRecord() {
  const plan = record.value.plan
  if (!plan) return
  record.value = { status: 'applying', plan }
  try {
    await api.plans.apply(plan.id)
    record.value = { status: 'created' }
    void loadTimers()
  } catch (err) {
    // The plan is used up either way; "Record" starts a fresh preview.
    record.value = { status: 'idle', error: planErrorMessage(err) }
  }
}
</script>
