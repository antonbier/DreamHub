package hubs

import (
	"context"
	"slices"
	"testing"

	dockerd_mock "github.com/antonbier/DreamHub/backend/internal/dockerd/mock"
	dockerhub_mock "github.com/antonbier/DreamHub/backend/internal/dockerhub/mock"
)

func TestAppWatch_Step(t *testing.T) {
	w := newAppWatch()
	steps := []struct {
		upgradable []string
		newly      []string
	}{
		// Baseline: pihole already upgradable at start — not announced.
		{[]string{"pihole"}, nil},
		// adguardhome becomes upgradable too: announced.
		{[]string{"pihole", "adguardhome"}, []string{"adguardhome"}},
		// Nothing new.
		{[]string{"pihole", "adguardhome"}, nil},
		// pihole gets reinstalled (no longer upgradable).
		{[]string{"adguardhome"}, nil},
		// pihole becomes upgradable again: announced a second time.
		{[]string{"pihole", "adguardhome"}, []string{"pihole"}},
	}
	for i, st := range steps {
		got := w.step(st.upgradable)
		slices.Sort(got)
		want := append([]string{}, st.newly...)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("step %d: newly = %v, want %v", i, got, want)
		}
	}
}

func TestApps_CheckUpgradable(t *testing.T) {
	h := &Apps{Docker: dockerd_mock.New(), DockerHub: dockerhub_mock.Client{}}
	upgradable := h.checkUpgradable(context.Background())
	// The mock fixtures are built so pihole's running digest lags behind
	// the registry's current one, adguardhome's matches
	// (internal/dockerd/mock, internal/dockerhub/mock).
	if !slices.Contains(upgradable, "pihole") {
		t.Errorf("got %v, want pihole flagged upgradable", upgradable)
	}
	if slices.Contains(upgradable, "adguardhome") {
		t.Errorf("got %v, adguardhome shouldn't be flagged (digest matches)", upgradable)
	}
}

func TestApps_CheckUpgradable_NilDependencies(t *testing.T) {
	h := &Apps{}
	if got := h.checkUpgradable(context.Background()); got != nil {
		t.Errorf("got %v, want nil with no Docker/DockerHub configured", got)
	}
}
