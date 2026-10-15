package updatewindow

import (
	"testing"
	"time"
)

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tz database")
	}
	return loc
}

func at(loc *time.Location, y int, mo time.Month, d, h, m int) time.Time {
	return time.Date(y, mo, d, h, m, 0, 0, loc)
}

func TestRecordings(t *testing.T) {
	loc := time.UTC
	base := at(loc, 2026, 10, 14, 12, 0)
	rec := func(name string, from, to int) Recording {
		return Recording{Name: name, Begin: base.Add(time.Duration(from) * time.Minute), End: base.Add(time.Duration(to) * time.Minute)}
	}
	guard := 15 * time.Minute
	tests := []struct {
		name       string
		recs       []Recording
		wantClosed bool
		reason     string
		recName    string
		untilMin   int // minutes after base
	}{
		{"none", nil, false, "", "", 0},
		{"running now", []Recording{rec("Tatort", -30, 30)}, true, ReasonRecording, "Tatort", 30},
		{"starts in 10 minutes", []Recording{rec("News", 10, 40)}, true, ReasonStarting, "News", 40},
		{"starts in exactly the guard time (inclusive)", []Recording{rec("News", 15, 40)}, true, ReasonStarting, "News", 40},
		{"starts in 16 minutes", []Recording{rec("News", 16, 40)}, false, "", "", 0},
		{"ended a minute ago", []Recording{rec("Old", -60, -1)}, false, "", "", 0},
		{"ends right now", []Recording{rec("Old", -60, 0)}, false, "", "", 0},
		{"back to back are one block", []Recording{rec("A", -10, 20), rec("B", 20, 50)}, true, ReasonRecording, "A", 50},
		{"next one starts inside the guard of the first's end", []Recording{rec("A", -10, 20), rec("B", 30, 60)}, true, ReasonRecording, "A", 60},
		{"gap longer than the guard is not bridged", []Recording{rec("A", -10, 20), rec("B", 40, 60)}, true, ReasonRecording, "A", 20},
		{"overlapping", []Recording{rec("B", 0, 90), rec("A", -20, 40)}, true, ReasonRecording, "A", 90},
		{"empty interval ignored", []Recording{rec("Zero", 0, 0)}, false, "", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, closed := Recordings(base, tt.recs, guard)
			if closed != tt.wantClosed {
				t.Fatalf("closed = %v, want %v (%+v)", closed, tt.wantClosed, c)
			}
			if !closed {
				return
			}
			if c.Reason != tt.reason || c.Name != tt.recName || !c.Until.Equal(base.Add(time.Duration(tt.untilMin)*time.Minute)) {
				t.Errorf("got %+v, want reason %s name %s until +%dm", c, tt.reason, tt.recName, tt.untilMin)
			}
		})
	}
}

func TestRecordings_NoGuard(t *testing.T) {
	now := at(time.UTC, 2026, 10, 14, 12, 0)
	r := Recording{Name: "X", Begin: now.Add(5 * time.Minute), End: now.Add(30 * time.Minute)}
	if _, closed := Recordings(now, []Recording{r}, 0); closed {
		t.Error("with no guard a recording that has not begun does not block")
	}
	if _, closed := Recordings(now, []Recording{r}, -time.Hour); closed {
		t.Error("a negative guard is no guard")
	}
}

func TestNight_Allows(t *testing.T) {
	loc := berlin(t)
	tests := []struct {
		name  string
		night Night
		t     time.Time
		want  bool
	}{
		{"off allows anything", Night{false, 120, 300}, at(loc, 2026, 10, 14, 12, 0), true},
		{"inside", Night{true, 120, 300}, at(loc, 2026, 10, 14, 3, 0), true},
		{"start is inside", Night{true, 120, 300}, at(loc, 2026, 10, 14, 2, 0), true},
		{"end is outside", Night{true, 120, 300}, at(loc, 2026, 10, 14, 5, 0), false},
		{"before", Night{true, 120, 300}, at(loc, 2026, 10, 14, 1, 59), false},
		{"across midnight, evening", Night{true, 23 * 60, 4 * 60}, at(loc, 2026, 10, 14, 23, 30), true},
		{"across midnight, early", Night{true, 23 * 60, 4 * 60}, at(loc, 2026, 10, 14, 3, 59), true},
		{"across midnight, day", Night{true, 23 * 60, 4 * 60}, at(loc, 2026, 10, 14, 12, 0), false},
		{"equal start and end is all day", Night{true, 120, 120}, at(loc, 2026, 10, 14, 12, 0), true},
		// 2026-10-25: the clocks go back at 03:00 CEST to 02:00 CET, so the
		// wall clock reads 02:30 twice. Both are inside a 02:00-03:00 window.
		{"DST end, first 02:30", Night{true, 120, 180}, time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC).In(loc), true},
		{"DST end, second 02:30", Night{true, 120, 180}, time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC).In(loc), true},
		{"DST end, 03:00 CET is outside 02:00-03:00", Night{true, 120, 180}, time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC).In(loc), false},
		{"DST end, 03:30 CET inside a window across midnight to 04:00", Night{true, 23 * 60, 4 * 60}, time.Date(2026, 10, 25, 2, 30, 0, 0, time.UTC).In(loc), true},
		{"DST end, 04:00 CET outside it", Night{true, 23 * 60, 4 * 60}, time.Date(2026, 10, 25, 3, 0, 0, 0, time.UTC).In(loc), false},
		// 2026-03-29: 02:00 CET jumps to 03:00 CEST; 02:30 does not exist.
		{"DST start, 03:10 CEST inside 02:30-04:00", Night{true, 150, 240}, at(loc, 2026, 3, 29, 3, 10), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.night.Allows(tt.t); got != tt.want {
				t.Errorf("Allows(%s) = %v, want %v", tt.t.Format(time.RFC3339), got, tt.want)
			}
		})
	}
}

func TestNight_NextOpen(t *testing.T) {
	loc := berlin(t)
	n := Night{true, 150, 240} // 02:30-04:00
	tests := []struct {
		name string
		from time.Time
		want time.Time
	}{
		{"open now", at(loc, 2026, 10, 14, 3, 0), at(loc, 2026, 10, 14, 3, 0)},
		{"evening: tonight's start tomorrow", at(loc, 2026, 10, 14, 20, 0), at(loc, 2026, 10, 15, 2, 30)},
		{"just after midnight: this night's start", at(loc, 2026, 10, 15, 0, 5), at(loc, 2026, 10, 15, 2, 30)},
		{"after the end", at(loc, 2026, 10, 14, 4, 0), at(loc, 2026, 10, 15, 2, 30)},
		// Over the clock change at the end of October the next start is
		// still 02:30 on the wall clock.
		{"into the repeated hour", at(loc, 2026, 10, 24, 12, 0), at(loc, 2026, 10, 25, 2, 30)},
		{"after it", at(loc, 2026, 10, 25, 12, 0), at(loc, 2026, 10, 26, 2, 30)},
		// The start that does not exist on 2026-03-29 is read as the first
		// minute after the gap.
		{"into the skipped hour", at(loc, 2026, 3, 28, 12, 0), at(loc, 2026, 3, 29, 3, 30)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := n.NextOpen(tt.from)
			if !got.Equal(tt.want) {
				t.Errorf("NextOpen(%s) = %s, want %s", tt.from.Format(time.RFC3339), got.Format(time.RFC3339), tt.want.Format(time.RFC3339))
			}
			if !got.Before(tt.from) && !n.Allows(got) {
				t.Errorf("the answer %s is itself outside the window", got.Format(time.RFC3339))
			}
		})
	}
}

func TestDecide(t *testing.T) {
	loc := berlin(t)
	night := Night{true, 120, 300}
	noon := at(loc, 2026, 10, 14, 12, 0)
	rec := []Recording{{Name: "Film", Begin: noon.Add(-time.Hour), End: noon.Add(time.Hour)}}

	// A manual update (night window not used) is held back by a recording only.
	if c, closed := Decide(noon, nil, DefaultGuard, night, false); closed {
		t.Errorf("manual, no recording: closed %+v", c)
	}
	c, closed := Decide(noon, rec, DefaultGuard, night, false)
	if !closed || c.Reason != ReasonRecording || !c.Until.Equal(noon.Add(time.Hour)) {
		t.Errorf("manual during a recording: %+v %v", c, closed)
	}
	// An automatic one also waits for the night.
	c, closed = Decide(noon, nil, DefaultGuard, night, true)
	if !closed || c.Reason != ReasonNight || !c.Until.Equal(at(loc, 2026, 10, 15, 2, 0)) {
		t.Errorf("automatic at noon: %+v %v", c, closed)
	}
	// A recording in the night: the recording is named first, and the night
	// window's own start is not mixed in.
	three := at(loc, 2026, 10, 15, 3, 0)
	c, closed = Decide(three, []Recording{{Name: "Late", Begin: three.Add(-time.Hour), End: three.Add(30 * time.Minute)}}, DefaultGuard, night, true)
	if !closed || c.Reason != ReasonRecording || c.Name != "Late" {
		t.Errorf("night with a recording: %+v %v", c, closed)
	}
	if _, closed = Decide(three, nil, DefaultGuard, night, true); closed {
		t.Error("night, no recording: should be open")
	}
}

func TestParseClock(t *testing.T) {
	for in, want := range map[string]int{"00:00": 0, "02:30": 150, "23:59": 1439} {
		got, err := ParseClock(in)
		if err != nil || got != want {
			t.Errorf("ParseClock(%q) = %d, %v", in, got, err)
		}
		if Clock(want) != in {
			t.Errorf("Clock(%d) = %q", want, Clock(want))
		}
	}
	for _, bad := range []string{"", "2:30", "24:00", "12:60", "ab:cd", "12-30", "12:3", "+1:30", " 1:30"} {
		if _, err := ParseClock(bad); err == nil {
			t.Errorf("ParseClock(%q) should fail", bad)
		}
	}
}
