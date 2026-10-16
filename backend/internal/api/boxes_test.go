package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/autotimer/mock"
	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/boxes"
	srmock "github.com/antonbier/DreamHub/backend/internal/serienrecorder/mock"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	wcmock "github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// addBox registers a second, remote mock box (docs/MULTIDEVICE_PLAN.md, M1)
// and returns it for the test to change.
func (e *env) addBox(t *testing.T, id string) *wcmock.Client {
	t.Helper()
	m := wcmock.New()
	e.app.Boxes.Add(store.Box{ID: id, Name: id}, boxes.Clients{WC: m, Series: srmock.New(), AutoTimer: mock.New()}, false)
	return m
}

func (e *env) timerNames(t *testing.T, query string) []string {
	t.Helper()
	resp, b := e.do(t, "GET", "/api/v1/timers"+query, "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("timers%s: %d %s", query, resp.StatusCode, b)
	}
	var body struct {
		Timers []struct{ Name string } `json:"timers"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tm := range body.Timers {
		names = append(names, tm.Name)
	}
	return names
}

func TestBox_ParameterPicksTheBox(t *testing.T) {
	e := newEnv(t)
	second := e.addBox(t, "bedroom")
	timers, _ := second.TimerList(context.Background())
	if len(timers) == 0 {
		t.Fatal("the mock box should have fixture timers")
	}
	if err := second.DeleteTimer(context.Background(), webcontrol.TimerRef{ServiceReference: timers[0].ServiceReference, Begin: timers[0].Begin, End: timers[0].End}); err != nil {
		t.Fatal(err)
	}

	first, other := e.timerNames(t, ""), e.timerNames(t, "?box=bedroom")
	if len(other) != len(first)-1 {
		t.Fatalf("the bedroom box lost a timer: first %v, bedroom %v", first, other)
	}
	// Without the parameter, and with the first box's ID, it is the first box.
	if explicit := e.timerNames(t, "?box=main"); strings.Join(explicit, "|") != strings.Join(first, "|") {
		t.Errorf("box=main differs from no parameter: %v vs %v", explicit, first)
	}
}

func TestBox_UnknownBoxIs404(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/timers?box=nowhere", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusNotFound || code != "box_not_found" {
		t.Fatalf("expected 404 box_not_found, got %d %s", resp.StatusCode, b)
	}
}

func TestBox_PlanAppliesToTheBoxItWasMadeFor(t *testing.T) {
	e := newEnv(t)
	second := e.addBox(t, "bedroom")
	before, _ := second.TimerList(context.Background())

	// Planned for the bedroom, applied by a request that names another box:
	// the plan still acts on the bedroom.
	resp, b := e.do(t, "POST", "/api/v1/timers/plan?box=bedroom", timerBody("Die Kanzlei (S01/E07)", ev(21, 45), ev(22, 30), 0, 0), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d %s", resp.StatusCode, b)
	}
	var p planResp
	json.Unmarshal(b, &p)
	resp, b = e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply?box=main", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	after, _ := second.TimerList(context.Background())
	if len(after) != len(before)+1 {
		t.Errorf("the timer should have been created on the bedroom box: %d -> %d", len(before), len(after))
	}
	if n := e.timerCount(t); n != len(before) {
		t.Errorf("the first box must be untouched: %d timers, expected %d", n, len(before))
	}
}

func TestBox_TimerOwnersAreKeyedByBox(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.addBox(t, "bedroom")
	if err := e.store.SetTimerOwner(boxCtx(ctx, "main"), "1:0:1:AA", 1000, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.TimerOwner(boxCtx(ctx, "bedroom"), "1:0:1:AA", 1000); err != store.ErrNotFound {
		t.Errorf("the same channel and begin on another box has no owner, got %v", err)
	}
	if id, err := e.store.TimerOwner(boxCtx(ctx, "main"), "1:0:1:AA", 1000); err != nil || id != 1 {
		t.Errorf("owner on the first box: %d %v", id, err)
	}
	// An unaddressed context means the first box.
	if id, err := e.store.TimerOwner(ctx, "1:0:1:AA", 1000); err != nil || id != 1 {
		t.Errorf("owner without a box: %d %v", id, err)
	}
}

func TestBox_RemoteBoxHasNoLocalFileOperationsOrAdministration(t *testing.T) {
	e := newEnv(t)
	e.addBox(t, "bedroom")

	// The first box is the machine DreamHub runs on: file operations work.
	_, b := e.do(t, "GET", "/api/v1/media/movies", "", nil)
	var first, remote struct {
		FileOps struct{ Available bool } `json:"fileOps"`
	}
	json.Unmarshal(b, &first)
	if !first.FileOps.Available {
		t.Fatalf("file operations should work on the first box: %s", b)
	}
	// A second box shares the same paths in this test, but not the disk.
	_, b = e.do(t, "GET", "/api/v1/media/movies?box=bedroom", "", nil)
	json.Unmarshal(b, &remote)
	if remote.FileOps.Available {
		t.Errorf("DreamHub's own disk must not be offered as the other box's: %s", b)
	}
	resp, b := e.do(t, "POST", "/api/v1/media/trash/plan?box=bedroom", `{"paths":["/media/hdd/movie/x.ts"]}`, nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "file_ops_unavailable" {
		t.Errorf("trash plan on the remote box: %d %s", resp.StatusCode, b)
	}
	// Administering the machine is the first box's alone.
	resp, b = e.do(t, "GET", "/api/v1/admin/services?box=bedroom", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "box_not_full" {
		t.Errorf("admin on the remote box: %d %s", resp.StatusCode, b)
	}
	if resp, b = e.do(t, "GET", "/api/v1/admin/services", "", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("admin on the first box: %d %s", resp.StatusCode, b)
	}
}

func TestBox_ConnectorsAndHubStatusAreCheckedPerBox(t *testing.T) {
	e := newEnv(t)
	second := e.addBox(t, "bedroom")
	second.NoBouquetEditor = true
	_, b := e.do(t, "GET", "/api/v1/channels/bouquets", "", nil)
	_, other := e.do(t, "GET", "/api/v1/channels/bouquets?box=bedroom", "", nil)
	if !strings.Contains(string(b), `"editor":true`) || !strings.Contains(string(other), `"editor":false`) {
		t.Errorf("editor availability must follow the box: %s / %s", b, other)
	}
}

func TestBox_JobsRunAgainstTheirBox(t *testing.T) {
	e := newEnv(t)
	second := e.addBox(t, "bedroom")
	second.RecordNow(fixedNow)
	// A recording on the bedroom box must not hold the first box's queue.
	_, b := e.do(t, "GET", "/api/v1/jobs", "", nil)
	if strings.Contains(string(b), `"blockedBy":"recording"`) {
		t.Errorf("a recording on another box holds the queue: %s", b)
	}
}

func boxCtx(ctx context.Context, id string) context.Context { return box.With(ctx, id) }

// A hub with no box of its own (DreamHub on an Unraid server, every box
// remote) is tested from M1 on, not added afterwards: the local box is no
// special case, capabilities decide.
func TestBox_HubWithoutAnyLocalFileSystem(t *testing.T) {
	e := newEnvWithOptions(t, wcmock.New(), mock.New(), nil)
	e.addBox(t, "bedroom")
	for _, q := range []string{"", "?box=bedroom"} {
		resp, b := e.do(t, "GET", "/api/v1/media/movies"+q, "", nil)
		var body struct {
			FileOps struct{ Available bool } `json:"fileOps"`
		}
		json.Unmarshal(b, &body)
		if resp.StatusCode != http.StatusOK || body.FileOps.Available {
			t.Errorf("media%s: %d %s", q, resp.StatusCode, b)
		}
		if n := len(e.timerNames(t, q)); n == 0 {
			t.Errorf("timers%s: the box answers, the hub needs no disk of its own", q)
		}
	}
}

// The navigation of a Basic box: no AppsHub, and the box's capabilities
// ride along so the UI can hide what that box will never have.
func TestBox_NavigationOfABasicBox(t *testing.T) {
	e := newEnv(t)
	e.addBox(t, "bedroom")
	if _, ok := navItems(t, e)["apps"]; !ok {
		t.Fatal("AppsHub belongs to the box DreamHub runs on")
	}
	for _, c := range []struct {
		query string
		full  bool
	}{{"", true}, {"?box=bedroom", false}} {
		resp, b := e.do(t, "GET", "/api/v1/navigation"+c.query, "", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("navigation%s: %d %s", c.query, resp.StatusCode, b)
		}
		var nav struct {
			Box struct {
				ID           string
				Full         bool
				Capabilities []string
			}
			Sections []struct{ Items []struct{ ID string } }
		}
		if err := json.Unmarshal(b, &nav); err != nil {
			t.Fatal(err)
		}
		if nav.Box.Full != c.full || strings.Contains(strings.Join(nav.Box.Capabilities, ","), "docker") != c.full {
			t.Errorf("navigation%s: box %+v", c.query, nav.Box)
		}
		hasApps := false
		for _, s := range nav.Sections {
			for _, it := range s.Items {
				hasApps = hasApps || it.ID == "apps"
			}
		}
		if hasApps != c.full {
			t.Errorf("navigation%s: AppsHub shown = %v", c.query, hasApps)
		}
	}
}

// brokenBox is a box whose device info fails the way a real one would.
type brokenBox struct {
	webcontrol.Client
	err error
}

func (b brokenBox) DeviceInfo(context.Context) (*webcontrol.DeviceInfo, error) { return nil, b.err }

// testBoxFactory connects "further boxes" to mock boxes; an address
// containing "locked" wants a login, "dead" does not answer.
func testBoxFactory(b store.Box) (boxes.Clients, error) {
	c := boxes.Clients{WC: wcmock.New(), Series: srmock.New(), AutoTimer: mock.New()}
	switch {
	case strings.Contains(b.URL, "locked"):
		c.WC = brokenBox{c.WC, webcontrol.ErrUnauthorized}
	case strings.Contains(b.URL, "dead"):
		c.WC = brokenBox{c.WC, errors.New("connection refused")}
	}
	return c, nil
}

type boxList struct {
	Boxes []struct {
		ID, Name, Color, URL string
		Full, Online         bool
		HasPassword          bool
	} `json:"boxes"`
	Default string `json:"default"`
}

func TestBoxes_AddTestEditAndRemove(t *testing.T) {
	e := newEnv(t)

	// One box: the list has the first box only, and no connection details for it.
	var list boxList
	e.json(t, "GET", "/api/v1/boxes", "", nil, 200, &list)
	if len(list.Boxes) != 1 || list.Boxes[0].ID != "main" || !list.Boxes[0].Full || list.Default != "main" || list.Boxes[0].URL != "" {
		t.Fatalf("boxes: %+v", list)
	}

	// Testing changes nothing and says what is wrong.
	var res struct {
		DeviceName string `json:"deviceName"`
	}
	e.json(t, "POST", "/api/v1/boxes/test", `{"url":"192.168.1.20"}`, nil, 200, &res)
	if res.DeviceName == "" {
		t.Errorf("the test should name the device: %+v", res)
	}
	for url, code := range map[string]string{"locked.lan": "box_unauthorized", "dead.lan": "box_unreachable"} {
		resp, b := e.do(t, "POST", "/api/v1/boxes/test", `{"url":"`+url+`"}`, nil)
		if got, _, _ := errCode(t, b); resp.StatusCode != http.StatusUnprocessableEntity || got != code {
			t.Errorf("test %s: %d %s", url, resp.StatusCode, b)
		}
	}
	resp, b := e.do(t, "POST", "/api/v1/boxes/test", `{"url":"ftp://x"}`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a bad address is a validation error: %d %s", resp.StatusCode, b)
	}
	e.json(t, "GET", "/api/v1/boxes", "", nil, 200, &list)
	if len(list.Boxes) != 1 {
		t.Fatalf("testing must not add a box: %+v", list)
	}

	// An unreachable box is not added.
	if resp, b := e.do(t, "POST", "/api/v1/boxes", `{"name":"Cellar","url":"dead.lan"}`, nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("adding an unreachable box: %d %s", resp.StatusCode, b)
	}

	// Add: the ID comes from the name, the password is never shown again.
	var added struct{ ID, Name, URL, Username string }
	e.json(t, "POST", "/api/v1/boxes", `{"name":"Living room","color":"green","url":"192.168.1.20/","username":"root","password":"secret"}`, nil, 201, &added)
	if added.ID != "living-room" || added.URL != "http://192.168.1.20" || added.Username != "root" {
		t.Fatalf("added: %+v", added)
	}
	e.json(t, "POST", "/api/v1/boxes", `{"name":"Living room","url":"192.168.1.21"}`, nil, 201, &added)
	if added.ID != "living-room-2" {
		t.Errorf("a second box with the same name gets its own ID: %s", added.ID)
	}
	e.json(t, "GET", "/api/v1/boxes", "", nil, 200, &list)
	if len(list.Boxes) != 3 || !list.Boxes[1].HasPassword || list.Boxes[1].Full {
		t.Fatalf("boxes after adding: %+v", list)
	}
	if _, b := e.do(t, "GET", "/api/v1/boxes", "", nil); strings.Contains(string(b), "secret") {
		t.Errorf("the password must not be returned: %s", b)
	}
	// The new box answers requests.
	if names := e.timerNames(t, "?box=living-room"); len(names) == 0 {
		t.Errorf("the new box should have its timers")
	}

	// Edit name and colour; the first box can be renamed but keeps its address.
	e.json(t, "PATCH", "/api/v1/boxes/living-room", `{"name":"Lounge","color":"red"}`, nil, 200, nil)
	e.json(t, "PATCH", "/api/v1/boxes/main", `{"name":"Bedroom","url":"10.0.0.1"}`, nil, 200, nil)
	e.json(t, "GET", "/api/v1/boxes", "", nil, 200, &list)
	if list.Boxes[0].Name != "Bedroom" || list.Boxes[1].Name != "Lounge" || list.Boxes[1].Color != "red" || list.Boxes[0].URL != "" {
		t.Errorf("after editing: %+v", list)
	}
	if resp, _ := e.do(t, "PATCH", "/api/v1/boxes/nowhere", `{"name":"x"}`, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("editing an unknown box: %d", resp.StatusCode)
	}

	// Removing is a plan; the first box can't be removed.
	if resp, _ := e.do(t, "POST", "/api/v1/boxes/main/remove/plan", "", nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("the first box can't be removed: %d", resp.StatusCode)
	}
	var p planResp
	e.json(t, "POST", "/api/v1/boxes/living-room/remove/plan", "", nil, 200, &p)
	e.json(t, "GET", "/api/v1/boxes", "", nil, 200, &list)
	if len(list.Boxes) != 3 {
		t.Fatalf("a plan removes nothing: %+v", list)
	}
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil, 200, nil)
	e.json(t, "GET", "/api/v1/boxes", "", nil, 200, &list)
	if len(list.Boxes) != 2 {
		t.Fatalf("after removing: %+v", list)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/timers?box=living-room", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a removed box is gone: %d", resp.StatusCode)
	}
}

func TestBoxes_ProfileMayUseOnlyItsBoxes(t *testing.T) {
	e := newEnv(t)
	admin, viewer, benID := e.householdWithBoxes(t)

	// Ben may use the bedroom only: without a parameter that is his box.
	e.json(t, "PATCH", "/api/v1/profiles/"+itoa(benID), `{"boxes":["bedroom"]}`, withProfile(admin), 200, nil)
	var list boxList
	e.json(t, "GET", "/api/v1/boxes", "", withProfile(viewer), 200, &list)
	if len(list.Boxes) != 1 || list.Boxes[0].ID != "bedroom" || list.Default != "bedroom" {
		t.Fatalf("Ben's boxes: %+v", list)
	}
	if resp, b := e.do(t, "GET", "/api/v1/timers?box=main", "", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a box the profile may not use: %d %s", resp.StatusCode, b)
	}
	if resp, b := e.do(t, "GET", "/api/v1/timers", "", withProfile(viewer)); resp.StatusCode != http.StatusOK {
		t.Errorf("the default is his box: %d %s", resp.StatusCode, b)
	}
	// Viewers can't manage boxes.
	if resp, _ := e.do(t, "POST", "/api/v1/boxes/test", `{"url":"x.lan"}`, withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a viewer may not test boxes: %d", resp.StatusCode)
	}
	// The administrator sees every box.
	e.json(t, "GET", "/api/v1/boxes", "", withProfile(admin), 200, &list)
	if len(list.Boxes) != 2 {
		t.Errorf("the administrator's boxes: %+v", list)
	}

	// Removing the only box a profile may use never widens it to "all".
	var p planResp
	e.json(t, "POST", "/api/v1/boxes/bedroom/remove/plan", "", withProfile(admin), 200, &p)
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(admin), 200, nil)
	var profiles struct {
		Profiles []struct {
			ID    int64
			Boxes []string
		}
	}
	e.json(t, "GET", "/api/v1/profiles", "", withProfile(admin), 200, &profiles)
	for _, pr := range profiles.Profiles {
		if pr.ID == benID && (len(pr.Boxes) != 1 || pr.Boxes[0] != "main") {
			t.Errorf("Ben falls back to the first box: %v", pr.Boxes)
		}
	}
}

// householdWithBoxes is household() plus a second box "bedroom".
func (e *env) householdWithBoxes(t *testing.T) (admin, viewer string, viewerID int64) {
	t.Helper()
	admin, viewer, viewerID = e.household(t)
	e.json(t, "POST", "/api/v1/boxes", `{"name":"Bedroom","url":"192.168.1.30"}`, withProfile(admin), 201, nil)
	return
}

func TestBoxes_OverviewShowsEachBoxAndSkipsOfflineOnes(t *testing.T) {
	e := newEnv(t)
	e.addBox(t, "bedroom")
	type overview struct {
		Boxes []struct {
			ID                         string
			Online, Standby, Recording bool
			Service                    string
		}
	}
	var ov overview
	e.json(t, "GET", "/api/v1/boxes/overview", "", nil, 200, &ov)
	if len(ov.Boxes) != 2 || ov.Boxes[0].ID != "main" || ov.Boxes[1].ID != "bedroom" || !ov.Boxes[1].Online {
		t.Fatalf("overview: %+v", ov)
	}
	if !ov.Boxes[1].Standby && ov.Boxes[1].Service == "" {
		t.Errorf("a box that is on shows what it plays: %+v", ov.Boxes[1])
	}
	e.app.Boxes.SetOnline("bedroom", false)
	ov = overview{}
	e.json(t, "GET", "/api/v1/boxes/overview", "", nil, 200, &ov)
	if ov.Boxes[1].Online || ov.Boxes[1].Service != "" {
		t.Errorf("an offline box is reported as such: %+v", ov.Boxes[1])
	}
	var list boxList
	e.json(t, "GET", "/api/v1/boxes", "", nil, 200, &list)
	if list.Boxes[1].Online {
		t.Errorf("the list knows it is offline: %+v", list.Boxes[1])
	}
}

// The second mock box has its own programme: nothing of one box's timers,
// recordings or conflicts shows in the other's views (M4).
func TestBox_SecondMockBoxHasItsOwnContent(t *testing.T) {
	e := newEnv(t)
	e.app.Boxes.Add(store.Box{ID: "living", Name: "living"}, boxes.Clients{WC: wcmock.NewSecond(), Series: srmock.New(), AutoTimer: mock.New()}, false)

	first, other := e.timerNames(t, ""), e.timerNames(t, "?box=living")
	for _, n := range other {
		for _, m := range first {
			if n == m {
				t.Fatalf("timer %q shows on both boxes", n)
			}
		}
	}
	if len(other) != 3 {
		t.Fatalf("second box timers = %v", other)
	}
	// Two tuners: its overlapping timers are no conflict.
	resp, b := e.do(t, "GET", "/api/v1/timers?box=living", "", nil)
	if resp.StatusCode != http.StatusOK || strings.Contains(string(b), `"conflict":true`) {
		t.Fatalf("second box timers: %d %s", resp.StatusCode, b)
	}
	resp, b = e.do(t, "GET", "/api/v1/deviceinfo?box=living", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), "wohnzimmer") {
		t.Fatalf("second box device info: %d %s", resp.StatusCode, b)
	}
}
