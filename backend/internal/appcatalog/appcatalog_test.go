package appcatalog_test

import (
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
)

func TestApps_HaveRequiredFields(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range appcatalog.Apps {
		if a.ID == "" || a.Name == "" || a.Image == "" || len(a.Arches) == 0 {
			t.Errorf("app %+v is missing a required field", a)
		}
		if seen[a.ID] {
			t.Errorf("duplicate app id %q", a.ID)
		}
		seen[a.ID] = true
	}
}

func TestApp_SupportsArch(t *testing.T) {
	piHole, ok := appcatalog.ByID("pihole")
	if !ok {
		t.Fatal("pihole missing from the curated list")
	}
	if !piHole.SupportsArch(dockerhub.Arch{Architecture: "arm64"}) {
		t.Error("pihole should support arm64")
	}
	if !piHole.SupportsArch(dockerhub.Arch{Architecture: "arm", Variant: "v7"}) {
		t.Error("pihole should support armhf (arm/v7)")
	}

	heimdall, ok := appcatalog.ByID("heimdall")
	if !ok {
		t.Fatal("heimdall missing from the curated list")
	}
	if heimdall.SupportsArch(dockerhub.Arch{Architecture: "arm", Variant: "v7"}) {
		t.Error("heimdall (amd64+arm64 only) must not claim armhf support")
	}
}

func TestByID_Unknown(t *testing.T) {
	if _, ok := appcatalog.ByID("not-a-real-app"); ok {
		t.Fatal("got ok=true for an unknown id")
	}
}
