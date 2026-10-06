// DreamHub StreamHub spike: play a DVB MPEG-TS (live or recorded) in the
// browser with no transcoding on the box. libav.js (WebAssembly) demuxes;
// H.264 goes to WebCodecs (the device's hardware) when the browser has it,
// else to libav.js; MPEG-2 video and MP2/AC3/AAC audio are decoded by
// libav.js. Web Audio is the clock; video frames follow it.
import LibAVMod from './libav/libav-6.10.9.0-dreamhub.mjs';

const LibAV = LibAVMod;
const base = new URL('./libav/', import.meta.url).href.replace(/\/$/, '');

export const stats = {
  video: '', audio: '', path: '', decodedFrames: 0, shownFrames: 0, droppedFrames: 0,
  audioFrames: 0, audioUnderruns: 0, videoDecodeMs: 0, videoDecodeCalls: 0,
  queue: 0, avOffsetMs: 0, startedAt: 0, mediaSeconds: 0, bytes: 0, errors: [],
};

const tb = (s) => s.time_base_num / s.time_base_den;

export async function play(url, canvas, opts = {}) {
  const [demux, adec, vdec] = await Promise.all([
    LibAV.LibAV({ base }), LibAV.LibAV({ base }), LibAV.LibAV({ base }),
  ]);
  const ctx2d = canvas.getContext('2d');

  // --- input: fetch, streamed into a reader device ---------------------
  const resp = await fetch(url);
  const reader = resp.body.getReader();
  await demux.mkreaderdev('in.ts');
  demux.onread = async (name) => {
    const { done, value } = await reader.read();
    if (value) stats.bytes += value.length;
    await demux.ff_reader_dev_send(name, done ? null : value);
  };

  const [fmt, streams] = await demux.ff_init_demuxer_file('in.ts', 'mpegts');
  const pkt = await demux.av_packet_alloc();
  const name = async (s) => demux.avcodec_get_name(s.codec_id);
  const vStream = streams.find((s) => s.codec_type === LibAV.AVMEDIA_TYPE_VIDEO);
  let aStream = null;
  for (const want of opts.audioPref ?? ['mp2', 'aac', 'aac_latm', 'ac3', 'eac3']) {
    for (const s of streams) {
      if (s.codec_type === LibAV.AVMEDIA_TYPE_AUDIO && (await name(s)) === want) { aStream = s; break; }
    }
    if (aStream) break;
  }
  stats.video = vStream ? await name(vStream) : 'none';
  stats.audio = aStream ? await name(aStream) : 'none';

  // --- audio: libav.js → Web Audio, the master clock --------------------
  const ac = new AudioContext({ latencyHint: 'playback' });
  await ac.resume().catch(() => {});
  let clock = null; // {pts, at}: media time pts plays at ac time `at`
  let nextAt = 0;
  const mediaNow = () => (clock ? clock.pts + (ac.currentTime - clock.at) : null);

  let aCtx = null;
  if (aStream) {
    const cp = await demux.ff_copyout_codecpar(aStream.codecpar);
    const [, c, p, f] = await adec.ff_init_decoder(stats.audio, { codecpar: cp, time_base: [aStream.time_base_num, aStream.time_base_den] });
    aCtx = { c, p, f, tb: tb(aStream) };
  }
  function toChannels(fr) {
    // → [left, right] Float32Arrays, downmixing 5.1 the simple way.
    const n = fr.nb_samples, planes = [];
    const ch = fr.channels ?? (Array.isArray(fr.data) ? fr.data.length : 2);
    const fmtf = fr.format;
    const get = (i) => {
      if (Array.isArray(fr.data)) {
        const d = fr.data[i];
        return d instanceof Float32Array ? d : Float32Array.from(d, (v) => v / 32768);
      }
      const out = new Float32Array(n);
      for (let k = 0; k < n; k++) {
        const v = fr.data[k * ch + i];
        out[k] = fmtf === LibAV.AV_SAMPLE_FMT_FLT ? v : v / 32768;
      }
      return out;
    };
    for (let i = 0; i < ch; i++) planes.push(get(i));
    if (ch === 1) return [planes[0], planes[0]];
    if (ch === 2) return planes;
    const L = new Float32Array(n), R = new Float32Array(n);
    const c = planes[2] ?? new Float32Array(n);
    for (let k = 0; k < n; k++) {
      L[k] = 0.6 * planes[0][k] + 0.42 * c[k] + 0.3 * (planes[4]?.[k] ?? 0);
      R[k] = 0.6 * planes[1][k] + 0.42 * c[k] + 0.3 * (planes[5]?.[k] ?? 0);
    }
    return [L, R];
  }
  async function decodeAudio(packets) {
    const frames = await adec.ff_decode_multi(aCtx.c, aCtx.p, aCtx.f, packets, { ignoreErrors: true });
    for (const fr of frames) {
      const pts = LibAV.i64tof64(fr.pts, fr.ptshi) * aCtx.tb;
      const [L, R] = toChannels(fr);
      const buf = ac.createBuffer(2, L.length, fr.sample_rate);
      buf.copyToChannel(L, 0); buf.copyToChannel(R, 1);
      if (!clock) { clock = { pts, at: ac.currentTime + 0.4 }; nextAt = clock.at; stats.startedAt = performance.now(); }
      let at = clock.at + (pts - clock.pts);
      if (at < ac.currentTime + 0.02) { stats.audioUnderruns++; continue; } // too late: skip, keep the clock
      const src = ac.createBufferSource(); src.buffer = buf; src.connect(ac.destination); src.start(at);
      nextAt = at + buf.duration;
      stats.audioFrames++;
    }
  }

  // --- video ------------------------------------------------------------
  const queue = []; // VideoFrames by timestamp (µs of media time)
  let sar = 1;
  let vDecode;
  if (vStream) {
    const vtb = tb(vStream);
    const cp = await demux.ff_copyout_codecpar(vStream.codecpar);
    sar = cp.sample_aspect_ratio && cp.sample_aspect_ratio[0] ? cp.sample_aspect_ratio[0] / cp.sample_aspect_ratio[1] : 1;
    let wc = null;
    if (stats.video === 'h264' && 'VideoDecoder' in window && !opts.noWebCodecs) {
      wc = { pending: [], dec: null, configured: false };
    }
    if (wc) stats.path = 'h264 → WebCodecs (pending SPS)';
    else stats.path = `${stats.video} → libav.js (WebAssembly)`;
    const [, c, p, f] = await vdec.ff_init_decoder(stats.video, { codecpar: cp, time_base: [vStream.time_base_num, vStream.time_base_den] });

    const pushFrame = (vf) => { queue.push(vf); stats.decodedFrames++; };
    async function swDecode(packets) {
      const t0 = performance.now();
      const frames = await vdec.ff_decode_multi(c, p, f, packets, { ignoreErrors: true, copyoutFrame: 'video' });
      stats.videoDecodeMs += performance.now() - t0; stats.videoDecodeCalls += frames.length;
      for (const fr of frames) {
        const pts = LibAV.i64tof64(fr.pts, fr.ptshi) * vtb;
        if (fr.sample_aspect_ratio && fr.sample_aspect_ratio[0] > 0) sar = fr.sample_aspect_ratio[0] / fr.sample_aspect_ratio[1];
        try {
          pushFrame(new VideoFrame(fr.data, { format: 'I420', codedWidth: fr.width, codedHeight: fr.height, timestamp: Math.round(pts * 1e6), layout: fr.layout }));
        } catch (e) { stats.errors.push('frame: ' + e.message); }
      }
    }
    async function wcDecode(packets) {
      for (const pk of packets) {
        const key = (pk.flags & LibAV.AV_PKT_FLAG_KEY) !== 0;
        const pts = LibAV.i64tof64(pk.pts, pk.ptshi) * vtb;
        if (!wc.configured) {
          if (!key) continue;
          const sps = findSPS(pk.data);
          if (!sps) continue;
          const codec = 'avc1.' + [sps[1], sps[2], sps[3]].map((b) => b.toString(16).padStart(2, '0')).join('');
          let config = null;
          for (const hw of ['prefer-hardware', 'no-preference']) {
            const c = { codec, optimizeForLatency: true, hardwareAcceleration: hw };
            const support = await VideoDecoder.isConfigSupported(c).catch(() => ({ supported: false }));
            if (support.supported) { config = c; break; }
          }
          if (!config) { wc = null; stats.path = 'h264 → libav.js (WebCodecs has no ' + codec + ')'; return swDecode(packets); }
          wc.dec = new VideoDecoder({
            output: (vf) => pushFrame(vf),
            // A decoder that fails after all (no codec behind the claim, a
            // broken stream) hands over to libav.js from the next key frame.
            error: (e) => { stats.errors.push('wc: ' + e.message); wc = null; stats.path = 'h264 → libav.js (WebCodecs failed)'; },
          });
          try { wc.dec.configure(config); } catch (e) { wc = null; stats.path = 'h264 → libav.js (WebCodecs failed)'; return swDecode(packets); }
          wc.configured = true; stats.path = 'h264 → WebCodecs (' + codec + ')';
        }
        if (!wc) return swDecode([pk]);
        if (wc.dec.state === 'closed') { wc = null; return swDecode([pk]); }
        wc.dec.decode(new EncodedVideoChunk({ type: key ? 'key' : 'delta', timestamp: Math.round(pts * 1e6), data: pk.data }));
        stats.decodedFramesWc = (stats.decodedFramesWc || 0) + 1;
      }
    }
    vDecode = (packets) => (wc ? wcDecode(packets) : swDecode(packets));
  }

  function findSPS(d) {
    for (let i = 0; i + 4 < d.length; i++) {
      if (d[i] === 0 && d[i + 1] === 0 && d[i + 2] === 1 && (d[i + 3] & 0x1f) === 7) return d.subarray(i + 3, i + 8);
    }
    return null;
  }

  // --- render loop: show the frame due at the audio clock --------------
  function render() {
    const now = mediaNow();
    if (now !== null) {
      queue.sort((a, b) => a.timestamp - b.timestamp);
      let show = null;
      while (queue.length && queue[0].timestamp / 1e6 <= now) {
        if (show) { show.close(); stats.droppedFrames++; }
        show = queue.shift();
      }
      if (show) {
        const w = show.displayWidth * sar, h = show.displayHeight;
        if (canvas.width !== Math.round(w) || canvas.height !== h) { canvas.width = Math.round(w); canvas.height = h; }
        ctx2d.drawImage(show, 0, 0, canvas.width, canvas.height);
        stats.avOffsetMs = Math.round((now - show.timestamp / 1e6) * 1000);
        show.close(); stats.shownFrames++;
      }
      stats.mediaSeconds = now - (clock ? clock.pts : 0);
    }
    stats.queue = queue.length;
    requestAnimationFrame(render);
  }
  requestAnimationFrame(render);

  // --- demux loop with back-pressure -----------------------------------
  (async () => {
    for (;;) {
      while (queue.length > 90 || (clock && nextAt - ac.currentTime > 3)) await new Promise((r) => setTimeout(r, 20));
      const [res, byStream] = await demux.ff_read_frame_multi(fmt, pkt, { limit: 256 * 1024 });
      const tasks = [];
      if (vStream && byStream[vStream.index]) tasks.push(vDecode(byStream[vStream.index]));
      if (aStream && byStream[aStream.index]) tasks.push(decodeAudio(byStream[aStream.index]));
      await Promise.all(tasks);
      if (res === LibAV.AVERROR_EOF) break;
      if (res !== 0 && res !== -LibAV.EAGAIN) { stats.errors.push('read: ' + res); break; }
    }
  })().catch((e) => stats.errors.push('loop: ' + e.message));
  return { stats, audioContext: ac };
}
