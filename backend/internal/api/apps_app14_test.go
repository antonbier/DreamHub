package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// App14: one update notion across packages and containers, the window,
// rollback, and the settings behind them. The mock box has two upgradable
// things on purpose: three packages (one of them touches enigma2) and Pi-hole's
// image.

// quietBox is a mock box without its fixture timers (they sit on the test
// clock's evening and would hold updates back).
func quietBox(t *testing.T) *mock.Client {
	t.Helper()
	m := mock.New()
	list, err := m.TimerList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, tm := range list {
		if err := m.DeleteTimer(context.Background(), webcontrol.TimerRef{ServiceReference: tm.ServiceReference, Begin: tm.Begin, End: tm.End}); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

type updView struct {
	Packages   []struct{ Name, From, To string }  `json:"packages"`
	Containers []struct{ ID, AppID, Name string } `json:"containers"`
	Count      int                                `json:"count"`
	SelfUpdate bool                               `json:"selfUpdate"`
	Window     struct {
		Closed bool   `json:"closed"`
		Reason string `json:"reason"`
		Name   string `json:"name"`
		Until  int64  `json:"until"`
	} `json:"window"`
	Auto struct{ Containers, Packages bool } `json:"auto"`
}

type updPlan struct {
	Plan struct {
		ID       string
		Action   string
		Changes  []struct{ Type string }
		Warnings []struct{ Code string }
	}
	Steps   []struct{ Kind, Name, Change string }
	Skipped []struct{ Name, Reason string }
	Order   []string
}

func (e *env) updates(t *testing.T) updView {
	t.Helper()
	var v updView
	e.json(t, "GET", "/api/v1/apps/updates", "", nil, 200, &v)
	return v
}

type failBody struct {
	Error struct {
		Code    string `json:"code"`
		Details struct {
			Done   []string `json:"done"`
			Failed []struct {
				Kind       string `json:"kind"`
				Name       string `json:"name"`
				Code       string `json:"code"`
				RolledBack bool   `json:"rolledBack"`
			} `json:"failed"`
			Skipped []string `json:"skipped"`
			// held back by the window
			Reason string `json:"reason"`
			Name   string `json:"name"`
			Until  int64  `json:"until"`
			// one app's own failure
			Restored *bool `json:"restored"`
		} `json:"details"`
	} `json:"error"`
}

func failOf(b []byte) failBody {
	var f failBody
	_ = json.Unmarshal(b, &f)
	return f
}

func (e *env) piholeDigest() string {
	d, _ := e.docker.ImageDigest(context.Background(), "pihole")
	return d
}

func (e *env) applyPlan(t *testing.T, id string, want int) []byte {
	t.Helper()
	resp, b := e.do(t, "POST", "/api/v1/plans/"+id+"/apply", "", nil)
	if resp.StatusCode != want {
		t.Fatalf("apply: expected %d, got %d: %s", want, resp.StatusCode, b)
	}
	return b
}

func TestUpdates_SummaryCountsPackagesAndContainers(t *testing.T) {
	e := newEnvWith(t, quietBox(t))
	v := e.updates(t)
	var pk []string
	for _, p := range v.Packages {
		pk = append(pk, p.Name)
	}
	slices.Sort(pk)
	if !slices.Equal(pk, []string{"enigma2-plugin-extensions-mediaportal", "enigma2-plugin-systemplugins-networkmanager", "oscam-cycecplus"}) {
		t.Errorf("packages: %v", pk)
	}
	if len(v.Containers) != 1 || v.Containers[0].AppID != "pihole" || v.Containers[0].Name != "Pi-hole" {
		t.Errorf("containers: %+v", v.Containers)
	}
	if v.Count != 4 || v.SelfUpdate || v.Window.Closed {
		t.Errorf("count %d self %v window %+v", v.Count, v.SelfUpdate, v.Window)
	}
	if v.Auto.Containers || v.Auto.Packages {
		t.Error("automatic updates must be off by default")
	}
	// DreamHub's own package is not part of "update all", but it is said.
	for i := range e.apt.Packages {
		if e.apt.Packages[i].Name == "dreamhub" {
			e.apt.Packages[i].Upgradable, e.apt.Packages[i].AvailableVersion = true, "0.4.0"
		}
	}
	v = e.updates(t)
	if !v.SelfUpdate || v.Count != 4 {
		t.Errorf("own package: self=%v count=%d", v.SelfUpdate, v.Count)
	}
	for _, p := range v.Packages {
		if p.Name == "dreamhub" {
			t.Error("DreamHub's own package is in the update-all list")
		}
	}
}

func TestUpdates_PlanOrderAndApply(t *testing.T) {
	e := newEnvWith(t, quietBox(t))
	var p updPlan
	e.json(t, "POST", "/api/v1/apps/updates/plan", "", nil, 200, &p)
	if len(p.Order) != 2 || p.Order[0] != "containers" || p.Order[1] != "packages" {
		t.Fatalf("order %v", p.Order)
	}
	// One preview that lists every container, then every package.
	var kinds []string
	for _, s := range p.Steps {
		kinds = append(kinds, s.Kind)
	}
	if !slices.Equal(kinds, []string{"container", "package", "package", "package"}) || p.Steps[0].Name != "Pi-hole" {
		t.Errorf("steps %+v", p.Steps)
	}
	if p.Plan.Changes[0].Type != "apps.update" || p.Plan.Changes[1].Type != "package.upgrade" {
		t.Errorf("changes %+v", p.Plan.Changes)
	}
	hasWarn := false
	for _, w := range p.Plan.Warnings {
		hasWarn = hasWarn || w.Code == "package.touchesEnigma2"
	}
	if !hasWarn {
		t.Errorf("a package that restarts enigma2 must be warned about: %+v", p.Plan.Warnings)
	}
	// Previewing changed nothing.
	if e.piholeDigest() != "sha256:pihole-pulled-old" || len(e.apt.Applied) != 0 {
		t.Fatal("the preview changed something")
	}

	e.applyPlan(t, p.Plan.ID, 200)
	if e.piholeDigest() == "sha256:pihole-pulled-old" {
		t.Error("Pi-hole was not updated")
	}
	if len(e.apt.Applied) != 1 || !strings.HasPrefix(e.apt.Applied[0], "upgrade ") || strings.Contains(e.apt.Applied[0], "dreamhub") {
		t.Errorf("apt calls: %v", e.apt.Applied)
	}
	// The containers went first: its previous image was kept, then removed
	// and created again.
	calls := strings.Join(e.docker.Calls, ",")
	if !strings.Contains(calls, "keep pihole,remove pihole,create pihole") {
		t.Errorf("docker calls: %s", calls)
	}
	if v := e.updates(t); v.Count != 0 {
		t.Errorf("after the update %d are still pending", v.Count)
	}
	// And the plan is single use.
	resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusNotFound || code != "plan_not_found" {
		t.Errorf("second apply: %d %s", resp.StatusCode, b)
	}
	// Nothing left: a second preview says so.
	resp, b = e.do(t, "POST", "/api/v1/apps/updates/plan", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "nothing_to_do" {
		t.Errorf("nothing to update: %d %s", resp.StatusCode, b)
	}
}

// A container step that fails stops everything after it, says so, and leaves
// the app running as it was — never removed.
func TestUpdates_ContainerFailureStopsLaterStepsAndRestoresTheApp(t *testing.T) {
	e := newEnvWith(t, quietBox(t))
	var p updPlan
	e.json(t, "POST", "/api/v1/apps/updates/plan", "", nil, 200, &p)
	pulls := 0
	e.docker.FailCreate = func(spec dockerd.ContainerSpec) error {
		if spec.AppID == "pihole" && !spec.FromKept {
			pulls++
			return &dockerd.SpecError{Code: "pull_failed", Message: "manifest unknown"}
		}
		return nil
	}
	b := e.applyPlan(t, p.Plan.ID, http.StatusBadGateway)
	f := failOf(b)
	// The step's own failure is named; the way back (a create from the kept
	// image) is not failed by the hook, so the app is restored.
	if f.Error.Code != "updates_failed" || len(f.Error.Details.Failed) != 1 {
		t.Fatalf("failure: %s", b)
	}
	if st := f.Error.Details.Failed[0]; st.Name != "Pi-hole" || st.Kind != "container" || !st.RolledBack || st.Code != "install_refused" {
		t.Errorf("failed step: %+v", st)
	}
	if !slices.Equal(f.Error.Details.Skipped, []string{"packages"}) {
		t.Errorf("skipped: %v", f.Error.Details.Skipped)
	}
	if pulls != 1 {
		t.Errorf("pulled %d times", pulls)
	}
	// Pi-hole is back on the version it had, the packages were not touched.
	list, _ := e.docker.List(context.Background())
	found := false
	for _, c := range list {
		found = found || c.AppID() == "pihole"
	}
	if !found {
		t.Fatal("the app was left removed")
	}
	if e.piholeDigest() != "sha256:pihole-pulled-old" {
		t.Errorf("Pi-hole runs %s, want its old image", e.piholeDigest())
	}
	if len(e.apt.Applied) != 0 {
		t.Errorf("packages were upgraded after a failed container step: %v", e.apt.Applied)
	}
}

// The combined plan reports a failed step with what was done, what failed and
// what was not tried: here the packages, after an update that did not run.
func TestUpdates_FailedStartIsRolledBackAndPackagesAreSkipped(t *testing.T) {
	e := newEnvWith(t, quietBox(t))
	var p updPlan
	e.json(t, "POST", "/api/v1/apps/updates/plan", "", nil, 200, &p)
	// The new Pi-hole stops right after it started; the kept one runs.
	e.docker.ExitAfterCreate = func(spec dockerd.ContainerSpec) bool { return spec.AppID == "pihole" && !spec.FromKept }
	b := e.applyPlan(t, p.Plan.ID, http.StatusBadGateway)
	f := failOf(b)
	if f.Error.Code != "updates_failed" || len(f.Error.Details.Failed) != 1 {
		t.Fatalf("failure: %s", b)
	}
	got := f.Error.Details.Failed[0]
	if got.Kind != "container" || got.Name != "Pi-hole" || got.Code != "update_rolled_back" || !got.RolledBack {
		t.Errorf("failed step: %+v", got)
	}
	if !slices.Equal(f.Error.Details.Skipped, []string{"packages"}) || len(f.Error.Details.Done) != 0 {
		t.Errorf("done %v skipped %v", f.Error.Details.Done, f.Error.Details.Skipped)
	}
	if e.piholeDigest() != "sha256:pihole-pulled-old" {
		t.Error("the app does not run its previous version again")
	}
	if len(e.apt.Applied) != 0 {
		t.Error("packages were upgraded after the container update failed")
	}
	// Running again, and there is nothing left to roll back to (it just did).
	row := e.row(t, "Pi-hole")
	if row.State != "running" || row.RollBack {
		t.Errorf("row after the automatic roll back: %+v", row)
	}
}

type updRow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	State    string `json:"state"`
	RollBack bool   `json:"rollBack"`
}

func (e *env) row(t *testing.T, name string) updRow {
	t.Helper()
	var v struct{ Apps []updRow }
	e.json(t, "GET", "/api/v1/apps/installed", "", nil, 200, &v)
	for _, r := range v.Apps {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no row %s", name)
	return updRow{}
}

func TestAppUpdateAndRollBack(t *testing.T) {
	e := newEnvWith(t, quietBox(t))
	if e.row(t, "Pi-hole").RollBack {
		t.Fatal("Roll back offered before any update")
	}
	// Nothing kept: refused plainly.
	resp, b := e.do(t, "POST", "/api/v1/apps/installed/pihole/rollback/plan", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "nothing_to_roll_back" {
		t.Fatalf("roll back with nothing kept: %d %s", resp.StatusCode, b)
	}

	var up struct{ Plan struct{ ID, Action string } }
	e.json(t, "POST", "/api/v1/apps/installed/pihole/update/plan", "", nil, 200, &up)
	if up.Plan.Action != "apps.update" {
		t.Fatalf("action %s", up.Plan.Action)
	}
	e.applyPlan(t, up.Plan.ID, 200)
	if e.piholeDigest() == "sha256:pihole-pulled-old" || !e.row(t, "Pi-hole").RollBack {
		t.Fatalf("after the update: digest %s, row %+v", e.piholeDigest(), e.row(t, "Pi-hole"))
	}

	var rb struct{ Plan struct{ ID, Action string } }
	e.json(t, "POST", "/api/v1/apps/installed/"+e.row(t, "Pi-hole").ID+"/rollback/plan", "", nil, 200, &rb)
	if rb.Plan.Action != "apps.rollback" {
		t.Fatalf("action %s", rb.Plan.Action)
	}
	e.applyPlan(t, rb.Plan.ID, 200)
	if e.piholeDigest() != "sha256:pihole-pulled-old" {
		t.Errorf("after the roll back Pi-hole runs %s", e.piholeDigest())
	}
	if r := e.row(t, "Pi-hole"); r.RollBack || r.State != "running" {
		t.Errorf("row after the roll back: %+v", r)
	}
	if !slices.Contains(e.docker.Calls, "create pihole from kept") {
		t.Errorf("calls: %v", e.docker.Calls)
	}
}

// The previous version can't be kept: the app is not touched at all.
func TestAppUpdate_NothingChangesWhenThePreviousVersionCannotBeKept(t *testing.T) {
	e := newEnvWith(t, quietBox(t))
	e.docker.FailKeep = dockerd.ErrUnavailable
	var up struct{ Plan struct{ ID string } }
	e.json(t, "POST", "/api/v1/apps/installed/pihole/update/plan", "", nil, 200, &up)
	b := e.applyPlan(t, up.Plan.ID, http.StatusBadGateway)
	if failOf(b).Error.Code != "update_failed" {
		t.Fatalf("failure: %s", b)
	}
	if slices.Contains(e.docker.Calls, "remove pihole") {
		t.Error("the app was removed although its previous version was not kept")
	}
	if e.row(t, "Pi-hole").State != "running" || e.piholeDigest() != "sha256:pihole-pulled-old" {
		t.Error("the app changed")
	}
}

// A roll back that cannot start the kept version says so and still leaves an
// app running (the one it replaced).
func TestAppRollBack_Failure(t *testing.T) {
	e := newEnvWith(t, quietBox(t))
	var up struct{ Plan struct{ ID string } }
	e.json(t, "POST", "/api/v1/apps/installed/pihole/update/plan", "", nil, 200, &up)
	e.applyPlan(t, up.Plan.ID, 200)
	updated := e.piholeDigest()

	e.docker.FailCreate = func(spec dockerd.ContainerSpec) error {
		if spec.FromKept {
			return dockerd.ErrNoKeptImage
		}
		return nil
	}
	var rb struct{ Plan struct{ ID string } }
	e.json(t, "POST", "/api/v1/apps/installed/pihole/rollback/plan", "", nil, 200, &rb)
	b := e.applyPlan(t, rb.Plan.ID, http.StatusBadGateway)
	f := failOf(b)
	if f.Error.Code != "rollback_failed" || f.Error.Details.Restored == nil || !*f.Error.Details.Restored {
		t.Fatalf("failure: %s", b)
	}
	// Whatever went wrong, an app is there, and it runs what it ran.
	if r := e.row(t, "Pi-hole"); r.State != "running" {
		t.Errorf("the app is %s", r.State)
	}
	if e.piholeDigest() != updated {
		t.Errorf("digest %s, want the updated one %s", e.piholeDigest(), updated)
	}
}

func TestUpdates_HeldWhileARecordingRunsOrIsAboutToStart(t *testing.T) {
	m := quietBox(t)
	if err := m.AddTimer(context.Background(), webcontrol.NewTimer{ServiceReference: "1:0:19:283D:3FB:1:C00000:0:0:0:", Name: "Tatort", Begin: fixedNow.Unix() - 600, End: fixedNow.Unix() + 3000}); err != nil {
		t.Fatal(err)
	}
	e := newEnvWith(t, m)

	// The summary says so, with the time it can be done.
	v := e.updates(t)
	if !v.Window.Closed || v.Window.Reason != "recording" || v.Window.Name != "Tatort" || v.Window.Until != fixedNow.Unix()+3000 {
		t.Fatalf("window: %+v", v.Window)
	}
	// And so does every way of starting an update.
	for _, path := range []string{"/api/v1/apps/updates/plan", "/api/v1/apps/installed/pihole/update/plan"} {
		resp, b := e.do(t, "POST", path, "", nil)
		f := failOf(b)
		if resp.StatusCode != http.StatusConflict || f.Error.Code != "updates_held" || f.Error.Details.Reason != "recording" || f.Error.Details.Name != "Tatort" || f.Error.Details.Until != fixedNow.Unix()+3000 {
			t.Errorf("%s: %d %s", path, resp.StatusCode, b)
		}
	}
	if e.piholeDigest() != "sha256:pihole-pulled-old" || len(e.apt.Applied) != 0 {
		t.Error("something was updated during a recording")
	}
	// A roll back is a recovery: never held back.
	e.json(t, "POST", "/api/v1/apps/installed/pihole/rollback/plan", "", nil, 409, nil) // "nothing kept", not "held"
}

func TestUpdates_HeldBeforeARecordingStartsAndTheGuardTimeIsASetting(t *testing.T) {
	m := quietBox(t)
	// Starts in 10 minutes.
	if err := m.AddTimer(context.Background(), webcontrol.NewTimer{ServiceReference: "1:0:19:283D:3FB:1:C00000:0:0:0:", Name: "News", Begin: fixedNow.Unix() + 600, End: fixedNow.Unix() + 2400}); err != nil {
		t.Fatal(err)
	}
	e := newEnvWith(t, m)
	if v := e.updates(t); !v.Window.Closed || v.Window.Reason != "starting" {
		t.Fatalf("window: %+v", v.Window)
	}
	// With a guard time of 5 minutes it is 10 away: free.
	e.json(t, "PATCH", "/api/v1/settings", `{"values":{"apps.updateGuardMinutes":5}}`, nil, 200, nil)
	if v := e.updates(t); v.Window.Closed {
		t.Fatalf("a 5 minute guard, recording in 10: %+v", v.Window)
	}
	e.json(t, "PATCH", "/api/v1/settings", `{"values":{"apps.updateGuardMinutes":15}}`, nil, 200, nil)
	if v := e.updates(t); !v.Window.Closed {
		t.Fatal("15 minutes is the default and holds it back")
	}
}

// The night window is for automatic updates; a person pressing "Update all"
// during the day is only held back by a recording.
func TestUpdates_NightWindowDoesNotHoldBackAManualUpdate(t *testing.T) {
	e := newEnvWith(t, quietBox(t))
	e.json(t, "PATCH", "/api/v1/settings", `{"values":{"apps.updateNightOnly":true,"apps.updateNightStart":"02:00","apps.updateNightEnd":"03:00"}}`, nil, 200, nil)
	if v := e.updates(t); v.Window.Closed {
		t.Fatalf("the night window held back a manual update: %+v", v.Window)
	}
	e.json(t, "POST", "/api/v1/apps/updates/plan", "", nil, 200, nil)
}

func TestUpdates_PermissionsAndCSRF(t *testing.T) {
	e := newEnvWith(t, quietBox(t))
	admin, viewer, _ := e.household(t)
	routes := []struct{ method, path string }{
		{"GET", "/api/v1/apps/updates"},
		{"POST", "/api/v1/apps/updates/plan"},
		{"POST", "/api/v1/apps/installed/pihole/update/plan"},
		{"POST", "/api/v1/apps/installed/pihole/rollback/plan"},
		{"POST", "/api/v1/apps/docker/images/kept/plan"},
	}
	for _, r := range routes {
		if resp, b := e.do(t, r.method, r.path, "", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
			t.Errorf("viewer %s %s: %d %s", r.method, r.path, resp.StatusCode, b)
		}
		if r.method == "POST" {
			resp, b := e.do(t, r.method, r.path, "", func(req *http.Request) {
				withProfile(admin)(req)
				req.Header.Del(httpx.CSRFHeader)
			})
			if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusForbidden || code != "csrf_failed" {
				t.Errorf("no CSRF header %s: %d %s", r.path, resp.StatusCode, b)
			}
		}
	}
	// A plan belongs to the person who previewed it.
	var p updPlan
	e.json(t, "POST", "/api/v1/apps/updates/plan", "", withProfile(admin), 200, &p)
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("another profile applying the plan: %d %s", resp.StatusCode, b)
	}
	if e.piholeDigest() != "sha256:pihole-pulled-old" {
		t.Error("a refused apply changed something")
	}
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(admin), 200, nil)
}

// The preview goes stale when what it listed changes.
func TestUpdates_PlanGoesStaleWhenWhatItListsChanges(t *testing.T) {
	e := newEnvWith(t, quietBox(t))
	var p updPlan
	e.json(t, "POST", "/api/v1/apps/updates/plan", "", nil, 200, &p)
	if _, err := e.apt.Apply(context.Background(), "upgrade", []string{"oscam-cycecplus"}); err != nil {
		t.Fatal(err)
	}
	resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "plan_stale") {
		t.Fatalf("expected plan_stale, got %d %s", resp.StatusCode, b)
	}
}

func TestUpdates_SettingsDefaultsAndChecks(t *testing.T) {
	e := newEnv(t)
	var v struct {
		Sections []struct {
			ID     string
			Fields []struct {
				Key       string
				Hidden    bool
				IsDefault bool
				Value     any
			}
		}
	}
	e.json(t, "GET", "/api/v1/settings", "", nil, 200, &v)
	want := map[string]any{
		"apps.autoUpdateContainers": false, "apps.autoUpdatePackages": false, "apps.updateNightOnly": false,
		"apps.updateNightStart": "02:00", "apps.updateNightEnd": "05:00", "apps.updateGuardMinutes": float64(15),
	}
	for _, s := range v.Sections {
		if s.ID != "apps" {
			continue
		}
		for _, f := range s.Fields {
			if w, ok := want[f.Key]; ok {
				if !f.Hidden || !f.IsDefault || f.Value != w {
					t.Errorf("%s: %+v, want default %v", f.Key, f, w)
				}
				delete(want, f.Key)
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("missing settings: %v", want)
	}
	for _, bad := range []string{`{"apps.updateNightStart":"25:00"}`, `{"apps.updateNightEnd":"5pm"}`, `{"apps.updateGuardMinutes":-1}`, `{"apps.updateGuardMinutes":1000}`} {
		resp, b := e.do(t, "PATCH", "/api/v1/settings", `{"values":`+bad+`}`, nil)
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s accepted: %d %s", bad, resp.StatusCode, b)
		}
	}
	e.json(t, "PATCH", "/api/v1/settings", `{"values":{"apps.updateNightStart":"23:30","apps.autoUpdateContainers":true}}`, nil, 200, nil)
}

// The clean-up leaves the kept previous versions alone; they have their own
// line and their own plan.
func TestKeptImages_ListedSeparatelyAndDroppedByTheirOwnPlan(t *testing.T) {
	e := newEnvWith(t, quietBox(t))
	var up struct{ Plan struct{ ID string } }
	e.json(t, "POST", "/api/v1/apps/installed/pihole/update/plan", "", nil, 200, &up)
	e.applyPlan(t, up.Plan.ID, 200)

	var imgs struct {
		Images    []struct{ ID string }    `json:"images"`
		Kept      []struct{ AppID string } `json:"kept"`
		KeptBytes int64                    `json:"keptBytes"`
	}
	e.json(t, "GET", "/api/v1/apps/docker/images", "", nil, 200, &imgs)
	if len(imgs.Kept) != 1 || imgs.Kept[0].AppID != "pihole" || imgs.KeptBytes == 0 {
		t.Fatalf("kept: %+v", imgs)
	}
	var kp struct{ Plan struct{ ID string } }
	e.json(t, "POST", "/api/v1/apps/docker/images/kept/plan", "", nil, 200, &kp)
	e.applyPlan(t, kp.Plan.ID, 200)
	if e.row(t, "Pi-hole").RollBack {
		t.Error("Roll back is still offered after the kept version was removed")
	}
	resp, b := e.do(t, "POST", "/api/v1/apps/docker/images/kept/plan", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "nothing_to_do" {
		t.Errorf("nothing kept: %d %s", resp.StatusCode, b)
	}
}

// Downgrade: offered only where the feed still carries an older version, and
// done as a plan.
func TestPackages_DowngradeOnlyWhereTheFeedOffersAnOlderVersion(t *testing.T) {
	e := newEnv(t)
	versions := func(name string) []string {
		var v struct{ Versions []string }
		e.json(t, "GET", "/api/v1/admin/packages/"+name+"/versions", "", nil, 200, &v)
		return v.Versions
	}
	if got := versions("oscam-cycecplus"); !slices.Equal(got, []string{"11700", "11650"}) {
		t.Errorf("oscam: %v", got)
	}
	// The usual case: the feed only has the latest — nothing to show.
	if got := versions("dropbear"); len(got) != 0 {
		t.Errorf("dropbear: %v", got)
	}
	if got := versions("dreamhub"); len(got) != 0 {
		t.Errorf("DreamHub's own package must not offer one: %v", got)
	}
	if got := versions("not-installed-at-all"); len(got) != 0 {
		t.Errorf("not installed: %v", got)
	}

	var p struct {
		Plan  struct{ ID, Action string }
		Steps []struct{ Kind, Name, OldVersion, NewVersion string }
	}
	e.json(t, "POST", "/api/v1/admin/packages/downgrade/plan", `{"name":"oscam-cycecplus","version":"11700"}`, nil, 200, &p)
	if p.Plan.Action != "package.downgrade" || len(p.Steps) != 1 || p.Steps[0].Kind != "downgrade" || p.Steps[0].OldVersion != "11742" || p.Steps[0].NewVersion != "11700" {
		t.Fatalf("plan: %+v", p)
	}
	if len(e.apt.Applied) != 0 {
		t.Fatal("the preview changed something")
	}
	e.applyPlan(t, p.Plan.ID, 200)
	if len(e.apt.Applied) != 1 || e.apt.Applied[0] != "downgrade oscam-cycecplus=11700" {
		t.Errorf("apt calls: %v", e.apt.Applied)
	}

	// Refusals: a version the feed doesn't offer, a bad string, DreamHub itself.
	for body, want := range map[string]struct {
		status int
		code   string
	}{
		`{"name":"oscam-cycecplus","version":"9999"}`:        {409, "version_not_offered"},
		`{"name":"oscam-cycecplus","version":"--force-yes"}`: {400, "invalid_body"},
		`{"name":"-o","version":"1"}`:                        {400, "invalid_body"},
		`{"name":"dreamhub","version":"0.1.0"}`:              {409, "package_self"},
		`{"name":"dropbear","version":"2022.82"}`:            {409, "version_not_offered"},
		`{"name":"dropbear"}`:                                {400, "invalid_body"},
	} {
		resp, b := e.do(t, "POST", "/api/v1/admin/packages/downgrade/plan", body, nil)
		if code, _, _ := errCode(t, b); resp.StatusCode != want.status || code != want.code {
			t.Errorf("%s: %d %s, want %d %s", body, resp.StatusCode, b, want.status, want.code)
		}
	}
}

func TestPackages_DowngradePermissionsAndCSRF(t *testing.T) {
	e := newEnv(t)
	admin, viewer, _ := e.household(t)
	for _, r := range []struct{ method, path, body string }{
		{"GET", "/api/v1/admin/packages/oscam-cycecplus/versions", ""},
		{"POST", "/api/v1/admin/packages/downgrade/plan", `{"name":"oscam-cycecplus","version":"11700"}`},
	} {
		if resp, b := e.do(t, r.method, r.path, r.body, withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
			t.Errorf("viewer %s %s: %d %s", r.method, r.path, resp.StatusCode, b)
		}
	}
	resp, b := e.do(t, "POST", "/api/v1/admin/packages/downgrade/plan", `{"name":"oscam-cycecplus","version":"11700"}`, func(req *http.Request) {
		withProfile(admin)(req)
		req.Header.Del(httpx.CSRFHeader)
	})
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusForbidden || code != "csrf_failed" {
		t.Errorf("no CSRF header: %d %s", resp.StatusCode, b)
	}
}
