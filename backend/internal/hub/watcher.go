package hub

import (
	"fmt"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/status"
)

// StatusWatcher detects a hub's level actually changing between two calls
// to Diff and reports it — the closest thing to a hub-emitted "event" v1
// has (CONCEPT.md's core event bus is still open; ROADMAP.md). There's no
// background poller: Diff piggybacks on whatever already computes hub
// statuses per request (GET /api/v1/navigation, /api/v1/hubs), so a
// transition is only ever noticed the next time one of those is called —
// an honest limitation, not a hidden one.
type StatusWatcher struct {
	mu   sync.Mutex
	last map[string]status.Level
}

// NewStatusWatcher returns an empty watcher.
func NewStatusWatcher() *StatusWatcher {
	return &StatusWatcher{last: map[string]status.Level{}}
}

// Transition is one hub's level actually changing.
type Transition struct {
	HubID   string
	HubName string
	From    status.Level
	To      status.Level
}

// Summary is an English, non-i18n description — matches how the change log
// already records things like "Updated settings: general.language"
// (internal/settings.Service.Update): plain developer/audit text, not a
// translated Problem.
func (t Transition) Summary() string {
	return fmt.Sprintf("%s became %s", t.HubName, t.To)
}

// Diff compares statuses against the last call and returns every hub whose
// level actually changed. The very first call for a given registry never
// reports anything — there's no "before" to compare against yet.
func (w *StatusWatcher) Diff(statuses []Status) []Transition {
	w.mu.Lock()
	defer w.mu.Unlock()

	var out []Transition
	for _, s := range statuses {
		prev, known := w.last[s.ID]
		if known && prev != s.Level {
			out = append(out, Transition{HubID: s.ID, HubName: s.Name, From: prev, To: s.Level})
		}
		w.last[s.ID] = s.Level
	}
	return out
}
