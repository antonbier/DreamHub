package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/antonbier/DreamHub/backend/internal/autotimer"
	atmock "github.com/antonbier/DreamHub/backend/internal/autotimer/mock"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	tmdbmock "github.com/antonbier/DreamHub/backend/internal/tmdb/mock"
	tvmazemock "github.com/antonbier/DreamHub/backend/internal/tvmaze/mock"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	aimock "github.com/antonbier/DreamHub/backend/internal/ai/mock"
	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/app"
	"github.com/antonbier/DreamHub/backend/internal/events"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/journald"
	jmock "github.com/antonbier/DreamHub/backend/internal/journald/mock"
	srmock "github.com/antonbier/DreamHub/backend/internal/serienrecorder/mock"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

var fixedNow = time.Date(2026, 9, 23, 18, 7, 0, 0, time.UTC)

type env struct {
	srv    *httptest.Server
	store  *store.Store
	svc    *settings.Service
	events *events.Bus
}

// newEnv builds the application exactly as production does (internal/app),
// against the mock WebControl client and a real SQLite store in a temp dir.
func newEnv(t *testing.T) *env { return newEnvWith(t, mock.New()) }

func newEnvWith(t *testing.T, wc webcontrol.Client) *env {
	return newEnvWithAutoTimer(t, wc, atmock.New())
}

func newEnvWithAutoTimer(t *testing.T, wc webcontrol.Client, at autotimer.Client) *env {
	t.Helper()
	// Recordings live in the mock box's in-memory FS, as in mock mode.
	var recFS recfs.FS
	if m, ok := wc.(*mock.Client); ok {
		recFS = m.FS
	}
	return newEnvWithOptions(t, wc, at, recFS)
}

func newEnvWithOptions(t *testing.T, wc webcontrol.Client, at autotimer.Client, recFS recfs.FS) *env {
	return newEnvFull(t, wc, at, recFS, false, jmock.New())
}

func newEnvWithReadOnlyFiles(t *testing.T, m *mock.Client) *env {
	return newEnvFull(t, m, atmock.New(), m.FS, true, jmock.New())
}

// newEnvWithLogs is for AdminHub tests that need a journald.Reader they
// control the content of, rather than the fixed mock fixture.
func newEnvWithLogs(t *testing.T, logs journald.Reader) *env {
	m := mock.New()
	return newEnvFull(t, m, atmock.New(), m.FS, false, logs)
}

func newEnvFull(t *testing.T, wc webcontrol.Client, at autotimer.Client, recFS recfs.FS, readOnly bool, logs journald.Reader) *env {
	t.Helper()
	a, err := app.New(context.Background(), app.Options{
		ReadOnlyFiles: readOnly,
		TMDB:          tmdbmock.New(),
		TVmaze:        tvmazemock.New(),
		DataDir:       t.TempDir(),
		WebControl:    wc,
		AutoTimer:     at,
		RecordingsFS:  recFS,
		Series:        srmock.New(),
		AI:            aimock.New(),
		Logs:          logs,
		Mock:          true,
		LookPath:      func(string) (string, error) { return "/usr/bin/journalctl", nil },
		Logger:        discardLogger(),
		Now:           func() time.Time { return fixedNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Store.Close() })
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(srv.Close)
	return &env{srv: srv, store: a.Store, svc: a.Settings, events: a.Events}
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

func TestQR_ReturnsAPNGWithoutAuth(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Get(e.srv.URL + "/api/v1/qr.png") // no Authorization header at all
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected image/png, got %q", ct)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) {
		t.Errorf("expected a PNG file, got %d bytes starting %x", len(b), b[:min(8, len(b))])
	}
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
	if len(body.Results) != 1 || body.Results[0].Title != "Oskars Kleid.mkv" || body.Results[0].HubID != "media" || !strings.HasPrefix(body.Results[0].Path, "/media") {
		t.Fatalf("expected exactly the mock's non-recording file, got %+v", body.Results)
	}
}

func TestGuide_BouquetsAndGrid(t *testing.T) {
	e := newEnv(t)

	_, b := e.do(t, "GET", "/api/v1/guide/bouquets", "", nil)
	var bouquets struct {
		Bouquets []struct {
			ServiceReference string `json:"serviceReference"`
			ServiceName      string `json:"serviceName"`
		} `json:"bouquets"`
	}
	json.Unmarshal(b, &bouquets)
	if len(bouquets.Bouquets) != 1 || bouquets.Bouquets[0].ServiceName != "Free TV" {
		t.Fatalf("unexpected bouquets: %+v", bouquets.Bouquets)
	}
	bRef := bouquets.Bouquets[0].ServiceReference

	// A 3-hour window covering the mock's evening schedule (mock.Evening,
	// 19:00-22:00 local) should return several events per channel, in order,
	// not just now/next.
	windowStart := mock.Evening.Add(19 * time.Hour).Unix()
	windowEnd := mock.Evening.Add(22 * time.Hour).Unix()
	q := url.Values{
		"bouquet": {bRef},
		"start":   {strconv.FormatInt(windowStart, 10)},
		"end":     {strconv.FormatInt(windowEnd, 10)},
	}
	resp, b := e.do(t, "GET", "/api/v1/guide/grid?"+q.Encode(), "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var grid struct {
		Channels []struct {
			ServiceName string `json:"serviceName"`
			Events      []struct {
				Title string `json:"title"`
			} `json:"events"`
		} `json:"channels"`
	}
	json.Unmarshal(b, &grid)
	if len(grid.Channels) != 3 {
		t.Fatalf("expected 3 channels, got %+v", grid.Channels)
	}
	for _, ch := range grid.Channels {
		if len(ch.Events) < 2 {
			t.Errorf("channel %q: expected several events in a 3-hour window, got %+v", ch.ServiceName, ch.Events)
		}
	}

	for _, missing := range []string{
		"start=" + strconv.FormatInt(windowStart, 10) + "&end=" + strconv.FormatInt(windowEnd, 10), // no bouquet
		"bouquet=" + url.QueryEscape(bRef) + "&end=" + strconv.FormatInt(windowEnd, 10),            // no start
		"bouquet=" + url.QueryEscape(bRef) + "&start=" + strconv.FormatInt(windowStart, 10),        // no end
	} {
		resp, _ := e.do(t, "GET", "/api/v1/guide/grid?"+missing, "", nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for %q, got %d", missing, resp.StatusCode)
		}
	}
}

func TestSeries_MarkersAndUpcoming(t *testing.T) {
	e := newEnv(t)

	_, b := e.do(t, "GET", "/api/v1/series/markers", "", nil)
	var markers struct {
		Markers []struct {
			Name string `json:"name"`
		} `json:"markers"`
	}
	json.Unmarshal(b, &markers)
	if len(markers.Markers) != 3 || markers.Markers[0].Name != "And Just Like That..." {
		t.Fatalf("unexpected markers: %+v", markers.Markers)
	}

	resp, b := e.do(t, "GET", "/api/v1/series/upcoming", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var upcoming struct {
		Episodes []struct {
			Series string `json:"series"`
			Title  string `json:"title"`
		} `json:"episodes"`
	}
	json.Unmarshal(b, &upcoming)
	if len(upcoming.Episodes) != 3 || upcoming.Episodes[0].Series != "Gute Zeiten, schlechte Zeiten" {
		t.Fatalf("unexpected upcoming episodes: %+v", upcoming.Episodes)
	}
}

func TestSearch_FindsARealSeriesByName(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/search?q=Verr%C3%A4ter", "", nil)
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
	if len(body.Results) != 1 || body.Results[0].HubID != "series" || body.Results[0].Path != "/series/10" {
		t.Fatalf("expected exactly the mock's tracked series, got %+v", body.Results)
	}
}

func TestDiscovery_StatusReportsUsageAndLimit(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/discovery/status", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var body struct {
		UsedToday  int   `json:"usedToday"`
		DailyLimit int64 `json:"dailyLimit"`
	}
	json.Unmarshal(b, &body)
	if body.UsedToday != 0 || body.DailyLimit != 200 {
		t.Errorf("expected 0 used of the default 200/day, got %+v", body)
	}
}

func TestDiscovery_GenerateRequiresAnAPIKey(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "POST", "/api/v1/discovery/recommendations", `{"bouquet":"any"}`, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 without an API key configured, got %d: %s", resp.StatusCode, b)
	}
}

func TestDiscovery_GenerateReturnsGroundedRecommendationsAndSpendsBudget(t *testing.T) {
	e := newEnv(t)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"ai.openai.apiKey":"sk-test-1234"}}`, nil)

	bRef := `1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "userbouquet.favourites.tv" ORDER BY bouquet`
	body := `{"bouquet":` + strconv.Quote(bRef) + `}`
	resp, b := e.do(t, "POST", "/api/v1/discovery/recommendations", body, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Recommendations []struct {
			Title            string `json:"title"`
			ServiceReference string `json:"serviceReference"`
			Reason           string `json:"reason"`
		} `json:"recommendations"`
		UsedToday  int   `json:"usedToday"`
		DailyLimit int64 `json:"dailyLimit"`
	}
	json.Unmarshal(b, &out)
	if len(out.Recommendations) == 0 {
		t.Fatalf("expected at least one recommendation from the mock's evening schedule, got %+v", out)
	}
	for _, r := range out.Recommendations {
		if r.Title == "" || r.ServiceReference == "" || r.Reason == "" {
			t.Errorf("every recommendation must be grounded in a real event: %+v", r)
		}
	}
	if out.UsedToday != 1 {
		t.Errorf("expected usedToday=1 after one call, got %d", out.UsedToday)
	}

	_, b = e.do(t, "GET", "/api/v1/discovery/status", "", nil)
	var status struct {
		UsedToday int `json:"usedToday"`
	}
	json.Unmarshal(b, &status)
	if status.UsedToday != 1 {
		t.Errorf("expected status to reflect the spent budget, got %+v", status)
	}
}

func TestDiscovery_GenerateRespectsTheDailyLimit(t *testing.T) {
	e := newEnv(t)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"ai.openai.apiKey":"sk-test-1234","ai.dailyLimit":0}}`, nil)

	bRef := `1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "userbouquet.favourites.tv" ORDER BY bouquet`
	body := `{"bouquet":` + strconv.Quote(bRef) + `}`
	resp, b := e.do(t, "POST", "/api/v1/discovery/recommendations", body, nil)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 with a 0/day limit, got %d: %s", resp.StatusCode, b)
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
	if strings.Join(ids, ",") != "general,auth,network,webcontrol,ai,tmdb,tvmaze,media,hubs" {
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
	// GuideHub contributes the EPG's matching programmes too; the timer
	// must be among the results, from TimerHub, linking to /timers.
	var timerHits int
	for _, r := range body.Results {
		if r.HubID == "timer" {
			timerHits++
			if r.Title != "The Rookie" || r.Path != "/timers" {
				t.Errorf("unexpected timer result: %+v", r)
			}
		}
	}
	if timerHits != 1 {
		t.Fatalf("expected exactly the mock's \"The Rookie\" timer from TimerHub, got %+v", body.Results)
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
		Results []struct {
			HubID string `json:"hubId"`
		} `json:"results"`
	}
	json.Unmarshal(b, &body)
	for _, r := range body.Results {
		if r.HubID == "timer" {
			t.Errorf("a disabled hub's timers must not be searchable, got %+v", body.Results)
		}
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
	if len(body.Tiles) != 5 {
		t.Fatalf("expected the 5 built-in tiles on a fresh install, got %+v", body.Tiles)
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
	if len(body.Tiles) != 5 || body.Tiles[0].Type != "hubs.status" || !body.Tiles[0].Hidden || body.Tiles[0].Width != 1 {
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

func TestSeries_Detail(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/series/markers/11", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var d struct {
		Marker   struct{ Name string } `json:"marker"`
		Upcoming []struct {
			Title string `json:"title"`
		} `json:"upcoming"`
		Recordings []struct {
			Title string `json:"title"`
			Path  string `json:"path"`
		} `json:"recordings"`
		Broadcasts []struct {
			Title    string `json:"title"`
			HasTimer bool   `json:"hasTimer"`
		} `json:"broadcasts"`
		Unavailable []string `json:"unavailable"`
	}
	json.Unmarshal(b, &d)
	if d.Marker.Name != "Tatort" || len(d.Unavailable) != 0 {
		t.Fatalf("unexpected marker/unavailable: %s", b)
	}
	if len(d.Upcoming) != 1 || d.Upcoming[0].Title != "Borowski und der gute Mensch" {
		t.Errorf("upcoming should hold only this series' episodes: %+v", d.Upcoming)
	}
	if len(d.Recordings) != 1 || !strings.Contains(d.Recordings[0].Path, "/Tatort/") {
		t.Errorf("expected the recording in the series folder: %+v", d.Recordings)
	}
	// "Tatort-Reiniger Spezial" is found by the box's substring search but
	// isn't this series.
	if len(d.Broadcasts) != 1 || d.Broadcasts[0].Title != "Tatort: Borowski und der gute Mensch" {
		t.Errorf("unexpected broadcasts: %+v", d.Broadcasts)
	}

	if resp, _ := e.do(t, "GET", "/api/v1/series/markers/999", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown marker: expected 404, got %d", resp.StatusCode)
	}
}

func TestDashboard_CustomTiles(t *testing.T) {
	e := newEnv(t)

	resp, b := e.do(t, "GET", "/api/v1/dashboard/sources", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"media.recordings"`) || !strings.Contains(string(b), `"guide.broadcasts"`) {
		t.Fatalf("expected the view sources, got %d: %s", resp.StatusCode, b)
	}

	// Evaluate a view: the mock's timers still to come at fixedNow, the
	// conflicting ones only.
	resp, b = e.do(t, "POST", "/api/v1/dashboard/view", `{"source":"timers.upcoming","query":{"warnOnly":true},"timeZone":"Europe/Rome"}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("view: expected 200, got %d: %s", resp.StatusCode, b)
	}
	var view struct {
		Items []struct {
			Title string `json:"title"`
			Warn  bool   `json:"warn"`
		} `json:"items"`
		Total int `json:"total"`
	}
	json.Unmarshal(b, &view)
	if view.Total != 2 || !view.Items[0].Warn {
		t.Errorf("expected the two overlapping mock timers: %s", b)
	}
	if resp, _ := e.do(t, "POST", "/api/v1/dashboard/view", `{"source":"nope"}`, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown source: expected 404, got %d", resp.StatusCode)
	}

	// Save a custom tile: it gets an ID, keeps its criteria, sits among the
	// built-ins; an unknown source or an empty title is refused.
	resp, b = e.do(t, "PUT", "/api/v1/dashboard",
		`{"tiles":[{"type":"custom","width":2,"custom":{"title":"Aufräumen","source":"media.recordings","display":"list","query":{"olderThanDays":90,"minSizeGB":3,"sort":"largest","limit":5}}},{"type":"hubs.status","width":1}]}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save custom tile: %d %s", resp.StatusCode, b)
	}
	var layout struct {
		Tiles []struct {
			Type   string `json:"type"`
			ID     string `json:"id"`
			Custom *struct {
				Title string `json:"title"`
				Query struct {
					MinSizeGB float64 `json:"minSizeGB"`
				} `json:"query"`
			} `json:"custom"`
		} `json:"tiles"`
	}
	_, b = e.do(t, "GET", "/api/v1/dashboard", "", nil)
	json.Unmarshal(b, &layout)
	if len(layout.Tiles) != 6 || layout.Tiles[0].Type != "custom" || layout.Tiles[0].ID == "" ||
		layout.Tiles[0].Custom == nil || layout.Tiles[0].Custom.Title != "Aufräumen" || layout.Tiles[0].Custom.Query.MinSizeGB != 3 {
		t.Fatalf("custom tile not saved as expected: %s", b)
	}
	for _, bad := range []string{
		`{"tiles":[{"type":"custom","width":2,"custom":{"title":"x","source":"nope","display":"list"}}]}`,
		`{"tiles":[{"type":"custom","width":2,"custom":{"title":" ","source":"media.recordings","display":"list"}}]}`,
		`{"tiles":[{"type":"custom","width":2,"custom":{"title":"x","source":"media.recordings","display":"poster"}}]}`,
	} {
		if resp, b := e.do(t, "PUT", "/api/v1/dashboard", bad, nil); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for %s, got %d: %s", bad, resp.StatusCode, b)
		}
	}
}

func TestDiscovery_TasteProfile(t *testing.T) {
	e := newEnv(t)

	// What "use my taste" would send, shown before anything is sent.
	resp, b := e.do(t, "GET", "/api/v1/discovery/taste", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var taste struct {
		Recordings []string `json:"recordings"`
		Series     []string `json:"series"`
		Timers     []string `json:"timers"`
	}
	json.Unmarshal(b, &taste)
	if len(taste.Recordings) == 0 || len(taste.Series) != 3 || len(taste.Timers) == 0 {
		t.Fatalf("expected recordings, the mock's 3 active series and upcoming timers: %s", b)
	}

	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"ai.openai.apiKey":"sk-test-1234"}}`, nil)
	bRef := `1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "userbouquet.favourites.tv" ORDER BY bouquet`
	var out struct {
		Recommendations []struct {
			Title string `json:"title"`
		} `json:"recommendations"`
		Taste struct {
			Series int `json:"series"`
		} `json:"taste"`
	}
	_, b = e.do(t, "POST", "/api/v1/discovery/recommendations", `{"bouquet":`+strconv.Quote(bRef)+`,"useTaste":true}`, nil)
	json.Unmarshal(b, &out)
	if out.Taste.Series != 3 {
		t.Errorf("useTaste must send the profile along: %s", b)
	}
	for _, r := range out.Recommendations {
		if r.Title == "Bauer sucht Frau International" || r.Title == "The Rookie" {
			t.Errorf("a broadcast a timer already covers must not be recommended: %s", b)
		}
	}

	_, b = e.do(t, "POST", "/api/v1/discovery/recommendations", `{"bouquet":`+strconv.Quote(bRef)+`}`, nil)
	out.Taste.Series = -1
	json.Unmarshal(b, &out)
	if out.Taste.Series != 0 {
		t.Errorf("without useTaste nothing about the user is sent: %s", b)
	}
}

func TestEnrich_LookupAndImages(t *testing.T) {
	e := newEnv(t)
	body := `{"items":[{"title":"Tatort: Borowski und der gute Mensch"},{"title":"Bones Spezial"}]}`

	// No key: switched off, not an error.
	resp, b := e.do(t, "POST", "/api/v1/enrich/lookup", body, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"enabled":false`) || !strings.Contains(string(b), `[null,null]`) {
		t.Fatalf("without a key: %d %s", resp.StatusCode, b)
	}

	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"tmdb.apiKey":"k-1234567890"}}`, nil)
	_, b = e.do(t, "POST", "/api/v1/enrich/lookup", body, nil)
	var out struct {
		Enabled bool `json:"enabled"`
		Results []*struct {
			Title  string   `json:"title"`
			Poster string   `json:"poster"`
			Rating float64  `json:"rating"`
			Genres []string `json:"genres"`
		} `json:"results"`
	}
	json.Unmarshal(b, &out)
	if !out.Enabled || len(out.Results) != 2 || out.Results[0] == nil || out.Results[0].Title != "Tatort" || out.Results[0].Poster == "" || out.Results[1] != nil {
		t.Fatalf("lookup: %s", b)
	}

	resp, b = e.do(t, "GET", "/api/v1/enrich/image/w185"+out.Results[0].Poster, "", nil)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" || len(b) == 0 {
		t.Errorf("image: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	for _, bad := range []string{"/api/v1/enrich/image/original/x.jpg", "/api/v1/enrich/image/w185/..%2Fx.jpg", "/api/v1/enrich/image/w185/x.svg"} {
		if resp, _ := e.do(t, "GET", bad, "", nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: expected 404, got %d", bad, resp.StatusCode)
		}
	}

	// Switched off in settings: nothing is looked up any more.
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"tmdb.enrich":false}}`, nil)
	_, b = e.do(t, "POST", "/api/v1/enrich/lookup", body, nil)
	if !strings.Contains(string(b), `"enabled":false`) {
		t.Errorf("enrich off: %s", b)
	}
}

func TestSearch_FindsRecordingsInSubfolders(t *testing.T) {
	e := newEnv(t)
	// The Tatort recording sits in a subfolder /web/movielist's top-folder
	// answer doesn't include; the recording index (on the box: the file
	// system) does.
	_, b := e.do(t, "GET", "/api/v1/search?q=Haus%20am%20Meer", "", nil)
	var body struct {
		Results []struct {
			HubID string `json:"hubId"`
			Title string `json:"title"`
			Path  string `json:"path"`
		} `json:"results"`
	}
	json.Unmarshal(b, &body)
	if len(body.Results) != 1 || body.Results[0].HubID != "media" ||
		body.Results[0].Path != "/media?dir="+url.QueryEscape("/media/hdd/movie/Tatort/") {
		t.Fatalf("expected the subfolder recording, opening its folder: %s", b)
	}
}

func TestSeries_EpisodeGuide(t *testing.T) {
	box := mock.New()
	// One recorded episode, with its code in the description as
	// SerienRecorder writes it.
	base := mock.Root + "20231005 2015 - RTL - Die Verräter"
	box.FS.Put(base+".ts", "recording")
	box.FS.Put(base+".ts.meta", "1:0:19:2EE3:441:1:C00000:0:0:0:\nDie Verräter - Vertraue Niemandem!\nS01E01 - Die Burg\n1696532400\n")
	e := newEnvWith(t, box)

	type guide struct {
		Episodes struct {
			State     string `json:"state"`
			Matchable bool   `json:"matchable"`
			Seasons   []struct {
				Season   int `json:"season"`
				Missing  int `json:"missing"`
				Episodes []struct {
					Number int    `json:"number"`
					Status string `json:"status"`
				} `json:"episodes"`
			} `json:"seasons"`
		} `json:"episodes"`
	}
	var g guide
	_, b := e.do(t, "GET", "/api/v1/series/markers/10", "", nil)
	json.Unmarshal(b, &g)
	if g.Episodes.State != "ok" || !g.Episodes.Matchable || len(g.Episodes.Seasons) != 2 || g.Episodes.Seasons[0].Season != 2 {
		t.Fatalf("guide, newest season first: %s", b)
	}
	s1 := g.Episodes.Seasons[1]
	if s1.Missing != 2 || s1.Episodes[0].Status != "recorded" || s1.Episodes[1].Status != "missing" {
		t.Errorf("season 1: E1 recorded, E2+E3 missing: %+v", s1)
	}
	if st := g.Episodes.Seasons[0].Episodes[1].Status; st != "upcoming" {
		t.Errorf("an episode still to come isn't missing: %s", st)
	}

	// Tatort's recording carries no code: nothing is called missing, and
	// SerienRecorder's timered E31 counts as planned.
	g = guide{}
	_, b = e.do(t, "GET", "/api/v1/series/markers/11", "", nil)
	json.Unmarshal(b, &g)
	if g.Episodes.State != "ok" || g.Episodes.Matchable || len(g.Episodes.Seasons) != 1 {
		t.Fatalf("Tatort guide: %s", b)
	}
	eps := g.Episodes.Seasons[0].Episodes
	if eps[0].Status != "aired" || eps[1].Status != "planned" {
		t.Errorf("Tatort: E30 aired (unmatchable), E31 planned: %+v", eps)
	}

	// Switched off: nothing is asked.
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"tvmaze.enabled":false}}`, nil)
	g = guide{}
	_, b = e.do(t, "GET", "/api/v1/series/markers/10", "", nil)
	json.Unmarshal(b, &g)
	if g.Episodes.State != "off" || len(g.Episodes.Seasons) != 0 {
		t.Errorf("TVmaze off: %s", b)
	}
}
