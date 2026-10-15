package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type packagePlan struct {
	Plan struct {
		ID       string `json:"id"`
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	} `json:"plan"`
	Steps []struct {
		Kind, Name, OldVersion, NewVersion string
	} `json:"steps"`
}

func (e *env) planPackages(t *testing.T, body string) (int, packagePlan, string) {
	t.Helper()
	resp, b := e.do(t, "POST", "/api/v1/admin/packages/plan", body, nil)
	var p packagePlan
	json.Unmarshal(b, &p)
	return resp.StatusCode, p, string(b)
}

func TestAdminPackages_InstallFromCatalog(t *testing.T) {
	e := newEnv(t)

	resp, b := e.do(t, "GET", "/api/v1/admin/packages?status=available&q=htop", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"htop"`) {
		t.Fatalf("catalog: %d %s", resp.StatusCode, b)
	}

	code, p, b2 := e.planPackages(t, `{"action":"install","packages":["htop"]}`)
	if code != http.StatusOK || len(p.Steps) != 1 || p.Steps[0].Kind != "install" || p.Steps[0].NewVersion == "" {
		t.Fatalf("plan: %d %s", code, b2)
	}
	// A preview changes nothing.
	if _, b := e.do(t, "GET", "/api/v1/admin/packages?q=htop", "", nil); strings.Contains(string(b), `"htop"`) {
		t.Fatal("preview alone installed the package")
	}

	resp, b = e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	if _, b := e.do(t, "GET", "/api/v1/admin/packages?q=htop", "", nil); !strings.Contains(string(b), `"htop"`) {
		t.Fatalf("htop not installed after apply: %s", b)
	}
	resp, b = e.do(t, "GET", "/api/v1/admin/packages/last-run", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"ok":true`) || !strings.Contains(string(b), "Setting up htop") {
		t.Fatalf("last run: %d %s", resp.StatusCode, b)
	}
	if _, b := e.do(t, "GET", "/api/v1/changes", "", nil); !strings.Contains(string(b), "Installed htop") {
		t.Fatalf("no change-log entry: %s", b)
	}
}

// "Update all" is ONE plan covering every upgradable package, applied once.
func TestAdminPackages_UpgradeAll(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/admin/packages?status=upgradable", "", nil)
	var before packageList
	json.Unmarshal(b, &before)
	if len(before.Packages) < 2 {
		t.Fatalf("the mock should offer several updates: %s", b)
	}

	code, p, b2 := e.planPackages(t, `{"action":"upgrade","packages":[]}`)
	if code != http.StatusOK || len(p.Steps) != len(before.Packages) {
		t.Fatalf("plan: %d %s", code, b2)
	}
	for _, s := range p.Steps {
		if s.Kind != "upgrade" || s.OldVersion == "" || s.NewVersion == "" {
			t.Errorf("step: %+v", s)
		}
	}
	// A preview alone changes nothing.
	if _, b := e.do(t, "GET", "/api/v1/admin/packages?status=upgradable", "", nil); !strings.Contains(string(b), `"upgradable":true`) {
		t.Fatal("preview alone upgraded packages")
	}
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	_, b = e.do(t, "GET", "/api/v1/admin/packages?status=upgradable", "", nil)
	var after packageList
	json.Unmarshal(b, &after)
	if len(after.Packages) != 0 {
		t.Fatalf("still upgradable after update all: %s", b)
	}
	if code, _, b := e.planPackages(t, `{"action":"upgrade","packages":[]}`); code != http.StatusConflict || !strings.Contains(b, "nothing_to_do") {
		t.Fatalf("second upgrade: %d %s", code, b)
	}
}

// Everything that plans or reads the package state is admin-only.
func TestAdminPackages_ViewerRefused(t *testing.T) {
	e := newEnv(t)
	_, viewer, _ := e.household(t)
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/v1/admin/packages", ""},
		{"POST", "/api/v1/admin/packages/plan", `{"action":"upgrade","packages":[]}`},
		{"POST", "/api/v1/admin/packages/plan", `{"action":"install","packages":["htop"]}`},
		{"POST", "/api/v1/admin/packages/explain", `{"name":"htop"}`},
		{"GET", "/api/v1/admin/packages/last-run", ""},
	} {
		if resp, _ := e.do(t, tc.method, tc.path, tc.body, withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
			t.Errorf("viewer %s %s: expected 403, got %d", tc.method, tc.path, resp.StatusCode)
		}
	}
	// And a plan can't be made without the CSRF header.
	if resp, _ := e.doRaw(t, "POST", "/api/v1/admin/packages/plan", []byte(`{"action":"upgrade","packages":[]}`), "application/json"); resp.StatusCode == http.StatusOK {
		t.Error("plan without the CSRF header was accepted")
	}
}

func TestAdminPackages_StaleAfterOtherChange(t *testing.T) {
	e := newEnv(t)
	_, first, _ := e.planPackages(t, `{"action":"install","packages":["htop"]}`)
	_, second, _ := e.planPackages(t, `{"action":"install","packages":["htop"]}`)
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+second.Plan.ID+"/apply", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply second: %d %s", resp.StatusCode, b)
	}
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+first.Plan.ID+"/apply", "", nil); resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "plan_stale") {
		t.Fatalf("first plan should be stale: %d %s", resp.StatusCode, b)
	}
}

func TestAdminPackages_Refusals(t *testing.T) {
	e := newEnv(t)
	for _, tc := range []struct{ body, code string }{
		{`{"action":"install","packages":["--purge"]}`, "invalid_body"},
		{`{"action":"purge","packages":["htop"]}`, "invalid_body"},
		{`{"action":"install","packages":["no-such-package"]}`, "apt_refused"},
		{`{"action":"install","packages":["dropbear"]}`, "nothing_to_do"},
		// The mock's cascade: removing the skin button takes DreamHub's
		// own package with it — never something DreamHub may do to itself.
		{`{"action":"remove","packages":["enigma2-plugin-skincomponents-dreamhubbutton"]}`, "package_self"},
	} {
		if _, _, b := e.planPackages(t, tc.body); !strings.Contains(b, tc.code) {
			t.Errorf("%s: got %s, want %s", tc.body, b, tc.code)
		}
	}
}
