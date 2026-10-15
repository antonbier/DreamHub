package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
)

type uninstallPlan struct {
	Plan struct {
		ID        string    `json:"id"`
		CreatedAt time.Time `json:"createdAt"`
		ExpiresAt time.Time `json:"expiresAt"`
	} `json:"plan"`
	Steps []struct {
		Kind string `json:"kind"`
	} `json:"steps"`
}

func (u uninstallPlan) kinds() string {
	var k []string
	for _, s := range u.Steps {
		k = append(k, s.Kind)
	}
	return strings.Join(k, ",")
}

// setUpDocker runs "Set up Docker" so there is something to uninstall.
func (e *env) setUpDocker(t *testing.T) {
	t.Helper()
	var p planResp
	e.json(t, "POST", "/api/v1/apps/docker/setup/plan", "", nil, 200, &p)
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil, 200, nil)
}

func (e *env) planUninstall(t *testing.T, body string, configure func(*http.Request)) (int, uninstallPlan, []byte) {
	t.Helper()
	resp, b := e.do(t, "POST", "/api/v1/apps/docker/uninstall/plan", body, configure)
	var p uninstallPlan
	_ = json.Unmarshal(b, &p)
	return resp.StatusCode, p, b
}

func TestAppsDockerUninstall_KeepsDataByDefaultAndIsIdempotent(t *testing.T) {
	e := newEnv(t)
	e.setUpDocker(t)

	code, p, b := e.planUninstall(t, "", nil)
	if code != 200 || p.kinds() != "containers,services,modules,files" {
		t.Fatalf("default plan: %d %s", code, b)
	}
	// A preview alone changes nothing.
	if _, b := e.do(t, "GET", "/api/v1/apps/docker", "", nil); !strings.Contains(string(b), `"ready":true`) {
		t.Fatalf("a preview uninstalled Docker: %s", b)
	}
	if len(e.installed(t).Apps) == 0 {
		t.Fatal("a preview removed the containers")
	}

	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil, 200, nil)
	_, b = e.do(t, "GET", "/api/v1/apps/docker", "", nil)
	if !strings.Contains(string(b), `"ready":false`) || !strings.Contains(string(b), `"ok":true`) || strings.Contains(string(b), "app data") {
		t.Fatalf("after uninstall: %s", b)
	}
	if n := len(e.installed(t).Apps); n != 0 {
		t.Fatalf("%d containers left", n)
	}
	if _, b := e.do(t, "GET", "/api/v1/changes", "", nil); !strings.Contains(string(b), "Uninstall Docker") || !strings.Contains(string(b), "keeping the app data") {
		t.Fatalf("no change-log entry: %s", b)
	}

	// Nothing left: the second uninstall has nothing to do.
	resp, b := e.do(t, "POST", "/api/v1/apps/docker/uninstall/plan", "", nil)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "nothing_to_do") {
		t.Fatalf("second plan: %d %s", resp.StatusCode, b)
	}
	// And Docker can be set up again.
	e.setUpDocker(t)
}

func TestAppsDockerUninstall_DeleteDataIsAnExplicitChoice(t *testing.T) {
	e := newEnv(t)
	e.setUpDocker(t)

	code, keep, b := e.planUninstall(t, `{"deleteData":false}`, nil)
	if code != 200 || strings.Contains(keep.kinds(), "data") {
		t.Fatalf("keep: %d %s", code, b)
	}
	code, del, b := e.planUninstall(t, `{"deleteData":true}`, nil)
	if code != 200 || !strings.HasSuffix(del.kinds(), ",data") || !strings.Contains(string(b), "/media/hdd/dreamhub-apps") {
		t.Fatalf("delete: %d %s", code, b)
	}
	e.json(t, "POST", "/api/v1/plans/"+del.Plan.ID+"/apply", "", nil, 200, nil)
	_, b = e.do(t, "GET", "/api/v1/apps/docker", "", nil)
	if !strings.Contains(string(b), "Removed the app data") {
		t.Fatalf("log: %s", b)
	}
	// The other plan was made for a state that no longer exists.
	resp, b := e.do(t, "POST", "/api/v1/plans/"+keep.Plan.ID+"/apply", "", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale plan: expected 409, got %d %s", resp.StatusCode, b)
	}
}

func TestAppsDockerUninstall_AdminOnlyCSRFAndOwner(t *testing.T) {
	e := newEnv(t)
	e.setUpDocker(t)
	admin, viewer, _ := e.household(t)

	if resp, _ := e.do(t, "POST", "/api/v1/apps/docker/uninstall/plan", "", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer uninstall: expected 403, got %d", resp.StatusCode)
	}
	noHeader := func(r *http.Request) {
		withProfile(admin)(r)
		r.Header.Del(httpx.CSRFHeader)
	}
	resp, b := e.do(t, "POST", "/api/v1/apps/docker/uninstall/plan", "", noHeader)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusForbidden || code != "csrf_failed" {
		t.Errorf("no CSRF header: expected 403 csrf_failed, got %d %s", resp.StatusCode, b)
	}

	code, p, b := e.planUninstall(t, "", withProfile(admin))
	if code != 200 {
		t.Fatalf("admin plan: %d %s", code, b)
	}
	if p.Plan.ExpiresAt.Sub(p.Plan.CreatedAt) != 10*time.Minute {
		t.Errorf("plans expire after 10 minutes, got %v", p.Plan.ExpiresAt.Sub(p.Plan.CreatedAt))
	}
	// Another profile can't confirm the admin's plan.
	if resp, _ := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer applies the admin's plan: expected 403, got %d", resp.StatusCode)
	}
	if _, b := e.do(t, "GET", "/api/v1/apps/docker", "", withProfile(admin)); !strings.Contains(string(b), `"ready":true`) {
		t.Errorf("a refused apply changed something: %s", b)
	}
}
