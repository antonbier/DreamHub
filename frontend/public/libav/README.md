# libav.js, DreamHub build

StreamHub's decoder (`src/streaming/tsPlayer.ts`): [libav.js](https://github.com/Yahweasel/libav.js)
6.10.9 — FFmpeg 9.0 compiled to WebAssembly — in a variant with only what DVB
broadcasts need. FFmpeg and libav.js are LGPL 2.1 (the license text is at the
top of each `.mjs`); these files are unmodified build output.

Variant `dreamhub` (configuration fragments):

```
["avformat","avcodec","demuxer-mpegts","parser-mpegvideo","decoder-mpeg2video","parser-h264","decoder-h264",
 "parser-hevc","parser-mpegaudio","decoder-mp2","decoder-mp3","parser-ac3","decoder-ac3",
 "decoder-eac3","parser-aac","decoder-aac","decoder-aac_latm"]
```

Rebuilding (needs [Emscripten](https://emscripten.org)):

```
git clone https://github.com/Yahweasel/libav.js && cd libav.js
(cd configs && ./mkconfig.js dreamhub '<the list above>')
make dist/libav-6.10.9.0-dreamhub.mjs dist/libav-6.10.9.0-dreamhub.wasm.mjs
cp dist/libav-6.10.9.0-dreamhub.{mjs,wasm.mjs,wasm.wasm} <here>
```

The stock libav.js packages leave out the MPEG codecs on purpose; the
patents on MPEG-2 video, MP2 and AC-3 have expired. No avfilter: its yadif
deinterlacer crashed the page in this single-threaded build, so the player
deinterlaces SD itself. See
`spikes/streamhub-player/README.md` for how the player was measured.
