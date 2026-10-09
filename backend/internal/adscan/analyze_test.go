package adscan

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestAnalyzeFindsTheSyntheticBreaks(t *testing.T) {
	src, _ := Synthetic{}.Open("/media/hdd/movie/20260923 2015 - RTL - Test.ts")
	want := src.(*synthetic).Ads()
	res, mask, err := Analyze(context.Background(), src, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if mask.Corner != "top-left" || mask.Weak() || !res.Learned {
		t.Fatalf("mask: corner %s, %d pixels, learned %v", mask.Corner, mask.Count, res.Learned)
	}
	if len(res.Blocks) != len(want) {
		t.Fatalf("found %+v, want %+v", res.Blocks, want)
	}
	for i, b := range res.Blocks {
		if math.Abs(b.Start-want[i].Start) > 1.5 || math.Abs(b.End-want[i].End) > 1.5 {
			t.Errorf("block %d: %.1f–%.1f, want %.1f–%.1f", i, b.Start, b.End, want[i].Start, want[i].End)
		}
		if b.Uncertain {
			t.Errorf("block %d uncertain on clean synthetic frames", i)
		}
	}
	t.Logf("%d probes (60 of them learning)", res.Probes)
	if res.Probes > 60+250 {
		t.Errorf("%d probes; the plan's budget is about 250 per hour plus learning", res.Probes)
	}

	// The second recording of the channel reuses the mask: no learning.
	src2, _ := Synthetic{}.Open("/media/hdd/movie/20260924 2015 - RTL - Test.ts")
	res2, _, err := Analyze(context.Background(), src2, &mask, Options{})
	if err != nil || res2.Learned || len(res2.Blocks) != 3 {
		t.Fatalf("with a known mask: learned %v, %d blocks, %v", res2.Learned, len(res2.Blocks), err)
	}
}

func TestAnalyzeRelearnsAForeignMask(t *testing.T) {
	src, _ := Synthetic{}.Open("/x.ts")
	// A mask from another corner, as if the channel had moved its logo.
	foreign := Mask{Corner: "bottom-right", X0: FrameW - 96, Y0: FrameH - 54, W: 96, H: 54, Dirs: make([]int8, 96*54), Count: 100}
	for i := range foreign.Dirs {
		foreign.Dirs[i] = -1
	}
	for i := 0; i < 100; i++ {
		foreign.Dirs[i*7] = int8(i % 8)
	}
	res, mask, err := Analyze(context.Background(), src, &foreign, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Learned || mask.Corner != "top-left" || len(res.Blocks) != 3 {
		t.Fatalf("learned %v, corner %s, %d blocks", res.Learned, mask.Corner, len(res.Blocks))
	}
}

func TestAnalyzeStopsWhenWaitSaysSo(t *testing.T) {
	src, _ := Synthetic{}.Open("/x.ts")
	stop := errors.New("cancelled")
	calls := 0
	_, _, err := Analyze(context.Background(), src, nil, Options{Wait: func(context.Context) error {
		calls++
		if calls == 3 {
			return stop
		}
		return nil
	}})
	if !errors.Is(err, stop) {
		t.Fatalf("got %v, want the Wait error", err)
	}
}

func TestFindGapsSkipsUnreadableFrames(t *testing.T) {
	// A damaged stretch (no frames 1000–1100 s) inside an ad break.
	probe := func(t float64) (bool, bool) {
		if t >= 1000 && t < 1100 {
			return false, false
		}
		return !(t >= 900 && t < 1300), true
	}
	gaps := findGaps(probe, 3600, DefaultSampling)
	if len(gaps) != 1 || math.Abs(gaps[0].Start-900) > 1 || math.Abs(gaps[0].End-1300) > 1 {
		t.Fatalf("got %+v", gaps)
	}
}

func TestWeakMaskMakesEverythingUncertain(t *testing.T) {
	m := Mask{Count: MinMaskPixels - 1}
	if !m.Weak() {
		t.Fatal("a mask below MinMaskPixels must be weak")
	}
}
