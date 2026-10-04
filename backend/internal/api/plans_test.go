package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	atmock "github.com/antonbier/DreamHub/backend/internal/autotimer/mock"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// ev is a time on the mock evening (2026-09-23, CEST). "Now" in these tests
// is 20:07 CEST (fixedNow).
func ev(h, m int) int64 {
	return mock.Evening.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute).Unix()
}

func timerBody(name string, begin, end int64, margins ...int) string {
	m := ""
	if len(margins) == 2 {
		m = fmt.Sprintf(`,"marginBefore":%d,"marginAfter":%d`, margins[0], margins[1])
	}
	return fmt.Sprintf(`{"serviceReference":"1:0:19:1:1:1:C00000:0:0:0:","serviceName":"ONE HD","name":%q,"begin":%d,"end":%d%s}`, name, begin, end, m)
}

type planResp struct {
	Plan struct {
		ID        string    `json:"id"`
		Action    string    `json:"action"`
		CreatedAt time.Time `json:"createdAt"`
		ExpiresAt time.Time `json:"expiresAt"`
		Changes   []struct {
			Type  string `json:"type"`
			After struct {
				Name        string `json:"name"`
				RecordBegin int64  `json:"recordBegin"`
				RecordEnd   int64  `json:"recordEnd"`
			} `json:"after"`
		} `json:"changes"`
		Warnings []struct {
			Code   string         `json:"code"`
			Params map[string]any `json:"params"`
		} `json:"warnings"`
	} `json:"plan"`
}

func (e *env) planTimer(t *testing.T, body string) planResp {
	t.Helper()
	resp, b := e.do(t, "POST", "/api/v1/timers/plan", body, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: expected 200, got %d: %s", resp.StatusCode, b)
	}
	var p planResp
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func (e *env) timerCount(t *testing.T) int {
	t.Helper()
	_, b := e.do(t, "GET", "/api/v1/timers", "", nil)
	var body struct {
		Timers []json.RawMessage `json:"timers"`
		Tuners int               `json:"tuners"`
	}
	json.Unmarshal(b, &body)
	if body.Tuners != 1 {
		t.Fatalf("the mock box has one tuner, got %d", body.Tuners)
	}
	return len(body.Timers)
}

func errCode(t *testing.T, b []byte) (code string, details map[string]httpx.FieldError, requestID string) {
	t.Helper()
	var env struct {
		Error struct {
			Code      string                      `json:"code"`
			RequestID string                      `json:"requestId"`
			Details   map[string]httpx.FieldError `json:"details"`
		} `json:"error"`
	}
	json.Unmarshal(b, &env)
	return env.Error.Code, env.Error.Details, env.Error.RequestID
}

func TestTimerPlan_PreviewChangesNothingUntilApplied(t *testing.T) {
	e := newEnv(t)
	if n := e.timerCount(t); n != 2 {
		t.Fatalf("expected the 2 fixture timers, got %d", n)
	}

	// Die Kanzlei 21:45–22:30 without lead-in: starts exactly when The Rookie
	// ends, so no conflict on the single tuner.
	p := e.planTimer(t, timerBody("Die Kanzlei (S01/E07)", ev(21, 45), ev(22, 30), 0, 10))
	if p.Plan.Action != "timer.create" || len(p.Plan.Warnings) != 0 {
		t.Fatalf("expected a clean timer.create plan, got %+v", p.Plan)
	}
	if got := p.Plan.Changes[0].After; got.RecordBegin != ev(21, 45) || got.RecordEnd != ev(22, 40) {
		t.Errorf("margins must be applied to the recording window, got %+v", got)
	}
	if p.Plan.ExpiresAt.Sub(p.Plan.CreatedAt) != 10*time.Minute {
		t.Errorf("plans expire after 10 minutes, got %v", p.Plan.ExpiresAt.Sub(p.Plan.CreatedAt))
	}
	if n := e.timerCount(t); n != 2 {
		t.Fatal("a preview must not change anything")
	}

	resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: expected 200, got %d: %s", resp.StatusCode, b)
	}
	if n := e.timerCount(t); n != 3 {
		t.Fatalf("expected the timer to exist after apply, got %d timers", n)
	}

	_, b = e.do(t, "GET", "/api/v1/changes", "", nil)
	var log struct {
		Changes []struct{ Action, Summary string } `json:"changes"`
	}
	json.Unmarshal(b, &log)
	if len(log.Changes) == 0 || log.Changes[0].Action != "timer.create" || !strings.Contains(log.Changes[0].Summary, "Die Kanzlei") {
		t.Errorf("the applied change must be in the change log, got %+v", log.Changes)
	}

	resp, b = e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusNotFound || code != "plan_not_found" {
		t.Errorf("a plan is single use: expected 404 plan_not_found, got %d %s", resp.StatusCode, b)
	}
}

func TestTimerPlan_DefaultLeadInRevealsSingleTunerConflict(t *testing.T) {
	e := newEnv(t)
	// Default 5 min lead-in → recording starts 21:40, while The Rookie runs
	// until 21:45.
	p := e.planTimer(t, timerBody("Die Kanzlei (S01/E07)", ev(21, 45), ev(22, 30)))
	if len(p.Plan.Warnings) != 1 || p.Plan.Warnings[0].Code != "timer.conflict" {
		t.Fatalf("expected a conflict warning, got %+v", p.Plan.Warnings)
	}
	with := p.Plan.Warnings[0].Params["with"].([]any)
	if with[0].(map[string]any)["name"] != "The Rookie" || p.Plan.Warnings[0].Params["tuners"].(float64) != 1 {
		t.Errorf("the warning must name the clashing timer and the tuner count, got %+v", p.Plan.Warnings[0].Params)
	}
	// A warning informs, it doesn't block: "create anyway" is the user's call.
	if resp, _ := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("a conflicting plan can still be applied, got %d", resp.StatusCode)
	}
}

func TestTimerPlan_StalePlanIsRejected(t *testing.T) {
	e := newEnv(t)
	a := e.planTimer(t, timerBody("Die Kanzlei (S01/E07)", ev(21, 45), ev(22, 30), 0, 0))
	b := e.planTimer(t, timerBody("Krieg der Zukunft", ev(22, 15), ev(23, 0), 0, 0))
	if resp, _ := e.do(t, "POST", "/api/v1/plans/"+b.Plan.ID+"/apply", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatal("applying the second plan should work")
	}
	// Plan A was previewed against a timer list that has changed since —
	// its conflict check no longer describes reality.
	resp, body := e.do(t, "POST", "/api/v1/plans/"+a.Plan.ID+"/apply", "", nil)
	if code, _, _ := errCode(t, body); resp.StatusCode != http.StatusConflict || code != "plan_stale" {
		t.Fatalf("expected 409 plan_stale, got %d %s", resp.StatusCode, body)
	}
	if n := e.timerCount(t); n != 3 {
		t.Errorf("a stale plan must not create anything, got %d timers", n)
	}
}

func TestTimerPlan_Validation(t *testing.T) {
	e := newEnv(t)
	for _, c := range []struct {
		body, field, code string
	}{
		{timerBody("X", ev(22, 0), ev(21, 0)), "end", "timer.endBeforeBegin"},
		{timerBody("X", ev(18, 0), ev(19, 0)), "end", "timer.inPast"},
		{timerBody("", ev(22, 0), ev(23, 0)), "name", "required"},
		{timerBody("X", ev(22, 0), ev(23, 0), 90, 0), "marginBefore", "out_of_range"},
	} {
		resp, b := e.do(t, "POST", "/api/v1/timers/plan", c.body, nil)
		code, details, _ := errCode(t, b)
		if resp.StatusCode != http.StatusBadRequest || code != "validation_failed" || details[c.field].Code != c.code {
			t.Errorf("%s: expected %s=%s, got %d %s", c.body, c.field, c.code, resp.StatusCode, b)
		}
	}
}

type readOnlyBox struct{ *mock.Client }

func (readOnlyBox) AddTimer(context.Context, webcontrol.NewTimer) error {
	return webcontrol.ErrWritesDisabled
}

func TestTimerPlan_WritesDisabledOnRealBoxUntilVerified(t *testing.T) {
	e := newEnvWith(t, readOnlyBox{mock.New()})
	p := e.planTimer(t, timerBody("Die Kanzlei (S01/E07)", ev(21, 45), ev(22, 30), 0, 0))
	resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "writes_disabled" {
		t.Fatalf("expected 409 writes_disabled, got %d %s", resp.StatusCode, b)
	}
	_, b = e.do(t, "GET", "/api/v1/changes", "", nil)
	if strings.Contains(string(b), "timer.create") {
		t.Error("nothing may be logged when the change wasn't applied")
	}
}

func TestCSRF(t *testing.T) {
	e := newEnv(t)
	noHeader := func(r *http.Request) { r.Header.Del(httpx.CSRFHeader) }
	foreign := func(r *http.Request) { r.Header.Set("Origin", "https://evil.example.com") }

	for _, c := range []struct {
		method, path, body string
		configure          func(*http.Request)
	}{
		{"POST", "/api/v1/timers/plan", timerBody("X", ev(22, 0), ev(23, 0)), noHeader},
		{"PATCH", "/api/v1/settings", `{"values":{"general.language":"de"}}`, noHeader},
		{"PATCH", "/api/v1/settings", `{"values":{"general.language":"de"}}`, foreign},
	} {
		resp, b := e.do(t, c.method, c.path, c.body, c.configure)
		if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusForbidden || code != "csrf_failed" {
			t.Errorf("%s %s: expected 403 csrf_failed, got %d %s", c.method, c.path, resp.StatusCode, b)
		}
	}
	sameOrigin := func(r *http.Request) { r.Header.Set("Origin", "http://"+r.Host) }
	if resp, _ := e.do(t, "PATCH", "/api/v1/settings", `{"values":{"general.language":"de"}}`, sameOrigin); resp.StatusCode != http.StatusOK {
		t.Errorf("same-origin request with the header must pass, got %d", resp.StatusCode)
	}
}

func TestHostCheck_AgainstDNSRebinding(t *testing.T) {
	e := newEnv(t)
	host := func(h string) func(*http.Request) { return func(r *http.Request) { r.Host = h } }

	resp, b := e.do(t, "GET", "/api/v1/health", "", host("evil.example.com"))
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusMisdirectedRequest || code != "host_not_allowed" {
		t.Fatalf("foreign host name: expected 421 host_not_allowed, got %d %s", resp.StatusCode, b)
	}
	for _, h := range []string{"192.168.1.43:8080", "[fdb2::1]:8080", "localhost:8080", "dreambox.local"} {
		if resp, _ := e.do(t, "GET", "/api/v1/health", "", host(h)); resp.StatusCode != http.StatusOK {
			t.Errorf("%s must be allowed, got %d", h, resp.StatusCode)
		}
	}
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"network.allowedHosts":"dreamhub.home.arpa, tv.example.org"}}`, nil)
	if resp, _ := e.do(t, "GET", "/api/v1/health", "", host("dreamhub.home.arpa:8080")); resp.StatusCode != http.StatusOK {
		t.Errorf("allowlisted host must be allowed, got %d", resp.StatusCode)
	}
}

func TestHubDisabled_RoutesAnswer404(t *testing.T) {
	e := newEnv(t)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"hubs.timer.enabled":false}}`, nil)
	resp, b := e.do(t, "GET", "/api/v1/timers", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusNotFound || code != "hub_disabled" {
		t.Errorf("expected 404 hub_disabled, got %d %s", resp.StatusCode, b)
	}
}

func TestAdminLogLevel(t *testing.T) {
	e := newEnv(t)
	get := func() map[string]string {
		_, b := e.do(t, "GET", "/api/v1/admin/log-level", "", nil)
		out := map[string]string{}
		json.Unmarshal(b, &out)
		return out
	}
	if get()["level"] != "info" {
		t.Fatal("log level starts at info")
	}
	e.do(t, "PUT", "/api/v1/admin/log-level", `{"level":"debug","minutes":5}`, nil)
	if got := get(); got["level"] != "debug" || got["until"] != "2026-09-23T18:12:00Z" {
		t.Errorf("debug must be on with a revert time, got %+v", got)
	}
	e.do(t, "PUT", "/api/v1/admin/log-level", `{"level":"info"}`, nil)
	if get()["level"] != "info" {
		t.Error("switching back to info must work")
	}
	resp, _ := e.do(t, "PUT", "/api/v1/admin/log-level", `{"level":"loud"}`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown level: expected 400, got %d", resp.StatusCode)
	}
	_, b := e.do(t, "GET", "/api/v1/changes", "", nil)
	if !strings.Contains(string(b), "admin.logLevel") {
		t.Error("log level changes must be recorded in the change log")
	}
}

func TestRequestID_InHeaderAndErrorBody(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/changes?limit=abc", "", nil)
	_, _, bodyID := errCode(t, b)
	headerID := resp.Header.Get(httpx.RequestIDHeader)
	if headerID == "" || bodyID != headerID {
		t.Fatalf("error body must carry the same request ID as the header: header %q, body %q", headerID, bodyID)
	}
	resp2, _ := e.do(t, "GET", "/api/v1/health", "", nil)
	if id2 := resp2.Header.Get(httpx.RequestIDHeader); id2 == "" || id2 == headerID {
		t.Errorf("every request needs its own ID, got %q and %q", headerID, id2)
	}
}

func timerBodyRef(ref, name string, begin, end int64, before, after int) string {
	return fmt.Sprintf(`{"serviceReference":%q,"serviceName":"x","name":%q,"begin":%d,"end":%d,"marginBefore":%d,"marginAfter":%d}`, ref, name, begin, end, before, after)
}

// The mock's "Bauer sucht Frau International" timer (RTL, 20:15–21:15).
const rtlRef = "1:0:19:2EE3:441:1:C00000:0:0:0:"

func deleteBody(ref string, begin, end int64) string {
	return fmt.Sprintf(`{"serviceReference":%q,"begin":%d,"end":%d}`, ref, begin, end)
}

func TestTimerDelete_PreviewThenApply(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "POST", "/api/v1/timers/delete/plan", deleteBody(rtlRef, ev(20, 15), ev(21, 15)), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: expected 200, got %d: %s", resp.StatusCode, b)
	}
	var p planResp
	json.Unmarshal(b, &p)
	if p.Plan.Action != "timer.delete" || len(p.Plan.Warnings) != 0 {
		t.Fatalf("expected a clean timer.delete plan, got %s", b)
	}
	if n := e.timerCount(t); n != 2 {
		t.Fatal("a preview must not change anything")
	}
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: expected 200, got %d: %s", resp.StatusCode, b)
	}
	if n := e.timerCount(t); n != 1 {
		t.Fatalf("expected the timer to be gone, got %d timers", n)
	}
	_, b = e.do(t, "GET", "/api/v1/changes", "", nil)
	if !strings.Contains(string(b), "timer.delete") {
		t.Error("the deletion must be in the change log")
	}
}

func TestTimerDelete_UnknownTimer(t *testing.T) {
	e := newEnv(t)
	// Same timer, end off by one second: the box matches exactly, so must we.
	resp, b := e.do(t, "POST", "/api/v1/timers/delete/plan", deleteBody(rtlRef, ev(20, 15), ev(21, 15)+1), nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusNotFound || code != "timer_not_found" {
		t.Fatalf("expected 404 timer_not_found, got %d %s", resp.StatusCode, b)
	}
}

// recordingBox reports the first fixture timer as recording right now.
type recordingBox struct{ *mock.Client }

func (b recordingBox) TimerList(ctx context.Context) ([]webcontrol.Timer, error) {
	timers, err := b.Client.TimerList(ctx)
	if len(timers) > 0 {
		timers[0].State = webcontrol.TimerRunning
	}
	return timers, err
}

func TestTimerDelete_RunningTimerWarnsItStopsTheRecording(t *testing.T) {
	e := newEnvWith(t, recordingBox{mock.New()})
	_, b := e.do(t, "POST", "/api/v1/timers/delete/plan", deleteBody(rtlRef, ev(20, 15), ev(21, 15)), nil)
	var p planResp
	json.Unmarshal(b, &p)
	if len(p.Plan.Warnings) != 1 || p.Plan.Warnings[0].Code != "timer.deleteRunning" {
		t.Fatalf("deleting a running timer aborts the recording; the preview must say so, got %s", b)
	}
}

func TestTimerPlan_DuplicateDroppedByTheBoxIsReported(t *testing.T) {
	e := newEnv(t)
	// Same channel, same recording start as the existing RTL timer (20:20
	// minus 5 min lead-in = 20:15), different end. The box drops this
	// silently and still says "added" (ARCHITECTURE.md).
	p := e.planTimer(t, timerBodyRef(rtlRef, "Bauer sucht Frau (again)", ev(20, 20), ev(21, 30), 5, 0))
	if len(p.Plan.Warnings) == 0 || p.Plan.Warnings[0].Code != "timer.duplicate" {
		t.Fatalf("expected the box's duplicate rule (same channel + begin) to warn, got %+v", p.Plan.Warnings)
	}
	resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "timer_not_created" {
		t.Fatalf("a timer the box dropped must not be reported as created: got %d %s", resp.StatusCode, b)
	}
	_, b = e.do(t, "GET", "/api/v1/changes", "", nil)
	if strings.Contains(string(b), "timer.create") {
		t.Error("nothing may be logged when the change didn't happen")
	}
}

func TestTimerPlan_SameTransponderIsNoConflict(t *testing.T) {
	e := newEnv(t)
	// VOX shares RTL's transponder: the single tuner records both.
	vox := "1:0:19:2F1C:441:1:C00000:0:0:0:"
	p := e.planTimer(t, timerBodyRef(vox, "Das perfekte Dinner", ev(20, 30), ev(21, 0), 0, 0))
	if len(p.Plan.Warnings) != 0 {
		t.Fatalf("same transponder must not be a conflict, got %+v", p.Plan.Warnings)
	}
}

type rejectingBox struct{ *mock.Client }

func (rejectingBox) AddTimer(context.Context, webcontrol.NewTimer) error {
	return &webcontrol.RejectedError{Op: "timeradd", Text: "Timer-Konflikt erkannt! / The Rookie"}
}

func TestTimerPlan_BoxRefusalIsShownWithItsText(t *testing.T) {
	e := newEnvWith(t, rejectingBox{mock.New()})
	p := e.planTimer(t, timerBody("Die Kanzlei (S01/E07)", ev(21, 45), ev(22, 30), 0, 0))
	resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), `"box_rejected"`) || !strings.Contains(string(b), "The Rookie") {
		t.Fatalf("expected 409 box_rejected carrying the box's text, got %d %s", resp.StatusCode, b)
	}
}

func TestTimers_ListMarksConflictsAndSource(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/timers", "", nil)
	var body struct {
		Timers []struct {
			Name     string `json:"name"`
			Conflict bool   `json:"conflict"`
			Source   string `json:"source"`
		} `json:"timers"`
	}
	json.Unmarshal(b, &body)
	// The mock's two timers overlap 21:00–21:15 on different transponders
	// of a single-tuner box: both are marked.
	if len(body.Timers) != 2 || !body.Timers[0].Conflict || !body.Timers[1].Conflict {
		t.Fatalf("expected both overlapping fixture timers marked as conflicting, got %s", b)
	}
}

func TestAutoTimers_InstalledAndNot(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/timers/autotimers", "", nil)
	if !strings.Contains(string(b), `"installed":true`) || !strings.Contains(string(b), `"Tatort"`) || !strings.Contains(string(b), `/autotimereditor/`) {
		t.Fatalf("expected the mock's AutoTimer and the editor link, got %s", b)
	}
	if resp, b := e.do(t, "GET", "/api/v1/timers/autotimers/simulate", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("simulate: expected 200, got %d %s", resp.StatusCode, b)
	}

	// No plugin on the box: a normal answer, TimerHub stays available.
	e = newEnvWithAutoTimer(t, mock.New(), &atmock.Client{NotInstalled: true})
	resp, b := e.do(t, "GET", "/api/v1/timers/autotimers", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"installed":false`) {
		t.Fatalf("a missing AutoTimer plugin must be a plain installed:false, got %d %s", resp.StatusCode, b)
	}
	_, b = e.do(t, "GET", "/api/v1/hubs", "", nil)
	var hubs struct {
		Hubs []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"hubs"`
	}
	json.Unmarshal(b, &hubs)
	for _, h := range hubs.Hubs {
		if h.ID == "timer" && h.Status != "available" {
			t.Errorf("TimerHub must stay available without the optional AutoTimer plugin, got %s", h.Status)
		}
	}
	_, b = e.do(t, "GET", "/api/v1/connectors", "", nil)
	if !strings.Contains(string(b), `"not_installed"`) {
		t.Errorf("the AutoTimer connector should report not_installed, got %s", b)
	}
}

// Every mutating route goes through the same CSRF check — including the
// read-only POSTs (views, enrichment lookups), which are harmless but must
// not be the one door a foreign page could knock on.
func TestCSRF_EveryMutatingRoute(t *testing.T) {
	e := newEnv(t)
	noHeader := func(r *http.Request) { r.Header.Del(httpx.CSRFHeader) }
	foreign := func(r *http.Request) { r.Header.Set("Origin", "http://192.168.1.99:8080") }
	routes := [][2]string{
		{"POST", "/api/v1/control/key"},
		{"POST", "/api/v1/control/zap"},
		{"PUT", "/api/v1/dashboard"},
		{"POST", "/api/v1/dashboard/view"},
		{"POST", "/api/v1/enrich/lookup"},
		{"POST", "/api/v1/media/rename/plan"},
		{"POST", "/api/v1/media/trash/plan"},
		{"POST", "/api/v1/media/rename/suggest"},
		{"POST", "/api/v1/discovery/recommendations"},
		{"POST", "/api/v1/timers/delete/plan"},
		{"POST", "/api/v1/tokens"},
		{"DELETE", "/api/v1/tokens/1"},
		{"POST", "/api/v1/plans/x/apply"},
		{"POST", "/api/v1/notifications/read"},
		{"PUT", "/api/v1/admin/log-level"},
	}
	for _, rt := range routes {
		for _, configure := range []func(*http.Request){noHeader, foreign} {
			resp, b := e.do(t, rt[0], rt[1], `{}`, configure)
			if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusForbidden || code != "csrf_failed" {
				t.Errorf("%s %s: expected 403 csrf_failed, got %d %s", rt[0], rt[1], resp.StatusCode, b)
			}
		}
	}
}

func toggleBody(ref string, begin, end int64, enabled bool) string {
	return fmt.Sprintf(`{"serviceReference":%q,"begin":%d,"end":%d,"enabled":%t}`, ref, begin, end, enabled)
}

func TestTimerToggle_DisableThenEnable(t *testing.T) {
	e := newEnv(t)
	timerDisabled := func() bool {
		_, b := e.do(t, "GET", "/api/v1/timers", "", nil)
		var out struct {
			Timers []struct {
				Name     string `json:"name"`
				Disabled bool   `json:"disabled"`
			} `json:"timers"`
		}
		json.Unmarshal(b, &out)
		for _, tm := range out.Timers {
			if strings.HasPrefix(tm.Name, "Bauer") {
				return tm.Disabled
			}
		}
		t.Fatalf("timer gone: %s", b)
		return false
	}
	for _, enabled := range []bool{false, true} {
		resp, b := e.do(t, "POST", "/api/v1/timers/toggle/plan", toggleBody(rtlRef, ev(20, 15), ev(21, 15), enabled), nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("plan (enabled=%v): %d %s", enabled, resp.StatusCode, b)
		}
		var p planResp
		json.Unmarshal(b, &p)
		if timerDisabled() != enabled { // still the opposite of the target
			t.Fatal("a preview must not change anything")
		}
		if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != http.StatusOK {
			t.Fatalf("apply: %d %s", resp.StatusCode, b)
		}
		if timerDisabled() != !enabled {
			t.Fatalf("after apply, disabled should be %v", !enabled)
		}
		if n := e.timerCount(t); n != 2 {
			t.Fatalf("taken out and put back, still two timers: %d", n)
		}
	}
	if resp, _ := e.do(t, "POST", "/api/v1/timers/toggle/plan", toggleBody(rtlRef, ev(20, 15), ev(21, 15), true), nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("already enabled: %d", resp.StatusCode)
	}
}
