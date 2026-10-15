package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type packageList struct {
	Packages []struct {
		Name       string   `json:"name"`
		Category   string   `json:"category"`
		Upgradable bool     `json:"upgradable"`
		NeededBy   []string `json:"neededBy"`
		Info       struct {
			Description   string   `json:"description"`
			Category      string   `json:"category"`
			CategoryLabel string   `json:"categoryLabel"`
			NeededBy      []string `json:"neededBy"`
			Source        string   `json:"source"`
		} `json:"info"`
	} `json:"packages"`
	Categories []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	} `json:"categories"`
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

// The "needed" view: what a hub uses, installed or not — the mock has
// neither installed, both in the feed.
func TestAdminPackages_Needed(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/admin/packages?status=needed", "", nil)
	var v struct {
		Packages []struct {
			Name     string   `json:"name"`
			Version  string   `json:"version"`
			NeededBy []string `json:"neededBy"`
			Missing  bool     `json:"missing"`
		} `json:"packages"`
	}
	json.Unmarshal(b, &v)
	if resp.StatusCode != http.StatusOK || len(v.Packages) != 2 {
		t.Fatalf("needed: %d %s", resp.StatusCode, b)
	}
	byName := map[string]int{}
	for i, p := range v.Packages {
		byName[p.Name] = i
	}
	ff, ov := v.Packages[byName["ffmpeg"]], v.Packages[byName["kernel-module-overlay"]]
	if !ff.Missing || ff.Version != "" || len(ff.NeededBy) != 1 || ff.NeededBy[0] != "media" {
		t.Errorf("ffmpeg: %+v", ff)
	}
	if !ov.Missing || len(ov.NeededBy) != 1 || ov.NeededBy[0] != "apps" {
		t.Errorf("overlay: %+v", ov)
	}

	// The other views mark it too.
	_, b = e.do(t, "GET", "/api/v1/admin/packages?status=available&q=ffmpeg", "", nil)
	json.Unmarshal(b, &v)
	if len(v.Packages) != 1 || len(v.Packages[0].NeededBy) != 1 {
		t.Errorf("list: %s", b)
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

// The three sources of a description: curated, the package's own, none.
func TestAdminPackages_Info(t *testing.T) {
	e := newEnv(t)
	get := func(q string) packageList {
		t.Helper()
		resp, b := e.do(t, "GET", "/api/v1/admin/packages?"+q, "", nil)
		var l packageList
		json.Unmarshal(b, &l)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d %s", q, resp.StatusCode, b)
		}
		return l
	}

	l := get("status=available&q=ffmpeg&lang=de")
	if len(l.Packages) != 1 {
		t.Fatalf("ffmpeg: %+v", l)
	}
	ff := l.Packages[0]
	if ff.Info.Source != "curated" || !strings.Contains(ff.Info.Description, "Video") || ff.Info.Category != "system" || ff.Info.CategoryLabel != "System" {
		t.Errorf("curated: %+v", ff.Info)
	}
	if len(ff.NeededBy) != 1 || ff.NeededBy[0] != "media" || len(ff.Info.NeededBy) != 1 {
		t.Errorf("neededBy: %+v", ff)
	}
	if en := get("status=available&q=ffmpeg&lang=en").Packages[0].Info.Description; en == ff.Info.Description {
		t.Error("the language should change the curated text")
	}

	// No entry: the package's own description, the category from its name.
	l = get("status=available&q=dvb-usb-extra&lang=it")
	if len(l.Packages) != 1 || l.Packages[0].Info.Source != "package" || l.Packages[0].Info.Description != "Extra DVB USB tuner drivers" ||
		l.Packages[0].Info.Category != "drivers" || l.Packages[0].Info.CategoryLabel != "Driver" {
		t.Errorf("package source: %+v", l.Packages)
	}

	// Nothing known at all.
	l = get("q=libfoo-internal")
	if len(l.Packages) != 1 || l.Packages[0].Info.Source != "none" || l.Packages[0].Info.Description != "" {
		t.Errorf("none source: %+v", l.Packages)
	}

	// Category chips: stable ids with labels, only those the view holds;
	// the filter takes the id.
	l = get("status=available&lang=de")
	ids := map[string]string{}
	for _, c := range l.Categories {
		ids[c.ID] = c.Label
	}
	if ids["drivers"] != "Treiber" || ids["extensions"] != "Erweiterungen" || ids["skins"] != "" {
		t.Errorf("categories: %+v", l.Categories)
	}
	l = get("status=available&category=drivers")
	if len(l.Packages) != 1 || l.Packages[0].Name != "enigma2-plugin-drivers-dvb-usb-extra" {
		t.Errorf("category filter: %+v", l.Packages)
	}
	// The description is searchable, the curated text included.
	if l = get("status=available&q=convert"); len(l.Packages) != 1 || l.Packages[0].Name != "ffmpeg" {
		t.Errorf("search in the curated text: %+v", l.Packages)
	}
}
