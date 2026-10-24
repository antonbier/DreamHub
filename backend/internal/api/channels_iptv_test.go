package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

const iptvList = "#EXTM3U\n" +
	"#EXTINF:-1 tvg-id=\"one.de\" tvg-logo=\"http://logos.example/one.png\" group-title=\"News\",One News\n" +
	"http://tv.example:8080/live/one.m3u8\n" +
	"#EXTINF:-1 group-title=\"News\",Two News\n" +
	"https://tv.example/live/two.m3u8?token=s3cret\n" +
	"#EXTINF:-1 group-title=\"Kids\",Toon\n" +
	"rtsp://user:hunter2@cam.example:554/toon\n" +
	"#EXTINF:-1 group-title=\"Kids\",Toon again\n" +
	"rtsp://user:hunter2@cam.example:554/toon\n" +
	"#EXTINF:-1,A file\n" +
	"file:///tmp/a.ts\n"

type iptvRead struct {
	Token    string `json:"token"`
	Name     string `json:"name"`
	Source   string `json:"source"`
	FromFile bool   `json:"fromFile"`
	Channels []struct {
		ID        int    `json:"id"`
		Name      string `json:"name"`
		Group     string `json:"group"`
		Host      string `json:"host"`
		HasLogin  bool   `json:"hasLogin"`
		HasLogo   bool   `json:"hasLogo"`
		Duplicate bool   `json:"duplicate"`
	} `json:"channels"`
	Groups []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	} `json:"groups"`
	Skipped []struct {
		Reason string `json:"reason"`
		Count  int    `json:"count"`
	} `json:"skipped"`
}

func (e *env) serveList(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
	t.Cleanup(srv.Close)
	return srv.URL + "/get.php?username=me&password=hunter2"
}

func TestIPTV_ReadHidesTheAddresses(t *testing.T) {
	e := newEnv(t)
	url := e.serveList(t, iptvList)
	resp, raw := e.do(t, "POST", "/api/v1/channels/iptv/read", `{"url":`+jsonStr(url)+`}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("read: %d %s", resp.StatusCode, raw)
	}
	for _, leak := range []string{"hunter2", "s3cret", "user:", "password", "username"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the answer holds %q: %s", leak, raw)
		}
	}
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/read", `{"url":`+jsonStr(url)+`}`, nil, 200, &r)
	if r.Token == "" || len(r.Channels) != 4 || len(r.Groups) != 2 {
		t.Fatalf("read: %+v", r)
	}
	if !r.Channels[0].HasLogo || r.Channels[0].HasLogin || !r.Channels[1].HasLogin || !r.Channels[2].HasLogin {
		t.Errorf("flags: %+v", r.Channels)
	}
	if !r.Channels[3].Duplicate || r.Channels[2].Duplicate {
		t.Errorf("the second Toon is the first one's address again: %+v", r.Channels)
	}
	if len(r.Skipped) != 1 || r.Skipped[0].Reason != "scheme" {
		t.Errorf("skipped: %+v", r.Skipped)
	}
	if strings.Contains(r.Source, "hunter2") || strings.Contains(r.Source, "?") {
		t.Errorf("source: %q", r.Source)
	}
}

func TestIPTV_ReadErrors(t *testing.T) {
	e := newEnv(t)
	html := e.serveList(t, "<html>Please log in</html>")
	for body, want := range map[string]string{
		`{"url":"ftp://h.example/a.m3u"}`:    "iptv_invalid_url",
		`{"url":` + jsonStr(html) + `}`:      "iptv_not_playlist",
		`{"url":"http://127.0.0.1:1/a.m3u"}`: "iptv_unreachable",
		`{}`:                                 "invalid_body",
	} {
		resp, b := e.do(t, "POST", "/api/v1/channels/iptv/read", body, nil)
		if resp.StatusCode < 400 || !strings.Contains(string(b), want) {
			t.Errorf("%s: %d %s, want %s", body, resp.StatusCode, b, want)
		}
	}
}

func TestIPTV_UploadAFile(t *testing.T) {
	e := newEnv(t)
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/upload?name=mine.m3u", iptvList, nil, 200, &r)
	if !r.FromFile || r.Name != "mine.m3u" || len(r.Channels) != 4 || r.Source != "" {
		t.Fatalf("upload: %+v", r)
	}
	if resp, b := e.do(t, "POST", "/api/v1/channels/iptv/upload", "not a playlist", nil); resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(b), "iptv_not_playlist") {
		t.Errorf("not a playlist: %d %s", resp.StatusCode, b)
	}
}

func (e *env) readList(t *testing.T) iptvRead {
	t.Helper()
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/upload?name=l.m3u", iptvList, nil, 200, &r)
	return r
}

func TestIPTV_NewBouquetAsOnePlan(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	r := e.readList(t)
	body := `{"token":` + jsonStr(r.Token) + `,"ids":[0,1,2,3],"newName":"Internet","kind":"tv"}`
	var p struct {
		Plan struct {
			ID      string `json:"id"`
			Summary string `json:"summary"`
			Changes []struct {
				Type string `json:"type"`
			} `json:"changes"`
		} `json:"plan"`
		Skipped []struct {
			Reason string `json:"reason"`
		} `json:"skipped"`
	}
	resp, raw := e.do(t, "POST", "/api/v1/channels/iptv/plan", body, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("plan: %d %s", resp.StatusCode, raw)
	}
	for _, leak := range []string{"hunter2", "s3cret", "cam.example", "tv.example"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the preview holds %q: %s", leak, raw)
		}
	}
	e.json(t, "POST", "/api/v1/channels/iptv/plan", body, nil, 200, &p)
	types := map[string]int{}
	for _, c := range p.Plan.Changes {
		types[c.Type]++
	}
	// the second Toon has the first one's address and is added only once
	if types["bouquet.create"] != 1 || types["channel.add"] != 3 {
		t.Fatalf("preview: %+v", types)
	}
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != 200 {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	var list struct {
		Bouquets []struct {
			Ref  string `json:"ref"`
			Name string `json:"name"`
		} `json:"bouquets"`
	}
	e.json(t, "GET", "/api/v1/channels/bouquets", "", nil, 200, &list)
	ref := ""
	for _, b := range list.Bouquets {
		if b.Name == "Internet (TV)" {
			ref = b.Ref
		}
	}
	if ref == "" {
		t.Fatalf("the bouquet is missing: %+v", list)
	}
	got := e.entries(t, ref)
	if len(got.Entries) != 3 || got.Entries[0].Name != "One News" ||
		got.Entries[0].Ref != "4097:0:1:0:0:0:0:0:0:0:http%3a//tv.example%3a8080/live/one.m3u8:One News" {
		t.Fatalf("entries: %+v", got.Entries)
	}
	if !strings.Contains(got.Entries[2].Ref, "rtsp%3a//user%3ahunter2@cam.example%3a554/toon") {
		t.Errorf("the address, with its login, reaches the box: %q", got.Entries[2].Ref)
	}

	// The same name again is refused; adding to the bouquet leaves out what is in it.
	if resp, b := e.do(t, "POST", "/api/v1/channels/iptv/plan", body, nil); resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "iptv_bouquet_exists") {
		t.Errorf("same name: %d %s", resp.StatusCode, b)
	}
	again := `{"token":` + jsonStr(r.Token) + `,"ids":[0,1],"bouquet":` + jsonStr(ref) + `}`
	if resp, b := e.do(t, "POST", "/api/v1/channels/iptv/plan", again, nil); resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "iptv_nothing_to_add") || !strings.Contains(string(b), "in_bouquet") {
		t.Errorf("nothing to add: %d %s", resp.StatusCode, b)
	}
	// A backup was made before the change.
	var backups struct {
		Backups []struct{ Filename string } `json:"backups"`
	}
	e.json(t, "GET", "/api/v1/channels/backups", "", nil, 200, &backups)
	if len(backups.Backups) == 0 {
		t.Error("no backup recorded")
	}
}

func TestIPTV_AddToAnExistingBouquet(t *testing.T) {
	e := newEnvWith(t, mock.New())
	before := e.entries(t, freeTV)
	r := e.readList(t)
	body := `{"token":` + jsonStr(r.Token) + `,"ids":[1],"bouquet":` + jsonStr(freeTV) + `}`
	e.planAndApply(t, "/api/v1/channels/iptv/plan", body, 200)
	after := e.entries(t, freeTV)
	if len(after.Entries) != len(before.Entries)+1 || after.Entries[len(after.Entries)-1].Name != "Two News" {
		t.Fatalf("after: %+v", after.Entries)
	}
	for i, x := range before.Entries {
		if after.Entries[i].Ref != x.Ref {
			t.Errorf("entry %d changed", i)
		}
	}
}

func TestIPTV_FailedStepRestoresTheBackup(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	before := e.entries(t, freeTV)
	r := e.readList(t)
	m.FailEditor = "addservicetobouquet"
	body := `{"token":` + jsonStr(r.Token) + `,"ids":[0,1],"bouquet":` + jsonStr(freeTV) + `}`
	_, b := e.planAndApply(t, "/api/v1/channels/iptv/plan", body, http.StatusConflict)
	if !strings.Contains(string(b), "channels_restored") || m.Restores != 1 {
		t.Fatalf("expected a restore: %s (restores %d)", b, m.Restores)
	}
	if len(e.entries(t, freeTV).Entries) != len(before.Entries) {
		t.Error("the bouquet wasn't restored")
	}
}

func TestIPTV_PlanValidation(t *testing.T) {
	e := newEnv(t)
	r := e.readList(t)
	tok := jsonStr(r.Token)
	for name, c := range map[string]struct {
		body   string
		status int
		code   string
	}{
		"unknown token":   {`{"token":"nope","ids":[0],"newName":"X","kind":"tv"}`, http.StatusGone, "iptv_session_gone"},
		"no target":       {`{"token":` + tok + `,"ids":[0]}`, 400, "validation_failed"},
		"two targets":     {`{"token":` + tok + `,"ids":[0],"newName":"X","kind":"tv","bouquet":"y"}`, 400, "validation_failed"},
		"no ids":          {`{"token":` + tok + `,"ids":[],"newName":"X","kind":"tv"}`, 400, "validation_failed"},
		"id out of range": {`{"token":` + tok + `,"ids":[99],"newName":"X","kind":"tv"}`, 400, "validation_failed"},
		"bad kind":        {`{"token":` + tok + `,"ids":[0],"newName":"X","kind":"video"}`, 400, "validation_failed"},
		"no bouquet":      {`{"token":` + tok + `,"ids":[0],"bouquet":"1:7:nope"}`, 400, "validation_failed"},
	} {
		resp, b := e.do(t, "POST", "/api/v1/channels/iptv/plan", c.body, nil)
		if resp.StatusCode != c.status || !strings.Contains(string(b), c.code) {
			t.Errorf("%s: %d %s", name, resp.StatusCode, b)
		}
	}
}

func TestIPTV_NeedsTheBouquetEditor(t *testing.T) {
	m := mock.New()
	m.NoBouquetEditor = true
	e := newEnvWith(t, m)
	r := e.readList(t)
	resp, b := e.do(t, "POST", "/api/v1/channels/iptv/plan", `{"token":`+jsonStr(r.Token)+`,"ids":[0],"newName":"X","kind":"tv"}`, nil)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "channels_no_editor") {
		t.Fatalf("no editor: %d %s", resp.StatusCode, b)
	}
}

func TestIPTV_AdminOnly(t *testing.T) {
	e := newEnv(t)
	_, viewer, _ := e.household(t)
	for _, path := range []string{"read", "upload", "plan"} {
		if resp, _ := e.do(t, "POST", "/api/v1/channels/iptv/"+path, "{}", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s as a viewer: %d", path, resp.StatusCode)
		}
	}
}
