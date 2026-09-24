package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/app"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

var fixedNow = time.Date(2026, 9, 23, 18, 7, 0, 0, time.UTC)

type env struct {
	srv   *httptest.Server
	store *store.Store
	svc   *settings.Service
}

// newEnv builds the application exactly as production does (internal/app),
// against the mock WebControl client and a real SQLite store in a temp dir.
func newEnv(t *testing.T) *env { return newEnvWith(t, mock.New()) }

func newEnvWith(t *testing.T, wc webcontrol.Client) *env {
	t.Helper()
	a, err := app.New(context.Background(), app.Options{
		DataDir:    t.TempDir(),
		WebControl: wc,
		Mock:       true,
		LookPath:   func(string) (string, error) { return "/usr/bin/journalctl", nil },
		Logger:     discardLogger(),
		Now:        func() time.Time { return fixedNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Store.Close() })
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(srv.Close)
	return &env{srv: srv, store: a.Store, svc: a.Settings}
}

func (e *env) do(t *testing.T, method, path, body string, configure func(*http.Request)) (*http.Response, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// A well-behaved client (the web UI) always sends the CSRF header on
	// mutating requests; tests of the CSRF check remove it via configure.
	if method != "GET" {
		req.Header.Set(httpx.CSRFHeader, "1")
	}
	if configure != nil {
		configure(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func TestHealth(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/health", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct{ Status, Version, Time string }
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Version != api.Version {
		t.Errorf("unexpected health body: %s", b)
	}
	if body.Time != "2026-09-23T18:07:00Z" {
		t.Errorf("health must report server time in UTC, got %q", body.Time)
	}
}

func TestDeviceInfo_UsesMock(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/deviceinfo", "", nil)
	var body struct {
		DeviceName string `json:"deviceName"`
	}
	json.Unmarshal(b, &body)
	if body.DeviceName != "one" {
		t.Errorf("expected device name %q (ARCHITECTURE.md fixture), got %q", "one", body.DeviceName)
	}
}

func TestMedia_MoviesDistinguishesRecordingsFromOtherMedia(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/media/movies", "", nil)
	var body struct {
		Movies []struct {
			Title       string `json:"title"`
			IsRecording bool   `json:"isRecording"`
		} `json:"movies"`
	}
	json.Unmarshal(b, &body)
	if len(body.Movies) != 2 {
		t.Fatalf("expected 2 mock movies, got %+v", body.Movies)
	}
	if body.Movies[0].Title != "Bauer sucht Frau International" || !body.Movies[0].IsRecording {
		t.Errorf("expected the .ts recording to be flagged isRecording: %+v", body.Movies[0])
	}
	if body.Movies[1].Title != "Oskars Kleid.mkv" || body.Movies[1].IsRecording {
		t.Errorf("a manually-copied .mkv must not be flagged isRecording: %+v", body.Movies[1])
	}
}

func TestMedia_DiskReportsUsagePercent(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/media/disk", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var body struct {
		Disks []struct {
			Model        string   `json:"model"`
			CapacityText string   `json:"capacityText"`
			FreeText     string   `json:"freeText"`
			UsedPercent  *float64 `json:"usedPercent"`
		} `json:"disks"`
	}
	json.Unmarshal(b, &body)
	if len(body.Disks) != 1 || body.Disks[0].CapacityText != "2 TB" || body.Disks[0].UsedPercent == nil {
		t.Fatalf("unexpected disks: %+v", body.Disks)
	}
	if pct := *body.Disks[0].UsedPercent; pct < 85 || pct > 90 {
		t.Errorf("expected ~87%% used (2 TB capacity, 263.947 GB free, matching the real device), got %.1f%%", pct)
	}
}

func TestSearch_FindsARealMovieByTitle(t *testing.T) {
	e := newEnv(t)
	// "Oskars Kleid" is unique to the mock's non-recording .mkv file — unlike
	// the mock's recorded movie, whose title happens to also match a mock
	// timer, this query isolates MediaHub's own search provider.
	resp, b := e.do(t, "GET", "/api/v1/search?q=Oskars", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var body struct {
		Results []struct {
			HubID string `json:"hubId"`
			Title string `json:"title"`
			Path  string `json:"path"`
		} `json:"results"`
	}
	json.Unmarshal(b, &body)
	if len(body.Results) != 1 || body.Results[0].Title != "Oskars Kleid.mkv" || body.Results[0].HubID != "media" || body.Results[0].Path != "/media" {
		t.Fatalf("expected exactly the mock's non-recording file, got %+v", body.Results)
	}
}

func TestSettings_GetReturnsCoreSections(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/settings", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var body struct {
		Sections []struct {
			ID     string `json:"id"`
			Fields []struct {
				Key   string `json:"key"`
				Value any    `json:"value"`
			} `json:"fields"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range body.Sections {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "general,auth,network,ai,tmdb,hubs" {
		t.Fatalf("expected core, connector and hub sections in order, got %v", ids)
	}
	if v := body.Sections[0].Fields[0]; v.Key != settings.KeyLanguage || v.Value != "en" {
		t.Errorf("expected default language en, got %+v", v)
	}
}

func TestSettings_PatchInvalidSavesNothing(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "PATCH", "/api/v1/settings",
		`{"values":{"general.language":"de","general.appearance":"neon","nope":1}}`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var env struct {
		Error struct {
			Code    string                         `json:"code"`
			Details map[string]settings.FieldError `json:"details"`
		} `json:"error"`
	}
	json.Unmarshal(b, &env)
	if env.Error.Code != "validation_failed" {
		t.Errorf("expected validation_failed, got %s", b)
	}
	if env.Error.Details["general.appearance"].Code != settings.ErrInvalidOption || env.Error.Details["nope"].Code != settings.ErrUnknown {
		t.Errorf("expected per-field codes, got %s", b)
	}
	// The valid language change must not have been saved either.
	if _, err := e.store.Setting(context.Background(), 1, settings.KeyLanguage); err != store.ErrNotFound {
		t.Errorf("nothing may be saved when any field is invalid")
	}
}

func TestSettings_EnableAuthThenRequireCredentials(t *testing.T) {
	e := newEnv(t)

	// Enabling auth without credentials is rejected (would lock everyone out).
	resp, _ := e.do(t, "PATCH", "/api/v1/settings", `{"values":{"auth.enabled":true}}`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("enabling auth without credentials: expected 400, got %d", resp.StatusCode)
	}

	resp, b := e.do(t, "PATCH", "/api/v1/settings",
		`{"values":{"auth.enabled":true,"auth.username":"armin","auth.password":"correct horse"}}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	if strings.Contains(string(b), "correct horse") {
		t.Fatal("password must never be returned")
	}

	// Now protected routes require credentials; health stays public.
	if resp, _ := e.do(t, "GET", "/api/v1/settings", "", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no credentials: expected 401, got %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/settings", "", func(r *http.Request) { r.SetBasicAuth("armin", "wrong password") }); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong password: expected 401, got %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/settings", "", func(r *http.Request) { r.SetBasicAuth("armin", "correct horse") }); resp.StatusCode != http.StatusOK {
		t.Errorf("correct credentials: expected 200, got %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/health", "", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("health must stay public, got %d", resp.StatusCode)
	}
}

func TestWhoami_DoublesAsACredentialsCheck(t *testing.T) {
	e := newEnv(t)

	resp, b := e.do(t, "GET", "/api/v1/whoami", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("auth disabled: expected 200, got %d: %s", resp.StatusCode, b)
	}
	var who struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	json.Unmarshal(b, &who)
	if who.Username == "" || who.Role != "admin" {
		t.Errorf("unexpected whoami body: %+v", who)
	}

	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"auth.enabled":true,"auth.username":"armin","auth.password":"correct horse"}}`, nil)

	if resp, _ := e.do(t, "GET", "/api/v1/whoami", "", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no credentials: expected 401, got %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/whoami", "", func(r *http.Request) { r.SetBasicAuth("armin", "wrong") }); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong password: expected 401, got %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/whoami", "", func(r *http.Request) { r.SetBasicAuth("armin", "correct horse") }); resp.StatusCode != http.StatusOK {
		t.Errorf("correct credentials: expected 200, got %d", resp.StatusCode)
	}
}

func TestSettings_PatchRejectsMalformedBody(t *testing.T) {
	e := newEnv(t)
	for _, body := range []string{`not json`, `{}`, `{"values":{},"extra":1}`} {
		if resp, _ := e.do(t, "PATCH", "/api/v1/settings", body, nil); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %q: expected 400, got %d", body, resp.StatusCode)
		}
	}
}

type navItem struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Problems []struct {
		Code      string         `json:"code"`
		Fix       string         `json:"fix"`
		FixParams map[string]any `json:"fixParams"`
	} `json:"problems"`
}

func navItems(t *testing.T, e *env) map[string]navItem {
	t.Helper()
	resp, b := e.do(t, "GET", "/api/v1/navigation", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("navigation: expected 200, got %d: %s", resp.StatusCode, b)
	}
	var nav struct {
		Labels   string `json:"labels"`
		Sections []struct {
			ID    string    `json:"id"`
			Items []navItem `json:"items"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(b, &nav); err != nil {
		t.Fatal(err)
	}
	out := map[string]navItem{}
	for _, s := range nav.Sections {
		for _, it := range s.Items {
			out[it.ID] = it
		}
	}
	return out
}

func TestNavigation_ExplainsMissingConfigurationAndUpdatesImmediately(t *testing.T) {
	e := newEnv(t)
	items := navItems(t, e)

	if items["dashboard"].Kind != "core" || items["settings"].Kind != "core" {
		t.Errorf("dashboard and settings must be core entries: %+v", items)
	}
	if items["media"].Status != "degraded" || items["media"].Name != "MediaHub" {
		t.Errorf("without a TMDB key MediaHub is degraded, got %+v", items["media"])
	}
	d := items["discovery"]
	if d.Status != "unavailable" || len(d.Problems) == 0 || d.Problems[0].Code != "connector.notConfigured" || d.Problems[0].FixParams["section"] != "ai" {
		t.Fatalf("without an AI key DiscoveryHub is unavailable with a fix pointing to the AI settings, got %+v", d)
	}

	if resp, b := e.do(t, "PATCH", "/api/v1/settings", `{"values":{"ai.openai.apiKey":"sk-test-1234"}}`, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("setting the key: %d %s", resp.StatusCode, b)
	}
	if got := navItems(t, e)["discovery"].Status; got != "available" {
		t.Errorf("DiscoveryHub must become available right after the key is saved (no waiting for the cache), got %s", got)
	}

	// The transition itself (unavailable -> available) must land in the
	// change log — internal/hub's StatusWatcher, wired in hubStatuses().
	_, b := e.do(t, "GET", "/api/v1/changes?limit=10", "", nil)
	if !strings.Contains(string(b), `"action":"hub.statusChanged"`) || !strings.Contains(string(b), "DiscoveryHub became available") {
		t.Errorf("expected a hub.statusChanged change log entry, got %s", b)
	}
}

func TestSearch_FindsARealTimerByName(t *testing.T) {
	e := newEnv(t)

	resp, b := e.do(t, "GET", "/api/v1/search?q=Rookie", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var body struct {
		Results []struct {
			HubID    string `json:"hubId"`
			HubLabel string `json:"hubLabel"`
			Title    string `json:"title"`
			Path     string `json:"path"`
		} `json:"results"`
	}
	json.Unmarshal(b, &body)
	if len(body.Results) != 1 || body.Results[0].Title != "The Rookie" || body.Results[0].HubID != "timer" || body.Results[0].Path != "/timers" {
		t.Fatalf("expected exactly the mock's \"The Rookie\" timer, got %+v", body.Results)
	}

	// No query, no results — not an error.
	_, b = e.do(t, "GET", "/api/v1/search", "", nil)
	var empty struct {
		Results []any `json:"results"`
	}
	json.Unmarshal(b, &empty)
	if len(empty.Results) != 0 {
		t.Errorf("expected no results for an empty query, got %+v", empty.Results)
	}
}

func TestSearch_SkipsADisabledHub(t *testing.T) {
	e := newEnv(t)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"hubs.timer.enabled":false}}`, nil)

	_, b := e.do(t, "GET", "/api/v1/search?q=Rookie", "", nil)
	var body struct {
		Results []any `json:"results"`
	}
	json.Unmarshal(b, &body)
	if len(body.Results) != 0 {
		t.Errorf("a disabled hub's timers must not be searchable, got %+v", body.Results)
	}
}

func TestTokens_CreateListRevoke(t *testing.T) {
	e := newEnv(t)

	resp, b := e.do(t, "POST", "/api/v1/tokens", `{"name":"My Phone"}`, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, b)
	}
	var created struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	json.Unmarshal(b, &created)
	if created.Name != "My Phone" || !strings.HasPrefix(created.Token, "dh_") {
		t.Fatalf("unexpected creation response: %+v", created)
	}

	// The new token authenticates once auth is enabled.
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"auth.enabled":true,"auth.username":"armin","auth.password":"correct horse"}}`, nil)
	resp, _ = e.do(t, "GET", "/api/v1/whoami", "", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+created.Token)
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("the created token must authenticate, got %d", resp.StatusCode)
	}
	resp, _ = e.do(t, "GET", "/api/v1/whoami", "", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer dh_not-a-real-token")
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("an unknown token must be rejected, got %d", resp.StatusCode)
	}
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"auth.enabled":false}}`, func(r *http.Request) { r.SetBasicAuth("armin", "correct horse") })

	_, b = e.do(t, "GET", "/api/v1/tokens", "", nil)
	var list struct {
		Tokens []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"tokens"`
	}
	json.Unmarshal(b, &list)
	if len(list.Tokens) != 1 || list.Tokens[0].ID != created.ID {
		t.Fatalf("expected the one created token, got %+v", list.Tokens)
	}
	if strings.Contains(string(b), created.Token) {
		t.Error("the token list must never include the secret")
	}

	resp, _ = e.do(t, "DELETE", "/api/v1/tokens/"+strconv.FormatInt(created.ID, 10), "", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
	resp, _ = e.do(t, "DELETE", "/api/v1/tokens/"+strconv.FormatInt(created.ID, 10), "", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("revoking again: expected 404, got %d", resp.StatusCode)
	}
}

func TestTokens_CreateRejectsEmptyName(t *testing.T) {
	e := newEnv(t)
	resp, _ := e.do(t, "POST", "/api/v1/tokens", `{"name":""}`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTokens_CreateRequiresCSRFHeader(t *testing.T) {
	e := newEnv(t)
	resp, _ := e.do(t, "POST", "/api/v1/tokens", `{"name":"x"}`, func(r *http.Request) { r.Header.Del(httpx.CSRFHeader) })
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestNavigation_DisabledHubDisappears(t *testing.T) {
	e := newEnv(t)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"hubs.series.enabled":false}}`, nil)
	if _, ok := navItems(t, e)["series"]; ok {
		t.Error("a disabled hub must not appear in the navigation")
	}
	// ...but it still shows up in diagnostics, marked as disabled.
	_, b := e.do(t, "GET", "/api/v1/hubs", "", nil)
	var body struct {
		Hubs []struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		} `json:"hubs"`
	}
	json.Unmarshal(b, &body)
	found := false
	for _, h := range body.Hubs {
		if h.ID == "series" {
			found = true
			if h.Enabled {
				t.Error("series must be reported as disabled")
			}
		}
	}
	if !found {
		t.Error("diagnostics must list disabled hubs too")
	}
}

func TestDashboard_GetReturnsTheFullCatalogByDefault(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/dashboard", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var body struct {
		Tiles []struct {
			Type   string `json:"type"`
			Label  string `json:"label"`
			Width  int    `json:"width"`
			Hidden bool   `json:"hidden"`
		} `json:"tiles"`
	}
	json.Unmarshal(b, &body)
	if len(body.Tiles) != 3 {
		t.Fatalf("expected the 3 built-in tiles on a fresh install, got %+v", body.Tiles)
	}
	for _, tile := range body.Tiles {
		if tile.Hidden || tile.Label == "" || tile.Width < 1 || tile.Width > 4 {
			t.Errorf("unexpected default tile: %+v", tile)
		}
	}
}

func TestDashboard_PutRoundTripsAndIsLogged(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "PUT", "/api/v1/dashboard",
		`{"tiles":[{"type":"hubs.status","width":1,"hidden":true},{"type":"timers.upcoming","width":4,"hidden":false}]}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	_, b = e.do(t, "GET", "/api/v1/dashboard", "", nil)
	var body struct {
		Tiles []struct {
			Type   string `json:"type"`
			Width  int    `json:"width"`
			Hidden bool   `json:"hidden"`
		} `json:"tiles"`
	}
	json.Unmarshal(b, &body)
	if len(body.Tiles) != 3 || body.Tiles[0].Type != "hubs.status" || !body.Tiles[0].Hidden || body.Tiles[0].Width != 1 {
		t.Fatalf("order/width/hidden not preserved: %+v", body.Tiles)
	}

	_, b = e.do(t, "GET", "/api/v1/changes?limit=1", "", nil)
	if !strings.Contains(string(b), "dashboard.update") {
		t.Errorf("expected the dashboard change to be logged, got %s", b)
	}
}

func TestDashboard_PutRejectsUnknownTileType(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "PUT", "/api/v1/dashboard", `{"tiles":[{"type":"made.up","width":2}]}`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, b)
	}
}

func TestDashboard_PutRejectsWidthOutOfRange(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "PUT", "/api/v1/dashboard", `{"tiles":[{"type":"timers.upcoming","width":9}]}`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, b)
	}
}

func TestDashboard_PutRequiresCSRFHeader(t *testing.T) {
	e := newEnv(t)
	resp, _ := e.do(t, "PUT", "/api/v1/dashboard", `{"tiles":[]}`, func(r *http.Request) { r.Header.Del(httpx.CSRFHeader) })
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 without the CSRF header, got %d", resp.StatusCode)
	}
}

func TestConnectors_NeverLeakSecrets(t *testing.T) {
	e := newEnv(t)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"ai.openai.apiKey":"sk-supersecret-9876","tmdb.apiKey":"tmdb-secret-5555"}}`, nil)
	for _, path := range []string{"/api/v1/connectors", "/api/v1/hubs", "/api/v1/navigation", "/api/v1/settings"} {
		_, b := e.do(t, "GET", path, "", nil)
		if strings.Contains(string(b), "supersecret") || strings.Contains(string(b), "tmdb-secret") {
			t.Errorf("%s leaks a secret: %s", path, b)
		}
	}
	_, b := e.do(t, "GET", "/api/v1/connectors", "", nil)
	var body struct {
		Connectors []struct{ ID, State string } `json:"connectors"`
	}
	json.Unmarshal(b, &body)
	states := map[string]string{}
	for _, c := range body.Connectors {
		states[c.ID] = c.State
	}
	for _, id := range []string{"webcontrol", "serienrecorder", "openai", "tmdb"} {
		if states[id] != "ok" {
			t.Errorf("mock mode with keys set: %s should be ok, got %q", id, states[id])
		}
	}
}
