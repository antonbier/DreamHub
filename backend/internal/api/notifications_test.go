package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNotifications_ListAndMarkRead(t *testing.T) {
	e := newEnv(t)

	resp, b := e.do(t, "GET", "/api/v1/notifications", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var empty struct {
		Notifications []any `json:"notifications"`
		Unread        int   `json:"unread"`
	}
	json.Unmarshal(b, &empty)
	if len(empty.Notifications) != 0 || empty.Unread != 0 {
		t.Fatalf("expected an empty inbox, got %+v", empty)
	}

	if _, err := e.events.Publish(context.Background(), "recording.started", map[string]any{"name": "Example"}); err != nil {
		t.Fatal(err)
	}

	_, b2 := e.do(t, "GET", "/api/v1/notifications", "", nil)
	var withOne struct {
		Notifications []struct {
			Key    string         `json:"key"`
			Params map[string]any `json:"params"`
			Read   bool           `json:"read"`
		} `json:"notifications"`
		Unread int `json:"unread"`
	}
	json.Unmarshal(b2, &withOne)
	if len(withOne.Notifications) != 1 || withOne.Notifications[0].Key != "recording.started" {
		t.Fatalf("expected the published event in the list, got %+v", withOne)
	}
	if withOne.Notifications[0].Read {
		t.Error("a fresh notification must be unread")
	}
	if withOne.Unread != 1 {
		t.Errorf("expected unread=1, got %d", withOne.Unread)
	}
	if withOne.Notifications[0].Params["name"] != "Example" {
		t.Errorf("expected params to round-trip, got %+v", withOne.Notifications[0].Params)
	}

	resp3, b3 := e.do(t, "POST", "/api/v1/notifications/read", "", nil)
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp3.StatusCode, b3)
	}

	_, b4 := e.do(t, "GET", "/api/v1/notifications", "", nil)
	var afterRead struct {
		Notifications []struct{ Read bool } `json:"notifications"`
		Unread        int                   `json:"unread"`
	}
	json.Unmarshal(b4, &afterRead)
	if afterRead.Unread != 0 || !afterRead.Notifications[0].Read {
		t.Errorf("expected everything read, got %+v", afterRead)
	}
}

func TestEventsStream_PushesAPublishedNotificationLive(t *testing.T) {
	e := newEnv(t)

	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/events/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %q", ct)
	}

	// Give the handler a moment to register its subscription before
	// publishing, then read until the pushed event line shows up.
	time.Sleep(20 * time.Millisecond)
	if _, err := e.events.Publish(context.Background(), "disk.aboveThreshold", map[string]any{"percent": 91.2}); err != nil {
		t.Fatal(err)
	}

	found := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.Contains(sc.Text(), `"disk.aboveThreshold"`) {
				found <- true
				return
			}
		}
		found <- false
	}()
	select {
	case ok := <-found:
		if !ok {
			t.Fatal("stream closed before the published event arrived")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the live event")
	}
}
