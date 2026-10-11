package hub

import (
	"context"
	"errors"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

type stubProvider struct {
	results []SearchResult
	err     error
}

func (p stubProvider) Search(context.Context, string) ([]SearchResult, error) {
	return p.results, p.err
}

func TestSearch_GroupsResultsAndFillsHubFields(t *testing.T) {
	r := v1Registry(t, true)
	r.AddSearchProvider(IDTimer, stubProvider{results: []SearchResult{{Title: "Die Kanzlei", Path: "/timers"}}})

	out := r.Search(context.Background(), allEnabled(r), allOK(), perm.ForRole(store.RoleAdmin), "kanzlei")
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1: %+v", len(out), out)
	}
	if out[0].HubID != IDTimer || out[0].HubLabel != "hub.timer" {
		t.Errorf("hub fields not filled in: %+v", out[0])
	}
}

func TestSearch_SkipsADisabledHub(t *testing.T) {
	r := v1Registry(t, true)
	r.AddSearchProvider(IDTimer, stubProvider{results: []SearchResult{{Title: "match"}}})

	v := allEnabled(r)
	v[EnabledKey(IDTimer)] = false
	if out := r.Search(context.Background(), v, allOK(), perm.ForRole(store.RoleAdmin), "q"); len(out) != 0 {
		t.Errorf("a disabled hub's provider must not be queried, got %+v", out)
	}
}

func TestSearch_SkipsAHubTheRoleCannotSee(t *testing.T) {
	r := v1Registry(t, true)
	r.AddSearchProvider(IDDevice, stubProvider{results: []SearchResult{{Title: "device match"}}})

	if out := r.Search(context.Background(), allEnabled(r), allOK(), perm.ForRole(store.RoleUser), "q"); len(out) != 0 {
		t.Errorf("DeviceHub is admin-only; a plain user must not see its results, got %+v", out)
	}
}

func TestSearch_SkipsAnUnavailableHub(t *testing.T) {
	r := v1Registry(t, true)
	r.AddSearchProvider(IDMedia, stubProvider{results: []SearchResult{{Title: "media match"}}})

	// MediaHub's own WebControl dependency isn't optional, so making it
	// unreachable is a reliable way to get a genuinely unavailable hub
	// (DeviceHub's own dependencies are both optional, see hub_test.go).
	reps := allOK()
	reps[connector.IDWebControl] = connector.Report{State: connector.Unreachable, Problem: &status.Problem{Code: status.CodeConnectorUnreachable}}
	if out := r.Search(context.Background(), allEnabled(r), reps, perm.ForRole(store.RoleAdmin), "q"); len(out) != 0 {
		t.Errorf("an unavailable hub's provider must not be queried, got %+v", out)
	}
}

func TestSearch_AProviderErrorDoesNotFailTheRest(t *testing.T) {
	r := v1Registry(t, true)
	r.AddSearchProvider(IDTimer, stubProvider{err: errors.New("webcontrol unavailable")})
	r.AddSearchProvider(IDDevice, stubProvider{results: []SearchResult{{Title: "ok"}}})

	out := r.Search(context.Background(), allEnabled(r), allOK(), perm.ForRole(store.RoleAdmin), "q")
	if len(out) != 1 || out[0].Title != "ok" {
		t.Errorf("a failing provider must be skipped, not fail the whole search: %+v", out)
	}
}
