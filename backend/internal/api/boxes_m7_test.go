package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/boxes"
	peermock "github.com/antonbier/DreamHub/backend/internal/peer/mock"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// A recording of one box plays on another box (M7): the target box's player
// fetches it from the hub with a ticket, the hub reads it from the source.
func TestBox_RecordingOfOneBoxPlaysOnAnother(t *testing.T) {
	boxes.DetectOwnAddr = func(string) string { return "192.168.1.5" }
	boxes.LookupHost = func(h string) ([]string, error) { return []string{h}, nil }
	t.Cleanup(func() { boxes.DetectOwnAddr = origDetect; boxes.LookupHost = origLookup })

	first := mock.New()
	e := newEnvWith(t, first)
	second := e.addBox(t, "bedroom")
	rec := mock.Root + "20260101 2015 - ZDF HD - Nur im Schlafzimmer.ts"
	second.FS.Put(rec, "0123456789")
	body := `{"target":"main","path":` + jsonString(rec) + `}`

	// The first box is DreamHub's own: it can play. The bedroom box is Basic
	// and can't be a target.
	resp, b := e.do(t, "POST", "/api/v1/stream/recording/play-on?box=bedroom", body, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("play on the first box: %d %s", resp.StatusCode, b)
	}
	if !first.MoviePlayer || !strings.Contains(first.LastZapped, "http%3a//127.0.0.1%3a8080/api/v1/stream/relay/") {
		t.Fatalf("the first box should play from the hub: %q", first.LastZapped)
	}
	streamPath := "/api/v1/stream/relay/" + strings.TrimSuffix(strings.SplitN(first.LastZapped, "/api/v1/stream/relay/", 2)[1], ":20260101 2015 - ZDF HD - Nur im Schlafzimmer")

	// The box's player (loopback = the first box) gets the bytes, Range too.
	resp, b = e.do(t, "GET", streamPath, "", nil)
	if resp.StatusCode != http.StatusOK || string(b) != "0123456789" {
		t.Fatalf("relay: %d %q", resp.StatusCode, b)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/stream/relay/nope/file.ts", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown ticket: %d", resp.StatusCode)
	}

	// Refused: the same box, a recording that isn't there, a box that
	// doesn't exist, a Basic target.
	for name, c := range map[string]struct {
		url, body string
		want      int
	}{
		"same box":  {"?box=bedroom", `{"target":"bedroom","path":` + jsonString(rec) + `}`, http.StatusConflict},
		"no file":   {"?box=bedroom", `{"target":"main","path":"` + mock.Root + `nope.ts"}`, http.StatusNotFound},
		"not a ts":  {"?box=bedroom", `{"target":"main","path":"/etc/passwd"}`, http.StatusNotFound},
		"no target": {"?box=bedroom", `{"target":"kitchen","path":` + jsonString(rec) + `}`, http.StatusNotFound},
		"basic box": {"?box=main", `{"target":"bedroom","path":` + jsonString(mock.Root+"x.ts") + `}`, http.StatusConflict},
		"no body":   {"?box=bedroom", ``, http.StatusBadRequest},
	} {
		if resp, b := e.do(t, "POST", "/api/v1/stream/recording/play-on"+c.url, c.body, nil); resp.StatusCode != c.want {
			t.Errorf("%s: %d %s, want %d", name, resp.StatusCode, b, c.want)
		}
	}

	// A paired bedroom box can be a target; its player is admitted only from
	// its own address.
	e.json(t, "POST", "/api/v1/boxes", `{"name":"Kitchen","url":"192.168.1.30"}`, nil, 201, nil)
	e.json(t, "POST", "/api/v1/boxes/kitchen/pair", `{"code":"`+peermock.Code+`"}`, nil, 200, nil)
	first.FS.Put(mock.Root+"20260102 - Tagesschau.ts", "abcdef")
	body = `{"target":"kitchen","path":` + jsonString(mock.Root+"20260102 - Tagesschau.ts") + `,"title":"Tagesschau"}`
	if resp, b := e.do(t, "POST", "/api/v1/stream/recording/play-on?box=main", body, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("play on the paired box: %d %s", resp.StatusCode, b)
	}
	en, _ := e.app.Boxes.Get("kitchen")
	ref := en.WC.(*mock.Client).LastZapped
	if !strings.Contains(ref, "http%3a//192.168.1.5%3a8080/api/v1/stream/relay/") || !strings.HasSuffix(ref, ":Tagesschau") {
		t.Fatalf("the paired box fetches from the hub's address: %q", ref)
	}
	path := "/api/v1/stream/relay/" + strings.TrimSuffix(strings.SplitN(ref, "/api/v1/stream/relay/", 2)[1], ":Tagesschau")
	// The test client is 127.0.0.1, not the bedroom box (192.168.1.30).
	if resp, _ := e.do(t, "GET", path, "", nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a request not from the target box: %d", resp.StatusCode)
	}
	boxes.LookupHost = func(string) ([]string, error) { return []string{"127.0.0.1"}, nil }
	if resp, b := e.do(t, "GET", path, "", nil); resp.StatusCode != http.StatusOK || string(b) != "abcdef" {
		t.Errorf("from the target box: %d %q", resp.StatusCode, b)
	}
	if resp, b := e.do(t, "GET", path, "", func(r *http.Request) { r.Header.Set("Range", "bytes=2-3") }); resp.StatusCode != http.StatusPartialContent || string(b) != "cd" {
		t.Errorf("seeking: %d %q", resp.StatusCode, b)
	}
}

// Starting a film on a box's screen needs the remote's permission and a box
// the profile may use.
func TestBox_PlayOnNeedsRemoteAndTheTargetBox(t *testing.T) {
	boxes.DetectOwnAddr = func(string) string { return "192.168.1.5" }
	t.Cleanup(func() { boxes.DetectOwnAddr = origDetect })
	e := newEnv(t)
	admin, viewer, benID := e.householdWithBoxes(t)
	e.json(t, "POST", "/api/v1/boxes/bedroom/pair", `{"code":"`+peermock.Code+`"}`, withProfile(admin), 200, nil)
	rec := mock.Root + "20260102 - Tagesschau.ts"
	body := `{"target":"bedroom","path":` + jsonString(rec) + `}`

	e.json(t, "PATCH", "/api/v1/profiles/"+itoa(benID), `{"boxes":["main"]}`, withProfile(admin), 200, nil)
	if resp, _ := e.do(t, "POST", "/api/v1/stream/recording/play-on?box=main", body, withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a target the profile may not use (or no remote): %d", resp.StatusCode)
	}
	e.json(t, "PATCH", "/api/v1/profiles/"+itoa(benID), `{"boxes":["main","bedroom"]}`, withProfile(admin), 200, nil)
	if resp, b := e.do(t, "POST", "/api/v1/stream/recording/play-on?box=main", body, withProfile(viewer)); resp.StatusCode == http.StatusForbidden {
		t.Errorf("both boxes allowed: %d %s", resp.StatusCode, b)
	}
}

var origLookup = boxes.LookupHost

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
