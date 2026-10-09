package adscan

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFFmpeg writes a stand-in for ffmpeg: it reads the first line of the
// slice it is fed ("PTS <seconds>"), answers with one black frame on stdout
// and that time as showinfo's pts_time on stderr.
func fakeFFmpeg(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffmpeg")
	script := fmt.Sprintf(`#!/bin/sh
read -r tag pts
cat >/dev/null
head -c %d /dev/zero
echo "[Parsed_showinfo_2] n:   0 pts:0 pts_time:$pts" >&2
`, FrameW*FrameH)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeRecording writes n TS-packet-sized blocks, each starting with its
// stream time: 1000 s plus 10 s per packet.
func fakeRecording(t *testing.T, n int, withAP bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rec.ts")
	var b strings.Builder
	var ap []byte
	for i := 0; i < n; i++ {
		line := fmt.Sprintf("PTS %d\n", 1000+i*10)
		b.WriteString(line + strings.Repeat(" ", tsPacket-len(line)))
		entry := make([]byte, 16)
		binary.BigEndian.PutUint64(entry, uint64(i*tsPacket))
		binary.BigEndian.PutUint64(entry[8:], uint64((1000+i*10)*ptsHz))
		ap = append(ap, entry...)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if withAP {
		if err := os.WriteFile(path+".ap", ap, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestFFmpegSourceSeeksByIndexAndByBytes(t *testing.T) {
	bin := fakeFFmpeg(t)
	for _, withAP := range []bool{true, false} {
		src, err := FFmpeg{Path: bin, SliceBytes: 4 * tsPacket}.Open(fakeRecording(t, 100, withAP))
		if err != nil {
			t.Fatal(err)
		}
		dur, err := src.Duration(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		// The last slice starts at packet 96: 960 s after the first.
		if dur != 960 {
			t.Fatalf("ap %v: duration %v, want 960", withAP, dur)
		}
		f, err := src.Frame(context.Background(), 480)
		if err != nil {
			t.Fatal(err)
		}
		if len(f.Pix) != FrameW*FrameH {
			t.Fatalf("frame of %d bytes", len(f.Pix))
		}
		// With the index the jump is exact; by bytes it's an estimate.
		if withAP && f.At != 480 {
			t.Errorf("with .ap: frame at %v, want 480", f.At)
		}
		if !withAP && math.Abs(f.At-480) > 30 {
			t.Errorf("by bytes: frame at %v, want about 480", f.At)
		}
		src.Close()
	}
}

func TestFFmpegSourceWithoutFrames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ffmpeg")
	os.WriteFile(path, []byte("#!/bin/sh\ncat >/dev/null\necho 'Invalid data' >&2\nexit 1\n"), 0o755)
	src, err := FFmpeg{Path: path, SliceBytes: 4 * tsPacket}.Open(fakeRecording(t, 10, false))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.Duration(context.Background()); err == nil {
		t.Fatal("a recording without a decodable frame must not get a duration")
	}
}

// TestFFmpegRealSample runs the whole analysis on a real recording when
// ADSCAN_SAMPLE names one (ADSCAN_FFMPEG the binary, default "ffmpeg"):
//
//	ADSCAN_SAMPLE=/path/to/copied/recording.ts go test -run RealSample -v ./internal/adscan
func TestFFmpegRealSample(t *testing.T) {
	sample := os.Getenv("ADSCAN_SAMPLE")
	if sample == "" {
		t.Skip("ADSCAN_SAMPLE not set")
	}
	bin := os.Getenv("ADSCAN_FFMPEG")
	if bin == "" {
		bin = "ffmpeg"
	}
	src, err := FFmpeg{Path: bin}.Open(sample)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	res, mask, err := Analyze(context.Background(), src, nil, Options{LearnFrames: 30})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("duration %.0f s, %d probes, mask %s with %d pixels (weak %v)", res.Duration, res.Probes, mask.Corner, mask.Count, mask.Weak())
	for _, b := range res.Blocks {
		t.Logf("ad break %.1f–%.1f s (uncertain %v)", b.Start, b.End, b.Uncertain)
	}
}
