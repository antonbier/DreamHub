package httpx

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHostAllowed(t *testing.T) {
	allow := []string{"dreamhub.home.arpa"}
	for host, want := range map[string]bool{
		"192.168.1.43":            true,
		"192.168.1.43:8080":       true,
		"[fdb2:f390::1]:8080":     true,
		"localhost:8080":          true,
		"dreambox.local":          true,
		"dm920:8765":              true, // single-label LAN name
		"DreamBox.Local.":         true,
		"dreamhub.home.arpa:8080": true,
		"evil.example.com":        false,
		"192.168.1.43.nip.io":     false, // classic rebinding-style name
		"":                        false,
	} {
		if got := HostAllowed(host, allow); got != want {
			t.Errorf("HostAllowed(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestRequestLoggingQuietsExtensionReRegistration(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	status := http.StatusOK
	h := RequestIDAndLogging(logger, time.Now, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
	do := func(method, path string) {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, path, nil))
	}
	do("POST", "/api/v1/extensions/register")
	if buf.Len() != 0 {
		t.Fatalf("successful re-registration logged at INFO: %s", buf.String())
	}
	do("GET", "/api/v1/status")
	if !strings.Contains(buf.String(), "/api/v1/status") {
		t.Fatalf("normal request not logged: %q", buf.String())
	}
	buf.Reset()
	status = http.StatusBadRequest
	do("POST", "/api/v1/extensions/register")
	if buf.Len() == 0 {
		t.Fatal("a failed registration must still be logged")
	}
}

func TestRequestLoggingQuietsMissingPicon(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	h := RequestIDAndLogging(logger, time.Now, http.NotFoundHandler())
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/guide/picon?ref=x", nil))
	if buf.Len() != 0 {
		t.Fatalf("missing picon logged at INFO: %s", buf.String())
	}
}
