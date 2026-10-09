# Spike: logo detection for AdScan

Part of [`docs/ADSCAN_PLAN.md`](../../docs/ADSCAN_PLAN.md) (#44). Run 2026-10-09 on the development machine with FFmpeg 7 (Flatpak runtime), not yet on the box.

The questions: can a channel's logo be learned from keyframes alone, does a mask score separate "logo" from "no logo", and does seeking by feeding FFmpeg a slice of the `.ts` over stdin work?

```sh
go build -o adscan .
FFMPEG=/path/to/ffmpeg ./adscan learn rec.ts             # writes rec.ts.mask.json
FFMPEG=/path/to/ffmpeg ./adscan scan  rec.ts mask.json   # score every keyframe
FFMPEG=/path/to/ffmpeg ./adscan seek  rec.ts mask.json   # one keyframe per 1.5 MB slice, 10 offsets
```

## How it works

- FFmpeg decodes keyframes only (`-skip_frame nokey`), scales them to 384×216 grayscale and writes raw frames to a pipe; `showinfo` on stderr gives each frame's timestamp, `-copyts` keeps the stream's own timestamps.
- Edges: Sobel, thresholded, with the direction quantized to eight bins. The direction includes the polarity, so a bright stroke on a dark background differs from a dark one on a bright background.
- Learning: in each of the four corners (a quarter of the width and height), a pixel belongs to the logo when it is an edge in the same direction (±1 bin) in at least 80 % of the frames. The corner with the most such pixels wins.
- Scoring: the share of mask pixels where the frame has an edge with the same direction (±1 bin) at the same place.

## Samples

Only short clips were at hand (`~/build/samples`, cut from box recordings for the StreamHub spike): RTL SD (MPEG-2, 720×576, 66 s, 108 keyframes, one about every 0.6 s) and ZDF HD (H.264, 1280×720, 26 s, 39 keyframes). Neither contains an ad break, so a "logo gone" boundary could not be tested; ZDF frames scored against RTL's mask stand in for "no logo".

## Results

- **Learning works.** The RTL logo came out cleanly as the top-left mask (153 pixels; the letters and their box are recognizable in the printed mask). Runner-up: top-right with 122–133 pixels, the temporary "Neue Folge" overlay. On a 66 s clip that overlay is as stable as the logo; over a whole recording it isn't. Still, the real implementation must learn from samples spread over the full recording, and should prefer the corner whose mask stays stable across *several* recordings.
- **Scoring needs edge direction, and no positional slack.** Three variants, RTL frames vs. ZDF frames against RTL's mask:

  | Score | RTL min / median | ZDF median / max | Separated? |
  |---|---|---|---|
  | edge present, ±1 px | 0.54 / 1.00 | 0.75 / 0.90 | no |
  | edge direction ±1 bin, ±1 px | 0.54 / 1.00 | 0.44 / 0.65 | no |
  | edge direction ±1 bin, exact pixel | 0.48 / 0.98 | 0.18 / 0.35 | **yes** |

  Textured backgrounds (trees in the ZDF clip) have edges everywhere, so "an edge is near" matches almost any logo; edge direction at the exact place doesn't. Background subtraction around the mask was tried and dropped: the logo's own box fills nearly all of its bounding box, so there's no background left to measure.
- **Weak frames exist.** One RTL keyframe scored 0.48: the logo is there, but on a bright, flat background with little contrast. The next-worst was 0.72. Single weak frames are why boundaries need confirmation by neighbours (bisection) and a minimum block length.
- **Semi-transparent logos are the weak case.** Learning the ZDF HD logo gave only 29 scattered pixels (top left, correct corner), because its edges vanish against light backgrounds and miss the 80 % share. Its own frames scored 1.00 in the median, but one scored 0.00, and RTL frames against it reached 0.45: with so few pixels, chance matches weigh more. A mask needs a minimum size to be trusted, and channels with a weak mask should show up as "uncertain" in the preview, or learn with a lower share over more frames. ZDF itself shows no ads during programs, but semi-transparent logos exist on commercial channels too.
- **Seeking by slices works.** 1.5 MB read at a TS-packet-aligned offset and piped to FFmpeg yields the first keyframe after it, with the correct stream timestamp (`-copyts`), in about 120 ms per seek on this machine, including starting FFmpeg through Flatpak.
- **Speed (this machine, not the box):** decoding all 108 SD keyframes of 66 s took ~300 ms. Box numbers are phase 0.

## Learning share: 0.6, not 0.8 (found while porting to the backend)

The spike learned with "an edge in at least 80 % of the frames". On a synthetic hour with 25 % ads (commercial channels: up to 12 minutes an hour, plus trailers without logo) that learned **one** pixel — the logo is missing from too many frames. Rerun with 0.6 on the clips:

| | 0.8 | 0.6 |
|---|---|---|
| RTL mask | 153 px | 227 px |
| RTL frames vs. RTL mask, min / p10 / median | 0.48 / 0.84 / 0.98 | 0.45 / 0.57 / 0.91 |
| ZDF frames vs. RTL mask, max | 0.35 | 0.31 |
| ZDF mask (semi-transparent) | 29 px (weak) | 95 px |
| RTL frames vs. ZDF mask, max | 0.45 | 0.33 |

0.6 separates as well or better and rescues the semi-transparent logo; the margin above the 0.42 threshold for the worst RTL frame is small (0.45), which is what bisection and "uncertain" are for. The backend (`internal/adscan`) uses 0.6.

## Prepared for the box (2026-10-09)

- **`phase0.sh`**: one read-only run on the box collects phase 0's facts into a report: FFmpeg version and build options (chromaprint? `-fps_mode` is 5.1+, the box's 4.1 needs `-vsync`), the `.ts.ap` / `.ts.cuts` layout of a real recording (hex dump, entry sizes), how common cut lists are on the disk, how Enigma2's `InfoBarGenerics` and AdvancedMovieSelection handle cut regions, the resume position and "seen", and timings (FFmpeg start, keyframe decode, a 1.5 MB slice over stdin, audio decode, disk read), all under `nice`/`ionice`. Tested locally on the sample clip.

  ```sh
  scp -O spikes/adscan/phase0.sh root@<box>:/tmp/
  ssh root@<box> 'sh /tmp/phase0.sh /media/hdd/movie/<recording>.ts > /tmp/adscan-phase0.txt 2>&1'
  ```
- **`sidecar.go`**: `.ts.ap` reader (16-byte entries, offset + PTS, unwrapped across the 33-bit PTS wrap, lookup by time) and `.ts.cuts` reader/writer (12-byte entries). Merging ad blocks into a cut list keeps marks and the resume position and **unites** the new blocks with the cut regions already there, so cuts made by hand in CutListEditor survive. Writes are atomic (temp file + rename). The layouts and Enigma2's IN/OUT rules (a lone IN cuts the start, a lone OUT the end) are taken from the open Enigma2 sources and must be confirmed by `phase0.sh`; whether the PTS in `.cuts` is the stream's own or relative to the recording's start is part of that.
- **`sampler.go`**: coarse sampling every 25 s, bisection to 1 s wherever neighbours disagree, minimum block length (60 s) and joining across short logo stretches (< 20 s, e.g. a station ident inside a break). On a synthetic hour with three breaks, a weak keyframe and a split break: all three blocks found to within 1 s with **190 probes**, inside the plan's 150–250 estimate. Without ads it takes only the 145 coarse samples. Gaps shorter than the sampling step can be missed between two "logo" samples, so the minimum block length must stay above the step.

## Open, for phase 0 and phase 1

- A full recording with ad breaks (RTL, VOX or ProSieben, about an hour), copied read-only from the box: learning over a whole recording, the logo-gone score, the threshold (somewhere around 0.4–0.45 on these clips), bisection.
- Timing on the box with its FFmpeg 4.1.9: process start, keyframe decode SD/HD, slices.
- `.ts.ap` as the seek index instead of fixed offsets.
- Semi-transparent logos (the ZDF HD logo) and animated ones.
