package hubs

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/events"
)

// AppsHub's update watch (docs/APPSHUB_PLAN.md, App5, "Updates through
// the existing notification inbox"): the same "detection, not
// automation" shape as AdminHub's own package watch
// (admin_packages_watch.go) — this never pulls or restarts anything
// itself, it only tells the admin a newer image exists so they can act
// on it through the normal install/remove plan flow (there's no
// "update in place" action; an update is a remove-then-reinstall the
// admin chooses to do, same as the plan's own "Updates" decision text).
// Only curated-catalog apps are checked (free-search installs don't
// exist yet — App4's own scope), one image:tag lookup per installed app
// against the registry's current digest (dockerhub.Client.Digest),
// compared with what's actually running (dockerd.Client.ImageDigest).
const (
	appWatchTick  = 15 * time.Minute // how often the settings are looked at; the check itself runs at the chosen interval
	appWatchFirst = 2 * time.Minute  // let the box settle after a boot, same as the package watch
)

// appWatch is the state between checks — same "already announced" shape
// as packageWatch, keyed by AppID rather than package name.
type appWatch struct {
	initialized bool
	known       map[string]bool // currently upgradable, already accounted for
}

func newAppWatch() *appWatch {
	return &appWatch{known: map[string]bool{}}
}

// step compares one check's upgradable AppIDs with the state so far and
// returns the ones that just became upgradable. An app that stops being
// upgradable (updated by a remove+reinstall, or removed) drops out of
// known, so it's announced again if it ever becomes upgradable a second
// time — the same rule packageWatch.step already uses.
func (w *appWatch) step(upgradable []string) []string {
	seen := map[string]bool{}
	var newly []string
	for _, id := range upgradable {
		seen[id] = true
		if w.initialized && !w.known[id] {
			newly = append(newly, id)
		}
	}
	w.known = seen
	w.initialized = true
	return newly
}

// imageUpgradable reports whether c's running image digest no longer
// matches the registry's current one for the same tag — "" (false) on
// any lookup failure (daemon hiccup, registry unreachable, no digest
// recorded for a locally built image) rather than guessing. Shared
// between the watch (checkUpgradable, below) and listInstalled's own
// per-row badge, so both ask the exact same question the exact same way.
func (h *Apps) imageUpgradable(ctx context.Context, c dockerd.Container) bool {
	if h.Docker == nil || h.DockerHub == nil {
		return false
	}
	app, ok := appcatalog.ByID(c.AppID())
	if !ok {
		return false // not a curated app (shouldn't happen yet — App4 only installs those)
	}
	image, tag := app.Image, "latest"
	if i := strings.LastIndex(c.Image, ":"); i >= 0 {
		image, tag = c.Image[:i], c.Image[i+1:]
	}
	current, err := h.Docker.ImageDigest(ctx, c.ID)
	if err != nil || current == "" {
		return false
	}
	upstream, err := h.DockerHub.Digest(ctx, image, tag)
	if err != nil || upstream == "" {
		return false
	}
	return current != upstream
}

// checkUpgradable returns the AppIDs of every installed, curated app
// whose running image digest no longer matches the registry's current
// one for the same tag. A lookup failure for one app (daemon hiccup,
// registry unreachable) is skipped, not fatal to the rest of the check.
func (h *Apps) checkUpgradable(ctx context.Context) []string {
	if h.Docker == nil || h.DockerHub == nil {
		return nil
	}
	containers, err := h.Docker.List(ctx)
	if err != nil {
		return nil
	}
	var upgradable []string
	for _, c := range containers {
		// One entry per app, even when several of its containers are.
		if !slices.Contains(upgradable, c.AppID()) && h.imageUpgradable(ctx, c) {
			upgradable = append(upgradable, c.AppID())
		}
	}
	return upgradable
}

// WatchApps runs until ctx is done; meant for its own goroutine, the
// same shape every other watch in this codebase (WatchServices,
// WatchPackages) uses.
func (h *Apps) WatchApps(ctx context.Context, bus Publisher, logger *slog.Logger) {
	w := newAppWatch()
	timer := time.NewTimer(appWatchFirst)
	defer timer.Stop()
	var last time.Time
	first := true
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			cfg := h.config(ctx)
			now := h.now()
			// Switched off in the Docker settings: no request to any registry.
			if cfg.UpdateNotify && (first || updateDue(last, now, cfg.UpdateHours)) {
				first = false
				last = now
				upgradable := h.checkUpgradable(ctx)
				if newly := w.step(upgradable); len(newly) > 0 {
					for _, id := range newly {
						app, ok := appcatalog.ByID(id)
						name := id
						if ok {
							name = app.Name
						}
						bus.Publish(ctx, events.AppUpgradable, map[string]any{"appId": id, "name": name})
					}
				} else if logger != nil && len(upgradable) == 0 {
					logger.Debug("app watch: nothing upgradable")
				}
			}
			timer.Reset(appWatchTick)
		}
	}
}

// updateDue reports whether the check is due again at the chosen interval.
func updateDue(last, now time.Time, hours int) bool {
	return now.Sub(last) >= time.Duration(hours)*time.Hour
}
