package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	plexmock "github.com/antonbier/DreamHub/backend/internal/plex/mock"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// boxWithEPGTitle answers every EPGMulti call with a single broadcast of
// the given title — enough to test GuideHub's "already in Plex" flag
// (#32) without depending on the mock's fixed schedule fixtures.
type boxWithEPGTitle struct {
	*mock.Client
	title string
}

func (b boxWithEPGTitle) EPGMulti(_ context.Context, _ string, start, end int64) ([]webcontrol.EPGEvent, error) {
	return []webcontrol.EPGEvent{{Title: b.title, ServiceReference: "svc1", ServiceName: "Test TV", Start: start, Duration: 3600}}, nil
}

// connectPlex signs in through the plex.tv code flow against the mock and
// picks the mock server's local address.
func connectPlex(t *testing.T, e *env) {
	t.Helper()
	resp, b := e.do(t, "POST", "/api/v1/plex/login", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"code":"DH42"`) || !strings.Contains(string(b), "plex.tv/link") {
		t.Fatalf("starting the sign-in: %d %s", resp.StatusCode, b)
	}
	_, b = e.do(t, "GET", "/api/v1/plex/login/4711", "", nil)
	if !strings.Contains(string(b), `"state":"waiting"`) {
		t.Fatalf("before the code is entered: %s", b)
	}
	_, b = e.do(t, "GET", "/api/v1/plex/login/4711", "", nil)
	if !strings.Contains(string(b), `"state":"linked"`) || !strings.Contains(string(b), plexmock.URL) || strings.Contains(string(b), plexmock.Token) {
		t.Fatalf("after the code: the account's servers, never a token: %s", b)
	}
	resp, b = e.do(t, "POST", "/api/v1/plex/login/4711/use", `{"serverId":"mock-machine","url":"`+plexmock.URL+`/"}`, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"connected":"Unraid"`) {
		t.Fatalf("choosing the server: %d %s", resp.StatusCode, b)
	}
	if resp, b := e.do(t, "POST", "/api/v1/plex/sync", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("sync: %d %s", resp.StatusCode, b)
	}
}

type plexItems struct {
	Items []struct {
		RatingKey       string `json:"ratingKey"`
		Title           string `json:"title"`
		Watched         bool   `json:"watched"`
		ViewedLeafCount int    `json:"viewedLeafCount"`
	} `json:"items"`
	Meta struct {
		Online   bool  `json:"online"`
		SyncedAt int64 `json:"syncedAt"`
	} `json:"meta"`
}

func (e *env) plexItems(t *testing.T, library string) plexItems {
	t.Helper()
	resp, b := e.do(t, "GET", "/api/v1/plex/libraries/"+library+"/items", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("items of %s: %d %s", library, resp.StatusCode, b)
	}
	var out plexItems
	json.Unmarshal(b, &out)
	return out
}

func (p plexItems) watched(key string) bool {
	for _, it := range p.Items {
		if it.RatingKey == key {
			return it.Watched
		}
	}
	return false
}

func TestPlex_NotConfigured(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/plex/status", "", nil)
	if !strings.Contains(string(b), `"configured":false`) {
		t.Errorf("fresh install: %s", b)
	}
	resp, b := e.do(t, "GET", "/api/v1/plex/libraries", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "plex_not_configured" {
		t.Errorf("libraries without Plex: %d %s", resp.StatusCode, b)
	}
}

func TestPlex_SignInAndBrowse(t *testing.T) {
	e := newEnv(t)
	connectPlex(t, e)

	_, b := e.do(t, "GET", "/api/v1/settings", "", nil)
	if strings.Contains(string(b), plexmock.Token) || !strings.Contains(string(b), `"`+plexmock.URL+`"`) {
		t.Errorf("settings: the address without a trailing slash, never the token: %s", b)
	}
	_, b = e.do(t, "GET", "/api/v1/plex/libraries", "", nil)
	if !strings.Contains(string(b), `"title":"Filme"`) || !strings.Contains(string(b), `"title":"Serien"`) || strings.Contains(string(b), "Musik") {
		t.Errorf("films and series libraries only: %s", b)
	}
	films := e.plexItems(t, "1")
	if len(films.Items) != 6 || !films.watched("101") || films.watched("102") || !films.Meta.Online || films.Meta.SyncedAt == 0 {
		t.Errorf("films: %+v", films)
	}
	_, b = e.do(t, "GET", "/api/v1/plex/items/201", "", nil)
	if !strings.Contains(string(b), `"children":[{`) || !strings.Contains(string(b), `"title":"Staffel 2"`) {
		t.Errorf("a series lists its seasons: %s", b)
	}
	_, b = e.do(t, "GET", "/api/v1/search?q=boot", "", nil)
	if !strings.Contains(string(b), `"path":"/plex?item=101"`) {
		t.Errorf("search finds Plex films: %s", b)
	}
	resp, _ := e.do(t, "GET", "/api/v1/plex/image?path=/library/metadata/101/thumb/1&w=240", "", nil)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("poster: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if resp, _ := e.do(t, "GET", "/api/v1/plex/image?path=/:/prefs&w=240", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("only an item's own pictures pass the proxy, got %d", resp.StatusCode)
	}
}

func TestPlex_WatchedOneDirectlyASeasonByPreview(t *testing.T) {
	e := newEnv(t)
	connectPlex(t, e)

	resp, b := e.do(t, "POST", "/api/v1/plex/items/102/watched", `{"watched":true}`, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"watched":true`) {
		t.Fatalf("marking a film: %d %s", resp.StatusCode, b)
	}
	if !e.plexItems(t, "1").watched("102") {
		t.Error("the library list must show the film as watched right away")
	}
	_, b = e.do(t, "GET", "/api/v1/changes?limit=1", "", nil)
	if !strings.Contains(string(b), "Marked as watched in Plex: Lola rennt") {
		t.Errorf("change log: %s", b)
	}

	// A season only through preview and confirm.
	resp, b = e.do(t, "POST", "/api/v1/plex/items/212/watched", `{"watched":true}`, nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusBadRequest || code != "plex_needs_preview" {
		t.Fatalf("a season directly: %d %s", resp.StatusCode, b)
	}
	p, status, b := e.plan(t, "/api/v1/plex/items/212/watched/plan", `{"watched":true}`)
	if status != http.StatusOK || len(p.Plan.Changes) != 3 || !strings.Contains(string(b), "Dark S02E01 · Folge 1") {
		t.Fatalf("season preview: %d %s", status, b)
	}
	if len(e.plex.Watched) != 1 {
		t.Fatalf("a preview changes nothing, got %v", e.plex.Watched)
	}
	if resp, b := e.apply(t, p.Plan.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	var series plexItems
	_, b = e.do(t, "GET", "/api/v1/plex/libraries/2/items", "", nil)
	json.Unmarshal(b, &series)
	if len(series.Items) != 1 || !series.Items[0].Watched || series.Items[0].ViewedLeafCount != 6 {
		t.Errorf("the series must count every episode as seen now: %s", b)
	}
}

func TestPlex_OfflineServesTheCache(t *testing.T) {
	e := newEnv(t)
	connectPlex(t, e)
	e.plex.Offline = true

	films := e.plexItems(t, "1")
	if len(films.Items) != 6 || films.Meta.Online || films.Meta.SyncedAt == 0 {
		t.Errorf("offline: the cached library, marked offline: %+v", films.Meta)
	}
	resp, b := e.do(t, "POST", "/api/v1/plex/items/103/watched", `{"watched":true}`, nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "plex_offline" {
		t.Errorf("marking while offline: %d %s", resp.StatusCode, b)
	}
	resp, b = e.do(t, "POST", "/api/v1/plex/items/103/play", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "plex_offline" {
		t.Errorf("playing while offline: %d %s", resp.StatusCode, b)
	}
	resp, _ = e.do(t, "GET", "/api/v1/plex/image?path=/library/metadata/104/thumb/1&w=240", "", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("an uncached picture while offline: %d", resp.StatusCode)
	}
}

func TestPlex_PlayOnTheBox(t *testing.T) {
	box := mock.New()
	e := newEnvWith(t, box)
	connectPlex(t, e)

	resp, b := e.do(t, "POST", "/api/v1/plex/items/101/play", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("play: %d %s", resp.StatusCode, b)
	}
	if !box.MoviePlayer {
		t.Error("a Plex film must open the movie player (seeking, pause), not play like a channel")
	}
	ref := box.LastZapped
	const prefix = "4097:0:1:0:0:0:0:0:0:0:http%3a//127.0.0.1%3a8080/api/v1/plex/stream/"
	if !strings.HasPrefix(ref, prefix) || !strings.HasSuffix(ref, "/file.mkv:Das Boot") || strings.Contains(ref, plexmock.Token) {
		t.Fatalf("the box plays DreamHub's stream URL, without the token: %q", ref)
	}
	streamPath := "/api/v1/plex/stream/" + strings.TrimSuffix(strings.TrimPrefix(ref, prefix), ":Das Boot")

	// The box's player has no DreamHub login: the stream is public, but only
	// for the box itself (the test client is on loopback too).
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"auth.enabled":true,"auth.username":"admin","auth.password":"secret-password"}}`, nil)
	res, err := http.Get(e.srv.URL + streamPath)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != "mock media /library/parts/101/1/file.mkv" {
		t.Errorf("stream: %d %q", res.StatusCode, body)
	}
	res, _ = http.Get(e.srv.URL + "/api/v1/plex/stream/nope/file.mkv")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("an unknown ticket: %d", res.StatusCode)
	}
	res.Body.Close()
}

func TestPlex_ResumeAndChooseVersionOrPart(t *testing.T) {
	box := mock.New()
	e := newEnvWith(t, box)
	connectPlex(t, e)

	// "Das Boot" (101) is already fully watched, offset 0: no resume.
	resp, b := e.do(t, "POST", "/api/v1/plex/items/101/play", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("play: %d %s", resp.StatusCode, b)
	}
	if box.LastPlayStartMs != 0 {
		t.Errorf("a finished film doesn't resume: got %d", box.LastPlayStartMs)
	}

	// "Lola rennt" (102) was stopped 30 minutes in: resume there (#30).
	resp, b = e.do(t, "POST", "/api/v1/plex/items/102/play", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("play: %d %s", resp.StatusCode, b)
	}
	if box.LastPlayStartMs != 1_800_000 {
		t.Errorf("resume at the item's viewOffset: got %d", box.LastPlayStartMs)
	}

	// There's only one version and one part fixtures-wise; asking for a
	// version or part that doesn't exist is rejected, not a panic.
	resp, b = e.do(t, "POST", "/api/v1/plex/items/101/play", `{"media":1,"part":0}`, nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "plex_no_file" {
		t.Errorf("no such version: %d %s", resp.StatusCode, b)
	}
	resp, b = e.do(t, "POST", "/api/v1/plex/items/101/play", `{"media":0,"part":5}`, nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "plex_no_file" {
		t.Errorf("no such part: %d %s", resp.StatusCode, b)
	}
}

func TestPlex_StreamReportsProgressToPlex(t *testing.T) {
	box := mock.New()
	e := newEnvWith(t, box)
	connectPlex(t, e)

	resp, b := e.do(t, "POST", "/api/v1/plex/items/101/play", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("play: %d %s", resp.StatusCode, b)
	}
	ref := box.LastZapped
	const prefix = "4097:0:1:0:0:0:0:0:0:0:http%3a//127.0.0.1%3a8080/api/v1/plex/stream/"
	streamPath := "/api/v1/plex/stream/" + strings.TrimSuffix(strings.TrimPrefix(ref, prefix), ":Das Boot")

	req, _ := http.NewRequest("GET", e.srv.URL+streamPath, nil)
	req.Header.Set("Range", "bytes=2100000000-") // halfway through the 4.2 GB fixture file
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	deadline := time.Now().Add(2 * time.Second)
	var calls []plexmock.TimelineCall
	for {
		calls = e.plex.TimelineSnapshot()
		if len(calls) != 0 || !time.Now().Before(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(calls) != 1 {
		t.Fatalf("one Timeline call for the Range request: %+v", calls)
	}
	tl := calls[0]
	if tl.RatingKey != "101" || tl.State != "playing" || tl.DurationMs != 149*60000 {
		t.Errorf("timeline call: %+v", tl)
	}
	// Half the 4.2 GB file ≈ half the 149-minute duration.
	if tl.TimeMs < 70*60000 || tl.TimeMs > 76*60000 {
		t.Errorf("approximate position from the byte offset: %+v", tl)
	}
}

func TestPlex_AlreadyInPlexFlagsBroadcasts(t *testing.T) {
	box := boxWithEPGTitle{Client: mock.New(), title: "Das Boot"} // exact title of fixture 101
	e := newEnvWith(t, box)
	connectPlex(t, e)

	resp, b := e.do(t, "GET", "/api/v1/guide/grid?bouquet=x&start=0&end=999999999999", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"title":"Das Boot"`) || !strings.Contains(string(b), `"inPlex":true`) {
		t.Errorf("a broadcast matching a Plex film's title is flagged: %s", b)
	}

	box2 := boxWithEPGTitle{Client: mock.New(), title: "Not In Plex"}
	e2 := newEnvWith(t, box2)
	connectPlex(t, e2)
	resp, b = e2.do(t, "GET", "/api/v1/guide/grid?bouquet=x&start=0&end=999999999999", "", nil)
	if resp.StatusCode != http.StatusOK || strings.Contains(string(b), `"inPlex"`) {
		t.Errorf("no match, no flag at all (omitempty): %s", b)
	}
}

func TestPlex_ChooseLibraries(t *testing.T) {
	e := newEnv(t)
	connectPlex(t, e)
	resp, b := e.do(t, "PUT", "/api/v1/plex/libraries", `{"selected":["1"]}`, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"id":"2","title":"Serien","type":"show","count":1,"selected":false`) {
		t.Fatalf("only films: %d %s", resp.StatusCode, b)
	}
	_, b = e.do(t, "GET", "/api/v1/search?q=dark", "", nil)
	if strings.Contains(string(b), "/plex?item=201") {
		t.Errorf("a library not shown isn't searched: %s", b)
	}
	e.do(t, "PUT", "/api/v1/plex/libraries", `{"selected":["1","2"]}`, nil)
	_, b = e.do(t, "GET", "/api/v1/settings", "", nil)
	if !strings.Contains(string(b), `"key":"plex.libraries"`) || strings.Contains(string(b), `"value":"1,2"`) {
		t.Errorf("every library chosen is stored as empty (new ones show up too): %s", b)
	}
}

func TestPlex_AnUnreadableAnswerIsNotOffline(t *testing.T) {
	e := newEnv(t)
	connectPlex(t, e)
	e.plex.Unreadable = true
	resp, b := e.do(t, "GET", "/api/v1/plex/items/102", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusBadGateway || code != "plex_bad_answer" {
		t.Fatalf("an unreadable answer: %d %s", resp.StatusCode, b)
	}
	e.plex.Unreadable = false
	// The server is still taken as there: the next item comes from it, not
	// from a cache that doesn't have it.
	resp, b = e.do(t, "GET", "/api/v1/plex/items/103", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"online":true`) {
		t.Errorf("after an unreadable answer: %d %s", resp.StatusCode, b)
	}
}

func TestPlex_FailoverToAnotherAddress(t *testing.T) {
	e := newEnv(t)
	connectPlex(t, e)
	_, b := e.do(t, "GET", "/api/v1/settings", "", nil)
	if !strings.Contains(string(b), `"https://plex.example.org"`) {
		t.Fatalf("sign-in saves the server's other addresses as fallbacks: %s", b)
	}

	// The chosen address stops answering (here: never did); the fallback is
	// the same server.
	resp, b := e.do(t, "PATCH", "/api/v1/settings", `{"values":{"plex.url":"https://plex.example.org","plex.fallbackUrls":"`+plexmock.URL+`"}}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings: %d %s", resp.StatusCode, b)
	}
	if resp, b := e.do(t, "POST", "/api/v1/plex/sync", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("sync through the fallback: %d %s", resp.StatusCode, b)
	}
	_, b = e.do(t, "GET", "/api/v1/plex/status", "", nil)
	if !strings.Contains(string(b), `"online":true`) || !strings.Contains(string(b), `"url":"`+plexmock.URL+`"`) {
		t.Errorf("online at the fallback: %s", b)
	}
	if films := e.plexItems(t, "1"); !films.Meta.Online || len(films.Items) != 6 {
		t.Errorf("reads go to the fallback: %+v", films.Meta)
	}

	resp, b = e.do(t, "PATCH", "/api/v1/settings", `{"values":{"plex.fallbackUrls":"ftp://nope"}}`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a fallback must be an http(s) address: %d %s", resp.StatusCode, b)
	}
}

// Every shown Plex library is a source for custom tiles: newest first, a
// picture per item, "not seen yet" as a filter (docs/archive/PROFILES_PLAN.md, P4).
func TestPlex_LibrariesAreTileSources(t *testing.T) {
	e := newEnv(t)
	connectPlex(t, e)
	_, b := e.do(t, "GET", "/api/v1/dashboard/sources", "", nil)
	if !strings.Contains(string(b), `"id":"plex.library.1"`) || !strings.Contains(string(b), `"unwatched"`) {
		t.Fatalf("Plex library missing from the sources: %s", b)
	}
	var all, unwatched struct {
		Items []struct {
			Title     string `json:"title"`
			Image     string `json:"image"`
			Path      string `json:"path"`
			Unwatched bool   `json:"unwatched"`
		} `json:"items"`
		Total int `json:"total"`
	}
	e.json(t, "POST", "/api/v1/dashboard/view", `{"source":"plex.library.1","query":{"limit":20}}`, nil, 200, &all)
	e.json(t, "POST", "/api/v1/dashboard/view", `{"source":"plex.library.1","query":{"unwatched":true,"limit":20}}`, nil, 200, &unwatched)
	if all.Total == 0 || all.Items[0].Image == "" || !strings.HasPrefix(all.Items[0].Path, "/plex?item=") {
		t.Fatalf("library view: %+v", all)
	}
	if unwatched.Total == 0 || unwatched.Total >= all.Total {
		t.Fatalf("unwatched filter: %d of %d", unwatched.Total, all.Total)
	}
	for _, it := range unwatched.Items {
		if !it.Unwatched {
			t.Fatalf("a watched film in the unwatched row: %+v", it)
		}
	}
	// A tile showing it as posters saves.
	resp, b := e.do(t, "PUT", "/api/v1/dashboard", `{"tiles":[{"type":"custom","width":4,"custom":{"title":"Neu","source":"plex.library.1","query":{"unwatched":true},"display":"posters"}}]}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("posters tile: %d %s", resp.StatusCode, b)
	}
}
