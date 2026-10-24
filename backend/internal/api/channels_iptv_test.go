package api_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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

// --- remembering a list and comparing it again ---------------------------------------

type liveList struct {
	mu   sync.Mutex
	text string
}

func (l *liveList) set(s string) { l.mu.Lock(); l.text = s; l.mu.Unlock() }

func (e *env) serveLiveList(t *testing.T, l *liveList) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		defer l.mu.Unlock()
		w.Write([]byte(l.text))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/get.php?username=me&password=hunter2"
}

const listV1 = "#EXTM3U\n" +
	"#EXTINF:-1 tvg-id=\"a\" group-title=\"News\",Alpha\nhttp://tv.example/a1.m3u8\n" +
	"#EXTINF:-1 tvg-id=\"b\" group-title=\"News\",Beta\nhttp://tv.example/b1.m3u8\n" +
	"#EXTINF:-1 tvg-id=\"c\" group-title=\"News\",Gamma\nhttp://tv.example/c1.m3u8\n" +
	"#EXTINF:-1 group-title=\"Kids\",Delta\nhttp://tv.example/d1.m3u8\n"

// Alpha's address changed, Beta is gone, Gamma and Delta as before, two new.
const listV2 = "#EXTM3U\n" +
	"#EXTINF:-1 tvg-id=\"a\" group-title=\"News\",Alpha HD\nhttp://tv.example/a2.m3u8\n" +
	"#EXTINF:-1 tvg-id=\"c\" group-title=\"News\",Gamma\nhttp://tv.example/c1.m3u8\n" +
	"#EXTINF:-1 group-title=\"Kids\",Delta\nhttp://tv.example/d1.m3u8\n" +
	"#EXTINF:-1 tvg-id=\"e\" group-title=\"News\",Epsilon\nhttp://tv.example/e1.m3u8\n" +
	"#EXTINF:-1 tvg-id=\"f\" group-title=\"News\",Zeta\nhttp://tv.example/f1.m3u8\n"

type iptvSources struct {
	Sources []struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Source     string `json:"source"`
		Bouquet    string `json:"bouquet"`
		BouquetRef string `json:"bouquetRef"`
		Gone       bool   `json:"gone"`
		ComparedAt string `json:"comparedAt"`
	} `json:"sources"`
}

type iptvCompared struct {
	Token string `json:"token"`
	Items []struct {
		ID      int    `json:"id"`
		Action  string `json:"action"`
		Name    string `json:"name"`
		OldName string `json:"oldName"`
	} `json:"items"`
	Unchanged int `json:"unchanged"`
}

func (c iptvCompared) ids(actions ...string) []int {
	var out []int
	for _, it := range c.Items {
		for _, a := range actions {
			if it.Action == a {
				out = append(out, it.ID)
			}
		}
	}
	return out
}

func idsJSON(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestIPTV_RememberAndCompare(t *testing.T) {
	e := newEnvWith(t, mock.New())
	live := &liveList{text: listV1}
	addr := e.serveLiveList(t, live)

	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/read", `{"url":`+jsonStr(addr)+`}`, nil, 200, &r)
	e.planAndApply(t, "/api/v1/channels/iptv/plan", `{"token":`+jsonStr(r.Token)+`,"ids":[0,1,2],"newName":"Net","kind":"tv","remember":true}`, 200)

	// remembered, with no address in sight
	resp, raw := e.do(t, "GET", "/api/v1/channels/iptv/sources", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("sources: %d %s", resp.StatusCode, raw)
	}
	for _, leak := range []string{"hunter2", "password", "username"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the list of sources holds %q: %s", leak, raw)
		}
	}
	var srcs iptvSources
	e.json(t, "GET", "/api/v1/channels/iptv/sources", "", nil, 200, &srcs)
	if len(srcs.Sources) != 1 || srcs.Sources[0].Bouquet != "Net (TV)" || srcs.Sources[0].Gone || srcs.Sources[0].ComparedAt == "" {
		t.Fatalf("sources: %+v", srcs)
	}
	src := srcs.Sources[0]
	base := "/api/v1/channels/iptv/sources/" + strconv.FormatInt(src.ID, 10)

	// nothing changed in the list: nothing to do
	var same iptvCompared
	e.json(t, "POST", base+"/compare", "", nil, 200, &same)
	if len(same.Items) != 0 || same.Unchanged != 3 {
		t.Fatalf("same list: %+v", same)
	}

	live.set(listV2)
	var c iptvCompared
	resp, raw = e.do(t, "POST", base+"/compare", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("compare: %d %s", resp.StatusCode, raw)
	}
	for _, leak := range []string{"hunter2", "a2.m3u8"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the comparison holds %q: %s", leak, raw)
		}
	}
	e.json(t, "POST", base+"/compare", "", nil, 200, &c)
	got := map[string]string{}
	for _, it := range c.Items {
		got[it.Name] = it.Action
	}
	// Alpha's new address (ticked by the screen), Beta gone, Epsilon and Zeta
	// new; Delta was never picked but is no news; Gamma is unchanged.
	if len(c.Items) != 4 || got["Alpha HD"] != "update" || got["Beta"] != "gone" || got["Epsilon"] != "new" || got["Zeta"] != "new" || c.Unchanged != 1 {
		t.Fatalf("compare: %+v", c)
	}
	if c.Items[0].OldName != "Alpha" {
		t.Errorf("the bouquet's own name is kept: %+v", c.Items[0])
	}

	// Carry out the update, the removal and Epsilon; leave Zeta.
	var epsilon int
	for _, it := range c.Items {
		if it.Name == "Epsilon" {
			epsilon = it.ID
		}
	}
	ids := append(c.ids("update", "gone"), epsilon)
	var p struct {
		Plan struct {
			Changes []struct {
				Type string `json:"type"`
			} `json:"changes"`
		} `json:"plan"`
	}
	body := `{"token":` + jsonStr(c.Token) + `,"ids":` + idsJSON(ids) + `}`
	e.json(t, "POST", base+"/plan", body, nil, 200, &p)
	types := map[string]int{}
	for _, ch := range p.Plan.Changes {
		types[ch.Type]++
	}
	if types["channel.update"] != 1 || types["channel.remove"] != 1 || types["channel.add"] != 1 || len(p.Plan.Changes) != 3 {
		t.Fatalf("preview: %+v", types)
	}
	e.planAndApply(t, base+"/plan", body, 200)
	after := e.entries(t, src.BouquetRef)
	var names []string
	for _, x := range after.Entries {
		names = append(names, x.Name)
	}
	if strings.Join(names, "|") != "Alpha|Gamma|Epsilon" {
		t.Fatalf("bouquet after: %v", names)
	}
	if !strings.Contains(after.Entries[0].Ref, "a2.m3u8") || strings.Contains(after.Entries[0].Ref, "a1.m3u8") {
		t.Errorf("Alpha has its new address in the same place: %q", after.Entries[0].Ref)
	}

	// Zeta was not taken, so it is offered again; nothing else is.
	var again iptvCompared
	e.json(t, "POST", base+"/compare", "", nil, 200, &again)
	if len(again.Items) != 1 || again.Items[0].Name != "Zeta" || again.Items[0].Action != "new" || again.Unchanged != 3 {
		t.Fatalf("second comparison: %+v", again)
	}

	// Forgetting a list leaves the bouquet alone.
	if resp, _ := e.do(t, "DELETE", base, "", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("forget: %d", resp.StatusCode)
	}
	e.json(t, "GET", "/api/v1/channels/iptv/sources", "", nil, 200, &srcs)
	if len(srcs.Sources) != 0 || len(e.entries(t, src.BouquetRef).Entries) != 3 {
		t.Fatalf("after forgetting: %+v", srcs)
	}
	if resp, _ := e.do(t, "POST", base+"/compare", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("compare after forgetting: %d", resp.StatusCode)
	}
}

func TestIPTV_CompareWhenTheBouquetIsGone(t *testing.T) {
	e := newEnvWith(t, mock.New())
	addr := e.serveLiveList(t, &liveList{text: listV1})
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/read", `{"url":`+jsonStr(addr)+`}`, nil, 200, &r)
	e.planAndApply(t, "/api/v1/channels/iptv/plan", `{"token":`+jsonStr(r.Token)+`,"ids":[0],"newName":"Net","kind":"tv","remember":true}`, 200)
	var srcs iptvSources
	e.json(t, "GET", "/api/v1/channels/iptv/sources", "", nil, 200, &srcs)
	e.planAndApply(t, "/api/v1/channels/bouquets/plan", `{"action":"delete","kind":"tv","ref":`+jsonStr(srcs.Sources[0].BouquetRef)+`}`, 200)
	e.json(t, "GET", "/api/v1/channels/iptv/sources", "", nil, 200, &srcs)
	if len(srcs.Sources) != 1 || !srcs.Sources[0].Gone {
		t.Fatalf("a bouquet that is gone is shown as gone: %+v", srcs)
	}
	resp, b := e.do(t, "POST", "/api/v1/channels/iptv/sources/"+strconv.FormatInt(srcs.Sources[0].ID, 10)+"/compare", "", nil)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "iptv_bouquet_gone") {
		t.Fatalf("compare: %d %s", resp.StatusCode, b)
	}
}

func TestIPTV_AFileIsNotRemembered(t *testing.T) {
	e := newEnvWith(t, mock.New())
	r := e.readList(t)
	e.planAndApply(t, "/api/v1/channels/iptv/plan", `{"token":`+jsonStr(r.Token)+`,"ids":[0],"newName":"Mine","kind":"tv","remember":true}`, 200)
	var srcs iptvSources
	e.json(t, "GET", "/api/v1/channels/iptv/sources", "", nil, 200, &srcs)
	if len(srcs.Sources) != 0 {
		t.Fatalf("a file has no address to compare with: %+v", srcs)
	}
}

func TestIPTV_NotRememberedUnlessAsked(t *testing.T) {
	e := newEnvWith(t, mock.New())
	addr := e.serveLiveList(t, &liveList{text: listV1})
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/read", `{"url":`+jsonStr(addr)+`}`, nil, 200, &r)
	e.planAndApply(t, "/api/v1/channels/iptv/plan", `{"token":`+jsonStr(r.Token)+`,"ids":[0],"newName":"Net","kind":"tv"}`, 200)
	var srcs iptvSources
	e.json(t, "GET", "/api/v1/channels/iptv/sources", "", nil, 200, &srcs)
	if len(srcs.Sources) != 0 {
		t.Fatalf("remembered without being asked: %+v", srcs)
	}
}

// --- a guide for internet channels ------------------------------------------------------

const twinList = "#EXTM3U\n" +
	"#EXTINF:-1 tvg-id=\"zdf\" group-title=\"DE\",ZDF\nhttp://tv.example/zdf1.m3u8\n" +
	"#EXTINF:-1 tvg-id=\"arte\" group-title=\"DE\",arte\nhttp://tv.example/arte1.m3u8\n" +
	"#EXTINF:-1 group-title=\"DE\",Das Erste Österreich\nhttp://tv.example/erste1.m3u8\n" +
	"#EXTINF:-1 group-title=\"DE\",Some Unknown Channel\nhttp://tv.example/unknown1.m3u8\n" +
	"#EXTINF:-1 group-title=\"DE\",ZDFneo HD\nhttp://tv.example/neo1.m3u8\n"

type twinSuggestions struct {
	Suggestions []struct {
		ID         int                   `json:"id"`
		Remembered *struct{ Ref string } `json:"remembered"`
		Sure       *struct {
			Ref  string `json:"ref"`
			Name string `json:"name"`
		} `json:"sure"`
		Candidates []struct {
			Ref  string `json:"ref"`
			Name string `json:"name"`
		} `json:"candidates"`
	} `json:"suggestions"`
}

func TestIPTV_SuggestsTheGuideOfAChannelOfTheBox(t *testing.T) {
	e := newEnvWith(t, mock.New())
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/upload?name=t.m3u", twinList, nil, 200, &r)
	var s twinSuggestions
	e.json(t, "POST", "/api/v1/channels/iptv/twins", `{"token":`+jsonStr(r.Token)+`,"ids":[0,1,2,3,4]}`, nil, 200, &s)
	if len(s.Suggestions) != 5 {
		t.Fatalf("suggestions: %+v", s)
	}
	sure := func(i int) string {
		if s.Suggestions[i].Sure == nil {
			return ""
		}
		return s.Suggestions[i].Sure.Name
	}
	// the quality suffix does not matter, on either side
	if sure(0) != "ZDF HD" || sure(1) != "arte HD" || sure(4) != "zdf_neo HD" {
		t.Errorf("sure: %q %q %q", sure(0), sure(1), sure(4))
	}
	// a longer name only suggests; an unknown channel offers nothing
	if sure(2) != "" || len(s.Suggestions[2].Candidates) != 1 || s.Suggestions[2].Candidates[0].Name != "Das Erste HD" {
		t.Errorf("Das Erste Österreich: %+v", s.Suggestions[2])
	}
	if sure(3) != "" || len(s.Suggestions[3].Candidates) != 0 {
		t.Errorf("unknown channel: %+v", s.Suggestions[3])
	}
	if resp, _ := e.do(t, "POST", "/api/v1/channels/iptv/twins", `{"token":`+jsonStr(r.Token)+`,"ids":[99]}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("an id out of range: %d", resp.StatusCode)
	}
}

func TestIPTV_SearchesTheBoxForAGuide(t *testing.T) {
	e := newEnvWith(t, mock.New())
	var res struct {
		Results []struct {
			Ref  string `json:"ref"`
			Name string `json:"name"`
			HD   bool   `json:"hd"`
		} `json:"results"`
	}
	e.json(t, "GET", "/api/v1/channels/iptv/twins/search?q=zdf", "", nil, 200, &res)
	names := []string{}
	for _, x := range res.Results {
		names = append(names, x.Name)
	}
	if strings.Join(names, "|") != "ZDF HD|zdf_neo HD" || !res.Results[0].HD {
		t.Errorf("search: %v", names)
	}
	e.json(t, "GET", "/api/v1/channels/iptv/twins/search?q=z", "", nil, 200, &res)
	if len(res.Results) != 0 {
		t.Errorf("one letter finds nothing: %+v", res)
	}
}

func TestIPTV_ChannelsGetTheIdentityOfTheirTwin(t *testing.T) {
	e := newEnvWith(t, mock.New())
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/upload?name=t.m3u", twinList, nil, 200, &r)
	body := `{"token":` + jsonStr(r.Token) + `,"ids":[0,3],"newName":"Guide","kind":"tv","twins":[{"id":0,"service":` + jsonStr(mock.TVZDF.ServiceReference) + `}]}`
	e.planAndApply(t, "/api/v1/channels/iptv/plan", body, 200)
	var list struct {
		Bouquets []struct{ Ref, Name string } `json:"bouquets"`
	}
	e.json(t, "GET", "/api/v1/channels/bouquets", "", nil, 200, &list)
	ref := ""
	for _, b := range list.Bouquets {
		if b.Name == "Guide (TV)" {
			ref = b.Ref
		}
	}
	got := e.entries(t, ref)
	if len(got.Entries) != 2 || got.Entries[0].Ref != "4097:0:19:2B66:3F3:1:C00000:0:0:0:http%3a//tv.example/zdf1.m3u8:ZDF" {
		t.Fatalf("a channel with a twin carries its identity: %+v", got.Entries)
	}
	if got.Entries[1].Ref != "4097:0:1:0:0:0:0:0:0:0:http%3a//tv.example/unknown1.m3u8:Some Unknown Channel" {
		t.Errorf("a channel without one stays plain: %q", got.Entries[1].Ref)
	}
	// the match is remembered for the next list
	var s twinSuggestions
	var again iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/upload?name=t.m3u", twinList, nil, 200, &again)
	e.json(t, "POST", "/api/v1/channels/iptv/twins", `{"token":`+jsonStr(again.Token)+`,"ids":[0,3]}`, nil, 200, &s)
	if s.Suggestions[0].Remembered == nil || s.Suggestions[0].Remembered.Ref != mock.TVZDF.ServiceReference || s.Suggestions[1].Remembered != nil {
		t.Errorf("remembered: %+v", s.Suggestions)
	}

	// a channel that is not in the choice, a made-up service, a heading
	for name, tw := range map[string]string{
		"not chosen": `{"id":1,"service":` + jsonStr(mock.TVZDF.ServiceReference) + `}`,
		"unknown":    `{"id":0,"service":"1:0:19:FFFF:FFF:1:C00000:0:0:0:"}`,
		"a bouquet":  `{"id":0,"service":` + jsonStr(freeTV) + `}`,
	} {
		bad := `{"token":` + jsonStr(again.Token) + `,"ids":[0],"newName":"X","kind":"tv","twins":[` + tw + `]}`
		if resp, b := e.do(t, "POST", "/api/v1/channels/iptv/plan", bad, nil); resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "twins") {
			t.Errorf("%s: %d %s", name, resp.StatusCode, b)
		}
	}
}

func TestIPTV_ACompareKeepsTheGuide(t *testing.T) {
	e := newEnvWith(t, mock.New())
	live := &liveList{text: "#EXTM3U\n#EXTINF:-1 tvg-id=\"zdf\",ZDF\nhttp://tv.example/zdf1.m3u8\n"}
	addr := e.serveLiveList(t, live)
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/read", `{"url":`+jsonStr(addr)+`}`, nil, 200, &r)
	e.planAndApply(t, "/api/v1/channels/iptv/plan", `{"token":`+jsonStr(r.Token)+`,"ids":[0],"newName":"Net","kind":"tv","remember":true,"twins":[{"id":0,"service":`+jsonStr(mock.TVZDF.ServiceReference)+`}]}`, 200)

	live.set("#EXTM3U\n#EXTINF:-1 tvg-id=\"zdf\",ZDF\nhttp://tv.example/zdf2.m3u8\n")
	var srcs iptvSources
	e.json(t, "GET", "/api/v1/channels/iptv/sources", "", nil, 200, &srcs)
	base := "/api/v1/channels/iptv/sources/" + strconv.FormatInt(srcs.Sources[0].ID, 10)
	var c iptvCompared
	e.json(t, "POST", base+"/compare", "", nil, 200, &c)
	if len(c.Items) != 1 || c.Items[0].Action != "update" {
		t.Fatalf("compare: %+v", c)
	}
	e.planAndApply(t, base+"/plan", `{"token":`+jsonStr(c.Token)+`,"ids":[0]}`, 200)
	got := e.entries(t, srcs.Sources[0].BouquetRef)
	if len(got.Entries) != 1 || got.Entries[0].Ref != "4097:0:19:2B66:3F3:1:C00000:0:0:0:http%3a//tv.example/zdf2.m3u8:ZDF" {
		t.Fatalf("the new address keeps the guide: %+v", got.Entries)
	}
}

// --- logos and the stream test -------------------------------------------------------------

// A small real picture: the logo of a channel.
var logoPNG = func() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}()

func (e *env) customLogoKeys(t *testing.T) []string {
	t.Helper()
	var r struct {
		Keys []string `json:"keys"`
	}
	e.json(t, "GET", "/api/v1/channels/logo/custom", "", nil, 200, &r)
	return r.Keys
}

func waitForLogos(t *testing.T, e *env, n int) []string {
	t.Helper()
	var keys []string
	for i := 0; i < 100; i++ {
		if keys = e.customLogoKeys(t); len(keys) >= n {
			return keys
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected %d logos, have %v", n, keys)
	return nil
}

func TestIPTV_FetchesTheLogosOfTheList(t *testing.T) {
	e := newEnvWith(t, mock.New())
	logos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/one.png" {
			w.Write(logoPNG)
			return
		}
		http.NotFound(w, r)
	}))
	defer logos.Close()
	list := "#EXTM3U\n" +
		"#EXTINF:-1 tvg-logo=\"" + logos.URL + "/one.png\",One\nhttp://tv.example/one.m3u8\n" +
		"#EXTINF:-1 tvg-logo=\"" + logos.URL + "/missing.png\",Two\nhttp://tv.example/two.m3u8\n" +
		"#EXTINF:-1,Three\nhttp://tv.example/three.m3u8\n"
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/upload?name=l.m3u", list, nil, 200, &r)

	// without asking, nothing is fetched
	var p struct {
		Plan  struct{ ID string } `json:"plan"`
		Logos struct {
			Count int      `json:"count"`
			Hosts []string `json:"hosts"`
		} `json:"logos"`
	}
	e.json(t, "POST", "/api/v1/channels/iptv/plan", `{"token":`+jsonStr(r.Token)+`,"ids":[0,1,2],"newName":"NoLogos","kind":"tv"}`, nil, 200, &p)
	if p.Logos.Count != 0 {
		t.Fatalf("logos without asking: %+v", p.Logos)
	}

	body := `{"token":` + jsonStr(r.Token) + `,"ids":[0,1,2],"newName":"Logos","kind":"tv","logos":true}`
	e.json(t, "POST", "/api/v1/channels/iptv/plan", body, nil, 200, &p)
	if p.Logos.Count != 2 || len(p.Logos.Hosts) != 1 || p.Logos.Hosts[0] != "127.0.0.1" {
		t.Fatalf("the preview says where logos come from: %+v", p.Logos)
	}
	if keys := e.customLogoKeys(t); len(keys) != 0 {
		t.Fatalf("a logo before the change was confirmed: %v", keys)
	}
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != 200 {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	// One's logo arrives; Two's address answers 404 and costs only Two's logo
	keys := waitForLogos(t, e, 1)
	time.Sleep(200 * time.Millisecond)
	if keys = e.customLogoKeys(t); len(keys) != 1 || !strings.Contains(keys[0], "_H") {
		t.Fatalf("logos: %v", keys)
	}
}

func TestIPTV_ALogoFollowsAChangedAddress(t *testing.T) {
	e := newEnvWith(t, mock.New())
	logos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(logoPNG) }))
	defer logos.Close()
	text := func(addr string) string {
		return "#EXTM3U\n#EXTINF:-1 tvg-id=\"one\" tvg-logo=\"" + logos.URL + "/one.png\",One\n" + addr + "\n"
	}
	live := &liveList{text: text("http://tv.example/one1.m3u8")}
	addr := e.serveLiveList(t, live)
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/read", `{"url":`+jsonStr(addr)+`}`, nil, 200, &r)
	e.planAndApply(t, "/api/v1/channels/iptv/plan", `{"token":`+jsonStr(r.Token)+`,"ids":[0],"newName":"Net","kind":"tv","remember":true,"logos":true}`, 200)
	first := waitForLogos(t, e, 1)

	live.set(text("http://tv.example/one2.m3u8"))
	var srcs iptvSources
	e.json(t, "GET", "/api/v1/channels/iptv/sources", "", nil, 200, &srcs)
	base := "/api/v1/channels/iptv/sources/" + strconv.FormatInt(srcs.Sources[0].ID, 10)
	var c iptvCompared
	e.json(t, "POST", base+"/compare", "", nil, 200, &c)
	e.planAndApply(t, base+"/plan", `{"token":`+jsonStr(c.Token)+`,"ids":[0]}`, 200)
	keys := e.customLogoKeys(t)
	if len(keys) != 2 {
		t.Fatalf("the logo should be under the new address too: before %v, after %v", first, keys)
	}
}

func TestIPTV_TestsTheStreams(t *testing.T) {
	e := newEnvWith(t, mock.New())
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) })
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusForbidden) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	list := "#EXTM3U\n#EXTINF:-1,Ok\n" + srv.URL + "/ok\n#EXTINF:-1,Login\n" + srv.URL + "/login\n#EXTINF:-1,Gone\n" + srv.URL + "/nothing\n#EXTINF:-1,Cam\nrtsp://cam.example/s\n"
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/upload?name=l.m3u", list, nil, 200, &r)
	var res struct {
		Results []struct {
			ID     int    `json:"id"`
			Result string `json:"result"`
		} `json:"results"`
	}
	resp, raw := e.do(t, "POST", "/api/v1/channels/iptv/test", `{"token":`+jsonStr(r.Token)+`,"ids":[0,1,2,3]}`, nil)
	if resp.StatusCode != 200 || strings.Contains(string(raw), srv.URL) {
		t.Fatalf("test: %d %s", resp.StatusCode, raw)
	}
	e.json(t, "POST", "/api/v1/channels/iptv/test", `{"token":`+jsonStr(r.Token)+`,"ids":[0,1,2,3]}`, nil, 200, &res)
	got := []string{}
	for _, x := range res.Results {
		got = append(got, x.Result)
	}
	if strings.Join(got, ",") != "ok,login,gone,unchecked" {
		t.Fatalf("results: %v", got)
	}
	if resp, _ := e.do(t, "POST", "/api/v1/channels/iptv/test", `{"token":`+jsonStr(r.Token)+`,"ids":[]}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("no ids: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "POST", "/api/v1/channels/iptv/test", `{"token":"nope","ids":[0]}`, nil); resp.StatusCode != http.StatusGone {
		t.Errorf("unknown token: %d", resp.StatusCode)
	}
}

// --- internet channels in the channel list (N2) -------------------------------------------

func TestIPTV_BouquetEntriesSayWhichAreInternetChannels(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	// Put one of each kind into a bouquet directly: a plain stream, a Netcast
	// entry, a Netcast placeholder, a recording played from the disk.
	for _, x := range []struct{ ref, name string }{
		{"4097:0:1:0:0:0:0:0:0:0:http%3a//tv.example/a.m3u8", "Plain"},
		{"8193:256:0:73:1:66:515476:0:0:0:netcastserver_lgat%3a//host/p/playlist.m3u8", "Netcast"},
		{"4097:0:16:197:4:85:C00000:0:0:0:None", "Placeholder"},
		{"4097:0:1:0:0:0:0:0:0:0:/media/hdd/movie/a.ts", "Recording"},
	} {
		if err := m.AddToBouquet(context.Background(), freeTV, x.ref, "", x.name); err != nil {
			t.Fatal(err)
		}
	}
	var got struct {
		Entries []struct {
			Ref    string `json:"ref"`
			Name   string `json:"name"`
			Stream string `json:"stream"`
			Source string `json:"source"`
		} `json:"entries"`
	}
	e.json(t, "GET", "/api/v1/channels/bouquet?ref="+urlq(freeTV), "", nil, 200, &got)
	seen := map[string][2]string{}
	for _, x := range got.Entries {
		seen[x.Name] = [2]string{x.Stream, x.Source}
	}
	want := map[string][2]string{
		"Plain": {"iptv", ""}, "Netcast": {"netcast", "lgat"}, "Placeholder": {"netcast", ""}, "Recording": {"", ""}, "RTL Television": {"", ""},
	}
	for name, w := range want {
		if seen[name] != w {
			t.Errorf("%s: %v, want %v", name, seen[name], w)
		}
	}
}

func TestTimers_AnInternetChannelCannotBeRecorded(t *testing.T) {
	e := newEnv(t)
	body := timerBodyRef("4097:0:19:2B66:3F3:1:C00000:0:0:0:http%3a//tv.example/zdf.m3u8:ZDF", "Heute", ev(21, 45), ev(22, 30), 5, 5)
	resp, b := e.do(t, "POST", "/api/v1/timers/plan", body, nil)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "timer_no_tuner") {
		t.Fatalf("a stream: %d %s", resp.StatusCode, b)
	}
	// a real channel is planned as before
	if resp, b := e.do(t, "POST", "/api/v1/timers/plan", timerBody("Die Kanzlei", ev(21, 45), ev(22, 30), 0, 0), nil); resp.StatusCode != 200 {
		t.Fatalf("a channel with a tuner: %d %s", resp.StatusCode, b)
	}
}

const guideXML = "<?xml version=\"1.0\"?><tv>" +
	"<channel id=\"one.de\"><display-name>Something</display-name></channel>" +
	"<channel id=\"x\"><display-name>Two News</display-name></channel>" +
	"<programme channel=\"x\"></programme></tv>"

func TestIPTV_GuideCoverage(t *testing.T) {
	e := newEnv(t)
	r := e.readList(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(guideXML)) }))
	t.Cleanup(srv.Close)
	var out struct {
		Known, Total, MissingCount int
		Missing                    []struct {
			ID   int
			Name string
		}
	}
	body := `{"token":` + jsonStr(r.Token) + `,"ids":[0,1,2],"url":` + jsonStr(srv.URL+"/g.xml") + `}`
	e.json(t, "POST", "/api/v1/channels/iptv/guide-coverage", body, nil, 200, &out)
	// "One News" matches by tvg-id, "Two News" by name, "Toon" not at all
	if out.Known != 2 || out.Total != 3 || out.MissingCount != 1 || len(out.Missing) != 1 || out.Missing[0].Name != "Toon" {
		t.Errorf("coverage: %+v", out)
	}
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("<html></html>")) }))
	t.Cleanup(html.Close)
	resp, b := e.do(t, "POST", "/api/v1/channels/iptv/guide-coverage", `{"token":`+jsonStr(r.Token)+`,"url":`+jsonStr(html.URL+"/g.xml")+`}`, nil)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(b), "guide_not_xmltv") {
		t.Errorf("html: %d %s", resp.StatusCode, b)
	}
	resp, b = e.do(t, "POST", "/api/v1/channels/iptv/guide-coverage", `{"token":"nope","url":"http://x.example/g.xml"}`, nil)
	if resp.StatusCode != http.StatusGone {
		t.Errorf("gone: %d %s", resp.StatusCode, b)
	}
}

func TestIPTV_GuideSourcesAndGitHub(t *testing.T) {
	e := newEnv(t)
	var cur struct{ Sources []struct{ ID, URL string } }
	e.json(t, "GET", "/api/v1/channels/iptv/guide-sources", "", nil, 200, &cur)
	if len(cur.Sources) == 0 || !strings.HasPrefix(cur.Sources[0].URL, "https://") {
		t.Errorf("curated: %+v", cur)
	}
	resp, b := e.do(t, "POST", "/api/v1/channels/iptv/github", `{"url":"https://example.com/a/b"}`, nil)
	if resp.StatusCode != 400 || !strings.Contains(string(b), "iptv_invalid_url") {
		t.Errorf("not github: %d %s", resp.StatusCode, b)
	}
}

type netcastSourcesView struct {
	Available bool `json:"available"`
	Listed    bool `json:"listed"`
	Files     []struct {
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		Sources []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Address  string `json:"address"`
			HasLogin bool   `json:"hasLogin"`
		} `json:"sources"`
	} `json:"files"`
}

func TestNetcast_SourcesAreListedWithoutLogins(t *testing.T) {
	e := newEnv(t)
	resp, raw := e.do(t, "GET", "/api/v1/channels/netcast/sources", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("sources: %d %s", resp.StatusCode, raw)
	}
	for _, leak := range []string{"hunter2", `"me"`, "Password"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the answer holds %q: %s", leak, raw)
		}
	}
	var v netcastSourcesView
	e.json(t, "GET", "/api/v1/channels/netcast/sources", "", nil, 200, &v)
	if !v.Available || !v.Listed || len(v.Files) != 3 || v.Files[0].Name != "netcastserver_gemini.json" || v.Files[2].Kind != "epg" {
		t.Fatalf("view: %+v", v)
	}
	var pluto bool
	for _, f := range v.Files {
		for _, s := range f.Sources {
			if s.ID == "pluto" {
				pluto = s.HasLogin
			}
		}
	}
	if !pluto {
		t.Errorf("Pluto has a login: %+v", v.Files)
	}
}

func TestNetcast_BoxWithoutFiles(t *testing.T) {
	m := mock.New()
	m.NoBoxFiles = true
	e := newEnvWith(t, m)
	var v netcastSourcesView
	e.json(t, "GET", "/api/v1/channels/netcast/sources", "", nil, 200, &v)
	if v.Available || len(v.Files) != 0 {
		t.Errorf("a box with no files: %+v", v)
	}
}

func TestNetcast_BoxThatCannotListFallsBackToKnownNames(t *testing.T) {
	m := mock.New()
	m.NoFileListing = true
	e := newEnvWith(t, m)
	var v netcastSourcesView
	e.json(t, "GET", "/api/v1/channels/netcast/sources", "", nil, 200, &v)
	if !v.Available || v.Listed || len(v.Files) != 3 {
		t.Errorf("fallback: %+v", v)
	}
}

type netcastPlanView struct {
	Plan struct {
		ID      string `json:"id"`
		Changes []struct {
			Type string `json:"type"`
		} `json:"changes"`
	} `json:"plan"`
	File string `json:"file"`
}

func TestNetcast_AddAGuideSourceAsAPlan(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	body := `{"action":"add","kind":"epg","name":"My Guide","address":"https://u:p@g.example/de.xml.gz?key=s3cret"}`
	resp, raw := e.do(t, "POST", "/api/v1/channels/netcast/plan", body, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("plan: %d %s", resp.StatusCode, raw)
	}
	for _, leak := range []string{"s3cret", "u:p"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the preview holds %q: %s", leak, raw)
		}
	}
	// nothing is written before the confirmation
	if _, ok := m.Files["/etc/enigma2/netcastepg_dhmyguide.json"]; ok {
		t.Fatal("the file exists before the plan is applied")
	}
	var p netcastPlanView
	e.json(t, "POST", "/api/v1/channels/netcast/plan", body, nil, 200, &p)
	if p.File != "netcastepg_dhmyguide.json" || len(p.Plan.Changes) != 1 || p.Plan.Changes[0].Type != "netcast.source.add" {
		t.Fatalf("plan: %+v", p)
	}
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != 200 {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	if got := string(m.Files["/etc/enigma2/netcastepg_dhmyguide.json"]); !strings.Contains(got, "s3cret") || !strings.Contains(got, `"id": "dhmyguide"`) {
		t.Errorf("file: %s", got)
	}
	var v netcastSourcesView
	e.json(t, "GET", "/api/v1/channels/netcast/sources", "", nil, 200, &v)
	if len(v.Files) != 4 {
		t.Errorf("sources: %+v", v)
	}
	if resp, b := e.do(t, "GET", "/api/v1/channels/netcast/sources", "", nil); strings.Contains(string(b), "s3cret") || resp.StatusCode != 200 {
		t.Errorf("the login is shown: %s", b)
	}
}

func TestNetcast_EditKeepsALoginAndACopyOfTheOldFile(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	old := string(m.Files["/etc/enigma2/netcastserver_plutotv.json"])
	// an update is previewed, then applied; the stored login stays
	var p netcastPlanView
	e.json(t, "POST", "/api/v1/channels/netcast/plan", `{"action":"update","kind":"server","file":"netcastserver_plutotv.json","id":"pluto","name":"Pluto DE"}`, nil, 200, &p)
	if p.Plan.Changes[0].Type != "netcast.source.update" {
		t.Fatalf("plan: %+v", p)
	}
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != 200 {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	now := string(m.Files["/etc/enigma2/netcastserver_plutotv.json"])
	if !strings.Contains(now, "Pluto DE") || !strings.Contains(now, "hunter2") {
		t.Errorf("file: %s", now)
	}
	if string(m.Files["/etc/enigma2/netcastserver_plutotv.json.dreamhub-backup"]) != old {
		t.Error("the old file was not kept")
	}
	// removing the source
	e.json(t, "POST", "/api/v1/channels/netcast/plan", `{"action":"remove","kind":"server","file":"netcastserver_plutotv.json","id":"pluto"}`, nil, 200, &p)
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != 200 {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	if strings.Contains(string(m.Files["/etc/enigma2/netcastserver_plutotv.json"]), "pluto") {
		t.Error("the source is still there")
	}
}

func TestNetcast_PlanRefusals(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	for body, want := range map[string]string{
		`{"action":"add","kind":"epg","name":"x"}`: "validation_failed",
		`{"action":"add","kind":"epg","name":"Gemini","address":"https://e.example/x.xml","id":"gemini","file":"netcastepg_gemini.json"}`: "netcast_source_exists",
		`{"action":"remove","kind":"server","file":"netcastserver_gemini.json","id":"nope"}`:                                              "netcast_no_source",
		`{"action":"remove","kind":"server","file":"netcastserver_missing.json","id":"x"}`:                                                "netcast_no_source",
		`{"action":"add","kind":"server","file":"../x.json","name":"x","address":"https://h/x"}`:                                          "validation_failed",
		`{"action":"explode","kind":"server"}`: "validation_failed",
	} {
		if resp, b := e.do(t, "POST", "/api/v1/channels/netcast/plan", body, nil); resp.StatusCode < 400 || !strings.Contains(string(b), want) {
			t.Errorf("%s: %d %s, want %s", body, resp.StatusCode, b, want)
		}
	}
	m.ReadOnlyFiles = true
	resp, b := e.do(t, "POST", "/api/v1/channels/netcast/plan", `{"action":"add","kind":"epg","name":"x","address":"https://h/x.xml"}`, nil)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "netcast_needs_plugin") {
		t.Errorf("read-only box: %d %s", resp.StatusCode, b)
	}
	var v netcastSourcesView
	e.json(t, "GET", "/api/v1/channels/netcast/sources", "", nil, 200, &v)
}

func TestNetcast_ChangedFileAndCorruptWriteAreCaught(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	const f = "/etc/enigma2/netcastserver_gemini.json"
	var p netcastPlanView
	e.json(t, "POST", "/api/v1/channels/netcast/plan", `{"action":"update","kind":"server","file":"netcastserver_gemini.json","id":"kodi","name":"Kodi 2"}`, nil, 200, &p)
	before := string(m.Files[f])
	m.Files[f] = append(m.Files[f], '\n') // somebody edited it since the preview
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode < 400 {
		t.Errorf("a changed file: %d %s", resp.StatusCode, b)
	}
	m.Files[f] = []byte(before)
	e.json(t, "POST", "/api/v1/channels/netcast/plan", `{"action":"update","kind":"server","file":"netcastserver_gemini.json","id":"kodi","name":"Kodi 2"}`, nil, 200, &p)
	m.CorruptWrites = true
	resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if resp.StatusCode < 400 || !strings.Contains(string(b), "netcast_write_mismatch") {
		t.Errorf("a corrupt write: %d %s", resp.StatusCode, b)
	}
}

func TestNetcast_ACachedListBecomesAnIPTVList(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/netcast/read", `{"id":"kodi"}`, nil, 200, &r)
	if r.Token == "" || !r.FromFile || r.Name != "Netcast: Kodi IPTV" || len(r.Channels) != 2 {
		t.Fatalf("read: %+v", r)
	}
	// it is an ordinary list now: a plan into a new bouquet works
	body := `{"token":` + jsonStr(r.Token) + `,"ids":[0,1],"newName":"Netcast Kodi","kind":"tv"}`
	var p struct{ Plan struct{ ID string } }
	e.json(t, "POST", "/api/v1/channels/iptv/plan", body, nil, 200, &p)
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != 200 {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	for body, want := range map[string]string{
		`{"id":"nolist"}`:      "netcast_list_missing",
		`{"id":"../../etc/x"}`: "invalid_body",
		`{}`:                   "invalid_body",
	} {
		if resp, b := e.do(t, "POST", "/api/v1/channels/netcast/read", body, nil); resp.StatusCode < 400 || !strings.Contains(string(b), want) {
			t.Errorf("%s: %d %s, want %s", body, resp.StatusCode, b, want)
		}
	}
	m.NoBoxFiles = true
	if resp, b := e.do(t, "POST", "/api/v1/channels/netcast/read", `{"id":"kodi"}`, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a box without files: %d %s", resp.StatusCode, b)
	}
}

// The box writes '%' and ':' of a name into the reference as %25 and %3a
// (Dreambox One, 2026-10-24); the read-back after the add has to expect that.
func TestIPTV_NamesWithPercentAndColon(t *testing.T) {
	e := newEnvWith(t, mock.New())
	list := "#EXTM3U\n#EXTINF:-1,100% Weihnachten\nhttp://a.example/1.m3u8\n#EXTINF:-1,Live: Sport | HD\nhttp://a.example/2.m3u8\n#EXTINF:-1,Plain\nhttp://a.example/3.m3u8\n"
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/iptv/upload?name=l.m3u", list, nil, 200, &r)
	var p struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	e.json(t, "POST", "/api/v1/channels/iptv/plan", `{"token":`+jsonStr(r.Token)+`,"ids":[0,1,2],"newName":"Odd names","kind":"tv"}`, nil, 200, &p)
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != 200 {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
}

func TestNetcast_SamsungAndPlutoListsAreNativeEntries(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	var r iptvRead
	e.json(t, "POST", "/api/v1/channels/netcast/read", `{"id":"samsungtvplusapi"}`, nil, 200, &r)
	// the plugin leaves "Sony One" out, and so do we
	if r.Name != "Netcast: samsungtvplusapi" && !strings.HasPrefix(r.Name, "Netcast:") || len(r.Channels) != 3 {
		t.Fatalf("read: %+v", r)
	}
	if r.Channels[0].Group != "Germany · News" {
		t.Errorf("Germany first, with its group: %+v", r.Channels[0])
	}
	var p struct{ Plan struct{ ID string } }
	e.json(t, "POST", "/api/v1/channels/iptv/plan", `{"token":`+jsonStr(r.Token)+`,"ids":[0,1],"newName":"Samsung","kind":"tv"}`, nil, 200, &p)
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != 200 {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	var list struct {
		Bouquets []struct{ Ref, Name string } `json:"bouquets"`
	}
	e.json(t, "GET", "/api/v1/channels/bouquets", "", nil, 200, &list)
	ref := ""
	for _, b := range list.Bouquets {
		if b.Name == "Samsung (TV)" {
			ref = b.Ref
		}
	}
	got := e.entries(t, ref)
	if len(got.Entries) != 2 || !strings.HasPrefix(got.Entries[0].Ref, "8193:256:0:") ||
		!strings.Contains(got.Entries[0].Ref, ":netcastserver_samsungtvplusapi%3a//jmp2.uk/stvp-DE100:Euronews LIVE") {
		t.Fatalf("entries: %+v", got.Entries)
	}
	var pr iptvRead
	e.json(t, "POST", "/api/v1/channels/netcast/read", `{"id":"plutotvapi"}`, nil, 200, &pr)
	if len(pr.Channels) != 2 || pr.Channels[0].Name != "CNNi" {
		t.Errorf("pluto: %+v", pr.Channels)
	}
	if resp, b := e.do(t, "POST", "/api/v1/channels/netcast/read", `{"id":"vavooapi"}`, nil); resp.StatusCode != 422 || !strings.Contains(string(b), "netcast_source_unsupported") {
		t.Errorf("vavoo: %d %s", resp.StatusCode, b)
	}
	// a playlist source of https addresses is native too, an http one stays a plain stream
	var kr iptvRead
	e.json(t, "POST", "/api/v1/channels/netcast/read", `{"id":"kodi"}`, nil, 200, &kr)
	e.json(t, "POST", "/api/v1/channels/iptv/plan", `{"token":`+jsonStr(kr.Token)+`,"ids":[0,1],"newName":"Kodi native","kind":"tv"}`, nil, 200, &p)
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != 200 {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	e.json(t, "GET", "/api/v1/channels/bouquets", "", nil, 200, &list)
	for _, b := range list.Bouquets {
		if b.Name == "Kodi native (TV)" {
			ref = b.Ref
		}
	}
	got = e.entries(t, ref)
	if len(got.Entries) != 2 || !strings.HasPrefix(got.Entries[0].Ref, "4097:") || !strings.HasPrefix(got.Entries[1].Ref, "8193:256:0:") ||
		!strings.Contains(got.Entries[1].Ref, "netcastserver_kodi%3a//tv.example/live/toon.m3u8") {
		t.Errorf("playlist entries: %+v", got.Entries)
	}
}

func TestNetcastHub_ReadAndPlayAChannelWithoutABouquet(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	var r iptvRead
	e.json(t, "POST", "/api/v1/netcast/read", `{"id":"kodi"}`, nil, 200, &r)
	if r.Token == "" || len(r.Channels) != 2 {
		t.Fatalf("read: %+v", r)
	}
	var src struct{ Available bool }
	e.json(t, "GET", "/api/v1/netcast/sources", "", nil, 200, &src)
	if !src.Available {
		t.Errorf("sources: %+v", src)
	}
	resp, b := e.do(t, "POST", "/api/v1/netcast/play", `{"token":`+jsonStr(r.Token)+`,"id":1}`, nil)
	if resp.StatusCode != 200 || !strings.HasPrefix(m.LastZapped, "8193:") && !strings.HasPrefix(m.LastZapped, "4097:") {
		t.Fatalf("play: %d %s zapped %q", resp.StatusCode, b, m.LastZapped)
	}
	for body, code := range map[string]int{
		`{"token":"nope","id":0}`:                   404,
		`{"token":` + jsonStr(r.Token) + `}`:        400,
		`{"token":` + jsonStr(r.Token) + `,"id":9}`: 400,
	} {
		if resp, b := e.do(t, "POST", "/api/v1/netcast/play", body, nil); resp.StatusCode != code {
			t.Errorf("%s: %d %s, want %d", body, resp.StatusCode, b, code)
		}
	}
}

func TestNetcastHub_SourcesShowTheirState_AndAnUncachedPlaylistIsFetched(t *testing.T) {
	list := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "#EXTM3U\n#EXTINF:-1 tvg-id=\"lg1.at\" group-title=\"News\",LG News\nhttps://lg.example/news.m3u8\n")
	}))
	defer list.Close()
	m := mock.New()
	m.Files["/etc/enigma2/netcastserver_gemini.json"] = []byte(`{"serverlist":[
		{"id":"kodi","name":"Kodi IPTV","mainUrl":"https://iptv.example/kodi.m3u","mediatype":"playlist_live"},
		{"id":"lgat","name":"LG Channels AT","mainUrl":"` + list.URL + `/lg.m3u8","mediatype":"playlist_live"},
		{"id":"login","name":"Paid","mainUrl":"https://paid.example/x.m3u","Username":"me","mediatype":"playlist"},
		{"id":"vavoo.livetv","name":"Vavoo","mediatype":"api"}]}`)
	e := newEnvWith(t, m)
	var src struct {
		Available bool
		Sources   []struct{ ID, State string }
	}
	e.json(t, "GET", "/api/v1/netcast/sources", "", nil, 200, &src)
	got := map[string]string{}
	for _, s := range src.Sources {
		got[s.ID] = s.State
	}
	want := map[string]string{"kodi": "ready", "lgat": "fetch", "login": "waiting", "vavoo.livetv": "unsupported"}
	for id, st := range want {
		if got[id] != st {
			t.Errorf("source %s: %q, want %q (all: %v)", id, got[id], st, got)
		}
	}
	if resp, _ := e.do(t, "POST", "/api/v1/channels/netcast/read", `{"id":"lgat"}`, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("ChannelHub's read still only reads the cache: %d", resp.StatusCode)
	}
	var r struct {
		Token    string
		Fetched  bool
		Channels []struct{ Name string }
	}
	e.json(t, "POST", "/api/v1/netcast/read", `{"id":"lgat"}`, nil, 200, &r)
	if !r.Fetched || len(r.Channels) != 1 || r.Channels[0].Name != "LG News" {
		t.Fatalf("fetched list: %+v", r)
	}
	if resp, b := e.do(t, "POST", "/api/v1/netcast/read", `{"id":"login"}`, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a source with a login is never fetched: %d %s", resp.StatusCode, b)
	}
}

func TestNetcastHub_GuideFromTheUsersOwnXMLTVSource(t *testing.T) {
	// fixedNow is 2026-09-23 18:07 UTC
	xmltv := `<?xml version="1.0"?><tv><channel id="one.de"><display-name>One News</display-name></channel>
<programme start="20260923180000 +0000" stop="20260923190000 +0000" channel="one.de"><title>Abendnachrichten</title></programme>
<programme start="20260923190000 +0000" stop="20260923200000 +0000" channel="one.de"><title>Wetter</title></programme>
<programme start="20260923180000 +0000" stop="20260923190000 +0000" channel="toon.x"><title>Kinderfilm</title></programme></tv>`
	guide := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, xmltv) }))
	defer guide.Close()
	m := mock.New()
	m.Files["/etc/enigma2/netcastepg_gemini.json"] = []byte(`{"entrys":[{"id":"g","name":"G","url":"` + guide.URL + `/epg.xml"}]}`)
	e := newEnvWith(t, m)
	var r iptvRead
	e.json(t, "POST", "/api/v1/netcast/read", `{"id":"kodi"}`, nil, 200, &r)
	type prog struct {
		Title string
		Start int64
	}
	var g struct {
		Programmes map[string]struct{ Now, Next *prog }
		Loading    bool
		Sources    int
	}
	body := `{"token":` + jsonStr(r.Token) + `,"ids":[0,1]}`
	for i := 0; i < 50; i++ {
		e.json(t, "POST", "/api/v1/netcast/guide", body, nil, 200, &g)
		if !g.Loading && len(g.Programmes) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	one := g.Programmes["0"] // "One News", matched by tvg-id
	if g.Sources != 1 || one.Now == nil || one.Now.Title != "Abendnachrichten" || one.Next == nil || one.Next.Title != "Wetter" {
		t.Fatalf("guide: %+v", g)
	}
	if _, ok := g.Programmes["1"]; ok {
		t.Errorf("Toon has no match in the guide: %+v", g.Programmes["1"])
	}
	if resp, _ := e.do(t, "POST", "/api/v1/netcast/guide", `{"token":"nope","ids":[0]}`, nil); resp.StatusCode != 404 {
		t.Errorf("unknown token: %d", resp.StatusCode)
	}
}

func TestNetcastHub_LogoComesThroughTheBackend(t *testing.T) {
	img := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8)))
		w.Write(b.Bytes())
	}))
	defer img.Close()
	list := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "#EXTM3U\n#EXTINF:-1 tvg-logo=\""+img.URL+"/l.png\",With\nhttps://x.example/a.m3u8\n#EXTINF:-1,Without\nhttps://x.example/b.m3u8\n")
	}))
	defer list.Close()
	m := mock.New()
	m.Files["/etc/enigma2/netcastserver_gemini.json"] = []byte(`{"serverlist":[{"id":"lgat","name":"L","mainUrl":"` + list.URL + `/l.m3u","mediatype":"playlist_live"}]}`)
	e := newEnvWith(t, m)
	var r iptvRead
	e.json(t, "POST", "/api/v1/netcast/read", `{"id":"lgat"}`, nil, 200, &r)
	resp, b := e.do(t, "GET", "/api/v1/netcast/logo?token="+r.Token+"&id=0", "", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" || len(b) == 0 {
		t.Fatalf("logo: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	for _, q := range []string{"token=" + r.Token + "&id=1", "token=" + r.Token + "&id=9", "token=x&id=0"} {
		if resp, _ := e.do(t, "GET", "/api/v1/netcast/logo?"+q, "", nil); resp.StatusCode != 404 {
			t.Errorf("%s: %d", q, resp.StatusCode)
		}
	}
}
