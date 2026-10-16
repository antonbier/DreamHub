package hubs

import (
	"context"
	"log/slog"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/events"
)

// A container that keeps stopping (docs/APPSHUB_PLAN.md, "Autostart and
// crash-loop protection", built in App14). The containers are created with
// "restart on failure, at most 5 times", so a crash loop ends by itself;
// what was missing is telling the person. The watch samples each container's
// restart count (the daemon's own, read by the helper) every half minute,
// and when it grew by three or more within ten minutes the inbox gets one
// notice naming the app, with a link to its logs — once per episode. An
// episode ends when the count has been quiet for the whole window (or the
// container was made again), so a later loop is announced again.

const (
	crashWindow    = 10 * time.Minute
	crashThreshold = 3
	crashTick      = 30 * time.Second
)

type crashSample struct {
	at    time.Time
	count int
}

// crashTracker is the state between two looks.
type crashTracker struct {
	samples   map[string][]crashSample // container id → what was seen in the window
	announced map[string]bool          // app id → the notice for this episode is out
}

// step takes one look at the containers and returns the apps that just entered
// a crash loop (each once per episode).
func (t *crashTracker) step(now time.Time, containers []dockerd.Container) []string {
	if t.samples == nil {
		t.samples, t.announced = map[string][]crashSample{}, map[string]bool{}
	}
	seen := map[string]bool{}
	loops := map[string]bool{} // apps with a container looping right now
	quiet := map[string]bool{} // apps with a container that has been quiet for the window
	var newly []string
	for _, c := range containers {
		seen[c.ID] = true
		list := t.samples[c.ID]
		// A container that crashes at once uses up its restarts within
		// seconds, before the second look can see the count grow. One that
		// is first seen already stopped after that many restarts is a loop
		// that has ended in a stop; a running one with an old count is not.
		stoppedAfterLoop := len(list) == 0 && c.State != "running" && c.RestartCount >= crashThreshold
		// A count that went down: the container was started by hand (the
		// daemon resets it) — a new beginning.
		if n := len(list); n > 0 && c.RestartCount < list[n-1].count {
			list = nil
		}
		list = append(list, crashSample{now, c.RestartCount})
		// Keep what lies inside the window, plus the last sample before it:
		// it shows that the container has been watched for the whole window.
		cut := 0
		for i, s := range list {
			if !s.at.Before(now.Add(-crashWindow)) {
				break
			}
			cut = i
		}
		list = list[cut:]
		t.samples[c.ID] = list
		inside := list
		covered := false
		if list[0].at.Before(now.Add(-crashWindow)) {
			inside, covered = list[1:], true
		}
		base := inside[0] // the oldest look inside the window
		switch {
		case stoppedAfterLoop, c.RestartCount-base.count >= crashThreshold:
			loops[c.AppID()] = true
		case covered && c.RestartCount == list[0].count:
			quiet[c.AppID()] = true
		}
	}
	for id := range t.samples {
		if !seen[id] {
			delete(t.samples, id) // the container is gone (removed, made again)
		}
	}
	for app := range loops {
		if !t.announced[app] {
			t.announced[app] = true
			newly = append(newly, app)
		}
	}
	for app := range t.announced {
		if !loops[app] && quiet[app] {
			delete(t.announced, app) // the episode is over
		}
	}
	return newly
}

// WatchContainers runs until ctx is done; meant for its own goroutine, like
// WatchApps.
func (h *Apps) WatchContainers(ctx context.Context, bus Publisher, logger *slog.Logger) {
	if h.Docker == nil {
		return
	}
	timer := time.NewTimer(crashTick)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			h.crashLook(ctx, bus, logger)
			timer.Reset(crashTick)
		}
	}
}

// crashLook is one look (split out for the tests).
func (h *Apps) crashLook(ctx context.Context, bus Publisher, logger *slog.Logger) {
	containers, err := h.Docker.List(ctx)
	if err != nil {
		return
	}
	h.updMu.Lock()
	newly := h.upd.crash.step(h.now(), containers)
	h.updMu.Unlock()
	for _, id := range newly {
		g, ok := findAppByID(containers, id)
		name := id
		if ok {
			name = groupName(g, g.all()[0].Image)
		}
		if logger != nil {
			logger.Warn("app keeps stopping", "app", id)
		}
		bus.Publish(ctx, events.AppCrashLoop, map[string]any{"appId": id, "name": name})
	}
}
