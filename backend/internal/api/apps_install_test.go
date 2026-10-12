package api_test

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
)

type installPlan struct {
	Plan struct {
		ID      string `json:"id"`
		Summary string `json:"summary"`
	} `json:"plan"`
}

func (e *env) planInstall(t *testing.T, body string) (int, installPlan, []byte) {
	t.Helper()
	resp, b := e.do(t, "POST", "/api/v1/apps/install/plan", body, nil)
	var p installPlan
	_ = json.Unmarshal(b, &p)
	return resp.StatusCode, p, b
}

type installedList struct {
	Apps []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		State    string `json:"state"`
		HostPort int    `json:"hostPort"`
	} `json:"apps"`
}

func (e *env) installed(t *testing.T) installedList {
	t.Helper()
	_, b := e.do(t, "GET", "/api/v1/apps/installed", "", nil)
	var v installedList
	_ = json.Unmarshal(b, &v)
	return v
}

func TestAppsInstall_PreviewAndApply(t *testing.T) {
	e := newEnv(t)
	before := e.installed(t)

	code, p, b := e.planInstall(t, `{"appId":"sonarr"}`)
	if code != http.StatusOK || p.Plan.ID == "" {
		t.Fatalf("plan: %d %s", code, b)
	}
	// Previewing must not install anything yet.
	if len(e.installed(t).Apps) != len(before.Apps) {
		t.Fatal("preview alone installed something")
	}

	resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}

	after := e.installed(t)
	if len(after.Apps) != len(before.Apps)+1 {
		t.Fatalf("expected one more installed app, got %+v", after)
	}
	var sonarr *struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		State    string `json:"state"`
		HostPort int    `json:"hostPort"`
	}
	for i := range after.Apps {
		if after.Apps[i].Name == "Sonarr" {
			sonarr = &after.Apps[i]
		}
	}
	if sonarr == nil {
		t.Fatalf("Sonarr not found in installed list: %+v", after)
	}
	if sonarr.State != "running" || sonarr.HostPort != 8989 {
		t.Fatalf("got %+v, want running on 8989", sonarr)
	}
}

func TestAppsInstall_UnknownApp(t *testing.T) {
	e := newEnv(t)
	code, _, _ := e.planInstall(t, `{"appId":"not-a-real-app"}`)
	if code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", code)
	}
}

func TestAppsInstall_AlreadyInstalledRefused(t *testing.T) {
	e := newEnv(t)
	// pihole is one of the mock Docker client's seeded fixtures
	// (internal/dockerd/mock), so it's already "installed" from the
	// start.
	code, _, b := e.planInstall(t, `{"appId":"pihole"}`)
	if code != http.StatusConflict {
		t.Fatalf("got %d, want 409: %s", code, b)
	}
}

// Apps share the box's network (docker --network host, the only kind its
// kernel allows), so an app keeps its own port: a different one is
// refused, and so is the app's own when something already holds it.
func TestAppsInstall_PortIsTheAppsOwn(t *testing.T) {
	e := newEnv(t)
	code, _, b := e.planInstall(t, `{"appId":"sonarr","hostPort":9999}`)
	if code != http.StatusConflict || !strings.Contains(string(b), "8989") {
		t.Fatalf("remapped port: got %d %s", code, b)
	}

	l, err := net.Listen("tcp", ":8989")
	if err != nil {
		t.Skip("port 8989 is busy on this machine:", err)
	}
	defer l.Close()
	code, _, b = e.planInstall(t, `{"appId":"sonarr"}`)
	if code != http.StatusConflict || !strings.Contains(string(b), "8989") {
		t.Fatalf("taken port: got %d %s", code, b)
	}
}

func TestAppsInstall_NoWebUIAppGetsNoPort(t *testing.T) {
	e := newEnv(t)
	code, p, b := e.planInstall(t, `{"appId":"wireguard"}`)
	if code != http.StatusOK {
		t.Fatalf("plan: %d %s", code, b)
	}
	resp, applyBody := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, applyBody)
	}
	after := e.installed(t)
	for _, a := range after.Apps {
		if a.Name == "WireGuard" && a.HostPort != 0 {
			t.Fatalf("wireguard (no web UI) got a host port: %+v", a)
		}
	}
}

func TestAppsRemove_PreviewAndApply(t *testing.T) {
	e := newEnv(t)
	before := e.installed(t)
	if len(before.Apps) == 0 {
		t.Fatal("expected at least one seeded mock app to remove")
	}
	id := before.Apps[0].ID

	resp, b := e.do(t, "POST", "/api/v1/apps/installed/"+id+"/remove/plan", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d %s", resp.StatusCode, b)
	}
	var p installPlan
	_ = json.Unmarshal(b, &p)

	resp, b = e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	after := e.installed(t)
	if len(after.Apps) != len(before.Apps)-1 {
		t.Fatalf("expected one fewer installed app, got %+v", after)
	}
}

func TestAppsStartStop(t *testing.T) {
	e := newEnv(t)
	before := e.installed(t)
	var stopped *struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		State    string `json:"state"`
		HostPort int    `json:"hostPort"`
	}
	for i := range before.Apps {
		if before.Apps[i].State == "exited" {
			stopped = &before.Apps[i]
		}
	}
	if stopped == nil {
		t.Fatal("expected a seeded mock app that starts out stopped")
	}

	resp, _ := e.do(t, "POST", "/api/v1/apps/installed/"+stopped.ID+"/start", "", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("start: %d", resp.StatusCode)
	}
	after := e.installed(t)
	if state(after, stopped.ID) != "running" {
		t.Fatalf("after start: got %q", state(after, stopped.ID))
	}

	resp, _ = e.do(t, "POST", "/api/v1/apps/installed/"+stopped.ID+"/stop", "", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("stop: %d", resp.StatusCode)
	}
	after = e.installed(t)
	if state(after, stopped.ID) != "exited" {
		t.Fatalf("after stop: got %q", state(after, stopped.ID))
	}
}

func state(list installedList, id string) string {
	for _, a := range list.Apps {
		if a.ID == id {
			return a.State
		}
	}
	return ""
}

func TestAppsInstall_AdminOnly(t *testing.T) {
	e := newEnv(t)
	_, viewer, _ := e.household(t)

	if resp, _ := e.do(t, "GET", "/api/v1/apps/installed", "", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer GET installed: expected 403, got %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "POST", "/api/v1/apps/install/plan", `{"appId":"sonarr"}`, withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer install: expected 403, got %d", resp.StatusCode)
	}
}
