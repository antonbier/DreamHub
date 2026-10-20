package hubs

import (
	"strconv"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/eit"
)

// Recordings cut short (#44, phase 3, "suspicious recordings"): the box
// keeps the announced programme next to a recording (<name>.eit); a
// recording far shorter than that stopped early — a restart, a full disk,
// a timer that was ended by hand. Seen on the reference box 2026-10-20:
// recordings of 57 min for a film announced as 2 h 04 min, confirmed with
// ffprobe on the file.
//
// Not flagged: a recording the user cut (ad cutting and cutting by hand
// copy the .eit, and the cut version is meant to be shorter), a short
// programme (under 10 minutes), a gap under 5 minutes, and anything over
// 85 % of the announced length (margins and a late start are normal).
const (
	shortMinProgramme = 10 * time.Minute
	shortMinGap       = 5 * time.Minute
)

// scheduledLength is the announced length of the programme a recording
// was made of; 0 when there is no (readable) .eit.
func (h *Media) scheduledLength(tsPath string) time.Duration {
	if h.FS == nil || !strings.HasSuffix(tsPath, ".ts") {
		return 0
	}
	b, err := readRegular(h.FS, strings.TrimSuffix(tsPath, ".ts")+".eit")
	if err != nil {
		return 0
	}
	ev, err := eit.Parse(b)
	if err != nil {
		return 0
	}
	return ev.Duration
}

// cutShort reports whether a recording of the given length stopped well
// before its programme did.
func cutShort(recorded, scheduled time.Duration) bool {
	return scheduled >= shortMinProgramme && recorded < scheduled*85/100 && scheduled-recorded >= shortMinGap
}

// parseMovieLength reads the movie list's length: "43:20" is minutes and
// seconds, "1:12:05" hours, minutes, seconds.
func parseMovieLength(s string) (time.Duration, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return 0, false
		}
		n[i] = v
	}
	if len(parts) == 2 {
		return time.Duration(n[0])*time.Minute + time.Duration(n[1])*time.Second, true
	}
	return time.Duration(n[0])*time.Hour + time.Duration(n[1])*time.Minute + time.Duration(n[2])*time.Second, true
}
