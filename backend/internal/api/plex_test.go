package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	plexmock "github.com/antonbier/DreamHub/backend/internal/plex/mock"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

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
