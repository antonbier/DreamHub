package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

var fixedNow = time.Date(2026, 9, 23, 18, 7, 0, 0, time.UTC)

type env struct {
	srv   *httptest.Server
	store *store.Store
	svc   *settings.Service
}

// newEnv builds the full router against the mock WebControl client and a
// real SQLite store in a temp dir. Auth comes from settings, like in
// production.
func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	user, err := st.EnsureDefaultUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reg := settings.NewRegistry()
	for _, s := range settings.CoreSections() {
		if err := reg.Register(s); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := settings.NewService(ctx, st, reg)
	if err != nil {
		t.Fatal(err)
	}
	h := api.NewRouter(api.Deps{
		WebControl: mock.Client{},
		Verifier:   auth.NewSettingsVerifier(svc),
		Settings:   svc,
		User:       user,
		Logger:     discardLogger(),
		Now:        func() time.Time { return fixedNow },
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{srv: srv, store: st, svc: svc}
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

func TestMovies_UsesMock(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/movies", "", nil)
	var movies []struct {
		Title string `json:"title"`
	}
	json.Unmarshal(b, &movies)
	if len(movies) != 1 || movies[0].Title != "Bauer sucht Frau International" {
		t.Errorf("unexpected movies: %s", b)
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
	if len(body.Sections) != 2 || body.Sections[0].ID != "general" || body.Sections[1].ID != "auth" {
		t.Fatalf("expected general and auth sections, got %s", b)
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

func TestSettings_PatchRejectsMalformedBody(t *testing.T) {
	e := newEnv(t)
	for _, body := range []string{`not json`, `{}`, `{"values":{},"extra":1}`} {
		if resp, _ := e.do(t, "PATCH", "/api/v1/settings", body, nil); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %q: expected 400, got %d", body, resp.StatusCode)
		}
	}
}
