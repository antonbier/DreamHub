package hubs

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/apt"
	"github.com/antonbier/DreamHub/backend/internal/events"
)

// AdminHub's package-update watch: "detection, not automation"
// (docs/archive/V1_PLAN.md, WP7) — found out about through the existing
// notification inbox, the one thing WebAdmin categorically can't do
// (docs/ADMINHUB_PLAN.md, "Update notifications, not a badge you have to
// go find"). It never installs anything itself; A2's install/remove plan
// is what an admin acts on afterwards. Packages already upgradable when
// DreamHub starts aren't announced (every restart would repeat them,
// same reasoning as admin_watch.go's service failures) — the browser
// shows them anyway.
const (
	packageWatchInterval = time.Hour
	packageWatchFirst    = 2 * time.Minute // let the box settle after a boot
)

// packageWatch is the state between checks.
type packageWatch struct {
	initialized bool
	known       map[string]bool // currently upgradable, already accounted for
}

func newPackageWatch() *packageWatch {
	return &packageWatch{known: map[string]bool{}}
}

// step compares one check's packages with the state so far and returns the
// ones that just became upgradable. A package that stops being upgradable
// (installed, or removed) drops out of known, so it's announced again if
// it ever becomes upgradable a second time.
func (w *packageWatch) step(pkgs []apt.Package) []apt.Package {
	seen := map[string]bool{}
	var newly []apt.Package
	for _, p := range pkgs {
		if !p.Upgradable {
			continue
		}
		seen[p.Name] = true
		if w.initialized && !w.known[p.Name] {
			newly = append(newly, p)
		}
	}
	w.known = seen
	w.initialized = true
	return newly
}

// WatchPackages runs until ctx is done; meant for its own goroutine.
func (h *Admin) WatchPackages(ctx context.Context, bus Publisher, logger *slog.Logger) {
	if h.Apt == nil {
		return
	}
	w := newPackageWatch()
	timer := time.NewTimer(packageWatchFirst)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			pkgs, err := h.Apt.List(ctx)
			if err != nil {
				if logger != nil {
					logger.Debug("package watch: list failed", "error", err)
				}
				if err == apt.ErrUnavailable {
					return
				}
			} else if newly := w.step(pkgs); len(newly) > 0 {
				names := make([]string, len(newly))
				for i, p := range newly {
					names[i] = p.Name
				}
				bus.Publish(ctx, events.PackageUpgradable, map[string]any{"count": len(newly), "names": strings.Join(names, ", ")})
			}
			timer.Reset(packageWatchInterval)
		}
	}
}
