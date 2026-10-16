package api_test

import (
	"context"
	"encoding/json"
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
