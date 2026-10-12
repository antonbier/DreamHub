package api_test

import (
	"net/http"
	"testing"
)

func TestAppsCatalog_List(t *testing.T) {
	e := newEnv(t)
	var v struct {
		Apps []struct {
			ID         string `json:"id"`
			Image      string `json:"image"`
			Compatible bool   `json:"compatible"`
		} `json:"apps"`
	}
	e.json(t, "GET", "/api/v1/apps/catalog", "", nil, 200, &v)
	if len(v.Apps) == 0 {
		t.Fatal("got an empty curated catalog")
	}
	var sawPihole bool
	for _, a := range v.Apps {
		if a.ID == "pihole" {
			sawPihole = true
			if !a.Compatible {
				t.Errorf("pihole should be marked compatible against the test binary's own GOARCH")
			}
		}
	}
	if !sawPihole {
		t.Fatalf("expected pihole in the curated catalog, got %+v", v.Apps)
	}
}

func TestAppsSearch(t *testing.T) {
	e := newEnv(t)
	var v struct {
		Results []struct {
			Image string `json:"image"`
		} `json:"results"`
	}
	e.json(t, "GET", "/api/v1/apps/search?q=pihole", "", nil, 200, &v)
	if len(v.Results) == 0 {
		t.Fatal("got no search results for \"pihole\"")
	}

	resp, _ := e.do(t, "GET", "/api/v1/apps/search", "", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("search without q: expected 400, got %d", resp.StatusCode)
	}
}

func TestAppsManifest(t *testing.T) {
	e := newEnv(t)
	var v struct {
		Architectures []struct {
			Architecture string `json:"architecture"`
		} `json:"architectures"`
		Compatible bool `json:"compatible"`
	}
	e.json(t, "GET", "/api/v1/apps/manifest?image=pihole/pihole", "", nil, 200, &v)
	if len(v.Architectures) == 0 {
		t.Fatal("got no architectures for pihole/pihole")
	}
	if !v.Compatible {
		t.Error("pihole/pihole should be compatible against the test binary's own GOARCH")
	}

	resp, b := e.do(t, "GET", "/api/v1/apps/manifest?image=not-a-valid-image-string", "", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid image: expected 400, got %d: %s", resp.StatusCode, b)
	}
}

// Everything under /api/v1/apps/{catalog,search,manifest} needs the
// system permission (docs/APPSHUB_PLAN.md, "Admin-only to manage") —
// unlike the AppsHub nav entry itself, which is for everyone.
func TestAppsCatalog_AdminOnly(t *testing.T) {
	e := newEnv(t)
	_, viewer, _ := e.household(t)

	for _, path := range []string{
		"/api/v1/apps/catalog",
		"/api/v1/apps/search?q=pihole",
		"/api/v1/apps/manifest?image=pihole/pihole",
	} {
		if resp, _ := e.do(t, "GET", path, "", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
			t.Errorf("viewer GET %s: expected 403, got %d", path, resp.StatusCode)
		}
	}
}
