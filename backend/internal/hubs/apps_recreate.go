package hubs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/plan"
)

// An installed app's containers made again (docs/APPSHUB_PLAN.md, App13
// "edit" and App14 "update" and "roll back"). The three differ only in what
// the new containers are made from:
//
//   - edit: the changed settings, the image as it is on the box;
//   - update: the same settings, the newest image of the same tag — the image
//     the app ran before is kept first, so it can come back;
//   - roll back: the same settings, the kept previous image.
//
// The app's folders are never touched, so its data stays. Whatever goes
// wrong, the app is put back as it was: a container is never left removed.

type recreateMode int

const (
	recreateEdit recreateMode = iota
	recreateUpdate
	recreateRollback
)

// recreate is one app's containers made again.
type recreate struct {
	app   appcatalog.App
	group appGroup
	root  string
	ids   []string                // the containers there now, the main one first
	specs []dockerd.ContainerSpec // the new ones, helpers first
	// undo are the containers as they were before an edit (nil when they
	// can't be rebuilt from stored choices). An update and a roll back
	// rebuild the old ones from specs.
	undo []dockerd.ContainerSpec
	body installBody
	prep *installPrep
}

// name is what the app is called.
func (rc *recreate) name() string { return rc.app.Name }

// prepareRecreate validates body as the new state of the installed app g and
// works out what has to be removed and created. def is what was stored at
// install (known: whether anything was).
func (h *Apps) prepareRecreate(ctx context.Context, containers []dockerd.Container, g appGroup, root string, body installBody, def appDefinition, known bool) (*recreate, *formError) {
	// What runs now: the stored choices (else the defaults and the port it
	// runs on) — its ports are free for the new containers, its limits count
	// as given back.
	oldBody := def.body(g.appID)
	if !known {
		oldBody = installBody{AppID: g.appID, ConfirmElevated: true, editID: g.appID}
		if cat, ok := appcatalog.ByID(g.appID); ok && cat.PortEditable() {
			if hp := g.all()[0].HostPort; hp != 0 && hp != cat.ContainerPort {
				oldBody.Values = map[string]string{cat.PortVar: fmt.Sprint(hp)}
			}
		}
	}
	cfg := h.config(ctx)
	target := &editTarget{group: g, root: root, oldPorts: map[[2]int]bool{}}
	var oldApp appcatalog.App
	var oldMain int
	oldKnown := false
	if app, _, ferr := h.resolveApp(oldBody); ferr == nil {
		oldApp, oldKnown = app, true
		target.oldPorts = portSetOf(app)
		oldMain, target.oldMB = memorySizes(app, oldBody.MemoryLimitMB, cfg)
	}
	if hp := g.all()[0].HostPort; hp != 0 {
		target.oldPorts[[2]int{hp, 0}] = true
	}

	prep, ferr := h.prepareInstall(ctx, body, containers, target)
	if ferr != nil {
		return nil, ferr
	}
	rc := &recreate{app: prep.app, group: g, root: root, specs: prep.specs, body: body, prep: prep}
	// To put the app back as it was if the new containers can't be made.
	if known && oldKnown {
		rc.undo = h.appSpecs(oldApp, oldMain, prep.tz, root, true, cfg.defaultMemoryMB())
	}
	for _, c := range g.all() {
		rc.ids = append(rc.ids, c.ID)
	}
	return rc, nil
}

// prepareSameSettings is prepareRecreate with the settings the app was
// installed with (the defaults, and the port it runs on, when nothing was
// stored): what an update and a roll back make.
func (h *Apps) prepareSameSettings(ctx context.Context, containers []dockerd.Container, g appGroup) (*recreate, *formError) {
	root := g.root(h.StorageRoot)
	def, known := h.loadDefinition(root, g.appID)
	var body installBody
	if known {
		body = def.body(g.appID)
	} else {
		body = installBody{AppID: g.appID, ConfirmElevated: true, editID: g.appID}
		if cat, ok := appcatalog.ByID(g.appID); ok && cat.PortEditable() {
			if hp := g.all()[0].HostPort; hp != 0 && hp != cat.ContainerPort {
				body.Values = map[string]string{cat.PortVar: fmt.Sprint(hp)}
			}
		}
		if _, curated := appcatalog.ByID(g.appID); !curated {
			return nil, &formError{http.StatusConflict, "not_editable", "this container was added before editing existed; remove it and add it again to change it", nil}
		}
	}
	return h.prepareRecreate(ctx, containers, g, root, body, def, known)
}

// recreateFailure is a plan failure naming the app the step was about.
func recreateFailure(status int, code, message string, app string, err error, more map[string]any) *plan.Failure {
	d := map[string]any{"app": app}
	for k, v := range more {
		d[k] = v
	}
	return &plan.Failure{Status: status, Code: code, Message: message, Details: d, Err: err}
}

// runRecreate removes the app's containers and makes the new ones. It
// returns nil when they are all started; on any error the app is back as it
// was (what is missing of it is created again) and the failure says what
// happened. In update mode the previous images are kept first and the app
// comes back on exactly those.
func (h *Apps) runRecreate(ctx context.Context, rc *recreate, mode recreateMode) *plan.Failure {
	app := rc.app.ID
	secrets, err := h.appSecrets(rc.app, rc.root)
	if err != nil {
		return recreateFailure(http.StatusInternalServerError, "edit_failed", "the app's passwords could not be read", app, err, nil)
	}
	expand := func(spec dockerd.ContainerSpec) dockerd.ContainerSpec {
		spec.Env = appcatalog.ExpandEnv(spec.Env, secrets)
		return spec
	}
	keptNow := false
	if mode == recreateUpdate {
		// Before anything changes: if the previous version can't be kept the
		// app is not touched at all.
		for _, id := range rc.ids {
			if _, err := h.Docker.KeepImage(ctx, id); err != nil {
				return recreateFailure(http.StatusBadGateway, "update_failed", "the previous version could not be kept, so the app was not changed", app, err, nil)
			}
		}
		keptNow = true
	}
	specs := slices.Clone(rc.specs)
	if mode == recreateRollback {
		for i := range specs {
			specs[i].FromKept = true
		}
	}
	// The way back: before an edit the containers as they were; for an
	// update or a roll back the same containers as now, an update's from the
	// image just kept.
	undo := rc.undo
	if mode != recreateEdit {
		undo = slices.Clone(rc.specs)
	}
	create := func(spec dockerd.ContainerSpec) (dockerd.Container, error) {
		c, err := h.Docker.Create(ctx, expand(spec))
		if errors.Is(err, dockerd.ErrNoKeptImage) && spec.FromKept && spec.Service != "" {
			// A helper that was not updated has nothing kept: it just runs
			// again as it is. (The app itself must have its kept version.)
			spec.FromKept = false
			c, err = h.Docker.Create(ctx, expand(spec))
		}
		return c, err
	}
	// restore puts the old containers back; false if one of them could not be.
	restore := func() bool {
		ok := true
		for _, spec := range undo {
			if keptNow {
				s := spec
				s.FromKept = true
				if _, err := h.Docker.Create(context.WithoutCancel(ctx), expand(s)); err == nil {
					continue
				}
			}
			if _, err := h.Docker.Create(context.WithoutCancel(ctx), expand(spec)); err != nil {
				ok = false
			}
		}
		return ok
	}
	// fail puts the app back and reports; the failure says whether that worked.
	fail := func(status int, code, message string, err error, more map[string]any) *plan.Failure {
		restored := restore()
		if more == nil {
			more = map[string]any{}
		}
		more["restored"] = restored
		if !restored {
			code, message = "rollback_failed", "the app could not be changed and its previous containers could not all be started again"
		}
		return recreateFailure(status, code, message, app, err, more)
	}

	// The app first, then what it depends on.
	for _, cid := range rc.ids {
		if err := h.Docker.Remove(ctx, cid); err != nil && !errors.Is(err, dockerd.ErrNotFound) {
			return fail(http.StatusBadGateway, "edit_failed", "the app could not be changed", err, nil)
		}
	}
	var created []string
	for _, spec := range specs {
		c, err := create(spec)
		if err != nil {
			for _, id := range created {
				_ = h.Docker.Remove(context.WithoutCancel(ctx), id)
			}
			if se, ok := dockerd.IsRefusal(err); ok {
				return fail(http.StatusUnprocessableEntity, "install_refused", se.Message, err, map[string]any{"reason": se.Code, "path": se.Path})
			}
			switch mode {
			case recreateUpdate:
				return fail(http.StatusBadGateway, "update_failed", "the update could not be made; the previous version is running again", err, nil)
			case recreateRollback:
				return fail(http.StatusBadGateway, "rollback_failed", "the previous version could not be started", err, nil)
			}
			return fail(http.StatusBadGateway, "edit_failed", "the app could not be changed; the previous settings were put back", err, nil)
		}
		created = append(created, c.ID)
	}
	switch mode {
	case recreateEdit:
		h.saveDefinition(rc.root, app, definitionOf(rc.body))
	case recreateRollback:
		// Nothing left to go back to.
		_, _ = h.Docker.DropKeptImages(context.WithoutCancel(ctx), app)
	}
	return nil
}

// --- running healthy for a minute --------------------------------------------

// defaultSettle is how long a new container must keep running before an
// update counts as good (and how long the previous version waits for a
// failure to be noticed).
const defaultSettle = time.Minute

func (h *Apps) settleTime() time.Duration {
	if h.Settle > 0 {
		return h.Settle
	}
	return defaultSettle
}

// settle watches the containers of the updated apps for the settle time and
// returns, per app, why it is not running well (nothing for a good one). An
// app counts as failed as soon as a container of it is not running, or the
// daemon has had to restart one.
func (h *Apps) settle(ctx context.Context, apps []*recreate) map[string]string {
	failed := map[string]string{}
	if len(apps) == 0 {
		return failed
	}
	total := h.settleTime()
	tick := min(5*time.Second, max(total/4, time.Millisecond))
	deadline := time.Now().Add(total)
	for {
		list, err := h.Docker.List(ctx)
		if err == nil {
			for _, rc := range apps {
				if failed[rc.app.ID] != "" {
					continue
				}
				found := false
				for _, c := range list {
					if c.AppID() != rc.app.ID {
						continue
					}
					found = true
					if c.State != "running" {
						failed[rc.app.ID] = fmt.Sprintf("%s is %s", containerLabel(c), c.State)
					} else if c.RestartCount > 0 {
						failed[rc.app.ID] = fmt.Sprintf("%s keeps restarting", containerLabel(c))
					}
				}
				if !found {
					failed[rc.app.ID] = "the app's containers are gone"
				}
			}
		}
		if !time.Now().Before(deadline) {
			return failed
		}
		select {
		case <-ctx.Done():
			return failed
		case <-time.After(tick):
		}
	}
}

func containerLabel(c dockerd.Container) string {
	if s := c.Service(); s != "" {
		return c.AppID() + " (" + s + ")"
	}
	return c.AppID()
}

// goBack puts an updated app back on the image it ran before, after the
// update turned out not to run: the containers there now are removed and made
// again from the kept images. It returns an error when that did not work
// either.
func (h *Apps) goBack(ctx context.Context, rc *recreate) error {
	list, err := h.Docker.List(ctx)
	if err != nil {
		return err
	}
	g, ok := findAppByID(list, rc.app.ID)
	if !ok {
		return errors.New("the app's containers are gone")
	}
	back := *rc
	back.ids = nil
	for _, c := range g.all() {
		back.ids = append(back.ids, c.ID)
	}
	if f := h.runRecreate(ctx, &back, recreateRollback); f != nil {
		return f
	}
	return nil
}

// findAppByID is the group of the containers of one app.
func findAppByID(containers []dockerd.Container, appID string) (appGroup, bool) {
	for _, g := range groupApps(containers) {
		if g.appID == appID {
			return g, true
		}
	}
	return appGroup{}, false
}
