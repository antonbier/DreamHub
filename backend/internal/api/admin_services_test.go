package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

type serviceList struct {
	Services []struct {
		Name   string `json:"name"`
		Active string `json:"active"`
	} `json:"services"`
}

func TestAdminServices_ListsTheBoxServices(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/admin/services", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var body serviceList
	json.Unmarshal(b, &body)
	var names []string
	state := map[string]string{}
	for _, s := range body.Services {
		names = append(names, s.Name)
		state[s.Name] = s.Active
	}
	// DefaultServices' order; the alternative names (smbd, zerotier-one,
	// oscam, …) don't exist on the mock box, as on the reference box, and
	// are left out.
	want := []string{"enigma2.service", "dreamhub.service", "smb.service", "nmb.service", "nfs-server.service", "avahi-daemon.service", "zerotierone.service", "oscam-cycecplus.service"}
	if !slices.Equal(names, want) {
		t.Fatalf("services = %v, want %v", names, want)
	}
	if state["nmb.service"] != "failed" || state["enigma2.service"] != "active" {
		t.Errorf("states: %v", state)
	}
}

type restartPlan struct {
	Plan struct {
		ID       string `json:"id"`
		Action   string `json:"action"`
		Warnings []struct {
			Code   string         `json:"code"`
			Params map[string]any `json:"params"`
		} `json:"warnings"`
	} `json:"plan"`
}

func (e *env) planRestart(t *testing.T, name string) (int, restartPlan) {
	t.Helper()
	resp, b := e.do(t, "POST", "/api/v1/admin/services/plan", `{"name":"`+name+`"}`, nil)
	var p restartPlan
	json.Unmarshal(b, &p)
	return resp.StatusCode, p
}

func TestAdminServices_RestartGoesThroughPreview(t *testing.T) {
	m := mock.New()
	// A recording running at the test clock (fixedNow, 20:07 local).
	m.AddTimer(context.Background(), webcontrol.NewTimer{ServiceReference: "1:0:19:283D:3FB:1:C00000:0:0:0:", Name: "Tatort", Begin: fixedNow.Unix() - 600, End: fixedNow.Unix() + 3000})
	e := newEnvWith(t, m)

	code, p := e.planRestart(t, "enigma2.service")
	if code != http.StatusOK || p.Plan.Action != "service.restart" {
		t.Fatalf("plan: %d %+v", code, p)
	}
	var codes []string
	for _, w := range p.Plan.Warnings {
		codes = append(codes, w.Code)
		if w.Code == "service.recordingsInterrupted" && w.Params["names"] != "Tatort" {
			t.Errorf("expected the running recording named, got %v", w.Params)
		}
	}
	if !slices.Equal(codes, []string{"service.restartEnigma2", "service.recordingsInterrupted"}) {
		t.Errorf("warnings = %v", codes)
	}
	if len(e.systemd.Restarted) != 0 {
		t.Fatal("a preview must not restart anything")
	}
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	if !slices.Equal(e.systemd.Restarted, []string{"enigma2.service"}) {
		t.Errorf("restarted = %v", e.systemd.Restarted)
	}
}

func TestAdminServices_StartingAFailedUnitAndRefusals(t *testing.T) {
	e := newEnv(t)
	if code, p := e.planRestart(t, "nmb.service"); code != http.StatusOK || p.Plan.Action != "service.start" || len(p.Plan.Warnings) != 0 {
		t.Errorf("a failed unit is started, without warnings: %d %+v", code, p)
	}
	if code, p := e.planRestart(t, "dreamhub.service"); code != http.StatusOK || len(p.Plan.Warnings) != 1 || p.Plan.Warnings[0].Code != "service.restartSelf" {
		t.Errorf("restarting DreamHub warns that the page reconnects: %d %+v", code, p)
	}
	// Only the listed units, and only ones the box has.
	for _, name := range []string{"sshd.service", "smbd.service", "../../etc/passwd"} {
		if code, _ := e.planRestart(t, name); code != http.StatusNotFound {
			t.Errorf("%s: expected 404, got %d", name, code)
		}
	}
}

func TestAdminServices_StalePreviewIsRefused(t *testing.T) {
	e := newEnv(t)
	_, p := e.planRestart(t, "smb.service")
	// Something else restarts it in between.
	e.systemd.Restart(context.Background(), "smb.service")
	resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("a preview made before another restart must not apply: %s", b)
	}
	if len(e.systemd.Restarted) != 1 {
		t.Errorf("restarted = %v", e.systemd.Restarted)
	}
}

func TestAdminServices_ReadOnlyRestartsNothing(t *testing.T) {
	e := newEnvWithReadOnlyFiles(t, mock.New())
	_, p := e.planRestart(t, "smb.service")
	resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "writes_disabled" {
		t.Fatalf("expected 409 writes_disabled, got %d %s", resp.StatusCode, b)
	}
	if len(e.systemd.Restarted) != 0 {
		t.Errorf("restarted = %v", e.systemd.Restarted)
	}
}
