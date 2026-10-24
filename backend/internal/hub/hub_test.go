package hub

import (
	"context"
	"errors"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

func v1Registry(t *testing.T, journalctl bool) *Registry {
	t.Helper()
	r := NewRegistry()
	for _, d := range V1() {
		if err := r.Add(d); err != nil {
			t.Fatal(err)
		}
	}
	lookPath := func(string) (string, error) {
		if journalctl {
			return "/usr/bin/journalctl", nil
		}
		return "", errors.New("not found")
	}
	for _, c := range DefaultCapabilities(lookPath) {
		r.AddCapability(c)
	}
	return r
}

// allOK returns connector reports where everything works.
func allOK() map[string]connector.Report {
	out := map[string]connector.Report{}
	for _, id := range []string{connector.IDWebControl, connector.IDSerienRecorder, connector.IDOpenAI, connector.IDTMDB, connector.IDPlex} {
		out[id] = connector.Report{ID: id, State: connector.OK}
	}
	return out
}

func allEnabled(r *Registry) settings.Values {
	v := settings.Values{}
	for _, d := range r.hubs {
		v[EnabledKey(d.ID)] = true
	}
	return v
}

func byID(ss []Status) map[string]Status {
	m := map[string]Status{}
	for _, s := range ss {
		m[s.ID] = s
	}
	return m
}

func TestStatuses_AllAvailable(t *testing.T) {
	r := v1Registry(t, true)
	for _, s := range r.Statuses(allEnabled(r), allOK()) {
		if s.Level != status.Available || len(s.Problems) != 0 {
			t.Errorf("%s: expected available, got %s %+v", s.ID, s.Level, s.Problems)
		}
	}
}

func TestStatuses_CachedConnectorOfflineOnlyDegrades(t *testing.T) {
	r := v1Registry(t, true)
	reps := allOK()
	reps[connector.IDPlex] = connector.Report{State: connector.Unreachable, Problem: &status.Problem{Code: status.CodeConnectorUnreachable}}
	if got := byID(r.Statuses(allEnabled(r), reps))[IDPlex].Level; got != status.Degraded {
		t.Errorf("Plex switched off: PlexHub shows its cache, want degraded, got %s", got)
	}
	reps[connector.IDPlex] = connector.Report{State: connector.NotConfigured, Problem: &status.Problem{Code: status.CodeConnectorNotConfigured}}
	if got := byID(r.Statuses(allEnabled(r), reps))[IDPlex].Level; got != status.Unavailable {
		t.Errorf("Plex not set up: want unavailable, got %s", got)
	}
}

func TestStatuses_RequiredVsOptional(t *testing.T) {
	r := v1Registry(t, true)
	reps := allOK()
	reps[connector.IDTMDB] = connector.Report{State: connector.NotConfigured, Problem: &status.Problem{Code: status.CodeConnectorNotConfigured}}
	reps[connector.IDOpenAI] = connector.Report{State: connector.NotConfigured, Problem: &status.Problem{Code: status.CodeConnectorNotConfigured}}
	got := byID(r.Statuses(allEnabled(r), reps))

	if got[IDMedia].Level != status.Degraded {
		t.Errorf("TMDB is optional for MediaHub: expected degraded, got %s", got[IDMedia].Level)
	}
	if got[IDDiscovery].Level != status.Unavailable || got[IDDiscovery].Problems[0].Code != status.CodeConnectorNotConfigured {
		t.Errorf("AI is required for DiscoveryHub: expected unavailable with reason, got %+v", got[IDDiscovery])
	}
	if got[IDTimer].Level != status.Available {
		t.Errorf("TimerHub doesn't need TMDB or AI, got %s", got[IDTimer].Level)
	}
}

func TestStatuses_BoxUnreachableMakesDependentHubsUnavailable(t *testing.T) {
	r := v1Registry(t, true)
	reps := allOK()
	reps[connector.IDWebControl] = connector.Report{State: connector.Unreachable, Problem: &status.Problem{Code: status.CodeConnectorUnreachable}}
	got := byID(r.Statuses(allEnabled(r), reps))
	for _, id := range []string{IDMedia, IDTimer, IDGuide, IDDiscovery, IDSeries} {
		if got[id].Level != status.Unavailable {
			t.Errorf("%s needs WebControl: expected unavailable, got %s", id, got[id].Level)
		}
	}
	if got[IDDevice].Level == status.Unavailable {
		t.Errorf("DeviceHub's WebControl dependency is optional (its admin logs/services don't need it), expected available or degraded, got %s", got[IDDevice].Level)
	}
}

func TestStatuses_HubDependencyAndCapability(t *testing.T) {
	r := v1Registry(t, false)
	v := allEnabled(r)
	v[EnabledKey(IDTimer)] = false
	got := byID(r.Statuses(v, allOK()))

	d := got[IDDiscovery]
	if d.Level != status.Degraded || d.Problems[0].Code != status.CodeHubDependencyUnavailable || d.Problems[0].Fix != status.FixEnableHub {
		t.Errorf("disabled TimerHub must degrade DiscoveryHub with a fix to enable it, got %+v", d)
	}
	a := got[IDDevice]
	if a.Level != status.Degraded || a.Problems[0].Code != status.CodeCapabilityMissing {
		t.Errorf("missing journalctl degrades DeviceHub (its journald need is optional), got %+v", a)
	}
}

func TestNavigation(t *testing.T) {
	r := v1Registry(t, true)
	v := allEnabled(r)
	v[EnabledKey(IDSeries)] = false
	statuses := r.Statuses(v, allOK())

	nav := r.Navigation(statuses, perm.ForRole(store.RoleAdmin), "hub")
	if nav.Labels != "hub" {
		t.Errorf("labels mode must be passed through, got %q", nav.Labels)
	}
	var ids []string
	for _, s := range nav.Sections {
		ids = append(ids, s.ID)
	}
	want := []string{"main", "media", "program", "device", "bottom"}
	if len(ids) != len(want) {
		t.Fatalf("expected sections %v, got %v", want, ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("expected sections %v, got %v", want, ids)
		}
	}
	if first := nav.Sections[0].Items[0]; first.ID != "dashboard" || first.Kind != "core" {
		t.Errorf("dashboard must come first, got %+v", first)
	}
	media := nav.Sections[1].Items
	if len(media) != 3 || media[0].ID != IDMedia || media[1].ID != IDPlex || media[2].ID != IDNetcast {
		t.Errorf("disabled SeriesHub must be hidden from the media group, got %+v", media)
	}
	program := nav.Sections[2].Items
	if program[0].ID != IDGuide || program[1].ID != IDStream || program[2].ID != IDRadio || program[3].ID != IDTimer || program[4].ID != IDDiscovery {
		t.Errorf("program group must follow declared order, got %+v", program)
	}
	bottom := nav.Sections[4].Items
	if bottom[0].ID != IDDevice || bottom[len(bottom)-1].ID != "settings" {
		t.Errorf("bottom must be DeviceHub then Settings, got %+v", bottom)
	}

	userNav := r.Navigation(statuses, perm.ForRole(store.RoleUser), "descriptive")
	for _, s := range userNav.Sections {
		for _, it := range s.Items {
			if it.ID == IDDevice {
				t.Fatal("DeviceHub must be hidden from non-admins")
			}
		}
	}
}

func TestNavigation_OmitsEmptyGroups(t *testing.T) {
	r := v1Registry(t, true)
	v := allEnabled(r)
	v[EnabledKey(IDControl)] = false
	v[EnabledKey(IDChannels)] = false
	for _, s := range r.Navigation(r.Statuses(v, allOK()), perm.ForRole(store.RoleAdmin), "descriptive").Sections {
		if s.ID == string(GroupDevice) {
			t.Fatal("device group has no enabled hub and must be omitted")
		}
	}
}

func TestAdd_RejectsDuplicatesAndUnknownGroups(t *testing.T) {
	r := NewRegistry()
	if err := r.Add(Descriptor{ID: "x", Group: GroupMedia}); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(Descriptor{ID: "x", Group: GroupMedia}); err == nil {
		t.Error("duplicate hub id must be rejected")
	}
	if err := r.Add(Descriptor{ID: "y", Group: "nowhere"}); err == nil {
		t.Error("unknown group must be rejected")
	}
}

// A box that is another machine: device capabilities (journalctl) say
// nothing about it, and a hub that administers DreamHub's own machine is
// hidden rather than greyed (docs/MULTIDEVICE_PLAN.md, M2).
func TestStatusesFor_RemoteBox(t *testing.T) {
	r := v1Registry(t, false)
	v := allEnabled(r)
	ctx := box.WithRemote(context.Background())
	got := byID(r.StatusesFor(ctx, v, allOK()))

	if d := got[IDDevice]; d.Level != status.Available || len(d.Problems) != 0 {
		t.Errorf("journalctl is a fact about DreamHub's machine, not about another box: %+v", d)
	}
	a := got[IDApps]
	if !a.Hidden || a.Level != status.Unavailable || a.Problems[0].Code != status.CodeBoxNotFull {
		t.Errorf("AppsHub must be hidden on a box DreamHub doesn't run on, got %+v", a)
	}
	for _, s := range r.Navigation(r.StatusesFor(ctx, v, allOK()), perm.ForRole(store.RoleAdmin), "").Sections {
		for _, it := range s.Items {
			if it.ID == IDApps {
				t.Error("AppsHub must not appear in the navigation of a remote box")
			}
		}
	}
	if local := byID(r.StatusesFor(context.Background(), v, allOK())); local[IDApps].Hidden {
		t.Error("AppsHub stays on the local box")
	}
}
