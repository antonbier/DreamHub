package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/perm"
)

type whoami struct {
	ID          int64    `json:"id"`
	Username    string   `json:"username"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	Profiles    bool     `json:"profiles"`
}

func withProfile(token string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set(api.ProfileHeader, token) }
}

func (e *env) json(t *testing.T, method, path, body string, configure func(*http.Request), want int, out any) {
	t.Helper()
	resp, b := e.do(t, method, path, body, configure)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: expected %d, got %d: %s", method, path, want, resp.StatusCode, b)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatal(err)
		}
	}
}

// household sets an admin PIN, adds a viewer "Ben" and returns both
// profiles' session tokens.
func (e *env) household(t *testing.T) (admin, viewer string, viewerID int64) {
	t.Helper()
	var me whoami
	e.json(t, "GET", "/api/v1/whoami", "", nil, 200, &me)
	// A second profile needs the admin to have a PIN first.
	resp, b := e.do(t, "POST", "/api/v1/profiles", `{"name":"Ben"}`, nil)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "profile_admin_pin_required") {
		t.Fatalf("expected profile_admin_pin_required, got %d: %s", resp.StatusCode, b)
	}
	e.json(t, "PUT", "/api/v1/profiles/me/pin", `{"pin":"1234"}`, nil, 200, nil)
	// From now on there's no implicit profile: the picker is needed.
	resp, b = e.do(t, "GET", "/api/v1/whoami", "", nil)
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(b), "profile_required") {
		t.Fatalf("an admin with a PIN must be picked, got %d: %s", resp.StatusCode, b)
	}
	var session struct {
		Token string `json:"token"`
	}
	e.json(t, "POST", "/api/v1/profiles/"+itoa(me.ID)+"/session", `{"pin":"1234"}`, nil, 200, &session)
	admin = session.Token
	var ben struct {
		ID int64 `json:"id"`
	}
	e.json(t, "POST", "/api/v1/profiles", `{"name":"Ben","color":"green"}`, withProfile(admin), 201, &ben)
	e.json(t, "POST", "/api/v1/profiles/"+itoa(ben.ID)+"/session", "", nil, 200, &session)
	return admin, session.Token, ben.ID
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestProfiles_SingleHouseholdNeedsNoPicker(t *testing.T) {
	e := newEnv(t)
	var me whoami
	e.json(t, "GET", "/api/v1/whoami", "", nil, 200, &me)
	if me.Role != "admin" || me.Profiles || len(me.Permissions) != len(perm.All()) {
		t.Fatalf("single profile: %+v", me)
	}
	var list struct {
		PickerNeeded bool `json:"pickerNeeded"`
	}
	e.json(t, "GET", "/api/v1/profiles", "", nil, 200, &list)
	if list.PickerNeeded {
		t.Fatal("a single profile without PIN needs no picker")
	}
}

func TestProfiles_ViewerIsLimitedByTheServer(t *testing.T) {
	e := newEnv(t)
	admin, viewer, _ := e.household(t)

	var me whoami
	e.json(t, "GET", "/api/v1/whoami", "", withProfile(viewer), 200, &me)
	if me.Username != "Ben" || me.Role != "user" || !me.Profiles {
		t.Fatalf("viewer: %+v", me)
	}

	// Watching works; system, settings and recordings management don't.
	for _, ok := range []string{"/api/v1/timers", "/api/v1/guide/bouquets", "/api/v1/media/movies", "/api/v1/dashboard"} {
		if resp, b := e.do(t, "GET", ok, "", withProfile(viewer)); resp.StatusCode != 200 {
			t.Errorf("viewer GET %s: %d %s", ok, resp.StatusCode, b)
		}
	}
	for _, path := range []string{"/api/v1/changes", "/api/v1/admin/logs", "/api/v1/tokens", "/api/v1/connectors"} {
		if resp, _ := e.do(t, "GET", path, "", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
			t.Errorf("viewer GET %s: expected 403, got %d", path, resp.StatusCode)
		}
	}
	if resp, _ := e.do(t, "POST", "/api/v1/media/trash/plan", `{"paths":["/media/hdd/movie/x.ts"]}`, withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer trash: expected 403, got %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "POST", "/api/v1/profiles", `{"name":"Eve"}`, withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer creates profile: expected 403, got %d", resp.StatusCode)
	}

	// Settings: only personal fields are shown, global ones refused.
	var view struct {
		Sections []struct {
			Fields []struct {
				Scope string `json:"scope"`
			} `json:"fields"`
		} `json:"sections"`
	}
	e.json(t, "GET", "/api/v1/settings", "", withProfile(viewer), 200, &view)
	for _, s := range view.Sections {
		for _, f := range s.Fields {
			if f.Scope != "user" {
				t.Fatalf("viewer sees a %s setting", f.Scope)
			}
		}
	}
	resp, b := e.do(t, "PATCH", "/api/v1/settings", `{"values":{"network.allowedHosts":"x"}}`, withProfile(viewer))
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "forbidden") {
		t.Errorf("viewer global setting: %d %s", resp.StatusCode, b)
	}

	// The navigation hides SystemHub and AdminHub.
	_, b = e.do(t, "GET", "/api/v1/navigation", "", withProfile(viewer))
	if strings.Contains(string(b), `"/system"`) || strings.Contains(string(b), `"/admin"`) {
		t.Errorf("viewer navigation shows system or admin: %s", b)
	}
	_, b = e.do(t, "GET", "/api/v1/navigation", "", withProfile(admin))
	if !strings.Contains(string(b), `"/system"`) {
		t.Errorf("admin navigation lacks system: %s", b)
	}
}

func TestProfiles_ViewerDeletesOnlyOwnTimers(t *testing.T) {
	e := newEnv(t)
	admin, viewer, _ := e.household(t)

	// Ben plans and applies a timer.
	resp, b := e.do(t, "POST", "/api/v1/timers/plan", timerBody("Die Kanzlei (S01/E07)", ev(21, 45), ev(22, 30), 0, 0), withProfile(viewer))
	if resp.StatusCode != 200 {
		t.Fatalf("viewer plans a timer: %d %s", resp.StatusCode, b)
	}
	var p planResp
	json.Unmarshal(b, &p)
	// Someone else's plan can't be applied by the admin either.
	if resp, _ := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(admin)); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("admin applies Ben's plan: expected 403, got %d", resp.StatusCode)
	}
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(viewer), 200, nil)

	var list struct {
		Timers []struct {
			ServiceReference string `json:"serviceReference"`
			Begin            int64  `json:"begin"`
			End              int64  `json:"end"`
			Name             string `json:"name"`
			MayChange        bool   `json:"mayChange"`
		} `json:"timers"`
	}
	e.json(t, "GET", "/api/v1/timers", "", withProfile(viewer), 200, &list)
	mine, others := 0, 0
	for _, tm := range list.Timers {
		body, _ := json.Marshal(map[string]any{"serviceReference": tm.ServiceReference, "begin": tm.Begin, "end": tm.End})
		resp, _ := e.do(t, "POST", "/api/v1/timers/delete/plan", string(body), withProfile(viewer))
		if tm.Name == "Die Kanzlei (S01/E07)" {
			mine++
			if !tm.MayChange || resp.StatusCode != 200 {
				t.Errorf("own timer: mayChange %v, delete plan %d", tm.MayChange, resp.StatusCode)
			}
		} else {
			others++
			if tm.MayChange || resp.StatusCode != http.StatusForbidden {
				t.Errorf("%q: mayChange %v, delete plan %d", tm.Name, tm.MayChange, resp.StatusCode)
			}
		}
	}
	if mine != 1 || others == 0 {
		t.Fatalf("expected Ben's timer among the mock's: mine %d, others %d", mine, others)
	}
	// The admin may change every timer.
	e.json(t, "GET", "/api/v1/timers", "", withProfile(admin), 200, &list)
	for _, tm := range list.Timers {
		if !tm.MayChange {
			t.Errorf("admin can't change %q", tm.Name)
		}
	}
}

func TestProfiles_NewProfileStartsWithTheCreatorsLanguage(t *testing.T) {
	e := newEnv(t)
	e.json(t, "PATCH", "/api/v1/settings", `{"values":{"general.language":"it"}}`, nil, 200, nil)
	_, viewer, _ := e.household(t)
	_, b := e.do(t, "GET", "/api/v1/settings", "", withProfile(viewer))
	if !strings.Contains(string(b), `"value":"it"`) {
		t.Fatalf("the new profile's language isn't the creator's: %s", b)
	}
}

func TestProfiles_SwitchesAndInvariants(t *testing.T) {
	e := newEnv(t)
	admin, viewer, benID := e.household(t)
	id := itoa(benID)

	// Taking away "plan recordings" is enforced at once.
	e.json(t, "PATCH", "/api/v1/profiles/"+id, `{"overrides":{"timers.plan":false}}`, withProfile(admin), 200, nil)
	if resp, _ := e.do(t, "POST", "/api/v1/timers/plan", timerBody("X", ev(21, 45), ev(22, 30)), withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("switched off: expected 403, got %d", resp.StatusCode)
	}
	// Only the listed switches exist.
	if resp, _ := e.do(t, "PATCH", "/api/v1/profiles/"+id, `{"overrides":{"system":true}}`, withProfile(admin)); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown switch: expected 400, got %d", resp.StatusCode)
	}

	var me whoami
	e.json(t, "GET", "/api/v1/whoami", "", withProfile(admin), 200, &me)
	// The last admin can't be demoted, nor lose the PIN while others exist,
	// nor delete itself.
	for body, code := range map[string]string{`{"role":"user"}`: "profile_last_admin", `{"pin":""}`: "profile_admin_pin_required"} {
		resp, b := e.do(t, "PATCH", "/api/v1/profiles/"+itoa(me.ID), body, withProfile(admin))
		if resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), code) {
			t.Errorf("%s: expected %s, got %d %s", body, code, resp.StatusCode, b)
		}
	}
	if resp, _ := e.do(t, "DELETE", "/api/v1/profiles/"+itoa(me.ID), "", withProfile(admin)); resp.StatusCode != http.StatusConflict {
		t.Errorf("self delete: expected 409, got %d", resp.StatusCode)
	}

	// Deleting Ben ends his session.
	e.json(t, "DELETE", "/api/v1/profiles/"+id, "", withProfile(admin), 204, nil)
	resp, b := e.do(t, "GET", "/api/v1/whoami", "", withProfile(viewer))
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(b), "profile_session_invalid") {
		t.Fatalf("deleted profile's session: %d %s", resp.StatusCode, b)
	}
}

func TestProfiles_WrongPinsLockTheProfile(t *testing.T) {
	e := newEnv(t)
	e.household(t)
	var me whoami
	var list struct {
		Profiles []struct {
			ID   int64  `json:"id"`
			Role string `json:"role"`
		} `json:"profiles"`
	}
	e.json(t, "GET", "/api/v1/profiles", "", nil, 200, &list)
	for _, p := range list.Profiles {
		if p.Role == "admin" {
			me.ID = p.ID
		}
	}
	path := "/api/v1/profiles/" + itoa(me.ID) + "/session"
	for i := 0; i < 5; i++ {
		if resp, _ := e.do(t, "POST", path, `{"pin":"0000"}`, nil); resp.StatusCode != http.StatusForbidden {
			t.Fatalf("try %d: expected 403, got %d", i, resp.StatusCode)
		}
	}
	// Locked now — even the right PIN waits.
	resp, _ := e.do(t, "POST", path, `{"pin":"1234"}`, nil)
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("expected 429 with Retry-After, got %d", resp.StatusCode)
	}
}

// Every route that changes something states its permission — or belongs to
// an admin-only hub — so a new hub can't hand viewers a delete button by
// forgetting to.
func TestRoutes_EveryChangingRouteDeclaresAPermission(t *testing.T) {
	e := newEnv(t)
	if len(e.routes) < 50 {
		t.Fatalf("expected the full route table, got %d routes", len(e.routes))
	}
	for _, r := range e.routes {
		method, _, _ := strings.Cut(r.Pattern, " ")
		if method == "GET" || method == "HEAD" || r.Declared {
			continue
		}
		if r.Needs[0] != perm.System {
			t.Errorf("%s changes something but declares no permission", r.Pattern)
		}
	}
}

func TestProfiles_ViewersStartWithTheTemplateAndCanReset(t *testing.T) {
	e := newEnv(t)
	admin, viewer, _ := e.household(t)

	// The admin designs the viewers' start page: live TV first, no timers.
	var tpl struct {
		Tiles []struct {
			Type   string `json:"type"`
			Width  int    `json:"width"`
			Hidden bool   `json:"hidden"`
		} `json:"tiles"`
		FromTemplate bool `json:"fromTemplate"`
	}
	e.json(t, "GET", "/api/v1/dashboard/template", "", withProfile(admin), 200, &tpl)
	for _, tile := range tpl.Tiles {
		if tile.Type == "hubs.status" {
			t.Fatal("the viewers' template offers the admin-only tile")
		}
	}
	e.json(t, "PUT", "/api/v1/dashboard/template",
		`{"tiles":[{"type":"stream.channels","width":4,"hidden":false},{"type":"timers.upcoming","width":2,"hidden":true}]}`,
		withProfile(admin), 200, nil)
	if resp, _ := e.do(t, "PUT", "/api/v1/dashboard/template", `{"tiles":[]}`, withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a viewer edits the template: expected 403, got %d", resp.StatusCode)
	}

	e.json(t, "GET", "/api/v1/dashboard", "", withProfile(viewer), 200, &tpl)
	if !tpl.FromTemplate || tpl.Tiles[0].Type != "stream.channels" || tpl.Tiles[0].Hidden {
		t.Fatalf("the viewer doesn't see the template: %+v", tpl)
	}
	// Customizing makes it the viewer's own; reset brings the template back.
	tpl.FromTemplate = false
	e.json(t, "PUT", "/api/v1/dashboard", `{"tiles":[{"type":"timers.upcoming","width":2,"hidden":false}]}`, withProfile(viewer), 200, &tpl)
	if tpl.FromTemplate || tpl.Tiles[0].Type != "timers.upcoming" {
		t.Fatalf("own layout: %+v", tpl)
	}
	e.json(t, "DELETE", "/api/v1/dashboard", "", withProfile(viewer), 200, &tpl)
	if !tpl.FromTemplate || tpl.Tiles[0].Type != "stream.channels" {
		t.Fatalf("after reset: %+v", tpl)
	}
	// The admin's own start page isn't the template.
	tpl.FromTemplate = false
	e.json(t, "GET", "/api/v1/dashboard", "", withProfile(admin), 200, &tpl)
	if tpl.FromTemplate || tpl.Tiles[0].Type != "control.nowPlaying" {
		t.Fatalf("admin: %+v", tpl)
	}
}

func TestChanges_FilterByProfile(t *testing.T) {
	e := newEnv(t)
	admin, _, benID := e.household(t)
	var me whoami
	e.json(t, "GET", "/api/v1/whoami", "", withProfile(admin), 200, &me)
	type changes struct {
		Changes []struct {
			UserID   int64  `json:"userId"`
			UserName string `json:"userName"`
			Action   string `json:"action"`
		} `json:"changes"`
	}
	var mine changes
	e.json(t, "GET", "/api/v1/changes?user="+itoa(me.ID), "", withProfile(admin), 200, &mine)
	if len(mine.Changes) == 0 {
		t.Fatal("the admin's changes are missing")
	}
	for _, c := range mine.Changes {
		if c.UserID != me.ID || c.UserName != "admin" {
			t.Fatalf("filtered by the admin, got %+v", c)
		}
	}
	// Ben made nothing himself.
	var ben changes
	e.json(t, "GET", "/api/v1/changes?user="+itoa(benID), "", withProfile(admin), 200, &ben)
	if len(ben.Changes) != 0 {
		t.Fatalf("Ben's changes: %+v", ben.Changes)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/changes?user=x", "", withProfile(admin)); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad user: %d", resp.StatusCode)
	}
}
