package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// discardLogger swallows log output so test runs stay quiet.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestServer(t *testing.T, authEnabled bool) *httptest.Server {
	t.Helper()
	verifier := auth.Credentials{
		Enabled:  authEnabled,
		Username: "u",
		Password: "p",
		Tokens:   map[string]bool{"test-token": true},
	}
	handler := api.NewRouter(mock.Client{}, verifier, discardLogger())
	return httptest.NewServer(handler)
}

func TestHealth(t *testing.T) {
	srv := newTestServer(t, false)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" {
		t.Errorf("expected status ok, got %q", body.Status)
	}
	if body.Version != api.Version {
		t.Errorf("expected version %q, got %q", api.Version, body.Version)
	}
}

func TestDeviceInfo_UsesMock(t *testing.T) {
	srv := newTestServer(t, false)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/deviceinfo")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		DeviceName string `json:"deviceName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.DeviceName != "one" {
		t.Errorf("expected device name %q (from ARCHITECTURE.md fixture), got %q", "one", body.DeviceName)
	}
}

func TestMovies_UsesMock(t *testing.T) {
	srv := newTestServer(t, false)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/movies")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var movies []struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&movies); err != nil {
		t.Fatal(err)
	}
	if len(movies) != 1 {
		t.Fatalf("expected 1 fixture movie, got %d", len(movies))
	}
	if movies[0].Title != "Bauer sucht Frau International" {
		t.Errorf("unexpected movie title: %q", movies[0].Title)
	}
}

func TestHealth_PublicWhenAuthEnabled(t *testing.T) {
	srv := newTestServer(t, true)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health must stay public so the TV plugin's version check works; got %d", resp.StatusCode)
	}
}

func TestAuth_RequiredWhenEnabled(t *testing.T) {
	srv := newTestServer(t, true)
	defer srv.Close()

	get := func(configure func(*http.Request)) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/deviceinfo", nil)
		if configure != nil {
			configure(req)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if code := get(nil); code != http.StatusUnauthorized {
		t.Errorf("no credentials: expected 401, got %d", code)
	}
	if code := get(func(r *http.Request) { r.SetBasicAuth("u", "wrong") }); code != http.StatusUnauthorized {
		t.Errorf("wrong password: expected 401, got %d", code)
	}
	if code := get(func(r *http.Request) { r.SetBasicAuth("u", "p") }); code != http.StatusOK {
		t.Errorf("correct Basic Auth: expected 200, got %d", code)
	}
	if code := get(func(r *http.Request) { r.Header.Set("Authorization", "Bearer test-token") }); code != http.StatusOK {
		t.Errorf("valid API token: expected 200, got %d", code)
	}
	if code := get(func(r *http.Request) { r.Header.Set("Authorization", "Bearer revoked") }); code != http.StatusUnauthorized {
		t.Errorf("unknown API token: expected 401, got %d", code)
	}
}
