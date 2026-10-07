// StreamHub's player: plays the box's MPEG-TS — live or a recording — in
// the browser, with nothing converted on the box (backend/internal/hubs/
// stream.go). The spike behind it, with measurements: spikes/streamhub-player.
//
// - libav.js (FFmpeg in WebAssembly, an own build with the DVB codecs:
//   public/libav/) demuxes the stream and decodes MPEG-2 video (SD
//   channels, deinterlaced here) and MP2/AC3/E-AC3/AAC audio.
// - H.264 (HD channels) goes to WebCodecs — the device's own hardware
//   decoder — when the browser has it, else to libav.js too.
// - Web Audio is the clock: audio is scheduled at its timestamps, and each
//   animation frame draws the video frame due now on a canvas.
//
// Demuxing, audio and video decoding run in three workers (one libav.js
// instance each), so a slow video decoder never starves the audio.

import { authHeaders } from '@/api/authState'
import { ApiError } from '@/api/client'

// libav.js ships no types for a custom build.
type LibAVInstance = any
type LibAVModule = any

const LIBAV_VERSION = '6.10.9.0'
let modulePromise: Promise<LibAVModule> | null = null

function loadLibAV(): Promise<LibAVModule> {
  if (!modulePromise) {
    const url = `/libav/libav-${LIBAV_VERSION}-dreamhub.mjs`
    modulePromise = import(/* @vite-ignore */ url).then((m) => m.default ?? m)
  }
  return modulePromise
}

export type Source =
  | { kind: 'live'; ref: string }
  // offset: where to start, 0–1 of the file (seeking restarts from there).
  | { kind: 'recording'; path: string; offset?: number }

export interface AudioTrack {
  index: number
  codec: string
  language: string
}

export type PlayerState = 'loading' | 'playing' | 'paused' | 'ended' | 'error'

export interface PlayerStats {
  video: string
  audio: string
  path: string // how video is decoded, for the info overlay
  width: number
  height: number
  shown: number
  dropped: number
  audioUnderruns: number
  avOffsetMs: number
  buffered: number // video frames waiting
  kbps: number
}

export interface PlayerEvents {
  state?: (s: PlayerState, error?: ApiError | Error) => void
  tracks?: (tracks: AudioTrack[], current: number) => void
  // Recordings: how far reading got (0–1 of the file, ahead of the
  // picture by the buffer) and the seconds played since the start point.
  progress?: (fraction: number, seconds: number) => void
}

const AUDIO_PREF = ['mp2', 'aac', 'aac_latm', 'ac3', 'eac3']
const MAX_QUEUE = 90 // decoded video frames waiting to be shown
const MAX_AUDIO_AHEAD = 3 // seconds of audio scheduled ahead
const START_DELAY = 0.4 // seconds of audio buffered before the clock starts

export class TsPlayer {
  readonly stats: PlayerStats = {
    video: '', audio: '', path: '', width: 0, height: 0, shown: 0, dropped: 0,
    audioUnderruns: 0, avOffsetMs: 0, buffered: 0, kbps: 0,
  }

  private canvas: HTMLCanvasElement
  private events: PlayerEvents
  private abort = new AbortController()
  private instances: LibAVInstance[] = []
  private ac: AudioContext | null = null
  private gain: GainNode | null = null
  private queue: VideoFrame[] = []
  private wc: VideoDecoder | null = null
  private raf = 0
  private stopped = false
  private volume = 1
  private muted = false
  private audioIndex: number | null = null
  private audioSwitch: number | null = null

  // Recordings: byte position and size, for progress and seeking.
  private size = 0
  private startByte = 0
  private bytes = 0

  constructor(canvas: HTMLCanvasElement, events: PlayerEvents = {}) {
    this.canvas = canvas
    this.events = events
  }

  /** Starts playing src; the returned promise settles once it plays or fails. */
  async play(src: Source, audioIndex?: number): Promise<void> {
    this.audioIndex = audioIndex ?? null
    this.events.state?.('loading')
    try {
      await this.run(src)
    } catch (err) {
      if (this.stopped) return
      this.events.state?.('error', err instanceof Error ? err : new Error(String(err)))
      this.stop()
    }
  }

  stop() {
    if (this.stopped) return
    this.stopped = true
    this.abort.abort()
    cancelAnimationFrame(this.raf)
    for (const f of this.queue) f.close()
    this.queue = []
    try {
      if (this.wc && this.wc.state !== 'closed') this.wc.close()
    } catch {
      /* already closed */
    }
    for (const i of this.instances) {
      try {
        i.terminate()
      } catch {
        /* gone */
      }
    }
    this.instances = []
    void this.ac?.close().catch(() => {})
  }

  pause() {
    void this.ac?.suspend().then(() => this.events.state?.('paused'))
  }

  resume() {
    void this.ac?.resume().then(() => this.events.state?.('playing'))
  }

  setVolume(v: number) {
    this.volume = v
    if (this.gain) this.gain.gain.value = this.muted ? 0 : v
  }

  setMuted(m: boolean) {
    this.muted = m
    if (this.gain) this.gain.gain.value = m ? 0 : this.volume
  }

  /** Switches the audio track: decoded from the next packet on. */
  selectAudio(index: number) {
    this.audioSwitch = index
  }

  private async fetchStream(src: Source): Promise<ReadableStreamDefaultReader<Uint8Array>> {
    const headers: Record<string, string> = {}
    Object.assign(headers, authHeaders())
    let url: string
    if (src.kind === 'live') {
      url = `/api/v1/stream/live?ref=${encodeURIComponent(src.ref)}`
    } else {
      url = `/api/v1/stream/recording?path=${encodeURIComponent(src.path)}`
      if (src.offset && src.offset > 0) {
        // The size first (HEAD), then a range aligned to TS packets.
        const head = await fetch(url, { method: 'HEAD', headers, signal: this.abort.signal })
        const size = Number(head.headers.get('Content-Length') ?? 0)
        if (size > 0) {
          this.startByte = Math.floor((size * Math.min(src.offset, 0.999)) / 188) * 188
          headers['Range'] = `bytes=${this.startByte}-`
        }
      }
    }
    const res = await fetch(url, { headers, signal: this.abort.signal, cache: 'no-store' })
    if (!res.ok || !res.body) {
      let code = 'unknown'
      let message = res.statusText
      try {
        const env = ((await res.json()) as { error?: { code?: string; message?: string } }).error
        code = env?.code ?? code
        message = env?.message ?? message
      } catch {
        /* not JSON */
      }
      throw new ApiError(res.status, code, message)
    }
    const range = res.headers.get('Content-Range') // "bytes 500-999/1000"
    this.size = range ? Number(range.split('/')[1]) : Number(res.headers.get('Content-Length') ?? 0)
    return res.body.getReader()
  }

  private async run(src: Source) {
    const reader = await this.fetchStream(src)
    const LibAV = await loadLibAV()
    const base = '/libav'
    const [demux, adec, vdec] = await Promise.all([LibAV.LibAV({ base }), LibAV.LibAV({ base }), LibAV.LibAV({ base })])
    this.instances = [demux, adec, vdec]
    if (this.stopped) return this.stop()

    let rateBytes = 0
    let rateSince = performance.now()
    await demux.mkreaderdev('in.ts')
    demux.onread = async (name: string) => {
      let chunk: ReadableStreamReadResult<Uint8Array>
      try {
        chunk = await reader.read()
      } catch {
        chunk = { done: true, value: undefined }
      }
      if (chunk.value) {
        this.bytes += chunk.value.length
        rateBytes += chunk.value.length
        const now = performance.now()
        if (now - rateSince > 2000) {
          this.stats.kbps = Math.round((rateBytes * 8) / (now - rateSince))
          rateBytes = 0
          rateSince = now
        }
      }
      if (!this.stopped) await demux.ff_reader_dev_send(name, chunk.done ? null : chunk.value)
    }

    const [fmt, streams] = await demux.ff_init_demuxer_file('in.ts', 'mpegts')
    const pkt = await demux.av_packet_alloc()
    const vStream = streams.find((s: any) => s.codec_type === LibAV.AVMEDIA_TYPE_VIDEO)
    const tracks: AudioTrack[] = []
    for (const s of streams) {
      if (s.codec_type !== LibAV.AVMEDIA_TYPE_AUDIO) continue
      const codec = await demux.avcodec_get_name(s.codec_id)
      tracks.push({ index: s.index, codec, language: s.metadata?.language ?? '' })
    }
    if (this.audioIndex === null || !tracks.some((t) => t.index === this.audioIndex)) {
      this.audioIndex = null
      for (const want of AUDIO_PREF) {
        const t = tracks.find((t) => t.codec === want)
        if (t) {
          this.audioIndex = t.index
          break
        }
      }
    }
    this.events.tracks?.(tracks, this.audioIndex ?? -1)

    // --- audio: the clock ----------------------------------------------
    const ac = new AudioContext({ latencyHint: 'playback' })
    this.ac = ac
    this.gain = ac.createGain()
    this.gain.gain.value = this.muted ? 0 : this.volume
    this.gain.connect(ac.destination)
    await ac.resume().catch(() => {})
    let clock: { pts: number; at: number } | null = null
    let nextAt = 0
    const mediaNow = () => (clock ? clock.pts + (ac.currentTime - clock.at) : null)

    // In a holder: assigned from closures, read in the loop below.
    const dec: { audio: { index: number; c: number; p: number; f: number; tb: number } | null } = { audio: null }
    const openAudio = async (index: number) => {
      if (dec.audio) await adec.ff_free_decoder(dec.audio.c, dec.audio.p, dec.audio.f)
      dec.audio = null
      const s = streams.find((x: any) => x.index === index)
      if (!s) return
      const cp = await demux.ff_copyout_codecpar(s.codecpar)
      const codec = tracks.find((t) => t.index === index)!.codec
      const [, c, p, f] = await adec.ff_init_decoder(codec, { codecpar: cp, time_base: [s.time_base_num, s.time_base_den] })
      dec.audio = { index, c, p, f, tb: s.time_base_num / s.time_base_den }
      this.stats.audio = codec
    }
    if (this.audioIndex !== null) await openAudio(this.audioIndex)

    const decodeAudio = async (packets: any[]) => {
      const audio = dec.audio
      if (!audio) return
      const frames = await adec.ff_decode_multi(audio.c, audio.p, audio.f, packets, { ignoreErrors: true })
      for (const fr of frames) {
        const pts = LibAV.i64tof64(fr.pts, fr.ptshi) * audio.tb
        const [L, R] = toStereo(fr, LibAV)
        if (!L.length) continue
        if (!clock) {
          clock = { pts, at: ac.currentTime + START_DELAY }
          this.events.state?.('playing')
        }
        const at = clock.at + (pts - clock.pts)
        if (at < ac.currentTime + 0.02) {
          this.stats.audioUnderruns++
          continue
        }
        const buf = ac.createBuffer(2, L.length, fr.sample_rate)
        buf.copyToChannel(L, 0)
        buf.copyToChannel(R, 1)
        const node = ac.createBufferSource()
        node.buffer = buf
        node.connect(this.gain!)
        node.start(at)
        nextAt = at + buf.duration
      }
    }

    // --- video ---------------------------------------------------------
    let sar = 1
    let decodeVideo: (packets: any[]) => Promise<void> = async () => {}
    if (vStream) {
      const vtb = vStream.time_base_num / vStream.time_base_den
      const codec = await demux.avcodec_get_name(vStream.codec_id)
      this.stats.video = codec
      const cp = await demux.ff_copyout_codecpar(vStream.codecpar)
      const [, c, p, f] = await vdec.ff_init_decoder(codec, { codecpar: cp, time_base: [vStream.time_base_num, vStream.time_base_den] })
      // SD broadcasts are interlaced: a linear blend takes the combing out
      // (blend(), below). FFmpeg's yadif crashed the page in this
      // single-threaded WebAssembly build.
      const deinterlace = codec === 'mpeg2video'
      this.stats.path = `${codec} → WebAssembly${deinterlace ? ' + deinterlace' : ''}`

      const push = (vf: VideoFrame) => {
        if (this.stopped) return vf.close()
        this.queue.push(vf)
      }
      const software = async (packets: any[]) => {
        const frames = await vdec.ff_decode_multi(c, p, f, packets, { ignoreErrors: true, copyoutFrame: 'video' })
        for (const fr of frames) {
          if (fr.sample_aspect_ratio?.[0] > 0) sar = fr.sample_aspect_ratio[0] / fr.sample_aspect_ratio[1]
          const pts = LibAV.i64tof64(fr.pts, fr.ptshi) * vtb
          if (fr.format !== LibAV.AV_PIX_FMT_YUV420P && fr.format !== LibAV.AV_PIX_FMT_YUVJ420P) continue
          if (deinterlace) blend(fr.data, fr.layout, fr.width, fr.height)
          try {
            push(new VideoFrame(fr.data, { format: 'I420', codedWidth: fr.width, codedHeight: fr.height, timestamp: Math.round(pts * 1e6), layout: fr.layout }))
          } catch {
            /* a frame WebCodecs won't take: skip it */
          }
        }
      }
      decodeVideo = software

      if (codec === 'h264' && 'VideoDecoder' in window) {
        let configured = false
        let failed = false
        decodeVideo = async (packets: any[]) => {
          if (failed) return software(packets)
          for (let i = 0; i < packets.length; i++) {
            const pk = packets[i]
            const key = (pk.flags & LibAV.AV_PKT_FLAG_KEY) !== 0
            const pts = LibAV.i64tof64(pk.pts, pk.ptshi) * vtb
            if (!configured) {
              const sps = key ? findSPS(pk.data) : null
              if (!sps) continue
              const config = await hardwareConfig('avc1.' + hex(sps[1]) + hex(sps[2]) + hex(sps[3]))
              if (!config) {
                failed = true
                this.stats.path = 'h264 → WebAssembly'
                return software(packets.slice(i))
              }
              this.wc = new VideoDecoder({
                output: push,
                // A decoder that fails after all hands over to libav.js.
                error: () => {
                  failed = true
                  this.stats.path = 'h264 → WebAssembly'
                },
              })
              this.wc.configure(config)
              configured = true
              this.stats.path = `h264 → WebCodecs (${config.codec})`
            }
            if (failed || this.wc!.state === 'closed') {
              failed = true
              return software(packets.slice(i))
            }
            this.wc!.decode(new EncodedVideoChunk({ type: key ? 'key' : 'delta', timestamp: Math.round(pts * 1e6), data: pk.data }))
          }
        }
      }
    }

    // --- render: the frame due at the audio clock ----------------------
    const ctx2d = this.canvas.getContext('2d')!
    const render = () => {
      if (this.stopped) return
      const now = mediaNow()
      if (now !== null) {
        this.queue.sort((a, b) => a.timestamp - b.timestamp)
        let show: VideoFrame | null = null
        while (this.queue.length && this.queue[0].timestamp / 1e6 <= now) {
          if (show) {
            show.close()
            this.stats.dropped++
          }
          show = this.queue.shift()!
        }
        if (show) {
          const w = Math.round(show.displayWidth * sar)
          const h = show.displayHeight
          if (this.canvas.width !== w || this.canvas.height !== h) {
            this.canvas.width = w
            this.canvas.height = h
          }
          ctx2d.drawImage(show, 0, 0, w, h)
          this.stats.width = w
          this.stats.height = h
          this.stats.avOffsetMs = Math.round((now - show.timestamp / 1e6) * 1000)
          show.close()
          this.stats.shown++
        }
        if (this.size > 0 && clock) this.events.progress?.((this.startByte + this.bytes) / this.size, Math.max(0, now - clock.pts))
      }
      this.stats.buffered = this.queue.length
      this.raf = requestAnimationFrame(render)
    }
    this.raf = requestAnimationFrame(render)

    // --- demux loop, with back-pressure --------------------------------
    for (;;) {
      while (!this.stopped && (this.queue.length > MAX_QUEUE || (clock && nextAt - ac.currentTime > MAX_AUDIO_AHEAD))) {
        await sleep(20)
      }
      if (this.stopped) return
      if (this.audioSwitch !== null) {
        this.audioIndex = this.audioSwitch
        this.audioSwitch = null
        await openAudio(this.audioIndex)
        this.events.tracks?.(tracks, this.audioIndex)
      }
      const [res, byStream] = await demux.ff_read_frame_multi(fmt, pkt, { limit: 256 * 1024 })
      if (this.stopped) return
      const work: Promise<void>[] = []
      if (vStream && byStream[vStream.index]) work.push(decodeVideo(byStream[vStream.index]))
      if (dec.audio && byStream[dec.audio.index]) work.push(decodeAudio(byStream[dec.audio.index]))
      await Promise.all(work)
      if (res === LibAV.AVERROR_EOF) break
      if (res !== 0 && res !== -LibAV.EAGAIN) throw new Error(`demux: ${res}`)
    }
    // The end: let what's buffered play out.
    while (!this.stopped && (this.queue.length || (clock && ac.currentTime < nextAt))) await sleep(100)
    if (!this.stopped) this.events.state?.('ended')
  }
}

type Samples = Float32Array<ArrayBuffer>

function toStereo(fr: any, LibAV: LibAVModule): [Samples, Samples] {
  const n: number = fr.nb_samples
  const planar = Array.isArray(fr.data)
  const ch: number = fr.channels ?? (planar ? fr.data.length : 2)
  const plane = (i: number): Samples => {
    if (planar) {
      const d = fr.data[i]
      return d instanceof Float32Array ? (d as Samples) : Float32Array.from(d as Int16Array, (v) => v / 32768)
    }
    const out = new Float32Array(n)
    const float = fr.format === LibAV.AV_SAMPLE_FMT_FLT
    for (let k = 0; k < n; k++) out[k] = float ? fr.data[k * ch + i] : fr.data[k * ch + i] / 32768
    return out
  }
  if (ch <= 1) {
    const m = plane(0)
    return [m, m]
  }
  if (ch === 2) return [plane(0), plane(1)]
  // 5.1 (L R C LFE Ls Rs): a plain downmix.
  const [l, r, c] = [plane(0), plane(1), plane(2)]
  const ls = ch > 4 ? plane(4) : null
  const rs = ch > 5 ? plane(5) : null
  const L = new Float32Array(n)
  const R = new Float32Array(n)
  for (let k = 0; k < n; k++) {
    L[k] = 0.55 * l[k] + 0.4 * c[k] + 0.3 * (ls?.[k] ?? 0)
    R[k] = 0.55 * r[k] + 0.4 * c[k] + 0.3 * (rs?.[k] ?? 0)
  }
  return [L, R]
}

// blend deinterlaces a YUV 4:2:0 frame in place: each line becomes
// ¼ the line above + ½ itself + ¼ the line below, which merges the two
// fields (a "linear blend"). Cheap — about a millisecond for SD — at the
// price of a little vertical softness.
function blend(data: Uint8Array, layout: { offset: number; stride: number }[], width: number, height: number) {
  const planes: [number, number][] = [[width, height], [width >> 1, height >> 1], [width >> 1, height >> 1]]
  let prev = new Uint8Array(width)
  let cur = new Uint8Array(width)
  for (let i = 0; i < 3 && i < layout.length; i++) {
    const [w, h] = planes[i]
    const { offset, stride } = layout[i]
    prev.set(data.subarray(offset, offset + w))
    for (let y = 1; y < h - 1; y++) {
      const row = offset + y * stride
      cur.set(data.subarray(row, row + w))
      const below = row + stride
      for (let x = 0; x < w; x++) data[row + x] = (prev[x] + 2 * cur[x] + data[below + x] + 2) >> 2
      const t = prev
      prev = cur
      cur = t
    }
  }
}

function findSPS(d: Uint8Array): Uint8Array | null {
  for (let i = 0; i + 4 < d.length; i++) {
    if (d[i] === 0 && d[i + 1] === 0 && d[i + 2] === 1 && (d[i + 3] & 0x1f) === 7) return d.subarray(i + 3, i + 8)
  }
  return null
}

async function hardwareConfig(codec: string): Promise<VideoDecoderConfig | null> {
  for (const hardwareAcceleration of ['prefer-hardware', 'no-preference'] as const) {
    const config: VideoDecoderConfig = { codec, optimizeForLatency: true, hardwareAcceleration }
    const support = await VideoDecoder.isConfigSupported(config).catch(() => ({ supported: false }))
    if (support.supported) return config
  }
  return null
}

const hex = (b: number) => b.toString(16).padStart(2, '0')
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))
