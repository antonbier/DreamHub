package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

func TestControlScreenshot_ReturnsAPNG(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/control/screenshot", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected image/png, got %q", ct)
	}
	if len(b) == 0 {
		t.Error("expected a non-empty PNG body")
	}
}

func TestControlPower_ReportsStandby(t *testing.T) {
	m := mock.New()
	m.Standby = true
	e := newEnvWith(t, m)
	_, b := e.do(t, "GET", "/api/v1/control/power", "", nil)
	var body struct {
		InStandby bool `json:"inStandby"`
	}
	json.Unmarshal(b, &body)
	if !body.InStandby {
		t.Errorf("expected inStandby=true, got %s", b)
	}
}

func TestControlKey_SendsToTheBoxAndValidates(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)

	resp, b := e.do(t, "POST", "/api/v1/control/key", `{"code":352}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	if m.LastKey != 352 {
		t.Errorf("expected the box to receive key 352, got %d", m.LastKey)
	}

	resp2, _ := e.do(t, "POST", "/api/v1/control/key", `{"code":0}`, nil)
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for a non-positive code, got %d", resp2.StatusCode)
	}

	resp3, _ := e.do(t, "POST", "/api/v1/control/key", `{"code":99999}`, nil)
	if resp3.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for an out-of-range code, got %d", resp3.StatusCode)
	}
}

func TestControlZap_SwitchesChannel(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)

	resp, b := e.do(t, "POST", "/api/v1/control/zap", `{"serviceReference":"1:0:19:2EE3:441:1:C00000:0:0:0:"}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	if m.LastZapped != "1:0:19:2EE3:441:1:C00000:0:0:0:" {
		t.Errorf("expected the box to zap to the requested channel, got %q", m.LastZapped)
	}

	resp2, _ := e.do(t, "POST", "/api/v1/control/zap", `{}`, nil)
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for a missing serviceReference, got %d", resp2.StatusCode)
	}
}

func TestControlMessage(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	resp, b := e.do(t, "POST", "/api/v1/control/message", `{"text":"  Essen ist fertig!  ","kind":"info","timeout":10}`, nil)
	if resp.StatusCode != http.StatusOK || m.LastMessage != "Essen ist fertig!" {
		t.Fatalf("expected the trimmed text on the box, got %d %s / %q", resp.StatusCode, b, m.LastMessage)
	}
	for _, bad := range []string{`{"text":""}`, `{"text":"x","kind":"shout"}`, `{"text":"x","timeout":-1}`, `{"text":"` + strings.Repeat("a", 301) + `"}`} {
		if resp, _ := e.do(t, "POST", "/api/v1/control/message", bad, nil); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", bad, resp.StatusCode)
		}
	}
}
