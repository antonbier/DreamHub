package hubs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/apt"
	aptmock "github.com/antonbier/DreamHub/backend/internal/apt/mock"
	configfsmock "github.com/antonbier/DreamHub/backend/internal/configfs/mock"
	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	dockermock "github.com/antonbier/DreamHub/backend/internal/dockerd/mock"
	dockerhubmock "github.com/antonbier/DreamHub/backend/internal/dockerhub/mock"
	"github.com/antonbier/DreamHub/backend/internal/events"
	meminfomock "github.com/antonbier/DreamHub/backend/internal/meminfo/mock"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	wcmock "github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// published records what an automatic run tells the notification inbox.
type published struct {
	mu   sync.Mutex
	list []publishedEvent
}

type publishedEvent struct {
	key    string
	params map[string]any
}

func (p *published) Publish(_ context.Context, key string, params any) (store.Notification, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m, _ := params.(map[string]any)
	p.list = append(p.list, publishedEvent{key, m})
	return store.Notification{}, nil
}

func (p *published) keys() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, e := range p.list {
		out = append(out, e.key)
	}
	return out
}

type updEnv struct {
	*Apps
	docker *dockermock.Client
	apt    *aptmock.Lister
	wc     *wcmock.Client
	bus    *published
	now    time.Time
}

// newUpdEnv is an Apps over in-memory clients with the clock at 12:00 Berlin
// time on a day without a clock change, the box quiet (no timers).
func newUpdEnv(t *testing.T) *updEnv {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tz database")
	}
	base := newSettingsApps(t)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	docker, apt, wc := dockermock.New(), aptmock.New(), wcmock.New()
	list, _ := wc.TimerList(context.Background())
	for _, tm := range list {
		_ = wc.DeleteTimer(context.Background(), webcontrol.TimerRef{ServiceReference: tm.ServiceReference, Begin: tm.Begin, End: tm.End})
	}
	e := &updEnv{docker: docker, apt: apt, wc: wc, bus: &published{}, now: time.Date(2026, 10, 14, 12, 0, 0, 0, loc)}
	h := base
	h.Docker, h.DockerHub, h.WC, h.Mem = docker, dockerhubmock.Client{}, wc, meminfomock.New()
	h.StorageRoot = "/media/hdd/dreamhub-apps"
	h.ConfigFS = configfsmock.New()
	h.PortFree = func(appcatalog.Port) bool { return true }
	h.Plans = plan.NewService(st, nil)
	h.Store = st
	h.Now = func() time.Time { return e.now }
	h.Loc = loc
	h.Settle = 10 * time.Millisecond
	h.Admin = &Admin{Apt: apt, Packages: apt, Store: st, WC: wc, Plans: h.Plans, Now: h.Now}
	e.Apps = h
	return e
}

func (e *updEnv) set(t *testing.T, kv map[string]any) {
	t.Helper()
	for k, v := range kv {
		if err := e.saveSetting(context.Background(), k, v); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
	}
}

// check runs the registry check the watch does before an automatic update.
func (e *updEnv) check() { e.setPending(e.checkUpgradable(context.Background()), e.now) }

func (e *updEnv) piholeDigest() string {
	d, _ := e.docker.ImageDigest(context.Background(), "pihole")
	return d
}

const oldPihole = "sha256:pihole-pulled-old"

func TestAutoUpdate_OffByDefault(t *testing.T) {
	e := newUpdEnv(t)
	e.check()
	e.autoUpdate(context.Background(), e.bus, nil)
	if e.piholeDigest() != oldPihole || len(e.apt.Applied) != 0 || len(e.bus.keys()) != 0 {
		t.Fatalf("something was updated with the switches off: %s %v %v", e.piholeDigest(), e.apt.Applied, e.bus.keys())
	}
}

func TestAutoUpdate_ContainersOnlyAndTheResultGoesToTheInbox(t *testing.T) {
	e := newUpdEnv(t)
	e.set(t, map[string]any{KeyAppsAutoContainers: true})
	e.check()
	e.autoUpdate(context.Background(), e.bus, nil)
	if e.piholeDigest() == oldPihole {
		t.Fatal("Pi-hole was not updated")
	}
	if len(e.apt.Applied) != 0 {
		t.Errorf("packages were upgraded with their switch off: %v", e.apt.Applied)
	}
	if got := e.bus.keys(); len(got) != 1 || got[0] != events.UpdatesApplied {
		t.Fatalf("inbox: %v", got)
	}
	if p := e.bus.list[0].params; p["names"] != "Pi-hole" || p["count"] != 1 {
		t.Errorf("params: %v", p)
	}
}

// Packages that restart enigma2 are not touched by an automatic run; the rest
// are.
func TestAutoUpdate_PackagesLeaveEnigma2Alone(t *testing.T) {
	e := newUpdEnv(t)
	e.set(t, map[string]any{KeyAppsAutoPackages: true})
	e.autoUpdate(context.Background(), e.bus, nil)
	if len(e.apt.Applied) != 1 || e.apt.Applied[0] != "upgrade oscam-cycecplus" {
		t.Fatalf("apt calls: %v (only the package that does not touch enigma2)", e.apt.Applied)
	}
	for _, p := range e.apt.Packages {
		if (p.Name == "enigma2-plugin-extensions-mediaportal" || p.Name == "enigma2-plugin-systemplugins-networkmanager") && !p.Upgradable {
			t.Errorf("%s was upgraded automatically", p.Name)
		}
	}
	if e.piholeDigest() != oldPihole {
		t.Error("containers were updated with their switch off")
	}
	if got := e.bus.keys(); len(got) != 1 || got[0] != events.UpdatesApplied {
		t.Fatalf("inbox: %v", got)
	}
}

// Window rules for automatic runs, table-driven: a recording in the way, one
// about to start, the night window, and a box that can't be asked.
func TestAutoUpdate_Window(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tz database")
	}
	at := func(y int, mo time.Month, d, h, m int) time.Time { return time.Date(y, mo, d, h, m, 0, 0, loc) }
	night := map[string]any{KeyAppsNightOnly: true, KeyAppsNightStart: "02:00", KeyAppsNightEnd: "05:00"}
	rec := func(begin, end time.Time) *webcontrol.NewTimer {
		return &webcontrol.NewTimer{ServiceReference: "1:0:19:283D:3FB:1:C00000:0:0:0:", Name: "Tatort", Begin: begin.Unix(), End: end.Unix()}
	}
	noon := at(2026, 10, 14, 12, 0)
	tests := []struct {
		name     string
		now      time.Time
		settings map[string]any
		timer    *webcontrol.NewTimer
		runs     bool
	}{
		{"free in the middle of the day", noon, nil, nil, true},
		{"a recording is running", noon, nil, rec(noon.Add(-time.Hour), noon.Add(time.Hour)), false},
		{"one starts in 10 minutes", noon, nil, rec(noon.Add(10*time.Minute), noon.Add(time.Hour)), false},
		{"one starts in 20 minutes (guard 15)", noon, nil, rec(noon.Add(20*time.Minute), noon.Add(time.Hour)), true},
		{"a longer guard time", noon, map[string]any{KeyAppsGuardMinutes: 30}, rec(noon.Add(20*time.Minute), noon.Add(time.Hour)), false},
		{"night window on, at noon", noon, night, nil, false},
		{"night window on, at 03:00", at(2026, 10, 15, 3, 0), night, nil, true},
		{"night window on, at 05:00 sharp", at(2026, 10, 15, 5, 0), night, nil, false},
		{"night window on, recording at 03:00", at(2026, 10, 15, 3, 0), night, rec(at(2026, 10, 15, 2, 30), at(2026, 10, 15, 3, 30)), false},
		// 2026-10-25: the clocks go back, 02:30 is on the clock twice.
		{"DST day, first 02:30 (CEST)", time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC).In(loc), night, nil, true},
		{"DST day, second 02:30 (CET)", time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC).In(loc), night, nil, true},
		{"DST day, 05:00 CET", time.Date(2026, 10, 25, 4, 0, 0, 0, time.UTC).In(loc), night, nil, false},
		{"DST day, 04:59 CET", time.Date(2026, 10, 25, 3, 59, 0, 0, time.UTC).In(loc), night, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newUpdEnv(t)
			e.now = tt.now
			if tt.timer != nil {
				_ = e.wc.AddTimer(context.Background(), *tt.timer)
			}
			e.set(t, map[string]any{KeyAppsAutoContainers: true})
			e.set(t, tt.settings)
			e.check()
			e.autoUpdate(context.Background(), e.bus, nil)
			if updated := e.piholeDigest() != oldPihole; updated != tt.runs {
				t.Errorf("updated = %v, want %v", updated, tt.runs)
			}
		})
	}
}

// A box that can't tell whether it records holds an automatic update back (a
// manual one goes ahead).
func TestAutoUpdate_WaitsWhenTheTimersCannotBeRead(t *testing.T) {
	e := newUpdEnv(t)
	e.WC = nil
	e.set(t, map[string]any{KeyAppsAutoContainers: true})
	e.check()
	e.autoUpdate(context.Background(), e.bus, nil)
	if e.piholeDigest() != oldPihole {
		t.Error("an automatic update ran without knowing about recordings")
	}
	if _, closed := e.updateClosed(context.Background(), false); closed {
		t.Error("a manual update is held back by an unreadable timer list")
	}
}

// Failures are reported to the inbox, with the reason, and not retried every
// 15 minutes.
func TestAutoUpdate_FailureIsReportedAndNotRetriedAtOnce(t *testing.T) {
	e := newUpdEnv(t)
	e.set(t, map[string]any{KeyAppsAutoContainers: true})
	e.check()
	tries := 0
	e.docker.FailCreate = func(spec dockerd.ContainerSpec) error {
		if !spec.FromKept {
			tries++
			return dockerd.ErrUnavailable
		}
		return nil
	}
	e.autoUpdate(context.Background(), e.bus, nil)
	if got := e.bus.keys(); len(got) != 1 || got[0] != events.UpdatesFailed {
		t.Fatalf("inbox: %v", got)
	}
	if p := e.bus.list[0].params; p["name"] != "Pi-hole" || !strings.Contains(p["reason"].(string), "unreachable") {
		t.Errorf("params: %v", p)
	}
	if e.piholeDigest() != oldPihole {
		t.Error("the app does not run its old version after a failed update")
	}
	// A quarter of an hour later: not tried again; a day later: tried.
	e.now = e.now.Add(15 * time.Minute)
	e.autoUpdate(context.Background(), e.bus, nil)
	if tries != 1 {
		t.Errorf("tried again after 15 minutes (%d tries)", tries)
	}
	e.now = e.now.Add(25 * time.Hour)
	e.autoUpdate(context.Background(), e.bus, nil)
	if tries != 2 {
		t.Errorf("not tried again after a day (%d tries)", tries)
	}
}

// An update that does not run is rolled back and says so, in a notice of its own.
func TestAutoUpdate_RolledBackNotice(t *testing.T) {
	e := newUpdEnv(t)
	e.set(t, map[string]any{KeyAppsAutoContainers: true})
	e.check()
	e.docker.ExitAfterCreate = func(spec dockerd.ContainerSpec) bool { return !spec.FromKept }
	e.autoUpdate(context.Background(), e.bus, nil)
	if got := e.bus.keys(); len(got) != 1 || got[0] != events.UpdatesRolledBack {
		t.Fatalf("inbox: %v", got)
	}
	if e.piholeDigest() != oldPihole {
		t.Error("the previous version is not running again")
	}
}

func TestRunUpdates_ContainersBeforePackagesAndAFailureStopsTheRest(t *testing.T) {
	ctx := context.Background()
	e := newUpdEnv(t)
	containers, _ := e.docker.List(ctx)
	g, _ := findAppByID(containers, "pihole")
	rc, ferr := e.prepareSameSettings(ctx, containers, g)
	if ferr != nil {
		t.Fatal(ferr.message)
	}
	// Both fail: the container step fails, so no package may be touched.
	e.docker.FailCreate = func(spec dockerd.ContainerSpec) error {
		if !spec.FromKept {
			return dockerd.ErrUnavailable
		}
		return nil
	}
	names := []string{"oscam-cycecplus"}
	steps, _ := e.apt.Simulate(ctx, "upgrade", names)
	rep := e.runUpdates(ctx, e.User, []*recreate{rc}, names, steps, "")
	if len(rep.Failures) != 1 || rep.Failures[0].Kind != "container" || !rep.Failures[0].RolledBack {
		t.Fatalf("report: %+v", rep)
	}
	if len(e.apt.Applied) != 0 || len(rep.Skipped) != 1 || rep.Skipped[0] != "packages" {
		t.Errorf("packages ran or are not reported as skipped: %v %v", e.apt.Applied, rep.Skipped)
	}
	if f := rep.failure(); f == nil || f.Code != "updates_failed" {
		t.Errorf("failure: %+v", f)
	}

	// All good: containers first, then the packages.
	e.docker.FailCreate = nil
	e.docker.Calls = nil
	containers, _ = e.docker.List(ctx)
	g, _ = findAppByID(containers, "pihole")
	rc, _ = e.prepareSameSettings(ctx, containers, g)
	rep = e.runUpdates(ctx, e.User, []*recreate{rc}, names, steps, "")
	if rep.failure() != nil || len(rep.Done) != 2 || rep.Done[0] != "Pi-hole" || rep.Done[1] != "packages" || rep.Packages != 1 {
		t.Fatalf("report: %+v", rep)
	}
	if len(e.apt.Applied) != 1 {
		t.Errorf("apt calls: %v", e.apt.Applied)
	}
}

// A package failure is reported after the containers were updated; the
// containers are not undone (they run well).
func TestRunUpdates_PackageFailureAfterContainers(t *testing.T) {
	ctx := context.Background()
	e := newUpdEnv(t)
	containers, _ := e.docker.List(ctx)
	g, _ := findAppByID(containers, "pihole")
	rc, _ := e.prepareSameSettings(ctx, containers, g)
	e.apt.ApplyErr = errors.New("exit status 100")
	steps := []apt.Step{{Kind: "upgrade", Name: "oscam-cycecplus", OldVersion: "11742", NewVersion: "11760"}}
	rep := e.runUpdates(ctx, e.User, []*recreate{rc}, []string{"oscam-cycecplus"}, steps, "")
	if len(rep.Failures) != 1 || rep.Failures[0].Kind != "package" || len(rep.Done) != 1 || rep.Done[0] != "Pi-hole" {
		t.Fatalf("report: %+v", rep)
	}
	if !strings.Contains(rep.Failures[0].Output, "dpkg returned an error") {
		t.Errorf("apt's output is not in the report: %q", rep.Failures[0].Output)
	}
	if e.piholeDigest() == oldPihole {
		t.Error("the updated container was undone because a package failed")
	}
}

// --- crash loop --------------------------------------------------------------

func cont(id, app string, restarts int) dockerd.Container {
	return dockerd.Container{ID: id, Name: "dreamhub-app-" + app, State: "running", RestartCount: restarts, Labels: map[string]string{dockerd.AppLabel: app}}
}

func stopped(id, app string, restarts int) dockerd.Container {
	c := cont(id, app, restarts)
	c.State = "exited"
	return c
}

func TestCrashTracker(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	type look struct {
		at    time.Duration
		cs    []dockerd.Container
		newly []string
	}
	tests := []struct {
		name  string
		looks []look
	}{
		{"three restarts in ten minutes announce once", []look{
			{0, []dockerd.Container{cont("a", "pihole", 0)}, nil},
			{1 * time.Minute, []dockerd.Container{cont("a", "pihole", 1)}, nil},
			{2 * time.Minute, []dockerd.Container{cont("a", "pihole", 2)}, nil},
			{3 * time.Minute, []dockerd.Container{cont("a", "pihole", 3)}, []string{"pihole"}},
			{4 * time.Minute, []dockerd.Container{cont("a", "pihole", 4)}, nil},
			{5 * time.Minute, []dockerd.Container{cont("a", "pihole", 5)}, nil},
		}},
		{"two restarts are not a loop", []look{
			{0, []dockerd.Container{cont("a", "pihole", 0)}, nil},
			{1 * time.Minute, []dockerd.Container{cont("a", "pihole", 2)}, nil},
			{9 * time.Minute, []dockerd.Container{cont("a", "pihole", 2)}, nil},
		}},
		{"slow restarts spread over more than the window are not a loop", []look{
			{0, []dockerd.Container{cont("a", "pihole", 0)}, nil},
			{6 * time.Minute, []dockerd.Container{cont("a", "pihole", 2)}, nil},
			{12 * time.Minute, []dockerd.Container{cont("a", "pihole", 3)}, nil},
			{18 * time.Minute, []dockerd.Container{cont("a", "pihole", 4)}, nil},
		}},
		{"a count that was already high at the first look is not news", []look{
			{0, []dockerd.Container{cont("a", "pihole", 5)}, nil},
			{1 * time.Minute, []dockerd.Container{cont("a", "pihole", 5)}, nil},
		}},
		{"first seen stopped after its restarts ran out is announced once", []look{
			{0, []dockerd.Container{stopped("a", "pihole", 5)}, []string{"pihole"}},
			{1 * time.Minute, []dockerd.Container{stopped("a", "pihole", 5)}, nil},
		}},
		{"a new episode after a quiet window is announced again", []look{
			{0, []dockerd.Container{cont("a", "pihole", 0)}, nil},
			{1 * time.Minute, []dockerd.Container{cont("a", "pihole", 3)}, []string{"pihole"}},
			{5 * time.Minute, []dockerd.Container{cont("a", "pihole", 3)}, nil},
			{11 * time.Minute, []dockerd.Container{cont("a", "pihole", 3)}, nil},
			{12 * time.Minute, []dockerd.Container{cont("a", "pihole", 3)}, nil}, // quiet for the window: the episode is over
			{13 * time.Minute, []dockerd.Container{cont("a", "pihole", 6)}, []string{"pihole"}},
		}},
		{"started by hand: the daemon's count starts over", []look{
			{0, []dockerd.Container{cont("a", "pihole", 5)}, nil},
			{1 * time.Minute, []dockerd.Container{cont("a", "pihole", 0)}, nil},
			{2 * time.Minute, []dockerd.Container{cont("a", "pihole", 1)}, nil},
			{3 * time.Minute, []dockerd.Container{cont("a", "pihole", 2)}, nil},
		}},
		{"two apps are told apart", []look{
			{0, []dockerd.Container{cont("a", "pihole", 0), cont("b", "adguardhome", 0)}, nil},
			{1 * time.Minute, []dockerd.Container{cont("a", "pihole", 1), cont("b", "adguardhome", 3)}, []string{"adguardhome"}},
		}},
		{"a made-again container is a new one", []look{
			{0, []dockerd.Container{cont("a", "pihole", 2)}, nil},
			{1 * time.Minute, []dockerd.Container{cont("a2", "pihole", 0)}, nil},
			{2 * time.Minute, []dockerd.Container{cont("a2", "pihole", 2)}, nil},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tr crashTracker
			for i, l := range tt.looks {
				got := tr.step(t0.Add(l.at), l.cs)
				if strings.Join(got, ",") != strings.Join(l.newly, ",") {
					t.Fatalf("look %d (+%s): announced %v, want %v", i, l.at, got, l.newly)
				}
			}
		})
	}
}

// The watch turns it into one inbox entry, with the app's id for the link to
// its logs.
func TestCrashLook_PublishesOncePerEpisode(t *testing.T) {
	e := newUpdEnv(t)
	ctx := context.Background()
	for i, n := range []int{0, 1, 2, 3, 4, 5} {
		e.docker.SetState("pihole", "running", n)
		e.now = e.now.Add(time.Minute)
		e.crashLook(ctx, e.bus, nil)
		if i < 3 && len(e.bus.keys()) != 0 {
			t.Fatalf("announced too early (%d restarts)", n)
		}
	}
	if got := e.bus.keys(); len(got) != 1 || got[0] != events.AppCrashLoop {
		t.Fatalf("inbox: %v", got)
	}
	if p := e.bus.list[0].params; p["appId"] != "pihole" || p["name"] != "Pi-hole" {
		t.Errorf("params: %v", p)
	}
}
