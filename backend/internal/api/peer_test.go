package api_test

import (
	"github.com/antonbier/DreamHub/backend/internal/boxes"
	wcmock "github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
	"net/http"
	"strings"
	"testing"

	peermock "github.com/antonbier/DreamHub/backend/internal/peer/mock"
)

type pairedBox struct {
	ID           string
	Paired       bool
	Capabilities []string
	Peer         *struct{ State, Version, Compat string }
}

func (e *env) boxView(t *testing.T, id string, auth func(*http.Request)) pairedBox {
	t.Helper()
	var l struct{ Boxes []pairedBox }
	e.json(t, "GET", "/api/v1/boxes", "", auth, 200, &l)
	for _, b := range l.Boxes {
		if b.ID == id {
			return b
		}
	}
	t.Fatalf("box %s not listed: %+v", id, l)
	return pairedBox{}
}

// Two real DreamHubs over HTTP: the box shows a code, the hub redeems it,
// the token works until it is revoked on the box.
func TestPairing_TwoDreamHubsOverHTTP(t *testing.T) {
	realPeer = true
	t.Cleanup(func() { realPeer = false })
	target, hub := newEnv(t), newEnv(t)
	// The target has a login, as a box on a LAN would.
	target.do(t, "PATCH", "/api/v1/settings", `{"values":{"auth.enabled":true,"auth.username":"armin","auth.password":"correct horse"}}`, nil)
	basic := func(r *http.Request) { r.SetBasicAuth("armin", "correct horse") }

	hub.json(t, "POST", "/api/v1/boxes", `{"name":"Bedroom","url":"192.168.1.30"}`, nil, 201, nil)
	pair := func(code string, want int) string {
		resp, b := hub.do(t, "POST", "/api/v1/boxes/bedroom/pair", `{"code":"`+code+`","url":"`+target.srv.URL+`"}`, nil)
		if resp.StatusCode != want {
			t.Fatalf("pair %q: expected %d, got %d: %s", code, want, resp.StatusCode, b)
		}
		return string(b)
	}

	// The handshake needs no login.
	var info struct {
		Version, DeviceName string
		API                 int
		Pairing             bool
	}
	target.json(t, "GET", "/api/v1/peer/info", "", nil, 200, &info)
	if info.API != 1 || info.Version == "" || info.Pairing {
		t.Fatalf("info: %+v", info)
	}

	// No code shown yet.
	if b := pair("1234 5678", 422); !strings.Contains(b, "peer_not_pairing") {
		t.Errorf("expected peer_not_pairing: %s", b)
	}
	// Showing one is for administrators.
	if resp, _ := target.do(t, "POST", "/api/v1/peer/code", "", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a code without a login: %d", resp.StatusCode)
	}
	var code struct{ Code string }
	target.json(t, "POST", "/api/v1/peer/code", "", basic, 200, &code)
	if len(strings.ReplaceAll(code.Code, " ", "")) != 8 {
		t.Fatalf("code: %q", code.Code)
	}
	if b := pair("0000 0000", 422); !strings.Contains(b, "peer_bad_code") {
		t.Errorf("expected peer_bad_code: %s", b)
	}
	pair(code.Code, 200)

	bv := hub.boxView(t, "bedroom", nil)
	if !bv.Paired || bv.Peer == nil || bv.Peer.State != "ok" || bv.Peer.Compat != "ok" {
		t.Fatalf("paired box: %+v", bv)
	}
	// The other DreamHub's own capabilities are part of the box's.
	has := false
	for _, c := range bv.Capabilities {
		has = has || c == "recordings.local"
	}
	if !has {
		t.Errorf("capabilities of a paired box should list what its DreamHub does: %v", bv.Capabilities)
	}
	// The pairing left a token on the box, revocable like any other.
	var tokens struct {
		Tokens []struct {
			ID   int64
			Name string
		}
	}
	target.json(t, "GET", "/api/v1/tokens", "", basic, 200, &tokens)
	if len(tokens.Tokens) != 1 || !strings.HasPrefix(tokens.Tokens[0].Name, "Hub: ") {
		t.Fatalf("tokens on the box: %+v", tokens)
	}

	// A code is good once.
	if b := pair(code.Code, 422); !strings.Contains(b, "peer_not_pairing") {
		t.Errorf("a used code must not work again: %s", b)
	}

	// Revoked on the box: the hub says so instead of failing oddly.
	target.json(t, "DELETE", "/api/v1/tokens/"+itoa(tokens.Tokens[0].ID), "", basic, 204, nil)
	if bv := hub.boxView(t, "bedroom", nil); bv.Peer == nil || bv.Peer.State != "revoked" {
		t.Errorf("a revoked token: %+v", bv.Peer)
	}
	hub.json(t, "DELETE", "/api/v1/boxes/bedroom/pair", "", nil, 200, nil)
	if bv := hub.boxView(t, "bedroom", nil); bv.Paired || bv.Peer != nil {
		t.Errorf("after unpairing: %+v", bv)
	}
}

// Five wrong guesses end the code, even if the sixth would be right.
func TestPairing_WrongGuessesEndTheCode(t *testing.T) {
	e := newEnv(t)
	var code struct{ Code string }
	e.json(t, "POST", "/api/v1/peer/code", "", nil, 200, &code)
	for i := 0; i < 5; i++ {
		resp, _ := e.do(t, "POST", "/api/v1/peer/pair", `{"code":"00000000","hubName":"x"}`, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("guess %d: %d", i, resp.StatusCode)
		}
	}
	resp, _ := e.do(t, "POST", "/api/v1/peer/pair", `{"code":"`+code.Code+`","hubName":"x"}`, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("the code should be gone, got %d", resp.StatusCode)
	}
}

// The version handshake says which end is too old.
func TestPairing_VersionHandshake(t *testing.T) {
	e := newEnv(t)
	e.json(t, "POST", "/api/v1/boxes", `{"name":"Bedroom","url":"192.168.1.30"}`, nil, 201, nil)
	body := `{"code":"` + peermock.Code + `"}`

	e.peer.Info_.API = 0
	resp, b := e.do(t, "POST", "/api/v1/boxes/bedroom/pair", body, nil)
	if resp.StatusCode != 422 || !strings.Contains(string(b), "box_too_old") {
		t.Errorf("an old DreamHub on the box: %d %s", resp.StatusCode, b)
	}
	e.peer.Info_.API, e.peer.Info_.MinHubAPI = 1, 99
	resp, b = e.do(t, "POST", "/api/v1/boxes/bedroom/pair", body, nil)
	if resp.StatusCode != 422 || !strings.Contains(string(b), "hub_too_old") {
		t.Errorf("an old hub: %d %s", resp.StatusCode, b)
	}
	e.peer.Info_.MinHubAPI = 1
	if bv := e.boxView(t, "bedroom", nil); bv.Paired {
		t.Error("a refused pairing must not leave the box paired")
	}
	e.json(t, "POST", "/api/v1/boxes/bedroom/pair", body, nil, 200, nil)
	if bv := e.boxView(t, "bedroom", nil); !bv.Paired {
		t.Error("the right code pairs")
	}
	// The first box is the one DreamHub runs on: nothing to pair.
	resp, b = e.do(t, "POST", "/api/v1/boxes/main/pair", body, nil)
	if resp.StatusCode != 409 || !strings.Contains(string(b), "box_is_local") {
		t.Errorf("pairing the local box: %d %s", resp.StatusCode, b)
	}
	e.peer.Down = true
	resp, b = e.do(t, "POST", "/api/v1/boxes/bedroom/pair", body, nil)
	if resp.StatusCode != 422 || !strings.Contains(string(b), "peer_unreachable") {
		t.Errorf("a box that is off: %d %s", resp.StatusCode, b)
	}
}

// A film is played on a paired box from the address that box reaches this
// DreamHub on — not from 127.0.0.1, which would be the box itself.
func TestPairing_PlexPlaysOnAPairedBoxFromTheHubsAddress(t *testing.T) {
	boxes.DetectOwnAddr = func(host string) string {
		if host != "192.168.1.30" {
			t.Errorf("detecting the route to %q", host)
		}
		return "192.168.1.5"
	}
	t.Cleanup(func() { boxes.DetectOwnAddr = origDetect })
	e := newEnv(t)
	connectPlex(t, e)
	e.json(t, "POST", "/api/v1/boxes", `{"name":"Bedroom","url":"192.168.1.30"}`, nil, 201, nil)

	// A Basic box has no DreamHub player: not offered, not played.
	resp, b := e.do(t, "POST", "/api/v1/plex/items/101/play?box=bedroom", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "plex_play_unavailable" {
		t.Fatalf("an unpaired box: %d %s", resp.StatusCode, b)
	}
	e.json(t, "POST", "/api/v1/boxes/bedroom/pair", `{"code":"`+peermock.Code+`"}`, nil, 200, nil)
	e.json(t, "POST", "/api/v1/plex/items/101/play?box=bedroom", "", nil, 200, nil)
	en, _ := e.app.Boxes.Get("bedroom")
	ref := en.WC.(*wcmock.Client).LastZapped
	if !strings.Contains(ref, "http%3a//192.168.1.5%3a8080/api/v1/plex/stream/") {
		t.Errorf("the paired box should fetch from the hub's address: %q", ref)
	}

	// The administrator can name the address (a hub behind a proxy, ZeroTier).
	e.json(t, "PATCH", "/api/v1/boxes/bedroom", `{"streamBase":"10.9.0.2:8080"}`, nil, 200, nil)
	e.json(t, "POST", "/api/v1/plex/items/101/play?box=bedroom", "", nil, 200, nil)
	en, _ = e.app.Boxes.Get("bedroom")
	if ref := en.WC.(*wcmock.Client).LastZapped; !strings.Contains(ref, "http%3a//10.9.0.2%3a8080/") {
		t.Errorf("the box's own setting wins: %q", ref)
	}
}

var origDetect = boxes.DetectOwnAddr
