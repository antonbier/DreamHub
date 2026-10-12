package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAppsDockerSetup_PreviewApplyThenNothingToDo(t *testing.T) {
	e := newEnv(t)

	resp, b := e.do(t, "GET", "/api/v1/apps/docker", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"ready":false`) {
		t.Fatalf("status: %d %s", resp.StatusCode, b)
	}

	resp, b = e.do(t, "POST", "/api/v1/apps/docker/setup/plan", "", nil)
	var p struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
		Steps []struct {
			Kind string `json:"kind"`
		} `json:"steps"`
	}
	json.Unmarshal(b, &p)
	if resp.StatusCode != http.StatusOK || len(p.Steps) != 4 || p.Steps[0].Kind != "overlay" || p.Steps[3].Kind != "services" {
		t.Fatalf("plan: %d %s", resp.StatusCode, b)
	}
	if _, b := e.do(t, "GET", "/api/v1/apps/docker", "", nil); !strings.Contains(string(b), `"ready":false`) {
		t.Fatal("a preview alone set Docker up")
	}

	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	resp, b = e.do(t, "GET", "/api/v1/apps/docker", "", nil)
	if !strings.Contains(string(b), `"ready":true`) || !strings.Contains(string(b), `"ok":true`) || !strings.Contains(string(b), "checksum verified") {
		t.Fatalf("after apply: %s", b)
	}
	if resp, b := e.do(t, "POST", "/api/v1/apps/docker/setup/plan", "", nil); resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "nothing_to_do") {
		t.Fatalf("second plan: %d %s", resp.StatusCode, b)
	}
	if _, b := e.do(t, "GET", "/api/v1/changes", "", nil); !strings.Contains(string(b), "Set up Docker") {
		t.Fatalf("no change-log entry: %s", b)
	}
}
