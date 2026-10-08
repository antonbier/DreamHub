<!--
  A visualization for the radio, in the spirit of Winamp's: a spectrum of
  bars with peaks that drop slowly, or an oscilloscope line. It reads the
  player's AnalyserNode and changes nothing about the sound. Purely
  decorative (aria-hidden); the mode switch is a real button. Off by
  default for people who asked their system for less motion, and it stops
  drawing while the page isn't visible.
-->
<template>
  <div class="flex items-stretch gap-2">
    <canvas ref="canvas" class="h-16 min-w-0 flex-1 rounded bg-field" aria-hidden="true" />
    <button
      type="button"
      class="shrink-0 rounded border border-line px-2 text-[12px] text-muted hover:bg-field hover:text-text"
      :title="t('radio.visual.switch')"
      @click="next"
    >
      {{ t(`radio.visual.${mode}`) }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ analyser: AnalyserNode | null }>()
const { t } = useI18n()

type Mode = 'bars' | 'scope' | 'off'
const MODES: Mode[] = ['bars', 'scope', 'off']
const KEY = 'dreamhub.radioVisual'
const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
function stored(): Mode {
  try {
    const v = localStorage.getItem(KEY) as Mode | null
    if (v && MODES.includes(v)) return v
  } catch {
    /* not remembered */
  }
  return reduced ? 'off' : 'bars'
}
const mode = ref<Mode>(stored())
function next() {
  mode.value = MODES[(MODES.indexOf(mode.value) + 1) % MODES.length]
  try {
    localStorage.setItem(KEY, mode.value)
  } catch {
    /* not remembered */
  }
}

const canvas = ref<HTMLCanvasElement | null>(null)
const BARS = 24
const peaks = new Float32Array(BARS)
let raf = 0

function color(name: string) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

function draw() {
  raf = requestAnimationFrame(draw)
  const c = canvas.value
  const a = props.analyser
  if (!c || document.hidden) return
  const dpr = window.devicePixelRatio || 1
  const w = Math.round(c.clientWidth * dpr)
  const h = Math.round(c.clientHeight * dpr)
  if (c.width !== w || c.height !== h) {
    c.width = w
    c.height = h
  }
  const g = c.getContext('2d')
  if (!g) return
  g.clearRect(0, 0, w, h)
  if (!a || mode.value === 'off') return

  if (mode.value === 'scope') {
    const data = new Uint8Array(a.fftSize)
    a.getByteTimeDomainData(data)
    g.strokeStyle = color('--dh-ok')
    g.lineWidth = 2 * dpr
    g.beginPath()
    for (let i = 0; i < data.length; i++) {
      const x = (i / (data.length - 1)) * w
      const y = (data[i] / 255) * h
      if (i === 0) g.moveTo(x, y)
      else g.lineTo(x, y)
    }
    g.stroke()
    return
  }

  // Bars: frequency bands on a log scale (bass on the left, like Winamp),
  // green at the bottom to red at the top, a peak cap each.
  const data = new Uint8Array(a.frequencyBinCount)
  a.getByteFrequencyData(data)
  const grad = g.createLinearGradient(0, h, 0, 0)
  grad.addColorStop(0, color('--dh-ok'))
  grad.addColorStop(0.6, color('--dh-warn'))
  grad.addColorStop(1, color('--dh-onair'))
  const gap = 2 * dpr
  const bw = (w - gap * (BARS - 1)) / BARS
  const minF = 2
  const maxF = data.length * 0.75
  for (let i = 0; i < BARS; i++) {
    const from = Math.floor(minF * Math.pow(maxF / minF, i / BARS))
    const to = Math.max(from + 1, Math.floor(minF * Math.pow(maxF / minF, (i + 1) / BARS)))
    let v = 0
    for (let k = from; k < to; k++) v = Math.max(v, data[k])
    const level = v / 255
    peaks[i] = Math.max(level, peaks[i] - 0.012)
    const x = i * (bw + gap)
    g.fillStyle = grad
    g.fillRect(x, h - level * h, bw, level * h)
    g.fillStyle = color('--dh-text')
    g.fillRect(x, h - peaks[i] * h - 2 * dpr, bw, 2 * dpr)
  }
}

onMounted(() => {
  raf = requestAnimationFrame(draw)
})
onBeforeUnmount(() => cancelAnimationFrame(raf))
watch(
  () => props.analyser,
  () => peaks.fill(0),
)
</script>
