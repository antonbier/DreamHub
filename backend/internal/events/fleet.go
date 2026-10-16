package events

import (
	"context"
	"sync"
	"time"
)

// Fleet runs one Poller per box and starts a new one when a box is added
// while DreamHub runs. Each box keeps its own snapshots, so one box
// finishing a recording is never mistaken for another's.
type Fleet struct {
	// IDs lists the registered boxes.
	IDs func() []string
	// New builds the poller for one box.
	New func(id string) *Poller
	// Check is how often IDs is asked for new boxes; 5 s when zero.
	Check time.Duration

	mu      sync.Mutex
	started map[string]context.CancelFunc
}

func containsID(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// Run starts the pollers and watches for new boxes until ctx is done.
func (f *Fleet) Run(ctx context.Context) {
	check := f.Check
	if check <= 0 {
		check = 5 * time.Second
	}
	for {
		f.mu.Lock()
		if f.started == nil {
			f.started = map[string]context.CancelFunc{}
		}
		ids := f.IDs()
		for _, id := range ids {
			if f.started[id] == nil {
				pctx, cancel := context.WithCancel(ctx)
				f.started[id] = cancel
				go f.New(id).Run(pctx)
			}
		}
		// A removed box is not polled any more.
		for id, cancel := range f.started {
			if !containsID(ids, id) {
				cancel()
				delete(f.started, id)
			}
		}
		f.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(check):
		}
	}
}
