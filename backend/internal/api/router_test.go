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
	verifier := auth.Credentials{Enabled: authEnabled, Username: "u", Password: "p"}
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

func TestAuth_RequiredWhenEnabled(t *testing.T) {
	srv := newTestServer(t, true)
	defer srv.Close()

	// No credentials at all -> 401.
	resp, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without credentials, got %d", resp.StatusCode)
	}

	// Correct Basic Auth -> 200.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/health", nil)
	req.SetBasicAuth("u", "p")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with correct credentials, got %d", resp2.StatusCode)
	}
}
