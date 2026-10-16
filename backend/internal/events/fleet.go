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
	started map[string]bool
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
			f.started = map[string]bool{}
		}
		for _, id := range f.IDs() {
			if !f.started[id] {
				f.started[id] = true
				go f.New(id).Run(ctx)
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
