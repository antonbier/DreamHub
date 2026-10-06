# Spike: live TV and recordings in the browser, no transcoding

StreamHub was built on this (2026-10-06): `frontend/src/streaming/tsPlayer.ts`.

Result of the StreamHub spike (2026-10-06). The question: can a browser play
what the box sends — MPEG-TS with MPEG-2 or H.264 video and MP2, AC3 or AAC
audio — without the box converting anything? The box can't: no usable
encoder in this image, too weak a CPU (`ARCHITECTURE.md`, "Hardware
encoder"). So the browser does it.

## How it works

- The box passes the stream through unchanged (a recording via WebControl's
  `/file`, live TV via port 8001). DreamHub would proxy it, so the page can
  read it (same origin) — `serve.py`'s `/box?file=` stands in for that.
- [libav.js](https://github.com/Yahweasel/libav.js) (FFmpeg in WebAssembly),
  in its own build with only what DVB needs (`libav-variant.json`): the
  MPEG-TS demuxer, MPEG-2 video, H.264 (software fallback), MP2/MP3, AC3/E-AC3
  and AAC/LATM decoders. 2.0 MB of WebAssembly. The stock libav.js packages
  leave out every MPEG codec on purpose, hence the own build; the relevant
  patents have expired.
- Three workers: demuxer, audio decoder, video decoder. H.264 goes to
  WebCodecs (the phone's or PC's own hardware decoder) when the browser has
  it, else to libav.js; a WebCodecs decoder that fails hands over to libav.js.
  MPEG-2 always goes to libav.js; frames become `VideoFrame`s drawn on a
  canvas.
- Web Audio is the clock: audio buffers are scheduled at their timestamps,
  and each animation frame shows the video frame due now. Reading pauses
  while 90 frames or 3 s of audio are buffered.

## Measured (headless Chromium on the development machine, 4 cores, no threads)

| Sample | Video / audio | Decoding | Shown | Audio | A/V offset |
| --- | --- | --- | --- | --- | --- |
| RTL Television (SD) | MPEG-2 576i / MP2 | libav.js: **4.0 ms per frame** (40 ms available at 25 fps) | 23.8 fps, 0 dropped | 0 underruns | 10 ms |
| ZDF HD | H.264 720p50 / MP2 | libav.js (worst case, no hardware): **13.2 ms per frame** (20 ms available) | 46 fps, 5 % dropped | 0 underruns | 16 ms |
| ZDF HD | H.264 720p50 / AC3 | WebCodecs | 45 fps | 0 underruns | 18 ms |
| ORF1 HD, streamed from the box | H.264 / AC3 | WebCodecs | 42 fps, first picture < 1 s | 0 underruns | 5 ms |

Headless Chromium's frame pacing explains most of the "dropped" frames;
phones and real browsers still need measuring (see below).

## Not done yet

- ~~Deinterlacing~~: done in StreamHub's player (a linear blend in
  JavaScript; FFmpeg's yadif crashed the page in this single-threaded build).
- Phones: decoding speed of MPEG-2 in WebAssembly on a mid-range phone;
  the threaded build (`.thr`, needs the page cross-origin isolated —
  COOP/COEP headers) if a single thread is too slow.
- Live specifics: the stream starts mid-GOP (the first picture waits for a
  key frame), PTS wrap-around after 26.5 hours, channel changes, picking an
  audio track and the subtitle/teletext streams.
- HEVC channels (UHD): WebCodecs where the device decodes HEVC; no software
  fallback (too slow in WebAssembly).
- The single tuner: watching another channel live switches the TV; during a
  recording only channels on the same transponder.

## Running it

```
# libav.js with the variant (needs Emscripten; ~10 minutes)
git clone https://github.com/Yahweasel/libav.js && cd libav.js
(cd configs && ./mkconfig.js dreamhub "$(cat …/libav-variant.json)")
make dist/libav-6.10.9.0-dreamhub.mjs dist/libav-6.10.9.0-dreamhub.wasm.mjs
cp dist/libav-6.10.9.0-dreamhub.* …/spikes/streamhub-player/libav/

# samples: any .ts, e.g. the first 40 MB of a recording, into samples/
BOX=http://<box> python3 serve.py 8099   # then http://127.0.0.1:8099/?src=sd-rtl.ts
```
