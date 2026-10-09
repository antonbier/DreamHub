# Ad detection (AdScan) — Plan

Status (2026-10-09, #44): spike done (`spikes/adscan/`), phase 0 prepared as a read-only script (`spikes/adscan/phase0.sh`). **Phase 1 backend built against the mock**: the queue (`internal/jobs`, #41), the detector (`internal/adscan`), the `media.adscan` job and `POST`/`GET /api/v1/media/adscan`. Not yet: the preview UI, writing `.cuts` (waits for phase 0's format check), settings, events, results following a rename. Read together with `CONCEPT.md`, `PRINCIPLES.md` and `ARCHITECTURE.md`; where this document conflicts with them, they win. Once a phase lands, its status moves to `ROADMAP.md`.

## Decisions

- **Part of MediaHub, not a new hub.** Ad detection is the first stage of MediaHub *trimming* (v1.1, #27): it produces cut suggestions, trimming applies them.
- **Everything runs locally on the box.** No cloud, no vision model, no AI provider (not even the existing OpenAI connector); no data leaves the box; works offline.
- **No Comskip / noad / markad.** Comskip decodes every frame (too slow on the box), noad is unmaintained, markad expects VDR recordings. A small dedicated detector on top of FFmpeg instead.
- **No hardware decoder.** DreamOS's closed drivers send decoded frames straight to the video plane; userspace can't read them. Speed comes from decoding as few frames as possible.
- **The system FFmpeg, not a bundled one.** The box has FFmpeg 4.1.9 from the standard feed and DreamHub's `.deb` already depends on it (`PROJECT_PLAN.md`). If something it lacks is ever needed, a self-contained build goes *next to* it (precedent: `ffmpeg-mp`), never replacing it.
- **Audio fingerprints are computed in Go.** The backend is a pure-Go static binary (`CGO_ENABLED=0`), so linking Chromaprint is out; FFmpeg decodes the audio to low-rate mono PCM, Go computes a Chromaprint-style fingerprint. No dependency on an FFmpeg built with `--enable-chromaprint` or on `fpcalc`.
- **Nothing per country or channel is hard-coded.** Logo masks are learned from the user's own recordings, so a French or Italian box works the same way. Ad-free channels are not a fixed list either: a channel whose recordings never show a sustained logo gap is *suggested* for the exclusion list; the user decides. Series patterns are learned, never configured.
- **Preview and confirm.** Detection only suggests; writing `.cuts` is a change: plan → apply, backup, change-log entry.
- **Automatic analysis after a recording belongs in AutomationHub** (#35, later), as a rule `recording.finished → analyze`. Until then: started by the user, one recording or a batch.

## Why (connected value)

| Output | Used by | Value |
|---|---|---|
| Ad blocks | MediaHub, StreamHub, TV playback | Skip ads (TV via `.cuts`, the browser player directly), trimming, net runtime |
| Real program start/end | TimerHub | Learned pre/post margins per channel/series → fewer conflicts on the single tuner, less disk |
| Intro/outro fingerprints | MediaHub, TimerHub, notifications | Detect wrong or truncated recordings |
| Episode fingerprints | MediaHub, SeriesHub | Duplicates, episode identification despite bad EPG data |
| Outro position | MediaHub | Precise "seen" status |

## Detection pipeline

Three layers, all local. Each yields candidate boundaries with a confidence; a merger combines them. Layer 1 alone must already be useful.

### 1. Logo detection (works from the first recording)

Channel logo visible → program; logo gone for a sustained time → ad block.

**Logo mask per channel** (keyed by service reference):
- Learned on a channel's first analysis: sample N keyframes spread over the recording, find the pixels in the four corner regions that stay stable (edges, not just brightness) across most samples → mask, reference image, corner. Ads are usually well under half the airtime, so the logo wins the vote.
- Stored under `<data dir>/adscan/logos/<sref>.{png,json}` and reused.
- Shown in settings as an image per channel: confirm, relearn, delete.
- Known weak cases: animated or semi-transparent logos, logos removed late or restored early, channels that keep the logo during ads. They lower confidence; they don't break anything.

**Frame extraction:**
- Keyframes only: `-skip_frame nokey`; output only the logo corner, downscaled, grayscale, raw over a pipe (`-vf crop=…,scale=…,format=gray -f rawvideo -`).
- Seeking: FFmpeg can't use Enigma2's `.ts.ap` index directly. The backend looks up the byte offset in `.ts.ap`, reads a short slice of the `.ts` (≈1–2 MB) at that offset and feeds it to FFmpeg's stdin; the PTS comes from the index or the stream. Without a usable `.ts.ap`: fall back to byte-offset estimation from the file size and duration.
- The Go side scores each frame against the mask (edge overlap / normalized correlation inside the mask) → logo yes/no + score.

**Coarse sampling + bisection** (the main speed-up):
- Sample one keyframe every ~20–30 s (setting). Where two neighbours disagree, bisect until the boundary is known to ~1 s.
- Independent intervals in parallel (6 cores; worker cap as a setting).
- Estimate: ~150–250 decoded frames per hour instead of ~3600 keyframes or ~90,000 frames. **Measure** (spike, then box).

### 2. Audio fingerprints (better with every recording)

Ad spots, jingles, separators, series intros and outros repeat; recurring segments are classified by how and where they recur.

- FFmpeg decodes the audio only (`-vn`, mono, ~11 kHz s16le) → Go fingerprint → short windows (~3 s) hashed into an SQLite index under `<data dir>/adscan/`.
- Segment types, inferred from recurrence: `spot` (across programs/channels → ad), `separator` (short, at block boundaries → second-exact block edge), `intro`/`outro` (same series, similar relative position), `episode` (whole-recording match → duplicate). User corrections in the preview override.
- Cost: this layer reads the whole file (HD ≈ 3–6 GB/h). Audio decoding is cheap, disk reading is roughly a minute per recorded hour — acceptable as a background job, but it's why layer 1 must stand alone.
- Bounded storage: prune segments that never recurred; retention as a setting, set after measuring.

### 3. Fine cut (only at boundaries)

`blackdetect` and `silencedetect` only in short windows (±10 s) around candidate boundaries; snap to a black frame/silence when one is close. Never over the whole file.

### Merging and confidence

- Agreement between layers raises confidence; a `separator` boundary beats a logo boundary.
- Minimum block length and minimum gap are settings (short logo dropouts are not ads).
- Blocks below the confidence threshold are *uncertain* in the preview. No external fallback — the user is the fallback.

## Series patterns (learned priors)

Key: channel + EPG title (optionally the time slot). Learned only from **confirmed** analyses, including corrections. Stored: number of blocks, positions **relative to the program start** (the timer pre-margin varies), typical lengths, known intro/outro/separator segments. Used for denser sampling near expected blocks, and for confidence (matches the pattern → no confirmation needed; strong deviation → flagged). Program start anchored by the intro fingerprint when known, else by the first logo-on boundary.

## Writing `.cuts`

- `.ts.cuts`: 12-byte entries, 64-bit big-endian PTS + 32-bit big-endian type (`0` IN, `1` OUT, `2` MARK, `3` LAST = resume position). **Verify on DreamOS** before the first write.
- Keep every entry that isn't ours (always keep type 3), add IN/OUT for confirmed blocks, write atomically (temp file + rename), back up the original before the first change, support undo.
- **Verify on the box**: does the DreamOS player skip OUT→IN regions (InfoBarCueSheetSupport), and which setting controls it? Where is "seen" stored (AdvancedMovieSelection, `.cuts` type 3, elsewhere)?
- Same rule as rename: never touch a recording that is being recorded or played.
- Each write: plan → apply, change-log entry (recording, blocks, backup).

## Integration with the core

- **Requirement:** capability `ffmpeg` (system package). Missing → *unavailable* with reason and fix.
- **Jobs:** the MediaHub background queue (#41), built generally with AdScan as its first user. Runs under `nice -n 19` / `ionice -c3`; pauses while a recording runs (asks TimerHub through the core API) and optionally during playback; cancellable, resumable; batches limitable to a time window (e.g. night).
- **API:** under `/api/v1/media/adscan/…`: start (one or many recordings), job status (SSE progress), results per recording, confirm/edit/discard blocks (→ plan/apply for `.cuts`), logo masks (list/view/relearn/delete), known segments (list/retype/delete), undo.
- **Events:** `adscan.finished`, `adscan.failed`, later `recording.suspicious`; the core turns them into notifications (message key + parameters).
- **Search / tiles:** "ads detected", "not analysed", "uncertain blocks", "ad share > X %".
- **Settings (MediaHub → Ad detection):** enabled, sampling interval, minimum block length, confidence threshold, max workers, batch window, pause during playback, excluded channels, fingerprint retention.
- **Time:** positions are PTS/durations from the stream, never from file names.
- **Mock:** a fake FFmpeg runner with canned frame scores and fixture `.ap`/`.cuts` files, so the hub, queue and UI are testable without the box (`PRINCIPLES.md`, mock-first).

## UI (MediaHub)

- Recording row/detail: "Detect ads" (multi-select for batch), status (not analysed / running / done / uncertain), net runtime.
- Preview: timeline with blocks coloured by confidence; keyframes just before and after each boundary; per block confirm / move boundary / discard; "Confirm all certain blocks" as the single primary action; then the `.cuts` plan to confirm.
- Settings: logo masks as images per channel; known segments by type.
- Follows `DESIGN.md`; accessible from the start (keyboard-operable timeline, labelled boundaries, colour never the only signal).

## Phases

| Phase | Content |
|---|---|
| **Spike** (no box) | `spikes/adscan/`: logo mask learning, coarse sampling + bisection and timing per frame on the local sample recordings (SD MPEG-2 RTL, HD H.264 ZDF) |
| **0 — Verify on device** | System FFmpeg 4.1.9: `-skip_frame nokey` + crop pipe + stdin slices on real recordings; `.ts.ap` format; `.cuts` types and seen storage; player skips OUT→IN; time per recorded hour on the box |
| **1 — Logo detection, manual** | Queue (#41), logo masks, sampling + bisection, fine cut, preview UI, `.cuts` plan/apply with backup/undo, pause during recordings, settings, events |
| **2 — Fingerprints + series patterns** | Go fingerprinting, segment index and types, separator boundaries, learned series patterns, targeted sampling |
| **3 — Connected features** | Learned timer margins (suggestions via preview-and-confirm), suspicious recordings, duplicates, net runtime + "GB of ads on disk" tile, intro skip in StreamHub, seen status |
| **Later** | Live "ads are over" notification (logo mask on the live signal), AutomationHub rule `recording.finished → analyze` (opt-in auto-confirm above a high confidence) |

## Out of scope

- Any cloud or AI-provider involvement.
- Comskip/noad/markad.
- Cutting the `.ts` file itself — that's trimming (#27), which consumes these results.

## Open questions

- Thresholds (sampling interval, block length, confidence) — tune on real recordings of the user's channels.
- Fingerprint DB growth — retention after measuring.
- Whether the browser player (StreamHub) reads ad blocks from DreamHub's own results or from `.cuts` (both possible; own results also cover unconfirmed blocks as "maybe skip").
