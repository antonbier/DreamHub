package hubs

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/updatewindow"
)

// One container's update and roll back as plans (docs/APPSHUB_PLAN.md,
// App14): the same plan → apply as an edit, built on the same "make the
// containers again" (apps_recreate.go) with the previous image kept.
//
// The window (updates_window.go) holds an update back while a recording
// runs or is about to start; a roll back is a recovery and never waits.

// recreatePlan turns a prepared recreate into a plan.
func (h *Apps) recreatePlan(ctx context.Context, rc *recreate, containers []dockerd.Container, mode recreateMode) plan.Plan {
	app, prep := rc.app, rc.prep
	images := make([]string, 0, len(rc.specs))
	for _, s := range rc.specs {
		images = append(images, s.Image)
	}
	name := rc.name()
	action, summary := "apps.edit", fmt.Sprintf("Change %s: its containers are made again with the new settings, up to %d MB RAM (its data in %s is kept)", name, prep.totalMB, configDir(rc.root, app.ID))
	switch mode {
	case recreateUpdate:
		action, summary = "apps.update", fmt.Sprintf("Update %s: its containers are made again from the newest image; the previous version is kept for a roll back (its data in %s is kept)", name, configDir(rc.root, app.ID))
	case recreateRollback:
		action, summary = "apps.rollback", fmt.Sprintf("Roll back %s to the version it ran before its last update (its data in %s is kept)", name, configDir(rc.root, app.ID))
	}
	g := rc.group
	user, _ := httpx.User(ctx)
	return h.Plans.Create(ctx, user, plan.Draft{
		Action: action,
		Changes: []plan.Change{{Type: action, Before: map[string]any{"id": g.all()[0].ID, "containers": rc.ids}, After: map[string]any{
			"appId": app.ID, "name": name, "custom": app.Custom, "images": images, "hostPort": app.ContainerPort,
			"memoryLimitMB": prep.totalMB, "storage": configDir(rc.root, app.ID), "tz": prep.tz,
			"containers": prep.preview(), "elevated": prep.elevated, "edit": true,
		}}},
		Warnings:    prep.warnings,
		Summary:     summary,
		Detail:      map[string]any{"appId": app.ID, "edit": true},
		Fingerprint: installedFingerprint(containers, app.ID),
		Current: func(ctx context.Context) (string, error) {
			list, err := h.Docker.List(ctx)
			if err != nil {
				return "", err
			}
			return installedFingerprint(list, app.ID), nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return &plan.Failure{Status: http.StatusConflict, Code: "writes_disabled", Message: "device writes are disabled"}
			}
			if mode == recreateUpdate {
				// Checked again at apply: a timer may have come close since
				// the preview.
				if c, closed := h.updateClosed(ctx, false); closed {
					return heldFailure(c)
				}
			}
			if f := h.runRecreate(ctx, rc, mode); f != nil {
				return f
			}
			if mode != recreateUpdate {
				return nil
			}
			if failed := h.settle(ctx, []*recreate{rc}); failed[app.ID] != "" {
				return h.undoFailedUpdate(ctx, rc, failed[app.ID])
			}
			return nil
		},
	})
}

// undoFailedUpdate puts the app back on its previous image and returns the
// failure to report: the update did not run (and that is why the previous
// version runs again), or even the way back failed.
func (h *Apps) undoFailedUpdate(ctx context.Context, rc *recreate, reason string) *plan.Failure {
	if err := h.goBack(context.WithoutCancel(ctx), rc); err != nil {
		return recreateFailure(http.StatusBadGateway, "rollback_failed", "the update did not run and the previous version could not be started again", rc.app.ID, err, map[string]any{"reason": reason})
	}
	return recreateFailure(http.StatusBadGateway, "update_rolled_back", "the update did not run; the previous version is running again", rc.app.ID, nil, map[string]any{"reason": reason})
}

// planUpdate answers POST /api/v1/apps/installed/{id}/update/plan.
func (h *Apps) planUpdate(w http.ResponseWriter, r *http.Request) {
	h.planSameSettings(w, r, recreateUpdate)
}

// planRollback answers POST /api/v1/apps/installed/{id}/rollback/plan.
func (h *Apps) planRollback(w http.ResponseWriter, r *http.Request) {
	h.planSameSettings(w, r, recreateRollback)
}

func (h *Apps) planSameSettings(w http.ResponseWriter, r *http.Request, mode recreateMode) {
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	ctx := r.Context()
	containers, err := h.Docker.List(ctx)
	if err != nil {
		writeDockerError(w, r, err, "could not look up the app")
		return
	}
	g, found := findApp(containers, r.PathValue("id"))
	if !found {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such installed app")
		return
	}
	if mode == recreateRollback {
		if !h.hasKept(ctx, g.appID) {
			httpx.WriteError(w, r, http.StatusConflict, "nothing_to_roll_back", "no previous version is kept for this app")
			return
		}
	} else if c, closed := h.updateClosed(ctx, false); closed {
		writeHeld(w, r, c)
		return
	}
	rc, ferr := h.prepareSameSettings(ctx, containers, g)
	if ferr != nil {
		ferr.write(w, r)
		return
	}
	pl := h.recreatePlan(ctx, rc, containers, mode)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

// keptApps are the apps whose previous image is kept (the row's "Roll back").
func (h *Apps) keptApps(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	if h.Docker == nil {
		return out
	}
	kept, err := h.Docker.KeptImages(ctx)
	if err != nil {
		return out
	}
	for _, k := range kept {
		out[k.AppID] = true
	}
	return out
}

func (h *Apps) hasKept(ctx context.Context, appID string) bool { return h.keptApps(ctx)[appID] }

// --- the window ---------------------------------------------------------------

// reasonUnknown: the timers can't be read, so an automatic update waits.
const reasonUnknown = "unknown"

// recordings reads the timers that record now or later; known is false when
// the box does not answer.
func (h *Apps) recordings(ctx context.Context) (recs []updatewindow.Recording, known bool) {
	if h.WC == nil {
		return nil, false
	}
	timers, err := h.WC.TimerList(ctx)
	if err != nil {
		return nil, false
	}
	now := h.now()
	for _, t := range timers {
		if t.Disabled || t.JustPlay {
			continue
		}
		begin, end := time.Unix(t.Begin, 0), time.Unix(t.End, 0)
		if t.Active() {
			// About to start or recording: in the way for as long as it
			// is, whatever its margins say.
			if begin.After(now) {
				begin = now
			}
			if !end.After(now) {
				end = now.Add(time.Minute)
			}
		}
		recs = append(recs, updatewindow.Recording{Name: t.Name, Begin: begin, End: end})
	}
	return recs, true
}

// location is the box's own time zone, the one the night window is read in.
func (h *Apps) location() *time.Location {
	if h.Loc != nil {
		return h.Loc
	}
	return time.Local
}

// updateClosed reports whether updates are held back now. auto adds what only
// automatic updates wait for: the optional night window, and the timers being
// readable (a manual update is allowed when the box doesn't answer — that is
// a common reason to update).
func (h *Apps) updateClosed(ctx context.Context, auto bool) (updatewindow.Closed, bool) {
	cfg := h.config(ctx)
	recs, known := h.recordings(ctx)
	now := h.now().In(h.location())
	if !known && auto {
		return updatewindow.Closed{Reason: reasonUnknown, Until: now.Add(15 * time.Minute)}, true
	}
	return updatewindow.Decide(now, recs, cfg.guard(), cfg.night(), auto)
}

// heldDetails are the details of a "held back" answer: what the sentence
// shown to the person is made of.
func heldDetails(c updatewindow.Closed) map[string]any {
	return map[string]any{"reason": c.Reason, "name": c.Name, "until": c.Until.Unix()}
}

// CodeUpdatesHeld is the error code of an update held back by the window.
const CodeUpdatesHeld = "updates_held"

func writeHeld(w http.ResponseWriter, r *http.Request, c updatewindow.Closed) {
	httpx.WriteErrorDetails(w, r, http.StatusConflict, CodeUpdatesHeld, "updates wait while a recording runs or is about to start", heldDetails(c))
}

func heldFailure(c updatewindow.Closed) *plan.Failure {
	return &plan.Failure{Status: http.StatusConflict, Code: CodeUpdatesHeld, Message: "updates wait while a recording runs or is about to start", Details: heldDetails(c)}
}
