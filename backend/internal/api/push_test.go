package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPush_TestAndForwarding(t *testing.T) {
	e := newEnv(t)
	if resp, _ := e.do(t, "POST", "/api/v1/push/test", "", nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("no topic yet: %d", resp.StatusCode)
	}
	if resp, b := e.do(t, "PATCH", "/api/v1/settings", `{"values":{"push.topic":"has/slash"}}`, nil); resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "push_topic") {
		t.Errorf("topic syntax is checked on save: %d %s", resp.StatusCode, b)
	}
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"push.topic":"dreamhub-test-1","general.language":"de"}}`, nil)
	if resp, b := e.do(t, "POST", "/api/v1/push/test", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("test push: %d %s", resp.StatusCode, b)
	}

	// Events go out when their kind is on: recording.finished is by
	// default, recording.started isn't.
	e.events.Publish(context.Background(), "recording.started", map[string]any{"name": "Heat", "service": "ARD"})
	e.events.Publish(context.Background(), "recording.finished", map[string]any{"name": "Heat", "service": "ARD"})
	deadline := time.Now().Add(2 * time.Second)
	for len(e.pushed.Messages()) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	msgs := e.pushed.Messages()
	if len(msgs) != 2 || msgs[0].Body != "Benachrichtigungen kommen an." || msgs[1].Title != "Aufnahme fertig" || msgs[1].Body != "Heat · ARD" {
		t.Errorf("sent: %+v", msgs)
	}
}
