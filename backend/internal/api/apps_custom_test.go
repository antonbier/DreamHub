package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
)

// App12: the install form and the admin's own containers. These tests go
// through the whole stack (router, permissions, CSRF, plan service, the
// mock helper that applies the same mount rules as the real one).

type formPlan struct {
	Plan struct {
		ID        string    `json:"id"`
		Summary   string    `json:"summary"`
		CreatedAt time.Time `json:"createdAt"`
		ExpiresAt time.Time `json:"expiresAt"`
		Changes   []struct {
			After struct {
				Name       string `json:"name"`
				Custom     bool   `json:"custom"`
				HostPort   int    `json:"hostPort"`
				Containers []struct {
					Service string `json:"service"`
					Image   string `json:"image"`
					Ports   []struct {
						Port  int  `json:"port"`
						UDP   bool `json:"udp"`
						Local bool `json:"local"`
					} `json:"ports"`
					Mounts []struct {
						Host      string `json:"host"`
						Container string `json:"container"`
						ReadOnly  bool   `json:"readOnly"`
					} `json:"mounts"`
					Env []struct {
						Key       string `json:"key"`
						Value     string `json:"value"`
						Masked    bool   `json:"masked"`
						Generated bool   `json:"generated"`
					} `json:"env"`
					Elevated *struct {
						Privileged bool     `json:"privileged"`
						CapAdd     []string `json:"capAdd"`
						Devices    []string `json:"devices"`
						PidHost    bool     `json:"pidHost"`
						IpcHost    bool     `json:"ipcHost"`
					} `json:"elevated"`
				} `json:"containers"`
			} `json:"after"`
		} `json:"changes"`
	} `json:"plan"`
}

type apiErr struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details struct {
			Issues []struct {
				Code    string `json:"code"`
				Key     string `json:"key"`
				Detail  string `json:"detail"`
				Message string `json:"message"`
			} `json:"issues"`
			Elevated []struct {
				Kind  string `json:"kind"`
				Value string `json:"value"`
			} `json:"elevated"`
			Field string `json:"field"`
		} `json:"details"`
	} `json:"error"`
}

func decodeErr(b []byte) apiErr {
	var v apiErr
	_ = json.Unmarshal(b, &v)
	return v
}

func (e *env) formPlan(t *testing.T, body string, configure func(*http.Request)) (int, formPlan, []byte) {
	t.Helper()
	resp, b := e.do(t, "POST", "/api/v1/apps/install/plan", body, configure)
	var p formPlan
	_ = json.Unmarshal(b, &p)
	return resp.StatusCode, p, b
}

// customBody builds {"custom": {...}, ...extra} with the compose text
// JSON-encoded.
func customBody(composeText string, rest string) string {
	enc, _ := json.Marshal(composeText)
	b := `{"custom":{"compose":` + string(enc) + `,"name":"My app"}`
	if rest != "" {
		b += "," + rest
	}
	return b + "}"
}

const whoamiCompose = `
services:
  whoami:
    image: traefik/whoami:v1.10
    environment:
      WHOAMI_PORT_NUMBER: ${WEB_PORT:-8097}
      WHOAMI_NAME: ${INSTANCE:-box}
      ADMIN_PASSWORD: ${ADMIN_PASSWORD}
    ports:
      - "${WEB_PORT:-8097}:${WEB_PORT:-8097}"
    volumes:
      - ./data:/data
      - /media/hdd/movie:/movies:ro
    mem_limit: 64m
x-dreamhub:
  webui:
    port: ${WEB_PORT:-8097}
  fields:
    WEB_PORT: {label: {en: Web port}}
    ADMIN_PASSWORD: {label: {en: Admin password}, required: true, secret: true}
`

func TestAppsForm_CatalogFieldsAndValues(t *testing.T) {
	e := newEnv(t)
	var v struct {
		Apps []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Note        string `json:"note"`
			Fields      []struct {
				Name        string `json:"name"`
				Label       string `json:"label"`
				Description string `json:"description"`
				Type        string `json:"type"`
				Default     string `json:"default"`
				WebPort     bool   `json:"webPort"`
				Advanced    bool   `json:"advanced"`
			} `json:"fields"`
		} `json:"apps"`
	}
	e.json(t, "GET", "/api/v1/apps/catalog?lang=de", "", nil, 200, &v)
	byID := map[string]int{}
	for i, a := range v.Apps {
		byID[a.ID] = i
	}
	vw := v.Apps[byID["vaultwarden"]]
	if len(vw.Fields) != 1 || vw.Fields[0].Name != "WEB_PORT" || !vw.Fields[0].WebPort || vw.Fields[0].Label != "Web-Port" || vw.Fields[0].Type != "port" || vw.Fields[0].Default != "8222" || vw.Fields[0].Advanced {
		t.Fatalf("vaultwarden fields: %+v", vw.Fields)
	}
	if !strings.Contains(vw.Note, "HTTPS") || !strings.Contains(vw.Note, "Reverse-Proxy") || !strings.Contains(vw.Description, "Passwort") {
		t.Fatalf("german texts: %q / %q", vw.Description, vw.Note)
	}
	if ag := v.Apps[byID["adguardhome"]]; len(ag.Fields) != 0 {
		t.Fatalf("a fixed-port app has no fields: %+v", ag.Fields)
	}
	e.json(t, "GET", "/api/v1/apps/catalog?lang=xx", "", nil, 200, &v) // unknown language: English
	if !strings.HasPrefix(v.Apps[byID["vaultwarden"]].Description, "A password manager") {
		t.Fatalf("fallback language: %q", v.Apps[byID["vaultwarden"]].Description)
	}

	// The form's value moves the port; the installed app shows it.
	code, p, b := e.formPlan(t, `{"appId":"vaultwarden","values":{"WEB_PORT":"9311"}}`, nil)
	if code != 200 || p.Plan.Changes[0].After.HostPort != 9311 {
		t.Fatalf("plan: %d %s", code, b)
	}
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil, 200, nil)
	found := false
	for _, a := range e.installed(t).Apps {
		found = found || (a.Name == "Vaultwarden" && a.HostPort == 9311)
	}
	if !found {
		t.Fatalf("not installed on 9311: %+v", e.installed(t))
	}
}

func TestAppsForm_ValueValidation(t *testing.T) {
	e := newEnv(t)
	for name, body := range map[string]struct{ body, code string }{
		"unknown field":    {`{"appId":"vaultwarden","values":{"NOPE":"1"}}`, "unknown_field"},
		"box port":         {`{"appId":"vaultwarden","values":{"WEB_PORT":"8080"}}`, "port_invalid"},
		"web port below":   {`{"appId":"vaultwarden","values":{"WEB_PORT":"80"}}`, "port_invalid"},
		"not a number":     {`{"appId":"vaultwarden","values":{"WEB_PORT":"abc"}}`, "port_invalid"},
		"too long":         {`{"appId":"vaultwarden","values":{"WEB_PORT":"` + strings.Repeat("9", 5000) + `"}}`, "value_invalid"},
		"both app kinds":   {`{"appId":"vaultwarden","custom":{"compose":"x"}}`, "invalid_body"},
		"neither":          {`{}`, "invalid_body"},
		"fixed port field": {`{"appId":"homeassistant","hostPort":9999}`, "port_fixed"},
	} {
		t.Run(name, func(t *testing.T) {
			code, _, b := e.formPlan(t, body.body, nil)
			if got := decodeErr(b).Error.Code; got != body.code || code < 400 {
				t.Fatalf("got %d %s, want %s", code, b, body.code)
			}
		})
	}
}

func TestAppsForm_ExtraRows(t *testing.T) {
	e := newEnv(t)
	body := `{"appId":"vaultwarden","extra":{"env":[{"key":"SIGNUPS_ALLOWED","value":"false"},{"key":"ADMIN_TOKEN","value":"hunter2"}],"ports":[{"port":9444},{"port":9445,"udp":true}],"paths":[{"host":"/media/hdd/backup","container":"/backup","readOnly":true},{"host":"./extra","container":"/extra"}]}}`
	code, p, b := e.formPlan(t, body, nil)
	if code != 200 {
		t.Fatalf("plan: %d %s", code, b)
	}
	c := p.Plan.Changes[0].After.Containers[0]
	envs := map[string]string{}
	masked := map[string]bool{}
	for _, v := range c.Env {
		envs[v.Key] = v.Value
		masked[v.Key] = v.Masked
	}
	if envs["SIGNUPS_ALLOWED"] != "false" || envs["ROCKET_PORT"] != "8222" {
		t.Fatalf("env: %+v", c.Env)
	}
	if !masked["ADMIN_TOKEN"] || envs["ADMIN_TOKEN"] != "" || strings.Contains(string(b), "hunter2") {
		t.Fatalf("a secret-looking value must not appear in the preview: %s", b)
	}
	var sawBackup, sawExtra bool
	for _, m := range c.Mounts {
		sawBackup = sawBackup || (m.Host == "/media/hdd/backup" && m.Container == "/backup" && m.ReadOnly)
		sawExtra = sawExtra || (m.Host == "/media/hdd/dreamhub-apps/vaultwarden.extra" && m.Container == "/extra")
	}
	if !sawBackup || !sawExtra {
		t.Fatalf("mounts: %+v", c.Mounts)
	}
	if len(c.Ports) != 3 || c.Ports[0].Port != 8222 {
		t.Fatalf("ports: %+v", c.Ports)
	}

	for name, rows := range map[string]string{
		"env name":        `"env":[{"key":"A B","value":"x"}]`,
		"box port":        `"ports":[{"port":8080}]`,
		"port zero":       `"ports":[{"port":0}]`,
		"relative path":   `"paths":[{"host":"media/x","container":"/x"}]`,
		"container path":  `"paths":[{"host":"/media/x","container":"x"}]`,
		"container dup":   `"paths":[{"host":"/media/x","container":"/data"}]`,
		"folder name":     `"paths":[{"host":"./My_Folder","container":"/x"}]`,
		"unknown row key": `"nope":[]`,
	} {
		t.Run(name, func(t *testing.T) {
			code, _, b := e.formPlan(t, `{"appId":"vaultwarden","extra":{`+rows+`}}`, nil)
			if code != 400 {
				t.Fatalf("got %d %s, want 400", code, b)
			}
		})
	}
}

// Every host path an admin can type is checked by the same rules, whether
// it comes from a compose file or from the advanced view's rows.
func TestAppsForm_DangerousMountsAreRefused(t *testing.T) {
	e := newEnv(t)
	paths := []struct{ path, reason string }{
		{"/", "mount_system"},
		{"/etc", "mount_system"},
		{"/etc/ssh", "mount_system"},
		{"/usr", "mount_system"},
		{"/var", "mount_system"},
		{"/var/lib/dreamhub", "mount_system"},
		{"/var/run/docker.sock", "mount_system"},
		{"/run/docker.sock", "mount_system"},
		{"/boot", "mount_system"},
		{"/proc", "mount_system"},
		{"/sys", "mount_system"},
		{"/dev", "mount_system"},
		{"/dev/sda", "mount_system"},
		{"/data", "mount_system"},
		{"/data/dreamhub", "mount_system"},
		{"/media/hdd/docker.sock", "mount_socket"},
		{"/media/hdd/dreamhub-apps", "mount_storage"},
		{"/media/hdd/dreamhub-apps/pihole", "mount_storage"},
		{"/media/hdd", "mount_storage"},
		{"/media", "mount_storage"},
		{"/media/../etc", "mount_unclean"},
		{"/media/hdd/movie/../../../etc", "mount_unclean"},
		{"/tmp/other", "mount_outside"},
		{"/tmp", "mount_dreamhub"}, // the test's DreamHub data directory lives below it
		{"/home", "mount_system"},
	}
	for _, c := range paths {
		t.Run("compose "+c.path, func(t *testing.T) {
			text := "services:\n app:\n  image: traefik/whoami\n  volumes:\n   - " + strconv.Quote(c.path+":/host") + "\n"
			code, _, b := e.formPlan(t, customBody(text, ""), nil)
			er := decodeErr(b)
			if code != http.StatusUnprocessableEntity || er.Error.Code != "mount_refused" || len(er.Error.Details.Issues) == 0 || er.Error.Details.Issues[0].Detail != c.reason {
				t.Fatalf("got %d %s, want mount_refused (%s)", code, b, c.reason)
			}
		})
		t.Run("row "+c.path, func(t *testing.T) {
			body := `{"appId":"syncthing","extra":{"paths":[{"host":` + strconv.Quote(c.path) + `,"container":"/host"}]}}`
			code, _, b := e.formPlan(t, body, nil)
			if c.reason == "mount_unclean" {
				// The row check and the mount rules both refuse it; either is fine.
				if code != 400 && code != http.StatusUnprocessableEntity {
					t.Fatalf("got %d %s", code, b)
				}
				return
			}
			er := decodeErr(b)
			if code != http.StatusUnprocessableEntity || er.Error.Code != "mount_refused" || er.Error.Details.Issues[0].Detail != c.reason {
				t.Fatalf("got %d %s, want mount_refused (%s)", code, b, c.reason)
			}
		})
	}
	// Nothing was installed by any of the above.
	for _, a := range e.installed(t).Apps {
		if a.Name == "My app" || a.Name == "Syncthing" {
			t.Fatalf("a refused request installed %s", a.Name)
		}
	}

	// The same paths via the paste step are refused before any form.
	for _, c := range paths[:6] {
		text := "services:\n app:\n  image: traefik/whoami\n  volumes:\n   - " + strconv.Quote(c.path+":/host") + "\n"
		enc, _ := json.Marshal(text)
		code, _, b := e.formPlan(t, `{"kind":"x"}`, nil)
		_ = code
		_ = b
		resp, body := e.do(t, "POST", "/api/v1/apps/custom/parse", `{"kind":"compose","text":`+string(enc)+`}`, nil)
		if resp.StatusCode != http.StatusUnprocessableEntity || decodeErr(body).Error.Code != "mount_refused" {
			t.Errorf("parse %s: %d %s", c.path, resp.StatusCode, body)
		}
	}
}

func TestAppsForm_AllowedMountsAndHostNetworkInstall(t *testing.T) {
	e := newEnv(t)
	body := customBody(whoamiCompose, `"values":{"ADMIN_PASSWORD":"s3cret-pw","INSTANCE":"living room"}`)
	code, p, b := e.formPlan(t, body, nil)
	if code != 200 {
		t.Fatalf("plan: %d %s", code, b)
	}
	if strings.Contains(string(b), "s3cret-pw") {
		t.Fatalf("a secret field's value leaked into the preview: %s", b)
	}
	a := p.Plan.Changes[0].After
	if !a.Custom || a.Name != "My app" || a.HostPort != 8097 || len(a.Containers) != 1 {
		t.Fatalf("after: %+v", a)
	}
	c := a.Containers[0]
	if c.Image != "traefik/whoami:v1.10" {
		t.Fatalf("image: %s", c.Image)
	}
	var data, movies bool
	for _, m := range c.Mounts {
		data = data || (m.Host == "/media/hdd/dreamhub-apps/mymyapp.data" && m.Container == "/data")
		movies = movies || (m.Host == "/media/hdd/movie" && m.Container == "/movies" && m.ReadOnly)
	}
	if !data || !movies {
		t.Fatalf("mounts: %+v", c.Mounts)
	}
	for _, v := range c.Env {
		if v.Key == "ADMIN_PASSWORD" && (!v.Masked || v.Value != "") {
			t.Fatalf("the admin password must be masked: %+v", v)
		}
		if v.Key == "WHOAMI_NAME" && v.Value != "living room" {
			t.Fatalf("env: %+v", v)
		}
	}
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil, 200, nil)
	var found bool
	for _, a := range e.installed(t).Apps {
		if a.Name == "My app" && a.HostPort == 8097 && a.State == "running" {
			found = true
		}
	}
	if !found {
		t.Fatalf("custom app missing or unnamed: %+v", e.installed(t))
	}
	// Installing it again is refused.
	code, _, b = e.formPlan(t, body, nil)
	if code != http.StatusConflict || decodeErr(b).Error.Code != "already_installed" {
		t.Fatalf("second install: %d %s", code, b)
	}
}

func TestAppsForm_RequiredFieldAndRefusedKeys(t *testing.T) {
	e := newEnv(t)
	code, _, b := e.formPlan(t, customBody(whoamiCompose, ""), nil)
	er := decodeErr(b)
	if code != http.StatusUnprocessableEntity || er.Error.Code != "compose_invalid" || len(er.Error.Details.Issues) == 0 || er.Error.Details.Issues[0].Code != "missing_value" || er.Error.Details.Issues[0].Key != "ADMIN_PASSWORD" {
		t.Fatalf("missing required: %d %s", code, b)
	}
	for name, c := range map[string]struct{ yaml, code string }{
		"build":        {"services:\n a:\n  build: .\n", "unsupported_key"},
		"networks":     {"services:\n a:\n  image: traefik/whoami\n  networks: [n]\n", "unsupported_key"},
		"top networks": {"services:\n a:\n  image: traefik/whoami\nnetworks:\n n: {}\n", "unsupported_key"},
		"secrets":      {"services:\n a:\n  image: traefik/whoami\nsecrets:\n s: {file: x}\n", "unsupported_key"},
		"configs":      {"services:\n a:\n  image: traefik/whoami\nconfigs:\n s: {file: x}\n", "unsupported_key"},
		"extends":      {"services:\n a:\n  extends: {service: b}\n", "unsupported_key"},
		"remap":        {"services:\n a:\n  image: traefik/whoami\n  ports: [\"8081:80\"]\n", "port_remap"},
		"not yaml":     {"a: [", "yaml"},
		"user":         {"services:\n a:\n  image: traefik/whoami\n  user: root\n", "unsupported_key"},
	} {
		t.Run(name, func(t *testing.T) {
			code, _, b := e.formPlan(t, customBody(c.yaml, ""), nil)
			er := decodeErr(b)
			if code != http.StatusUnprocessableEntity || er.Error.Code != "compose_invalid" {
				t.Fatalf("got %d %s", code, b)
			}
			for _, i := range er.Error.Details.Issues {
				if i.Code == c.code && i.Message != "" {
					return
				}
			}
			t.Fatalf("no %s issue in %s", c.code, b)
		})
	}
	code, _, b = e.formPlan(t, customBody("services:\n a:\n  image: nosuch/image-xyz\n", ""), nil)
	if code != http.StatusConflict || decodeErr(b).Error.Code != "image_not_found" {
		t.Fatalf("unknown image: %d %s", code, b)
	}
}

// Privileged mode, capabilities, devices and host PID/IPC each need the
// explicit confirmation; nothing else does; and what is never allowed
// stays refused even when confirmed.
func TestAppsForm_ElevatedNeedsConfirmation(t *testing.T) {
	e := newEnv(t)
	for name, c := range map[string]struct{ line, kind string }{
		"privileged": {"privileged: true", "privileged"},
		"cap_add":    {"cap_add: [NET_ADMIN]", "capability"},
		"devices":    {"devices: [\"/dev/ttyUSB0:/dev/zigbee\"]", "device"},
		"pid host":   {"pid: host", "pid"},
		"ipc host":   {"ipc: host", "ipc"},
	} {
		t.Run(name, func(t *testing.T) {
			text := "services:\n app:\n  image: koenkk/zigbee2mqtt\n  " + c.line + "\n"
			code, _, b := e.formPlan(t, customBody(text, ""), nil)
			er := decodeErr(b)
			if code != http.StatusConflict || er.Error.Code != "confirmation_required" || len(er.Error.Details.Elevated) != 1 || er.Error.Details.Elevated[0].Kind != c.kind {
				t.Fatalf("unconfirmed: %d %s", code, b)
			}
			code, p, b := e.formPlan(t, customBody(text, `"confirmElevated":true`), nil)
			if code != 200 || p.Plan.Changes[0].After.Containers[0].Elevated == nil {
				t.Fatalf("confirmed: %d %s", code, b)
			}
			if len(p.Plan.Changes[0].After.Containers[0].Elevated.CapAdd)+len(p.Plan.Changes[0].After.Containers[0].Elevated.Devices) == 0 && !strings.Contains(string(b), "apps.elevated") {
				t.Fatalf("the plan should warn: %s", b)
			}
			e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil, 200, nil)
			// Remove it again for the next case.
			var list installedList = e.installed(t)
			for _, a := range list.Apps {
				if a.Name == "My app" {
					var rp installPlan
					e.json(t, "POST", "/api/v1/apps/installed/"+a.ID+"/remove/plan", "", nil, 200, &rp)
					e.json(t, "POST", "/api/v1/plans/"+rp.Plan.ID+"/apply", "", nil, 200, nil)
				}
			}
		})
	}
	// Plain containers, host network and a memory limit need nothing extra.
	text := "services:\n app:\n  image: traefik/whoami\n  network_mode: host\n  mem_limit: 64m\n"
	if code, _, b := e.formPlan(t, customBody(text, ""), nil); code != 200 {
		t.Fatalf("host network: %d %s", code, b)
	}
	// A confirmation does not unlock a refused mount ...
	text = "services:\n app:\n  image: traefik/whoami\n  privileged: true\n  volumes: [\"/:/host\"]\n"
	code, _, b := e.formPlan(t, customBody(text, `"confirmElevated":true`), nil)
	if code != http.StatusUnprocessableEntity || decodeErr(b).Error.Code != "mount_refused" {
		t.Fatalf("confirmed privileged with /: %d %s", code, b)
	}
	// ... nor a forbidden device, and the refusal comes before the question.
	for _, dev := range []string{"/dev/mem", "/dev/mmcblk0", "/dev/mtd3", "/dev/kmem"} {
		text = "services:\n app:\n  image: traefik/whoami\n  devices: [\"" + dev + "\"]\n"
		for _, rest := range []string{"", `"confirmElevated":true`} {
			code, _, b = e.formPlan(t, customBody(text, rest), nil)
			if code != http.StatusUnprocessableEntity || decodeErr(b).Error.Code != "device_refused" {
				t.Fatalf("%s (%q): %d %s", dev, rest, code, b)
			}
		}
	}
	// A catalog app never asks for any of it.
	_, p, _ := e.formPlan(t, `{"appId":"syncthing"}`, nil)
	if p.Plan.Changes[0].After.Containers[0].Elevated != nil {
		t.Fatal("a catalog app was elevated")
	}
}

func TestAppsForm_ParseSteps(t *testing.T) {
	e := newEnv(t)
	type parsed struct {
		App struct {
			Compose string `json:"compose"`
			ID      string `json:"id"`
			Name    string `json:"name"`
			Fields  []struct {
				Name     string `json:"name"`
				Label    string `json:"label"`
				Required bool   `json:"required"`
				Secret   bool   `json:"secret"`
				Type     string `json:"type"`
			} `json:"fields"`
			Services []struct{ Name, Image string } `json:"services"`
			Elevated []struct{ Kind string }        `json:"elevated"`
			Notes    []struct{ Code string }        `json:"notes"`
			WebPort  int                            `json:"webPort"`
		} `json:"app"`
	}
	enc := func(s string) string { b, _ := json.Marshal(s); return string(b) }

	var v parsed
	e.json(t, "POST", "/api/v1/apps/custom/parse", `{"kind":"compose","name":"Who","text":`+enc(whoamiCompose)+`}`, nil, 200, &v)
	if v.App.Name != "Who" || v.App.ID != "mywho" || v.App.WebPort != 8097 || len(v.App.Fields) != 3 || v.App.Fields[0].Name != "WEB_PORT" {
		t.Fatalf("compose: %+v", v.App)
	}
	var pw bool
	for _, f := range v.App.Fields {
		if f.Name == "ADMIN_PASSWORD" {
			pw = f.Required && f.Secret
		}
	}
	if !pw {
		t.Fatalf("required secret field missing: %+v", v.App.Fields)
	}

	// An image name makes the smallest compose file, with an optional web port.
	v = parsed{}
	e.json(t, "POST", "/api/v1/apps/custom/parse", `{"kind":"image","text":"ghcr.io/example/tool:2.1","name":"Tool"}`, nil, 200, &v)
	if v.App.Name != "Tool" || len(v.App.Services) != 1 || v.App.Services[0].Image != "ghcr.io/example/tool:2.1" || len(v.App.Fields) != 1 || v.App.Fields[0].Name != "WEB_PORT" || v.App.Fields[0].Required {
		t.Fatalf("image: %+v", v.App)
	}
	// ... which installs with the app's own port as the web port.
	code, p, b := e.formPlan(t, customBody(v.App.Compose, `"values":{"WEB_PORT":"9500"}`), nil)
	if code != 200 || p.Plan.Changes[0].After.HostPort != 9500 || p.Plan.Changes[0].After.Containers[0].Image != "ghcr.io/example/tool:2.1" {
		t.Fatalf("image plan: %d %s", code, b)
	}
	for _, bad := range []string{"", "x y", "Bad/Image", "a@sha256:abc", "--privileged"} {
		resp, body := e.do(t, "POST", "/api/v1/apps/custom/parse", `{"kind":"image","text":`+enc(bad)+`}`, nil)
		if resp.StatusCode < 400 {
			t.Errorf("image %q: %d %s", bad, resp.StatusCode, body)
		}
	}

	// An Unraid template becomes the same kind of form.
	unraid := `<Container version="2"><Name>Notes</Name><Repository>library/nginx:1.27</Repository><Privileged>false</Privileged>
<WebUI>http://[IP]:[PORT:80]/</WebUI>
<Config Name="Port" Target="80" Default="8099" Mode="tcp" Type="Port">8099</Config>
<Config Name="Appdata" Target="/config" Default="/mnt/user/appdata/notes" Mode="rw" Type="Path">/mnt/user/appdata/notes</Config>
<Config Name="Key" Target="API_KEY" Default="" Type="Variable" Required="true" Mask="true"/>
<Config Name="GPU" Target="/dev/dri" Type="Device">/dev/dri</Config></Container>`
	v = parsed{}
	e.json(t, "POST", "/api/v1/apps/custom/parse", `{"kind":"unraid","text":`+enc(unraid)+`}`, nil, 200, &v)
	if v.App.Name != "Notes" || v.App.WebPort != 80 && v.App.WebPort != 0 {
		t.Fatalf("unraid: %+v", v.App)
	}
	var codes []string
	for _, n := range v.App.Notes {
		codes = append(codes, n.Code)
	}
	if !strings.Contains(strings.Join(codes, ","), "unraid_ignored") || !strings.Contains(strings.Join(codes, ","), "unraid_port_remap") {
		t.Fatalf("notes: %v", codes)
	}
	// Port 80 is the box's own: the app can't be installed on it.
	code, _, b = e.formPlan(t, customBody(v.App.Compose, `"values":{"API_KEY":"k"}`), nil)
	if code != http.StatusConflict || decodeErr(b).Error.Code != "port_in_use" {
		t.Fatalf("unraid app on port 80: %d %s", code, b)
	}
	if resp, _ := e.do(t, "POST", "/api/v1/apps/custom/parse", `{"kind":"unraid","text":"<nope/>"}`, nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("not a template: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "POST", "/api/v1/apps/custom/parse", `{"kind":"other","text":"x"}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown kind: %d", resp.StatusCode)
	}
}

// Viewers can't touch any of it, every POST needs the CSRF header, and a
// plan is single-use, expires, and belongs to its owner — as for every
// other plan.
func TestAppsForm_PermissionsCSRFAndPlanRules(t *testing.T) {
	e := newEnv(t)
	admin, viewer, _ := e.household(t)
	body := customBody("services:\n app:\n  image: traefik/whoami\n", "")

	for _, path := range []string{"/api/v1/apps/install/plan", "/api/v1/apps/custom/parse"} {
		b := body
		if strings.HasSuffix(path, "parse") {
			b = `{"kind":"image","text":"traefik/whoami"}`
		}
		if resp, _ := e.do(t, "POST", path, b, withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
			t.Errorf("viewer POST %s: expected 403, got %d", path, resp.StatusCode)
		}
		noHeader := func(r *http.Request) {
			withProfile(admin)(r)
			r.Header.Del(httpx.CSRFHeader)
		}
		resp, rb := e.do(t, "POST", path, b, noHeader)
		if code := decodeErr(rb).Error.Code; resp.StatusCode != http.StatusForbidden || code != "csrf_failed" {
			t.Errorf("no CSRF header on %s: expected 403 csrf_failed, got %d %s", path, resp.StatusCode, rb)
		}
	}
	if resp, _ := e.do(t, "POST", "/api/v1/apps/install/plan", `{"appId":"syncthing"}`, withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer catalog install: %d", resp.StatusCode)
	}

	code, p, b := e.formPlan(t, body, withProfile(admin))
	if code != 200 {
		t.Fatalf("admin plan: %d %s", code, b)
	}
	if p.Plan.ExpiresAt.Sub(p.Plan.CreatedAt) != 10*time.Minute {
		t.Errorf("plans expire after 10 minutes, got %v", p.Plan.ExpiresAt.Sub(p.Plan.CreatedAt))
	}
	if resp, _ := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("the viewer applies the admin's plan: expected 403, got %d", resp.StatusCode)
	}
	var before installedList
	e.json(t, "GET", "/api/v1/apps/installed", "", withProfile(admin), 200, &before)
	if len(before.Apps) != 2 {
		t.Fatalf("a refused apply installed something: %+v", before.Apps)
	}
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(admin), 200, nil)
	if resp, _ := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(admin)); resp.StatusCode == http.StatusOK {
		t.Error("a plan was applied twice")
	}
}
