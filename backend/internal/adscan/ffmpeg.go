package adscan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
)

// FFmpeg is the box's Opener: it decodes single keyframes with the system
// FFmpeg (4.1 on the reference box), feeding it a short slice of the .ts
// over stdin, so a sample is a jump, not a read through the file
// (spikes/adscan/README.md).
type FFmpeg struct {
	// Path is the ffmpeg binary.
	Path string
	// Prefix runs FFmpeg at low priority, e.g. nice -n 19 ionice -c3; the
	// box's recordings and enigma2 come first.
	Prefix []string
	// SliceBytes is how much of the .ts one sample reads; 1.5 MB when 0.
	SliceBytes int64
}

// Open implements Opener.
func (f FFmpeg) Open(path string) (Source, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fi, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	s := &ffSource{f: f, file: file, size: fi.Size()}
	if s.f.SliceBytes <= 0 {
		s.f.SliceBytes = 1536 * 1024
	}
	// Enigma2's access-point index: seeking by time without guessing. A
	// missing or unreadable one falls back to byte positions.
	if data, err := os.ReadFile(path + ".ap"); err == nil {
		if aps, err := ParseAP(data); err == nil && len(aps) > 1 {
			s.aps = aps
		}
	}
	return s, nil
}

type ffSource struct {
	f    FFmpeg
	file *os.File
	size int64
	aps  []AccessPoint

	// base and dur, in stream seconds, once known.
	base, dur float64
	known     bool
}

func (s *ffSource) Close() error { return s.file.Close() }

// Duration decodes the first and the last keyframe: their timestamps give
// the recording's start and length, whatever the index says.
func (s *ffSource) Duration(ctx context.Context) (float64, error) {
	if s.known {
		return s.dur, nil
	}
	first, _, err := s.decode(ctx, 0)
	if err != nil {
		return 0, fmt.Errorf("adscan: reading the recording's start: %w", err)
	}
	last, _, err := s.decode(ctx, s.lastSlice())
	if err != nil {
		return 0, fmt.Errorf("adscan: reading the recording's end: %w", err)
	}
	s.base, s.dur, s.known = first, unwrap(last-first), true
	return s.dur, nil
}

func (s *ffSource) lastSlice() int64 {
	return max(0, (s.size-s.f.SliceBytes)/tsPacket*tsPacket)
}

func (s *ffSource) Frame(ctx context.Context, at float64) (Frame, error) {
	if _, err := s.Duration(ctx); err != nil {
		return Frame{}, err
	}
	var off int64
	if ap, ok := APAt(s.aps, at); ok {
		off = ap.Offset
	} else {
		off = int64(float64(s.size) * at / s.dur)
	}
	off = min(off/tsPacket*tsPacket, s.lastSlice())
	pts, pix, err := s.decode(ctx, off)
	if err != nil {
		return Frame{}, err
	}
	return Frame{At: unwrap(pts - s.base), Pix: pix}, nil
}

// tsPacket: slices start on a DVB packet boundary.
const tsPacket = 188

// The PTS is a 33-bit counter at 90 kHz; it wraps after about 26.5 hours.
const (
	ptsHz      = 90000
	ptsWrap    = 1 << 33
	ptsWrapSec = float64(ptsWrap) / ptsHz
)

func unwrap(d float64) float64 {
	if d < 0 {
		d += ptsWrapSec
	}
	return d
}

var ptsRe = regexp.MustCompile(`pts_time:\s*(-?[0-9.]+)`)

// decode returns the first keyframe in the slice at off: its stream time in
// seconds (-copyts keeps the stream's own clock) and its pixels.
func (s *ffSource) decode(ctx context.Context, off int64) (float64, []byte, error) {
	args := append([]string{}, s.f.Prefix...)
	args = append(args, s.f.Path, "-hide_banner", "-loglevel", "info",
		"-skip_frame", "nokey", "-copyts", "-i", "-",
		"-map", "0:v:0", "-an", "-sn",
		"-vf", fmt.Sprintf("scale=%d:%d,format=gray,showinfo", FrameW, FrameH),
		"-frames:v", "1",
		// -fps_mode only exists from FFmpeg 5.1.
		"-vsync", "passthrough",
		"-f", "rawvideo", "-")
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = io.NewSectionReader(s.file, off, s.f.SliceBytes)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return 0, nil, ctx.Err()
	}
	if errors.Is(err, exec.ErrNotFound) {
		return 0, nil, err
	}
	pix := stdout.Bytes()
	m := ptsRe.FindSubmatch(stderr.Bytes())
	if len(pix) < FrameW*FrameH || m == nil {
		return 0, nil, ErrNoFrame
	}
	pts, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return 0, nil, ErrNoFrame
	}
	return pts, pix[:FrameW*FrameH], nil
}
