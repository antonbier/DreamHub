// Package logging provides DreamHub's runtime-switchable log level
// (CONCEPT.md, "AdminHub"): info by default, debug on request, and debug
// always reverts to info after a set time so logs don't fill up.
package logging

import (
	"log/slog"
	"sync"
	"time"
)

// Level controls the process-wide log level.
type Level struct {
	v   slog.LevelVar
	now func() time.Time

	mu    sync.Mutex
	until time.Time
	timer *time.Timer
}

// NewLevel returns a controller starting at info.
func NewLevel(now func() time.Time) *Level {
	if now == nil {
		now = time.Now
	}
	l := &Level{now: now}
	l.v.Set(slog.LevelInfo)
	return l
}

// Leveler is passed to slog handlers.
func (l *Level) Leveler() slog.Leveler { return &l.v }

// SetDebug switches to debug for d, after which it reverts to info. It
// returns when the revert will happen.
func (l *Level) SetDebug(d time.Duration) time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.timer != nil {
		l.timer.Stop()
	}
	l.v.Set(slog.LevelDebug)
	l.until = l.now().Add(d).UTC()
	l.timer = time.AfterFunc(d, l.SetInfo)
	return l.until
}

// SetInfo switches back to info immediately.
func (l *Level) SetInfo() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
	l.v.Set(slog.LevelInfo)
	l.until = time.Time{}
}

// Get returns "debug" or "info" and, for debug, when it reverts.
func (l *Level) Get() (string, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.v.Level() == slog.LevelDebug {
		return "debug", l.until
	}
	return "info", time.Time{}
}
