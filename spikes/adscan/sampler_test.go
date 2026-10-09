package main

import (
	"math"
	"testing"
)

// timeline is a synthetic recording: the logo is off inside the given
// spans and on everywhere else.
func timeline(off ...span) probeFunc {
	return func(t float64) bool {
		for _, s := range off {
			if t >= s.Start && t < s.End {
				return false
			}
		}
		return true
	}
}

var defaults = sampling{Step: 25, Precision: 1, MinBlock: 60, MinGap: 20}

func TestFindAdsOneHourThreeBreaks(t *testing.T) {
	// A typical commercial-channel hour: three breaks of 4–7 minutes, one
	// weak keyframe (a logo on a bright background scored as "off") and a
	// break that is split by a 10 s station ident with the logo.
	probe := timeline(
		span{612.3, 931.8},
		span{1500, 1500.5}, // weak frame
		span{2104.6, 2300}, span{2310, 2420.2},
		span{3200.4, 3600},
	)
	ads, probes := findAds(probe, 3600, defaults)
	want := []span{{612.3, 931.8}, {2104.6, 2420.2}, {3200.4, 3600}}
	if len(ads) != len(want) {
		t.Fatalf("got %v, want %v", ads, want)
	}
	for i := range want {
		if math.Abs(ads[i].Start-want[i].Start) > 1 || math.Abs(ads[i].End-want[i].End) > 1 {
			t.Errorf("block %d: got %.1f–%.1f, want %.1f–%.1f", i, ads[i].Start, ads[i].End, want[i].Start, want[i].End)
		}
	}
	// The plan's estimate was 150–250 decoded frames per hour.
	t.Logf("%d probes for one hour", probes)
	if probes > 250 {
		t.Errorf("%d probes, the plan's budget is 250 per hour", probes)
	}
}

func TestFindAdsNoAds(t *testing.T) {
	ads, probes := findAds(timeline(), 3600, defaults)
	if len(ads) != 0 {
		t.Fatalf("got %v on a recording without ads", ads)
	}
	if probes > 3600/25+2 {
		t.Errorf("%d probes; without boundaries only the coarse samples are needed", probes)
	}
}

func TestFindAdsStartsInBreak(t *testing.T) {
	// The timer's pre-margin catches the end of the previous ad break.
	ads, _ := findAds(timeline(span{0, 140.2}), 1800, defaults)
	if len(ads) != 1 || ads[0].Start != 0 || math.Abs(ads[0].End-140.2) > 1 {
		t.Fatalf("got %v", ads)
	}
}
