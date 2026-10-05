package hub

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/events"
	"github.com/antonbier/DreamHub/backend/internal/push"
)

// Every event key the bus can publish belongs to exactly one hub's area,
// has a push text, and has a notification text in each web UI language —
// so a new event can't be added halfway.
func TestEveryEventIsDeclaredAndTranslated(t *testing.T) {
	owner := map[string]string{}
	for _, d := range V1() {
		for _, key := range d.Emits {
			if prev, dup := owner[key]; dup {
				t.Errorf("%s declared by both %s and %s", key, prev, d.ID)
			}
			owner[key] = d.ID
		}
	}
	for _, key := range events.All {
		if owner[key] == "" {
			t.Errorf("%s: no hub declares it in Emits", key)
		}
		if !push.Known(key) {
			t.Errorf("%s: no push text (internal/push/messages.go)", key)
		}
	}
	for key := range owner {
		found := false
		for _, k := range events.All {
			found = found || k == key
		}
		if !found {
			t.Errorf("%s: declared but not in events.All", key)
		}
	}
	for _, lang := range []string{"en", "de", "it"} {
		b, err := os.ReadFile("../../../frontend/src/locales/" + lang + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var loc struct {
			Notification map[string]any `json:"notification"`
		}
		if err := json.Unmarshal(b, &loc); err != nil {
			t.Fatal(err)
		}
		for _, key := range events.All {
			if !hasPath(loc.Notification, key) {
				t.Errorf("%s.json: no notification.%s", lang, key)
			}
		}
	}
}

// hasPath looks up a dotted key ("recording.finished") in nested maps.
func hasPath(m map[string]any, dotted string) bool {
	var cur any = m
	for _, part := range strings.Split(dotted, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		if cur, ok = mm[part]; !ok {
			return false
		}
	}
	_, isText := cur.(string)
	return isText
}
