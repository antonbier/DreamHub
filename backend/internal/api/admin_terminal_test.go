package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type terminalStatus struct {
	Available bool `json:"available"`
	AuthOn    bool `json:"authOn"`
	Enabled   bool `json:"enabled"`
}

func (e *env) terminalStatus(t *testing.T, configure func(*http.Request)) terminalStatus {
	t.Helper()
	_, b := e.do(t, "GET", "/api/v1/admin/terminal/status", "", configure)
	var s terminalStatus
	json.Unmarshal(b, &s)
	return s
}

func TestAdminTerminal_NeedsAuthEnabledFirst(t *testing.T) {
	e := newEnv(t)
	if s := e.terminalStatus(t, nil); !s.Available || s.AuthOn || s.Enabled {
		t.Fatalf("status: %+v", s)
	}
	resp, _ := e.do(t, "POST", "/api/v1/admin/terminal/enable", `{"password":"anything"}`, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 auth_required, got %d", resp.StatusCode)
	}
}

func TestAdminTerminal_WrongPasswordRejected(t *testing.T) {
	e := newEnv(t)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"auth.enabled":true,"auth.username":"armin","auth.password":"correct horse"}}`, nil)
	auth := func(r *http.Request) { r.SetBasicAuth("armin", "correct horse") }

	resp, _ := e.do(t, "POST", "/api/v1/admin/terminal/enable", `{"password":"wrong"}`, auth)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a wrong password, got %d", resp.StatusCode)
	}
	if s := e.terminalStatus(t, auth); !s.AuthOn || s.Enabled {
		t.Fatalf("status after a rejected enable: %+v", s)
	}
}

func TestAdminTerminal_EnableDisableAndProxy(t *testing.T) {
	shell := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer shell.Close()

	e := newEnvWithTerminalTarget(t, shell.URL)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"auth.enabled":true,"auth.username":"armin","auth.password":"correct horse"}}`, nil)
	auth := func(r *http.Request) { r.SetBasicAuth("armin", "correct horse") }

	// Before enabling: the proxy refuses.
	if resp, _ := e.do(t, "GET", "/api/v1/admin/terminal/proxy/", "", auth); resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 before enabling, got %d", resp.StatusCode)
	}

	resp, _ := e.do(t, "POST", "/api/v1/admin/terminal/enable", `{"password":"correct horse"}`, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enable: %d", resp.StatusCode)
	}
	if s := e.terminalStatus(t, auth); !s.Enabled {
		t.Fatalf("status after enable: %+v", s)
	}

	resp, _ = e.do(t, "GET", "/api/v1/admin/terminal/proxy/some/path", "", auth)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Seen-Path") != "/some/path" {
		t.Fatalf("proxy didn't reach the fake shellinaboxd with the stripped path: %d %q", resp.StatusCode, resp.Header.Get("X-Seen-Path"))
	}

	e.do(t, "POST", "/api/v1/admin/terminal/disable", "", auth)
	if s := e.terminalStatus(t, auth); s.Enabled {
		t.Fatalf("status after disable: %+v", s)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/admin/terminal/proxy/", "", auth); resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 after disabling, got %d", resp.StatusCode)
	}
}

// Switching password protection off later must close the terminal again.
func TestAdminTerminal_ClosesWhenAuthIsSwitchedOff(t *testing.T) {
	shell := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer shell.Close()
	e := newEnvWithTerminalTarget(t, shell.URL)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"auth.enabled":true,"auth.username":"armin","auth.password":"correct horse"}}`, nil)
	auth := func(r *http.Request) { r.SetBasicAuth("armin", "correct horse") }
	if resp, _ := e.do(t, "POST", "/api/v1/admin/terminal/enable", `{"password":"correct horse"}`, auth); resp.StatusCode != http.StatusOK {
		t.Fatalf("enable: %d", resp.StatusCode)
	}
	if resp, b := e.do(t, "PATCH", "/api/v1/settings", `{"values":{"auth.enabled":false}}`, auth); resp.StatusCode != http.StatusOK {
		t.Fatalf("auth off: %d %s", resp.StatusCode, b)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/admin/terminal/proxy/", "", nil); resp.StatusCode != http.StatusConflict {
		t.Fatalf("proxy stayed open without password protection: %d", resp.StatusCode)
	}
	if s := e.terminalStatus(t, nil); s.Enabled {
		t.Fatalf("status still enabled: %+v", s)
	}
}
