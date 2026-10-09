package hubs

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/events"
	"github.com/antonbier/DreamHub/backend/internal/systemd"
)

// AdminHub's service watch: "detection, not automation" (docs/archive/V1_PLAN.md,
// WP7) — it notices a service of the box going down or coming back and
// says so (the notification inbox, push if switched on), it never restarts
// anything itself. A unit counts as down only once it reads "failed" on
// two checks in a row, so one that systemd restarts by itself within a
// minute doesn't send anything; "back" follows only a "down" that was
// announced. Units already failed when DreamHub starts aren't announced
// (every restart of DreamHub would repeat them) — Admin shows them anyway.
// DreamHub itself isn't watched: it can't report its own failure.

const (
	serviceWatchInterval = time.Minute
	serviceWatchFirst    = 90 * time.Second // let the box settle after a boot
)

// serviceWatch is the state between checks.
type serviceWatch struct {
	initialized bool
	failedOnce  map[string]bool // read "failed" on the last check
	announced   map[string]bool // "down" sent, "back" still due
}

func newServiceWatch() *serviceWatch {
	return &serviceWatch{failedOnce: map[string]bool{}, announced: map[string]bool{}}
}

// step compares one check's units with the state so far and returns the
// events to publish.
func (w *serviceWatch) step(units []systemd.Unit) (failed, recovered []systemd.Unit) {
	for _, u := range units {
		if u.Name == "dreamhub.service" {
			continue
		}
		isFailed := u.Active == "failed"
		switch {
		case !w.initialized:
			// The state at start is the baseline: neither announced nor due.
			if isFailed {
				w.announced[u.Name] = true // already down: no "down", but "back" is worth saying
			}
		case isFailed && w.failedOnce[u.Name] && !w.announced[u.Name]:
			w.announced[u.Name] = true
			failed = append(failed, u)
		case u.Active == "active" && w.announced[u.Name]:
			delete(w.announced, u.Name)
			recovered = append(recovered, u)
		}
		w.failedOnce[u.Name] = isFailed
	}
	w.initialized = true
	return failed, recovered
}

// WatchServices runs until ctx is done; meant for its own goroutine.
func (h *Admin) WatchServices(ctx context.Context, bus Publisher, logger *slog.Logger) {
	if h.Systemd == nil {
		return
	}
	w := newServiceWatch()
	timer := time.NewTimer(serviceWatchFirst)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			units, err := h.Systemd.Status(ctx, h.services())
			if err != nil {
				if logger != nil {
					logger.Debug("service watch: status failed", "error", err)
				}
				// No systemd (a development machine): stop asking.
				if err == systemd.ErrUnavailable {
					return
				}
			} else {
				failed, recovered := w.step(units)
				for _, u := range failed {
					bus.Publish(ctx, events.ServiceFailed, map[string]any{"name": strings.TrimSuffix(u.Name, ".service"), "unit": u.Name})
				}
				for _, u := range recovered {
					bus.Publish(ctx, events.ServiceRecovered, map[string]any{"name": strings.TrimSuffix(u.Name, ".service"), "unit": u.Name})
				}
			}
			timer.Reset(serviceWatchInterval)
		}
	}
}
