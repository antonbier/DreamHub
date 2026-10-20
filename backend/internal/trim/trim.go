// Package trim cuts parts out of a recording without re-encoding (MediaHub
// trimming, #27): FFmpeg's concat demuxer reads the parts to keep from the
// original and copies their packets into a new file. Cuts land on
// keyframes, so a cut can keep up to a second more than asked.
//
// It only writes the new file; what happens to the original (renamed and
// trashed) is MediaHub's business (internal/hubs/media_trim.go).
package trim

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/adscan"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
)

// Cutter writes the parts of src listed in keep (seconds from the start of
// a recording duration seconds long) to dst. progress reports the share
// written, 0–1.
type Cutter interface {
	Cut(ctx context.Context, src, dst string, keep []adscan.Span, duration float64, progress func(share float64)) error
}

// MinPart: shorter pieces between two cuts aren't worth keeping (a second
// of programme between two breaks is a boundary artefact).
const MinPart = 2.0

// Keep turns the parts to cut out of a recording of duration seconds into
// the parts to keep, in order; overlapping cuts merge.
func Keep(duration float64, cut []adscan.Span) []adscan.Span {
	cs := append([]adscan.Span(nil), cut...)
	sort.Slice(cs, func(i, j int) bool { return cs[i].Start < cs[j].Start })
	var keep []adscan.Span
	at := 0.0
	for _, c := range cs {
		start, end := max(0, c.Start), min(duration, c.End)
		if end <= at {
			continue
		}
		if start-at >= MinPart {
			keep = append(keep, adscan.Span{Start: at, End: start})
		}
		at = max(at, end)
	}
	if duration-at >= MinPart {
		keep = append(keep, adscan.Span{Start: at, End: duration})
	}
	return keep
}

// Length is the total of spans.
func Length(spans []adscan.Span) float64 {
	t := 0.0
	for _, s := range spans {
		t += s.End - s.Start
	}
	return t
}

// FFmpeg is the box's Cutter.
type FFmpeg struct {
	Path   string
	Prefix []string // nice/ionice, as for ad detection
	// Probe finds where the recording's clock starts: the concat demuxer's
	// in/out points are stream timestamps, not seconds from the start.
	Probe adscan.FFmpeg
}

// Cut implements Cutter.
func (f FFmpeg) Cut(ctx context.Context, src, dst string, keep []adscan.Span, duration float64, progress func(share float64)) error {
	if len(keep) == 0 {
		return errors.New("trim: nothing to keep")
	}
	// Never write over anything (-n below too, but not every build exits
	// with an error on it).
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("trim: %s exists", dst)
	}
	start, err := f.Probe.Start(ctx, src)
	if err != nil {
		return err
	}
	list := dst + ".list"
	if err := os.WriteFile(list, []byte(concatList(src, start, keep, duration)), 0o600); err != nil {
		return err
	}
	defer os.Remove(list)

	args := append([]string{}, f.Prefix...)
	args = append(args, f.Path, "-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "concat", "-safe", "0", "-i", list,
		// Video, every audio track, subtitles and teletext; the box's
		// private data streams aren't worth risking the cut for.
		"-map", "0:v", "-map", "0:a", "-map", "0:s?",
		"-c", "copy", "-f", "mpegts",
		"-progress", "pipe:1", "-nostats",
		"-n", dst)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	// FFmpeg reports bytes written (its time is N/A once subtitles are
	// copied too); the new file is about the kept share of the old one.
	var expected float64
	if fi, err := os.Stat(src); err == nil && duration > 0 {
		expected = float64(fi.Size()) * Length(keep) / duration
	}
	readProgress(stdout, func(written float64) {
		if progress != nil && expected > 0 {
			progress(min(written/expected, 1))
		}
	})
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("trim: ffmpeg: %v: %s", err, tail(stderr.String()))
	}
	// DreamHub runs with a strict umask, so FFmpeg's file would be
	// readable by root only; a recording is shared (Samba, NFS, other
	// plugins), so the cut version gets the original's permissions.
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(src); err == nil {
		mode = fi.Mode().Perm()
	}
	return os.Chmod(dst, mode)
}

// concatList is the concat demuxer's script: the same file once per part,
// with in and out points on the stream's own clock. A part running to the
// recording's end gets no out point: the measured duration ends a little
// before the file does, and the end of the programme must not be lost.
func concatList(src string, start float64, keep []adscan.Span, duration float64) string {
	var b strings.Builder
	b.WriteString("ffconcat version 1.0\n")
	quoted := "'" + strings.ReplaceAll(src, "'", `'\''`) + "'"
	for _, k := range keep {
		fmt.Fprintf(&b, "file %s\ninpoint %.3f\n", quoted, start+k.Start)
		if k.End < duration {
			fmt.Fprintf(&b, "outpoint %.3f\n", start+k.End)
		}
	}
	return b.String()
}

// readProgress follows FFmpeg's -progress output ("total_size=…", bytes).
func readProgress(r io.Reader, report func(bytes float64)) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "total_size="); ok {
			if n, err := strconv.ParseFloat(v, 64); err == nil && n >= 0 {
				report(n)
			}
		}
	}
}

func tail(s string) string {
	if len(s) > 400 {
		return s[len(s)-400:]
	}
	return s
}

// Mock is mock mode's Cutter: the mock box's recordings hold no video, so
// it writes a stand-in file of the kept share's size into the mock FS.
type Mock struct {
	FS recfs.FS
}

// Cut implements Cutter.
func (m Mock) Cut(ctx context.Context, src, dst string, keep []adscan.Span, duration float64, progress func(share float64)) error {
	data, err := m.FS.ReadFile(src)
	if err != nil {
		return err
	}
	if progress != nil {
		progress(1)
	}
	return m.FS.Create(dst, []byte("trimmed: "+string(data)))
}
