package api_test

import (
	"context"
	"encoding/json"
	"fmt"
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
