package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func (e *env) feedPlan(t *testing.T, body string) (int, string, []string) {
	t.Helper()
	resp, b := e.do(t, "POST", "/api/v1/admin/feeds/plan", body, nil)
	var p struct {
		Plan struct {
			ID       string `json:"id"`
			Warnings []struct {
				Code string `json:"code"`
			} `json:"warnings"`
		} `json:"plan"`
	}
	json.Unmarshal(b, &p)
	var codes []string
	for _, w := range p.Plan.Warnings {
		codes = append(codes, w.Code)
	}
	return resp.StatusCode, p.Plan.ID, codes
}

func TestAdminFeeds_SwitchOffAndOn(t *testing.T) {
	e := newEnv(t)
	code, id, _ := e.feedPlan(t, `{"action":"disable","name":"svenh-feed"}`)
	if code != http.StatusOK {
		t.Fatalf("plan: %d", code)
	}
	if _, b := e.do(t, "GET", "/api/v1/admin/feeds", "", nil); strings.Contains(string(b), `"name":"svenh-feed","url":"http://svenh-feeds.de/feeds/dreambox-sh4/unstable","enabled":false`) {
		t.Fatal("a preview switched the feed off")
	}
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+id+"/apply", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	if _, b := e.do(t, "GET", "/api/v1/admin/feeds", "", nil); !strings.Contains(string(b), `"name":"svenh-feed","url":"http://svenh-feeds.de/feeds/dreambox-sh4/unstable","enabled":false`) {
		t.Fatalf("not switched off: %s", b)
	}
	// Already off: nothing to do.
	if code, _, _ := e.feedPlan(t, `{"action":"disable","name":"svenh-feed"}`); code != http.StatusConflict {
		t.Fatalf("second disable: %d", code)
	}
}

func TestAdminFeeds_AddWarnsAndRefusesBadInput(t *testing.T) {
	e := newEnv(t)
	code, id, warnings := e.feedPlan(t, `{"action":"add","name":"my-feed","url":"https://example.org/feed"}`)
	if code != http.StatusOK || len(warnings) != 1 || warnings[0] != "feed.unsigned" {
		t.Fatalf("plan: %d %v", code, warnings)
	}
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+id+"/apply", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	if _, b := e.do(t, "GET", "/api/v1/admin/feeds", "", nil); !strings.Contains(string(b), `"my-feed"`) {
		t.Fatalf("not added: %s", b)
	}
	for _, body := range []string{
		`{"action":"add","name":"../x","url":"https://a.b"}`,
		`{"action":"add","name":"ok","url":"file:///etc/passwd"}`,
		`{"action":"add","name":"my-feed","url":"https://a.b"}`,
	} {
		if code, _, _ := e.feedPlan(t, body); code != http.StatusBadRequest && code != http.StatusConflict {
			t.Errorf("%s: %d", body, code)
		}
	}
}
