package hubs

import (
	"slices"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/apt"
)

func pkgNames(pkgs []apt.Package) []string {
	var out []string
	for _, p := range pkgs {
		out = append(out, p.Name)
	}
	return out
}

func TestPackageWatch(t *testing.T) {
	w := newPackageWatch()
	steps := []struct {
		pkgs  []apt.Package
		newly []string
	}{
		// Baseline: a already upgradable at start — not announced.
		{[]apt.Package{{Name: "a", Upgradable: true}, {Name: "b"}}, nil},
		// b becomes upgradable too: announced.
		{[]apt.Package{{Name: "a", Upgradable: true}, {Name: "b", Upgradable: true}}, []string{"b"}},
		// Nothing new.
		{[]apt.Package{{Name: "a", Upgradable: true}, {Name: "b", Upgradable: true}}, nil},
		// a gets installed (no longer upgradable); c is new and upgradable.
		{[]apt.Package{{Name: "a"}, {Name: "b", Upgradable: true}, {Name: "c", Upgradable: true}}, []string{"c"}},
		// a becomes upgradable again: announced a second time.
		{[]apt.Package{{Name: "a", Upgradable: true}, {Name: "b", Upgradable: true}, {Name: "c", Upgradable: true}}, []string{"a"}},
	}
	for i, st := range steps {
		got := pkgNames(w.step(st.pkgs))
		if !slices.Equal(got, st.newly) {
			t.Errorf("step %d: newly = %v, want %v", i, got, st.newly)
		}
	}
}
