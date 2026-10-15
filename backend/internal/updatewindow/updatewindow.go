// Package updatewindow decides when updates may run (docs/APPSHUB_PLAN.md,
// App14): never while a recording runs or is about to start, and — for the
// automatic ones, if the person asked for it — only at night. Pure: the
// caller reads the timers and the clock; nothing here talks to the box.
package updatewindow

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// DefaultGuard is how long before a recording starts updates already stay
// out of the way.
const DefaultGuard = 15 * time.Minute

// Recording is one timer that records (or will record).
type Recording struct {
	Name       string
	Begin, End time.Time
}

// Reasons a window is closed.
const (
	ReasonRecording = "recording" // a recording runs now
	ReasonStarting  = "starting"  // one starts within the guard time
	ReasonNight     = "night"     // outside the night window
)

// Closed says why updates can't run now and when they can again (the
// first moment nothing in the way remains; for a recording that is when it
// has ended).
type Closed struct {
	Reason string
	// Name of the recording in the way (empty for the night window).
	Name  string
	Until time.Time
}

// Recordings reports whether updates are held back at now: a recording
// is running, or one begins within guard. Consecutive or overlapping
// recordings are one block, so Until is when the last of them has ended.
func Recordings(now time.Time, recs []Recording, guard time.Duration) (Closed, bool) {
	if guard < 0 {
		guard = 0
	}
	list := make([]Recording, 0, len(recs))
	for _, r := range recs {
		if r.End.After(r.Begin) {
			list = append(list, r)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Begin.Before(list[j].Begin) })

	var first *Recording
	until := now
	for i := range list {
		r := list[i]
		// The block of r: from guard before it begins until it has ended.
		if !now.Before(r.End) {
			continue // over
		}
		if first == nil {
			if now.Before(r.Begin.Add(-guard)) {
				continue // far enough away (the list is sorted: so are the rest, but overlaps may start earlier)
			}
			first = &list[i]
			until = r.End
			continue
		}
		// Chain: a later recording whose guard reaches the end so far.
		if !r.Begin.Add(-guard).After(until) && r.End.After(until) {
			until = r.End
		}
	}
	if first == nil {
		return Closed{}, false
	}
	reason := ReasonStarting
	if !now.Before(first.Begin) {
		reason = ReasonRecording
	}
	return Closed{Reason: reason, Name: first.Name, Until: until}, true
}

// Night is the optional "only update between" window, in minutes since
// local midnight. Start after End crosses midnight; Start equal to End
// means the whole day (nothing is held back).
type Night struct {
	Enabled    bool
	Start, End int
}

// ParseClock reads "HH:MM" (24 hours) into minutes since midnight.
func ParseClock(s string) (int, error) {
	var h, m int
	if len(s) != 5 || s[2] != ':' {
		return 0, fmt.Errorf("updatewindow: %q is not HH:MM", s)
	}
	if _, err := fmt.Sscanf(s, "%02d:%02d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 || strings.ContainsAny(s, " +-") {
		return 0, fmt.Errorf("updatewindow: %q is not HH:MM", s)
	}
	return h*60 + m, nil
}

// Clock formats minutes since midnight as "HH:MM".
func Clock(min int) string { return fmt.Sprintf("%02d:%02d", min/60, min%60) }

// Allows reports whether t (read on the wall clock of its own location) lies
// inside the window. A wall-clock time that happens twice (the hour a
// clock change repeats) is inside the window both times.
func (n Night) Allows(t time.Time) bool {
	if !n.Enabled || n.Start == n.End {
		return true
	}
	m := t.Hour()*60 + t.Minute()
	if n.Start < n.End {
		return m >= n.Start && m < n.End
	}
	return m >= n.Start || m < n.End
}

// NextOpen is t itself when the window is open, else the next time its start
// is on the clock (a start the clock skips is read as the first minute
// after the gap).
func (n Night) NextOpen(t time.Time) time.Time {
	if n.Allows(t) {
		return t
	}
	loc := t.Location()
	day := time.Date(t.Year(), t.Month(), t.Day(), n.Start/60, n.Start%60, 0, 0, loc)
	if !day.After(t) {
		day = time.Date(t.Year(), t.Month(), t.Day()+1, n.Start/60, n.Start%60, 0, 0, loc)
	}
	return day
}

// Decide combines both: it reports why updates can't run at now, or false
// when they may. Recordings come first (the more urgent reason). When the
// night window is the reason, Until is its next opening; a recording that
// ends before then doesn't change that. night applies only when
// useNight is set (automatic updates).
func Decide(now time.Time, recs []Recording, guard time.Duration, night Night, useNight bool) (Closed, bool) {
	if c, closed := Recordings(now, recs, guard); closed {
		return c, true
	}
	if useNight && !night.Allows(now) {
		return Closed{Reason: ReasonNight, Until: night.NextOpen(now)}, true
	}
	return Closed{}, false
}
