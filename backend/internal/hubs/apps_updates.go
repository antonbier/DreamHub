package hubs

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/apt"
	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/events"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// One update notion across Packages and Docker (docs/APPSHUB_PLAN.md, App14):
// a single count (packages with a newer version, containers with a newer
// image), one plan that updates all of it, and automatic updates for what can
// be done without anyone looking.
//
// Order — containers first, then the box's packages. A container update is
// the reversible half: the previous image is kept and an update that does not
// run is rolled back by itself. A package upgrade is the half that can leave
// the box different for good (dpkg), and the one that can restart enigma2 or
// touch the Docker kernel module. So the reversible step goes first, and a
// failure there stops the run before any package has changed. If packages
// went first, a failed upgrade would leave the containers on a changed
// system, and a package failing after the containers were updated would leave
// a half-updated box nobody asked for.

// updateState is what the update code remembers between calls (Apps.upd).
type updateState struct {
	checked  time.Time            // the last registry check
	pending  []string             // the apps it found upgradable
	failedAt map[string]time.Time // an automatic update that failed (apps; "packages")
	crash    crashTracker
}

// --- the summary ------------------------------------------------------------

type updatePackage struct {
	Name          string `json:"name"`
	From          string `json:"from"`
	To            string `json:"to"`
	TouchesEnigma bool   `json:"touchesEnigma2,omitempty"`
}

type updateContainer struct {
	ID    string `json:"id"`
	AppID string `json:"appId"`
	Name  string `json:"name"`
}

type updatesView struct {
	Packages   []updatePackage   `json:"packages"`
	Containers []updateContainer `json:"containers"`
	// Count is what "Update all" would change: packages and containers.
	Count int `json:"count"`
	// SelfUpdate: DreamHub's own package has a newer version. It is not part
	// of "Update all" (it restarts DreamHub in the middle of apt-get); it is
	// updated with the box's own tools.
	SelfUpdate bool `json:"selfUpdate"`
	// Window says whether an update could start now; Closed carries why
	// (recording, starting) and Until when it can.
	Window struct {
		Closed bool   `json:"closed"`
		Reason string `json:"reason,omitempty"`
		Name   string `json:"name,omitempty"`
		Until  int64  `json:"until,omitempty"`
	} `json:"window"`
	Auto struct {
		Containers bool `json:"containers"`
		Packages   bool `json:"packages"`
	} `json:"auto"`
}

// upgradablePackages lists the packages that "Update all" would change
// (DreamHub's own excluded) and whether DreamHub itself has an update.
func (h *Apps) upgradablePackages(ctx context.Context) (list []updatePackage, self bool) {
	if h.Admin == nil || h.Admin.Apt == nil {
		return nil, false
	}
	pkgs, err := h.Admin.Apt.List(ctx)
	if err != nil {
		return nil, false
	}
	for _, p := range pkgs {
		if !p.Upgradable {
			continue
		}
		if p.Name == selfPackage {
			self = true
			continue
		}
		list = append(list, updatePackage{Name: p.Name, From: p.Version, To: p.AvailableVersion, TouchesEnigma: strings.HasPrefix(p.Name, "enigma2")})
	}
	return list, self
}

// upgradableContainers lists the installed apps with a newer image.
func (h *Apps) upgradableContainers(ctx context.Context, containers []dockerd.Container) []updateContainer {
	var out []updateContainer
	for _, id := range h.checkUpgradable(ctx) {
		if g, ok := findAppByID(containers, id); ok {
			out = append(out, updateContainer{ID: g.all()[0].ID, AppID: id, Name: groupName(g, g.all()[0].Image)})
		}
	}
	return out
}

// updatesSummary answers GET /api/v1/apps/updates.
func (h *Apps) updatesSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	view := updatesView{Packages: []updatePackage{}, Containers: []updateContainer{}}
	var self bool
	if pk, s := h.upgradablePackages(ctx); pk != nil {
		view.Packages, self = pk, s
	} else {
		self = s
	}
	view.SelfUpdate = self
	if h.Docker != nil {
		if containers, err := h.Docker.List(ctx); err == nil {
			if c := h.upgradableContainers(ctx, containers); c != nil {
				view.Containers = c
			}
		}
	}
	view.Count = len(view.Packages) + len(view.Containers)
	if c, closed := h.updateClosed(ctx, false); closed {
		view.Window.Closed, view.Window.Reason, view.Window.Name, view.Window.Until = true, c.Reason, c.Name, c.Until.Unix()
	}
	cfg := h.config(ctx)
	view.Auto.Containers, view.Auto.Packages = cfg.AutoContainers, cfg.AutoPackages
	httpx.WriteJSON(w, http.StatusOK, view)
}

// --- the plan ---------------------------------------------------------------

type updateSkipped struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// updateTargets prepares every container update and the package upgrade; apps
// that can't be updated (their settings no longer validate, say) are named,
// not silently dropped.
func (h *Apps) updateTargets(ctx context.Context) (rcs []*recreate, skipped []updateSkipped, containers []dockerd.Container, err error) {
	if h.Docker == nil {
		return nil, nil, nil, nil
	}
	containers, err = h.Docker.List(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, u := range h.upgradableContainers(ctx, containers) {
		g, _ := findAppByID(containers, u.AppID)
		rc, ferr := h.prepareSameSettings(ctx, containers, g)
		if ferr != nil {
			skipped = append(skipped, updateSkipped{Name: u.Name, Reason: ferr.message})
			continue
		}
		rcs = append(rcs, rc)
	}
	return rcs, skipped, containers, nil
}

// packageStep is one package the combined plan changes.
type updateStep struct {
	Kind    string `json:"kind"` // container | package
	Name    string `json:"name"`
	AppID   string `json:"appId,omitempty"`
	Change  string `json:"change,omitempty"` // package: upgrade | install | remove
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Enigma2 bool   `json:"touchesEnigma2,omitempty"`
}

// planUpdateAll answers POST /api/v1/apps/updates/plan.
func (h *Apps) planUpdateAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if c, closed := h.updateClosed(ctx, false); closed {
		writeHeld(w, r, c)
		return
	}
	rcs, skipped, containers, err := h.updateTargets(ctx)
	if err != nil {
		writeDockerError(w, r, err, "could not look up the apps")
		return
	}
	var pkgNames []string
	var pkgSteps []apt.Step
	if list, _ := h.upgradablePackages(ctx); len(list) > 0 && h.Admin != nil && h.Admin.Packages != nil {
		for _, p := range list {
			pkgNames = append(pkgNames, p.Name)
		}
		steps, err := h.Admin.Packages.Simulate(ctx, apt.ActionUpgrade, pkgNames)
		if !h.Admin.writeSimulateError(w, r, err) {
			return
		}
		for _, s := range steps {
			if s.Name == selfPackage {
				httpx.WriteError(w, r, http.StatusConflict, "package_self", "DreamHub can't change its own package; update or remove it with the box's own tools")
				return
			}
		}
		pkgSteps = steps
	}
	if len(rcs) == 0 && len(pkgSteps) == 0 {
		httpx.WriteError(w, r, http.StatusConflict, "nothing_to_do", "there is nothing to update")
		return
	}

	// The preview: every container, then every package, in the order they run.
	var steps []updateStep
	var changes []plan.Change
	for _, rc := range rcs {
		steps = append(steps, updateStep{Kind: "container", Name: rc.name(), AppID: rc.app.ID})
		changes = append(changes, plan.Change{Type: "apps.update", After: map[string]any{"appId": rc.app.ID, "name": rc.name(), "images": specImages(rc.specs)}})
	}
	for _, s := range pkgSteps {
		steps = append(steps, updateStep{Kind: "package", Name: s.Name, Change: s.Kind, From: s.OldVersion, To: s.NewVersion, Enigma2: strings.HasPrefix(s.Name, "enigma2")})
		c := plan.Change{Type: "package." + s.Kind}
		if s.OldVersion != "" {
			c.Before = map[string]string{"name": s.Name, "version": s.OldVersion}
		}
		if s.NewVersion != "" {
			c.After = map[string]string{"name": s.Name, "version": s.NewVersion}
		}
		changes = append(changes, c)
	}
	var warnings []status.Problem
	if len(pkgSteps) > 0 {
		warnings = packageWarnings(pkgSteps, pkgNames)
	}
	for _, rc := range rcs {
		warnings = append(warnings, rc.prep.warnings...)
	}

	user, _ := httpx.User(ctx)
	summary := describeUpdateAll(rcs, pkgSteps)
	fp := updateFingerprint(containers, rcs, pkgSteps)
	pl := h.Plans.Create(user, plan.Draft{
		Action:   "apps.updateAll",
		Changes:  changes,
		Warnings: warnings,
		Summary:  summary,
		Detail:   map[string]any{"containers": len(rcs), "packages": len(pkgSteps)},
		// What the person saw: the same apps and the same packages. A
		// newer image or version since then means looking again.
		Fingerprint: fp,
		Current: func(ctx context.Context) (string, error) {
			list, err := h.Docker.List(ctx)
			if h.Docker == nil {
				list, err = nil, nil
			}
			if err != nil {
				return "", err
			}
			var now []apt.Step
			if len(pkgNames) > 0 {
				if now, err = h.Admin.Packages.Simulate(ctx, apt.ActionUpgrade, pkgNames); err != nil {
					return "", err
				}
			}
			return updateFingerprint(list, rcs, now), nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return &plan.Failure{Status: http.StatusConflict, Code: "writes_disabled", Message: "device writes are disabled"}
			}
			if c, closed := h.updateClosed(ctx, false); closed {
				return heldFailure(c)
			}
			rep := h.runUpdates(ctx, user, rcs, pkgNames, pkgSteps, summary)
			if f := rep.failure(); f != nil {
				return f
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl, "steps": steps, "skipped": orEmpty(skipped), "order": []string{"containers", "packages"}})
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func specImages(specs []dockerd.ContainerSpec) []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.Image)
	}
	return out
}

func updateFingerprint(containers []dockerd.Container, rcs []*recreate, steps []apt.Step) string {
	var b strings.Builder
	for _, rc := range rcs {
		b.WriteString(rc.app.ID + "=" + installedFingerprint(containers, rc.app.ID) + "\n")
	}
	return b.String() + stepsFingerprint(steps)
}

func describeUpdateAll(rcs []*recreate, steps []apt.Step) string {
	var parts []string
	if len(rcs) > 0 {
		names := make([]string, len(rcs))
		for i, rc := range rcs {
			names[i] = rc.name()
		}
		parts = append(parts, "Updated containers: "+strings.Join(names, ", "))
	}
	if len(steps) > 0 {
		parts = append(parts, describeSteps(steps))
	}
	return strings.Join(parts, "; ")
}

// --- running it ---------------------------------------------------------------

// updateFailure is one step that did not work.
type updateFailure struct {
	Kind       string `json:"kind"` // container | package
	Name       string `json:"name"`
	AppID      string `json:"appId,omitempty"`
	Code       string `json:"code"`
	Reason     string `json:"reason"`
	RolledBack bool   `json:"rolledBack,omitempty"`
	Output     string `json:"output,omitempty"`
}

// updateReport is what one run did, in order.
type updateReport struct {
	Done     []string        // names of the containers updated, then "packages"
	Failures []updateFailure // the first one stopped the run
	Skipped  []string        // what was not tried because of it
	Packages int             // packages changed
}

// failure is the plan failure that reports a run with a failed step: it names
// what was done, what failed and why, and what was not tried.
func (rep updateReport) failure() *plan.Failure {
	if len(rep.Failures) == 0 {
		return nil
	}
	first := rep.Failures[0]
	return &plan.Failure{Status: http.StatusBadGateway, Code: "updates_failed", Message: "an update step failed; the later ones were not started",
		Details: map[string]any{"done": orEmpty(rep.Done), "failed": rep.Failures, "skipped": orEmpty(rep.Skipped)},
		Err:     fmt.Errorf("%s %s: %s", first.Kind, first.Name, first.Reason)}
}

// failureReason is the short reason of a failed step for the report.
func failureReason(f *plan.Failure) string {
	if f.Err != nil {
		return f.Err.Error()
	}
	return f.Message
}

// runUpdates is the whole run — manual (a confirmed plan) or automatic. In
// order: every container (a failure stops the rest), the updated containers
// watched until they have run well for the settle time (one that does not is
// rolled back), then, only if all that went well, the packages.
func (h *Apps) runUpdates(ctx context.Context, user store.User, rcs []*recreate, pkgNames []string, pkgSteps []apt.Step, summary string) updateReport {
	var rep updateReport
	var updated []*recreate
	for i, rc := range rcs {
		f := h.runRecreate(ctx, rc, recreateUpdate)
		if f != nil {
			det, _ := f.Details.(map[string]any)
			restored, _ := det["restored"].(bool)
			rep.Failures = append(rep.Failures, updateFailure{Kind: "container", Name: rc.name(), AppID: rc.app.ID, Code: f.Code, Reason: failureReason(f), RolledBack: restored})
			for _, rest := range rcs[i+1:] {
				rep.Skipped = append(rep.Skipped, rest.name())
			}
			break
		}
		updated = append(updated, rc)
		rep.Done = append(rep.Done, rc.name())
	}
	// Watched together, so a long run does not wait a minute per container.
	if bad := h.settle(ctx, updated); len(bad) > 0 {
		var kept []string
		for _, rc := range updated {
			reason, failed := bad[rc.app.ID]
			if !failed {
				kept = append(kept, rc.name())
				continue
			}
			uf := updateFailure{Kind: "container", Name: rc.name(), AppID: rc.app.ID, Reason: reason}
			if f := h.undoFailedUpdate(ctx, rc, reason); f.Code == "update_rolled_back" {
				uf.Code, uf.RolledBack = "update_rolled_back", true
			} else {
				uf.Code = "rollback_failed"
			}
			rep.Failures = append(rep.Failures, uf)
		}
		rep.Done = kept
	}
	if len(rep.Failures) > 0 {
		if len(pkgSteps) > 0 {
			rep.Skipped = append(rep.Skipped, "packages")
		}
		return rep
	}
	if len(pkgSteps) > 0 && h.Admin != nil {
		if f := h.Admin.applyPackages(ctx, user, string(apt.ActionUpgrade), pkgSteps, describeSteps(pkgSteps), func(ctx context.Context) (string, error) {
			return h.Admin.Packages.Apply(ctx, apt.ActionUpgrade, pkgNames)
		}); f != nil {
			det, _ := f.Details.(map[string]any)
			out, _ := det["output"].(string)
			rep.Failures = append(rep.Failures, updateFailure{Kind: "package", Name: "packages", Code: f.Code, Reason: failureReason(f), Output: out})
			return rep
		}
		rep.Done = append(rep.Done, "packages")
		rep.Packages = len(pkgSteps)
	}
	return rep
}

// --- automatic updates --------------------------------------------------------

// setPending remembers what the last registry check found.
func (h *Apps) setPending(ids []string, at time.Time) {
	h.updMu.Lock()
	defer h.updMu.Unlock()
	h.upd.checked, h.upd.pending = at, slices.Clone(ids)
}

// autoUpdate applies what the person allowed to be applied by itself:
// container updates, and package upgrades that do not restart enigma2 —
// only inside the window (no recording running or close, the night window if
// set), never more than once a day for the same thing when it failed, and
// with the result written to the notification inbox.
func (h *Apps) autoUpdate(ctx context.Context, bus Publisher, logger *slog.Logger) {
	cfg := h.config(ctx)
	if (!cfg.AutoContainers && !cfg.AutoPackages) || h.ReadOnly {
		return
	}
	if _, closed := h.updateClosed(ctx, true); closed {
		return
	}
	now := h.now()
	h.updMu.Lock()
	if h.upd.failedAt == nil {
		h.upd.failedAt = map[string]time.Time{}
	}
	retryAfter := time.Duration(cfg.UpdateHours) * time.Hour
	waiting := func(key string) bool {
		t, ok := h.upd.failedAt[key]
		return ok && now.Sub(t) < retryAfter
	}
	pending := slices.Clone(h.upd.pending)
	h.updMu.Unlock()

	var rcs []*recreate
	if cfg.AutoContainers && h.Docker != nil {
		containers, err := h.Docker.List(ctx)
		if err == nil {
			for _, id := range pending {
				g, ok := findAppByID(containers, id)
				if !ok || waiting(id) {
					continue
				}
				// Looked at again: it may have been updated since the check.
				if !h.imageUpgradable(ctx, g.all()[0]) {
					continue
				}
				rc, ferr := h.prepareSameSettings(ctx, containers, g)
				if ferr != nil {
					if logger != nil {
						logger.Debug("auto update: not prepared", "app", id, "reason", ferr.message)
					}
					continue
				}
				rcs = append(rcs, rc)
			}
		}
	}
	var pkgNames []string
	var pkgSteps []apt.Step
	if cfg.AutoPackages && h.Admin != nil && h.Admin.Packages != nil && !waiting("packages") {
		pkgNames, pkgSteps = h.autoPackageSteps(ctx)
	}
	if len(rcs) == 0 && len(pkgSteps) == 0 {
		return
	}
	rep := h.runUpdates(ctx, h.User, rcs, pkgNames, pkgSteps, describeUpdateAll(rcs, pkgSteps))
	h.reportAuto(ctx, bus, now, rep)
}

// autoPackageSteps are the upgrades an automatic update may apply: nothing
// that restarts enigma2 (those wait for a person, or a window with no
// recording, through the manual plan) and never DreamHub's own package.
func (h *Apps) autoPackageSteps(ctx context.Context) ([]string, []apt.Step) {
	list, _ := h.upgradablePackages(ctx)
	var names []string
	for _, p := range list {
		if !p.TouchesEnigma {
			names = append(names, p.Name)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	steps, err := h.Admin.Packages.Simulate(ctx, apt.ActionUpgrade, names)
	if err != nil || len(steps) == 0 || touchesEnigma2(steps) {
		return nil, nil
	}
	for _, s := range steps {
		if s.Name == selfPackage {
			return nil, nil
		}
	}
	return names, steps
}

// reportAuto writes the result of an automatic run to the inbox: one entry
// for what was updated, one per failure.
func (h *Apps) reportAuto(ctx context.Context, bus Publisher, now time.Time, rep updateReport) {
	h.updMu.Lock()
	if h.upd.failedAt == nil {
		h.upd.failedAt = map[string]time.Time{}
	}
	for _, f := range rep.Failures {
		key := "packages"
		if f.Kind == "container" {
			key = f.AppID
		}
		h.upd.failedAt[key] = now
	}
	h.updMu.Unlock()
	if bus == nil {
		return
	}
	if rep.Packages+len(rep.Done) > 0 {
		var names []string
		count := 0
		for _, n := range rep.Done {
			if n == "packages" {
				count += rep.Packages
				names = append(names, fmt.Sprintf("%d packages", rep.Packages))
				continue
			}
			count++
			names = append(names, n)
		}
		bus.Publish(ctx, events.UpdatesApplied, map[string]any{"count": count, "names": strings.Join(names, ", ")})
	}
	for _, f := range rep.Failures {
		// "Rolled back": the update ran but did not stay running. Any other
		// failure is just that — the app runs as before, the reason says why.
		key := events.UpdatesFailed
		if f.Code == "update_rolled_back" {
			key = events.UpdatesRolledBack
		}
		bus.Publish(ctx, key, map[string]any{"name": f.Name, "reason": f.Reason})
	}
}
