package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// A remote box's streams pass through the hub: live TV and recordings come
// from that box, never from the first one (M5).
func TestBox_StreamsOfARemoteBoxPassThroughTheHub(t *testing.T) {
	first := mock.New()
	e := newEnvWith(t, first)
	second := e.addBox(t, "bedroom")

	ts := bytes.Repeat(append([]byte{0x47}, make([]byte, 187)...), 50)
	file := filepath.Join(t.TempDir(), "live.ts")
	os.WriteFile(file, ts, 0o600)
	second.StreamFile = file

	// Only the bedroom box streams.
	resp, b := e.do(t, "GET", "/api/v1/stream/live?box=bedroom&ref="+url.QueryEscape(zdfRef), "", nil)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(b, ts) || second.LastStreamed != zdfRef || first.LastStreamed != "" {
		t.Fatalf("live from the bedroom box: %d %d bytes, asked %q / %q", resp.StatusCode, len(b), second.LastStreamed, first.LastStreamed)
	}
	if resp, b := e.do(t, "GET", "/api/v1/stream/live?ref="+url.QueryEscape(zdfRef), "", nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("the first box has no stream file: %d %s", resp.StatusCode, b)
	}

	// A recording that exists on the bedroom box only: through the box's
	// file access (the hub has no disk of it), Range included.
	rec := mock.Root + "20260101 2015 - ZDF HD - Nur im Schlafzimmer.ts"
	second.FS.Put(rec, "0123456789")
	resp, b = e.do(t, "GET", "/api/v1/stream/recording?box=bedroom&path="+url.QueryEscape(rec), "", nil)
	if resp.StatusCode != http.StatusOK || string(b) != "0123456789" {
		t.Errorf("recording of the bedroom box: %d %q", resp.StatusCode, b)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/stream/recording?path="+url.QueryEscape(rec), "", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("the first box does not have it: %d", resp.StatusCode)
	}
}

// The bell names the box once there are several, and a profile sees only
// the events of the boxes it may use (M5).
func TestNotifications_NameTheBoxAndRespectProfiles(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin, viewer, benID := e.household(t)

	e.events.Publish(ctx, "recording.finished", map[string]any{"name": "Alt"})
	type list struct {
		Notifications []struct {
			Params map[string]any
			Box    *struct{ ID, Name string }
		}
		Unread int
	}
	var l list
	e.json(t, "GET", "/api/v1/notifications", "", withProfile(admin), 200, &l)
	if len(l.Notifications) != 1 || l.Notifications[0].Box != nil {
		t.Fatalf("one box: no box name expected: %+v", l)
	}

	e.json(t, "POST", "/api/v1/boxes", `{"name":"Bedroom","url":"192.168.1.30"}`, withProfile(admin), 201, nil)
	e.events.Publish(ctx, "recording.finished", map[string]any{"name": "Neu", "box": "bedroom"})
	e.events.Publish(ctx, "recording.finished", map[string]any{"name": "Haupt"}) // untagged: the first box

	e.json(t, "GET", "/api/v1/notifications", "", withProfile(admin), 200, &l)
	got := map[string]string{}
	for _, n := range l.Notifications {
		if n.Box != nil {
			got[n.Params["name"].(string)] = n.Box.Name
		}
	}
	if len(l.Notifications) != 3 || got["Neu"] != "Bedroom" || got["Haupt"] == "" {
		t.Errorf("boxes named: %+v %v", l, got)
	}

	e.json(t, "PATCH", "/api/v1/profiles/"+itoa(benID), `{"boxes":["bedroom"]}`, withProfile(admin), 200, nil)
	e.json(t, "GET", "/api/v1/notifications", "", withProfile(viewer), 200, &l)
	if len(l.Notifications) != 1 || l.Notifications[0].Params["name"] != "Neu" || l.Unread != 1 {
		t.Errorf("Ben sees the bedroom only: %+v", l)
	}
}

// Push says which box an event is about, but only with several boxes.
func TestPush_NamesTheBoxWithSeveralBoxes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"push.topic":"dreamhub-test-2","general.language":"en"}}`, nil)
	wait := func(n int) []string {
		deadline := time.Now().Add(2 * time.Second)
		for len(e.pushed.Messages()) < n && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
		var out []string
		for _, m := range e.pushed.Messages() {
			out = append(out, m.Body)
		}
		return out
	}
	e.events.Publish(ctx, "recording.finished", map[string]any{"name": "Heat", "service": "ARD"})
	if got := wait(1); len(got) != 1 || got[0] != "Heat · ARD" {
		t.Fatalf("one box: %v", got)
	}
	e.addBox(t, "bedroom")
	e.events.Publish(ctx, "recording.finished", map[string]any{"name": "Heat", "service": "ARD", "box": "bedroom"})
	if got := wait(2); len(got) != 2 || !strings.HasSuffix(got[1], "· bedroom") {
		t.Fatalf("several boxes: %v", got)
	}
}
