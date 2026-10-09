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

## Open, for phase 0 and phase 1

- A full recording with ad breaks (RTL, VOX or ProSieben, about an hour), copied read-only from the box: learning over a whole recording, the logo-gone score, the threshold (somewhere around 0.4–0.45 on these clips), bisection.
- Timing on the box with its FFmpeg 4.1.9: process start, keyframe decode SD/HD, slices.
- `.ts.ap` as the seek index instead of fixed offsets.
- Semi-transparent logos (the ZDF HD logo) and animated ones.
