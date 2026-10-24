package api_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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
