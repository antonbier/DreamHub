// Package mock is journald's mock reader (PRINCIPLES.md, "Mock-first
// development") — a fixed, small journal so AdminHub's log viewer is usable
// in local dev without a real systemd.
package mock

import (
	"context"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/journald"
)

// New returns a reader over a fixed set of entries, "recorded" a few
// minutes apart ending now.
func New() *Reader {
	base := time.Now().Add(-20 * time.Minute)
	entries := []journald.Entry{
		{Time: base, Unit: "enigma2.service", Level: journald.LevelInfo, Message: "enigma2 started"},
		{Time: base.Add(2 * time.Minute), Unit: "dreamhub.service", Level: journald.LevelInfo, Message: "dreamhub starting"},
		{Time: base.Add(5 * time.Minute), Unit: "enigma2.service", Level: journald.LevelWarning, Message: "tuner 0: signal weak on transponder 11493 H"},
		{Time: base.Add(11 * time.Minute), Unit: "dreamhub.service", Level: journald.LevelErr, Message: "webcontrol: 502 from /web/timeradd"},
		{Time: base.Add(18 * time.Minute), Unit: "serienrecorder.service", Level: journald.LevelInfo, Message: "scan complete: 4 new episodes tracked"},
	}
	return &Reader{entries: entries}
}

type Reader struct{ entries []journald.Entry }

func (r *Reader) Read(ctx context.Context, q journald.Query) ([]journald.Entry, error) {
	var out []journald.Entry
	for _, e := range r.entries {
		if q.Unit != "" && e.Unit != q.Unit {
			continue
		}
		if q.MaxLevel > 0 && q.MaxLevel < journald.LevelDebug && e.Level > q.MaxLevel {
			continue
		}
		if q.Text != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(q.Text)) {
			continue
		}
		out = append(out, e)
	}
	limit := q.Limit
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}
