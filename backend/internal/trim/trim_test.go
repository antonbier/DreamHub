package trim

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/adscan"
)

func TestKeep(t *testing.T) {
	cases := []struct {
		name string
		cut  []adscan.Span
		want []adscan.Span
	}{
		{"three breaks", []adscan.Span{{Start: 600, End: 900}, {Start: 2000, End: 2300}, {Start: 3300, End: 3600}},
			[]adscan.Span{{Start: 0, End: 600}, {Start: 900, End: 2000}, {Start: 2300, End: 3300}}},
		{"overlapping and unsorted", []adscan.Span{{Start: 1000, End: 1500}, {Start: 800, End: 1200}},
			[]adscan.Span{{Start: 0, End: 800}, {Start: 1500, End: 3600}}},
		{"from the start", []adscan.Span{{Start: 0, End: 120}},
			[]adscan.Span{{Start: 120, End: 3600}}},
		{"a sliver between two breaks is dropped", []adscan.Span{{Start: 600, End: 900}, {Start: 901, End: 1200}},
			[]adscan.Span{{Start: 0, End: 600}, {Start: 1200, End: 3600}}},
	}
	for _, c := range cases {
		if got := Keep(3600, c.cut); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	if l := Length(Keep(3600, cases[0].cut)); l != 2700 {
		t.Errorf("length %v, want 2700", l)
	}
}

func TestConcatListQuotesAndOffsets(t *testing.T) {
	got := concatList("/media/hdd/movie/Grey's Anatomy.ts", 4469.755, []adscan.Span{{Start: 0, End: 20}, {Start: 40, End: 60}}, 60)
	// The last part runs to the file's end: no out point.
	want := "ffconcat version 1.0\nfile '/media/hdd/movie/Grey'\\''s Anatomy.ts'\ninpoint 4469.755\noutpoint 4489.755\n" +
		"file '/media/hdd/movie/Grey'\\''s Anatomy.ts'\ninpoint 4509.755\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestReadProgress(t *testing.T) {
	var last float64
	readProgress(strings.NewReader("frame=10\ntotal_size=1500\nout_time_us=N/A\ntotal_size=2500\nprogress=end\n"), func(s float64) { last = s })
	if last != 2500 {
		t.Fatalf("last %v, want 2.5", last)
	}
}

// TestFFmpegRealSample cuts a real recording when ADSCAN_SAMPLE names one
// (ADSCAN_FFMPEG the binary): keeps 0–20 s and 40 s to the end.
func TestFFmpegRealSample(t *testing.T) {
	sample := os.Getenv("ADSCAN_SAMPLE")
	if sample == "" {
		t.Skip("ADSCAN_SAMPLE not set")
	}
	bin := os.Getenv("ADSCAN_FFMPEG")
	if bin == "" {
		bin = "ffmpeg"
	}
	dst := filepath.Join(t.TempDir(), "out.ts")
	c := FFmpeg{Path: bin, Probe: adscan.FFmpeg{Path: bin}}
	var share float64
	keep := []adscan.Span{{Start: 0, End: 20}, {Start: 40, End: 60}}
	if err := c.Cut(context.Background(), sample, dst, keep, 60, func(s float64) { share = s }); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(dst)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("no output: %v", err)
	}
	src, _ := adscan.FFmpeg{Path: bin}.Open(dst)
	defer src.Close()
	d, err := src.Duration(context.Background())
	// Duration counts to the start of the last 1.5 MB, a few seconds short.
	t.Logf("output %d bytes, %.1f s long by keyframes, progress %.2f", fi.Size(), d, share)
	if err != nil || d < 34 || d > 42 {
		t.Fatalf("output duration %v (%v), want about 40 s", d, err)
	}
	if share < 0.9 {
		t.Errorf("progress ended at %.2f", share)
	}
	// Writing over an existing file is refused (-n).
	if err := c.Cut(context.Background(), sample, dst, keep, 60, nil); err == nil {
		t.Fatal("cut over an existing file")
	}
}
