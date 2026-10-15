package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
)

// App13: container list (auto-start, edit), Docker settings, image clean-up.

const edComp = "services:\n app:\n  image: traefik/whoami\n  volumes:\n   - .:/data\n"

type rowT struct {
	ID        string `json:"id"`
	AppID     string `json:"appId"`
	Name      string `json:"name"`
	State     string `json:"state"`
	Autostart bool   `json:"autostart"`
	Editable  bool   `json:"editable"`
}

func (e *env) rows(t *testing.T, cfg func(*http.Request)) []rowT {
	t.Helper()
	var v struct{ Apps []rowT }
	e.json(t, "GET", "/api/v1/apps/installed", "", cfg, 200, &v)
	return v.Apps
}

func rowOf(rows []rowT, name string) *rowT {
	for i := range rows {
		if rows[i].Name == name {
			return &rows[i]
		}
	}
	return nil
}

func (e *env) installAndApply(t *testing.T, body string) { e.installAndApplyAs(t, nil, body) }

func (e *env) installAndApplyAs(t *testing.T, cfg func(*http.Request), body string) {
	t.Helper()
	code, p, b := e.formPlan(t, body, cfg)
	if code != 200 {
		t.Fatalf("plan: %d %s", code, b)
	}
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", cfg, 200, nil)
}

func TestAppsAutostart_DefaultToggleAndRules(t *testing.T) {
	e := newEnv(t)
	admin, viewer, _ := e.household(t)
	a := withProfile(admin)
	e.installAndApplyAs(t, a, customBody(edComp, ""))
	row := rowOf(e.rows(t, a), "My app")
	if row == nil || !row.Autostart || !row.Editable {
		t.Fatalf("a new app follows the default (on): %+v", row)
	}
	if r := rowOf(e.rows(t, a), "Pi-hole"); r == nil || r.Autostart {
		t.Fatalf("an app that was there before has no auto-start: %+v", r)
	}
	path := "/api/v1/apps/installed/" + row.ID + "/autostart"
	if resp, _ := e.do(t, "PUT", path, `{"enabled":false}`, withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer: %d", resp.StatusCode)
	}
	resp, rb := e.do(t, "PUT", path, `{"enabled":false}`, func(r *http.Request) {
		withProfile(admin)(r)
		r.Header.Del(httpx.CSRFHeader)
	})
	if resp.StatusCode != http.StatusForbidden || decodeErr(rb).Error.Code != "csrf_failed" {
		t.Errorf("no CSRF header: %d %s", resp.StatusCode, rb)
	}
	e.json(t, "PUT", path, `{"enabled":false}`, withProfile(admin), 200, nil)
	if r := rowOf(e.rows(t, a), "My app"); r.Autostart {
		t.Error("the switch did not turn auto-start off")
	}
	e.json(t, "PUT", path, `{"enabled":true}`, withProfile(admin), 200, nil)
	if r := rowOf(e.rows(t, a), "My app"); !r.Autostart {
		t.Error("the switch did not turn auto-start on")
	}
	if resp, _ := e.do(t, "PUT", "/api/v1/apps/installed/nope/autostart", `{"enabled":true}`, a); resp.StatusCode != 404 {
		t.Errorf("unknown app: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "PUT", path, `{}`, a); resp.StatusCode != 400 {
		t.Errorf("no value: %d", resp.StatusCode)
	}

	// The default for new apps is a setting.
	e.json(t, "PATCH", "/api/v1/settings", `{"values":{"apps.autostartDefault":false}}`, a, 200, nil)
	e.installAndApplyAs(t, a, strings.Replace(customBody(edComp, ""), "My app", "Second", 1))
	if r := rowOf(e.rows(t, a), "Second"); r == nil || r.Autostart {
		t.Fatalf("with the default off, a new app has no auto-start: %+v", r)
	}
}

func TestAppsDockerSettings_SchemaDefaultsAndAdminOnly(t *testing.T) {
	e := newEnv(t)
	admin, viewer, _ := e.household(t)
	a := withProfile(admin)
	var v struct {
		Sections []struct {
			ID     string
			Fields []struct {
				Key       string
				Hidden    bool
				IsDefault bool
				Value     any
			}
		}
	}
	e.json(t, "GET", "/api/v1/settings", "", a, 200, &v)
	found := map[string]bool{}
	for _, s := range v.Sections {
		if s.ID != "apps" {
			continue
		}
		for _, f := range s.Fields {
			found[f.Key] = true
			// The boot pass records the boot it ran for.
			if !f.Hidden || (!f.IsDefault && f.Key != "apps.autostartBoot") {
				t.Errorf("%s: hidden=%v default=%v", f.Key, f.Hidden, f.IsDefault)
			}
		}
	}
	for _, k := range []string{"apps.storageRoot", "apps.autostartDefault", "apps.memoryLimitMB", "apps.updateCheckHours", "apps.updateNotify"} {
		if !found[k] {
			t.Errorf("missing setting %s", k)
		}
	}
	// A viewer neither sees nor changes them.
	var vv struct{ Sections []struct{ ID string } }
	e.json(t, "GET", "/api/v1/settings", "", withProfile(viewer), 200, &vv)
	for _, s := range vv.Sections {
		if s.ID == "apps" {
			t.Error("a viewer sees the Docker settings")
		}
	}
	if resp, _ := e.do(t, "PATCH", "/api/v1/settings", `{"values":{"apps.updateNotify":false}}`, withProfile(viewer)); resp.StatusCode == 200 {
		t.Error("a viewer changed a Docker setting")
	}
	// Out of range, then changed, then reset.
	if resp, _ := e.do(t, "PATCH", "/api/v1/settings", `{"values":{"apps.memoryLimitMB":10}}`, a); resp.StatusCode == 200 {
		t.Error("10 MB accepted")
	}
	e.json(t, "PATCH", "/api/v1/settings", `{"values":{"apps.memoryLimitMB":512,"apps.updateCheckHours":6}}`, a, 200, &v)
	changed := 0
	for _, s := range v.Sections {
		for _, f := range s.Fields {
			if s.ID == "apps" && !f.IsDefault && f.Key != "apps.autostartBoot" {
				changed++
			}
		}
	}
	if changed != 2 {
		t.Errorf("%d settings changed, want 2", changed)
	}
	e.json(t, "PATCH", "/api/v1/settings", `{"values":{"apps.memoryLimitMB":null}}`, a, 200, &v)
}

func TestAppsDockerSettings_MemoryDefaultAndStorageDefault(t *testing.T) {
	e := newEnv(t)
	e.json(t, "PATCH", "/api/v1/settings", `{"values":{"apps.memoryLimitMB":384,"apps.storageRoot":"/media/usb/dreamhub-apps"}}`, nil, 200, nil)
	var ch struct {
		Choices []struct {
			Path    string
			Default bool
		}
	}
	e.json(t, "GET", "/api/v1/apps/storage", "", nil, 200, &ch)
	for _, c := range ch.Choices {
		if c.Default != (c.Path == "/media/usb/dreamhub-apps") {
			t.Errorf("default flag on %s: %v", c.Path, c.Default)
		}
	}
	// A container that states no limit gets the setting; the preview shows where it goes.
	code, p, b := e.formPlan(t, customBody(edComp, ""), nil)
	if code != 200 {
		t.Fatalf("%d %s", code, b)
	}
	s := string(b)
	if !strings.Contains(s, `"memoryLimitMB":384`) || !strings.Contains(s, "/media/usb/dreamhub-apps/mymyapp") {
		t.Fatalf("setting not used: %s", s)
	}
	_ = p
	// A storage setting that is no longer one of the places falls back.
	e.json(t, "PATCH", "/api/v1/settings", `{"values":{"apps.storageRoot":"/media/gone/dreamhub-apps"}}`, nil, 200, nil)
	_, _, b = e.formPlan(t, customBody(edComp, ""), nil)
	if !strings.Contains(string(b), "/media/hdd/dreamhub-apps/mymyapp") {
		t.Fatalf("no fallback to the box default: %s", b)
	}
}

type editViewT struct {
	Kind      string            `json:"kind"`
	Known     bool              `json:"known"`
	Values    map[string]string `json:"values"`
	SecretSet []string          `json:"secretSet"`
	Extra     struct {
		Env []struct {
			Key, Value string
			Keep       bool
		}
	} `json:"extra"`
	MemoryLimitMB int `json:"memoryLimitMB"`
	Storage       struct{ Path string }
}

func TestAppsEdit_PrefillRecreateKeepsDataAndSecrets(t *testing.T) {
	e := newEnv(t)
	admin, viewer, _ := e.household(t)
	a := withProfile(admin)
	comp := "x-dreamhub:\n  webui:\n    port: ${WEB_PORT:-9301}\nservices:\n app:\n  image: traefik/whoami\n  environment:\n   - TOKEN=${TOKEN:-}\n  volumes:\n   - .:/data\n"
	e.installAndApplyAs(t, a, customBody(comp, `"values":{"TOKEN":"hunter2"},"memoryLimitMB":200,"extra":{"env":[{"key":"API_KEY","value":"abc"},{"key":"MODE","value":"one"}],"ports":[],"paths":[]}`))
	e.appsConfigFS.Files["/media/hdd/dreamhub-apps/mymyapp"] = map[string][]byte{"data.db": []byte("precious")}
	row := rowOf(e.rows(t, a), "My app")
	editPath := "/api/v1/apps/installed/" + row.ID + "/edit"

	var v editViewT
	resp, b := e.do(t, "GET", editPath, "", withProfile(viewer))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer: %d", resp.StatusCode)
	}
	resp, b = e.do(t, "GET", editPath, "", withProfile(admin))
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if strings.Contains(string(b), "hunter2") || strings.Contains(string(b), `"abc"`) {
		t.Fatalf("a secret was sent back to the browser: %s", b)
	}
	_ = json.Unmarshal(b, &v)
	if v.Kind != "custom" || !v.Known || v.MemoryLimitMB != 200 || v.Storage.Path != "/media/hdd/dreamhub-apps" ||
		len(v.SecretSet) != 1 || v.SecretSet[0] != "TOKEN" || len(v.Extra.Env) != 2 || !v.Extra.Env[0].Keep || v.Extra.Env[1].Value != "one" {
		t.Fatalf("form not pre-filled: %+v", v)
	}

	// Change a variable and the web port, leave the secrets empty.
	body := customBody(comp, `"values":{"WEB_PORT":"9302"},"memoryLimitMB":200,"extra":{"env":[{"key":"API_KEY","value":""},{"key":"MODE","value":"two"}],"ports":[],"paths":[]}`)
	planPath := editPath + "/plan"
	if resp, _ := e.do(t, "POST", planPath, body, withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer plan: %d", resp.StatusCode)
	}
	resp, rb := e.do(t, "POST", planPath, body, func(r *http.Request) {
		withProfile(admin)(r)
		r.Header.Del(httpx.CSRFHeader)
	})
	if resp.StatusCode != http.StatusForbidden || decodeErr(rb).Error.Code != "csrf_failed" {
		t.Errorf("no CSRF header: %d %s", resp.StatusCode, rb)
	}
	resp, rb = e.do(t, "POST", planPath, body, withProfile(admin))
	if resp.StatusCode != 200 {
		t.Fatalf("plan: %d %s", resp.StatusCode, rb)
	}
	var p formPlan
	_ = json.Unmarshal(rb, &p)
	if strings.Contains(string(rb), "hunter2") || strings.Contains(string(rb), `"abc"`) {
		t.Fatalf("a secret is in the preview: %s", rb)
	}
	if !strings.Contains(string(rb), "/media/hdd/dreamhub-apps/mymyapp") || !strings.Contains(string(rb), `"edit":true`) {
		t.Fatalf("preview: %s", rb)
	}
	if rowOf(e.rows(t, a), "My app").ID != row.ID {
		t.Fatal("previewing changed something")
	}
	// Owner binding, then single use.
	if resp, _ := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("another profile applied the plan: %d", resp.StatusCode)
	}
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(admin), 200, nil)
	if resp, _ := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(admin)); resp.StatusCode == 200 {
		t.Error("a plan was applied twice")
	}
	rows := e.rows(t, a)
	if r := rowOf(rows, "My app"); r == nil || r.State != "running" || !r.Autostart {
		t.Fatalf("after the edit: %+v", r)
	}
	if string(e.appsConfigFS.Files["/media/hdd/dreamhub-apps/mymyapp"]["data.db"]) != "precious" {
		t.Fatal("the data directory was touched")
	}
	// The kept secrets are still there, the changes are stored.
	def := string(e.appsConfigFS.Files["/media/hdd/dreamhub-apps"]["mymyapp.app.json"])
	for _, want := range []string{"hunter2", "abc", "two", "9302"} {
		if !strings.Contains(def, want) {
			t.Errorf("stored choices lack %q: %s", want, def)
		}
	}
	// Nothing in the list was duplicated.
	n := 0
	for _, r := range rows {
		if r.Name == "My app" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d rows for the app", n)
	}
}

func TestAppsEdit_CatalogAppAndRefusals(t *testing.T) {
	e := newEnv(t)
	// The seeded Pi-hole was installed before choices were stored: the form
	// starts from the defaults, and the edit still plans.
	row := rowOf(e.rows(t, nil), "Pi-hole")
	var v editViewT
	e.json(t, "GET", "/api/v1/apps/installed/"+row.ID+"/edit", "", nil, 200, &v)
	if v.Kind != "catalog" || v.Known {
		t.Fatalf("%+v", v)
	}
	if resp, b := e.do(t, "POST", "/api/v1/apps/installed/"+row.ID+"/edit/plan", `{"appId":"pihole"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("catalog edit plan: %d %s", resp.StatusCode, b)
	}
	// Not another app's id, not a custom file for a catalog app.
	if resp, _ := e.do(t, "POST", "/api/v1/apps/installed/"+row.ID+"/edit/plan", `{"appId":"vaultwarden"}`, nil); resp.StatusCode != 400 {
		t.Errorf("other app: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "POST", "/api/v1/apps/installed/"+row.ID+"/edit/plan", customBody(edComp, ""), nil); resp.StatusCode != 400 {
		t.Errorf("custom for a catalog app: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/apps/installed/nope/edit", "", nil); resp.StatusCode != 404 {
		t.Errorf("unknown: %d", resp.StatusCode)
	}
	// A refused mount is refused on edit too.
	e.installAndApply(t, customBody(edComp, ""))
	r2 := rowOf(e.rows(t, nil), "My app")
	bad := "services:\n app:\n  image: traefik/whoami\n  volumes:\n   - /etc:/x\n"
	if resp, b := e.do(t, "POST", "/api/v1/apps/installed/"+r2.ID+"/edit/plan", customBody(bad, ""), nil); resp.StatusCode != 422 {
		t.Errorf("mount of /etc on edit: %d %s", resp.StatusCode, b)
	}
}

func TestAppsImages_ListPlanApply(t *testing.T) {
	e := newEnv(t)
	admin, viewer, _ := e.household(t)
	a := withProfile(admin)
	var v struct {
		Images           []struct{ ID string }
		ReclaimableBytes int64
	}
	if resp, _ := e.do(t, "GET", "/api/v1/apps/docker/images", "", withProfile(viewer)); resp.StatusCode != 403 {
		t.Errorf("viewer list: %d", resp.StatusCode)
	}
	e.json(t, "GET", "/api/v1/apps/docker/images", "", withProfile(admin), 200, &v)
	if len(v.Images) != 3 || v.ReclaimableBytes == 0 {
		t.Fatalf("%+v", v)
	}
	path := "/api/v1/apps/docker/images/prune/plan"
	if resp, _ := e.do(t, "POST", path, "", withProfile(viewer)); resp.StatusCode != 403 {
		t.Errorf("viewer plan: %d", resp.StatusCode)
	}
	resp, rb := e.do(t, "POST", path, "", func(r *http.Request) {
		withProfile(admin)(r)
		r.Header.Del(httpx.CSRFHeader)
	})
	if resp.StatusCode != 403 || decodeErr(rb).Error.Code != "csrf_failed" {
		t.Errorf("no CSRF: %d %s", resp.StatusCode, rb)
	}
	var p formPlan
	e.json(t, "POST", path, "", withProfile(admin), 200, &p)
	// previewing removed nothing
	e.json(t, "GET", "/api/v1/apps/docker/images", "", a, 200, &v)
	if len(v.Images) != 3 {
		t.Fatal("the preview removed images")
	}
	if resp, _ := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(viewer)); resp.StatusCode != 403 {
		t.Errorf("owner: %d", resp.StatusCode)
	}
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(admin), 200, nil)
	if resp, _ := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(admin)); resp.StatusCode == 200 {
		t.Error("applied twice")
	}
	e.json(t, "GET", "/api/v1/apps/docker/images", "", a, 200, &v)
	if len(v.Images) != 0 {
		t.Fatal("images left")
	}
	if resp, _ := e.do(t, "POST", path, "", a); resp.StatusCode != 409 {
		t.Errorf("nothing to do: %d", resp.StatusCode)
	}
}
