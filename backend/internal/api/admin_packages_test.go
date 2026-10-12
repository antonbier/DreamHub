package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

type packageList struct {
	Packages []struct {
		Name       string `json:"name"`
		Category   string `json:"category"`
		Upgradable bool   `json:"upgradable"`
	} `json:"packages"`
	Categories []string `json:"categories"`
}

func TestAdminPackages_ListAndFilter(t *testing.T) {
	e := newEnv(t)

	resp, b := e.do(t, "GET", "/api/v1/admin/packages", "", nil)
	var list packageList
	json.Unmarshal(b, &list)
	if resp.StatusCode != http.StatusOK || len(list.Packages) == 0 {
		t.Fatalf("list: %d %+v", resp.StatusCode, list)
	}

	resp, b = e.do(t, "GET", "/api/v1/admin/packages?status=upgradable", "", nil)
	json.Unmarshal(b, &list)
	if resp.StatusCode != http.StatusOK || len(list.Packages) == 0 {
		t.Fatalf("upgradable filter: %d %+v", resp.StatusCode, list)
	}
	for _, p := range list.Packages {
		if !p.Upgradable {
			t.Errorf("status=upgradable returned a non-upgradable package: %+v", p)
		}
	}

	resp, b = e.do(t, "GET", "/api/v1/admin/packages?q=does-not-exist-xyz", "", nil)
	json.Unmarshal(b, &list)
	if resp.StatusCode != http.StatusOK || len(list.Packages) != 0 {
		t.Fatalf("q filter: %d %+v", resp.StatusCode, list)
	}
}

func TestAdminFeeds_List(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/admin/feeds", "", nil)
	var v struct {
		Feeds []struct {
			Name  string `json:"name"`
			Known bool   `json:"known"`
		} `json:"feeds"`
	}
	json.Unmarshal(b, &v)
	if resp.StatusCode != http.StatusOK || len(v.Feeds) == 0 {
		t.Fatalf("feeds: %d %+v", resp.StatusCode, v)
	}
	sawUnknown := false
	for _, f := range v.Feeds {
		if !f.Known {
			sawUnknown = true
		}
	}
	if !sawUnknown {
		t.Error("expected at least one feed flagged as added by hand")
	}
}

func TestAdminPackages_ExplainNeedsAnAPIKey(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "POST", "/api/v1/admin/packages/explain", `{"name":"dropbear","description":"Lightweight SSH server"}`, nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "ai_not_configured" {
		t.Fatalf("expected 409 ai_not_configured, got %d %s", resp.StatusCode, b)
	}
}

func TestAdminPackages_ExplainReturnsAnExplanation(t *testing.T) {
	e := newEnv(t)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"ai.openai.apiKey":"sk-test-1234"}}`, nil)

	resp, b := e.do(t, "POST", "/api/v1/admin/packages/explain", `{"name":"dropbear","description":"Lightweight SSH server"}`, nil)
	var v struct {
		Explanation string `json:"explanation"`
	}
	json.Unmarshal(b, &v)
	if resp.StatusCode != http.StatusOK || v.Explanation == "" {
		t.Fatalf("explain: %d %+v", resp.StatusCode, v)
	}
}
