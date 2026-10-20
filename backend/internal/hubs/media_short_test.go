package hubs

import (
	"testing"
	"time"
)

func TestParseMovieLength(t *testing.T) {
	for in, want := range map[string]time.Duration{"43:20": 43*time.Minute + 20*time.Second, "1:12:05": time.Hour + 12*time.Minute + 5*time.Second} {
		if got, ok := parseMovieLength(in); !ok || got != want {
			t.Errorf("%q = %v %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "90", "a:b", "-1:00", "1:2:3:4"} {
		if _, ok := parseMovieLength(in); ok {
			t.Errorf("%q should not parse", in)
		}
	}
}

// The reference box's findings (2026-10-20): a film announced for 2 h 04
// and recorded for 57 min is cut short; normal margins are not.
func TestCutShort(t *testing.T) {
	m := time.Minute
	for _, c := range []struct {
		rec, sched time.Duration
		want       bool
	}{
		{57 * m, 124 * m, true},
		{97 * m, 140 * m, true},
		{140 * m, 140 * m, false},
		{150 * m, 140 * m, false}, // margins
		{125 * m, 140 * m, false}, // a late start and early end
		{40 * m, 45 * m, false},   // gap under 5 minutes... and over 85 %
		{20 * m, 25 * m, true},    // 80 %, a gap of 5 minutes
		{3 * m, 8 * m, false},     // too short a programme to say
	} {
		got := cutShort(c.rec, c.sched)
		if got != c.want {
			t.Errorf("cutShort(%v, %v) = %v", c.rec, c.sched, got)
		}
	}
}
