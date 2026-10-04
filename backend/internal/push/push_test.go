package push

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompose(t *testing.T) {
	loc := time.FixedZone("CEST", 2*3600)
	m, ok := Compose("series.missingAiring", json.RawMessage(`{"series":"FBI","code":"S05E07","title":"Gefangenenübergabe","service":"SAT.1","start":1790187300}`), "de", loc)
	if !ok || m.Title != "Fehlende Folge läuft" || m.Body != "FBI S05E07 „Gefangenenübergabe“ – 23.09. 20:15 auf SAT.1" {
		t.Errorf("got %+v", m)
	}
	m, _ = Compose("recording.finished", json.RawMessage(`{}`), "en", loc)
	if m.Body != "" {
		t.Errorf("placeholders the event lacks go away: %q", m.Body)
	}
	if m, _ := Compose("disk.aboveThreshold", json.RawMessage(`{"model":"ST2000","percent":93}`), "xx", loc); m.Body != "ST2000 is 93% full." || m.Priority != 4 {
		t.Errorf("unknown language falls back to English: %+v", m)
	}
	if _, ok := Compose("hub.statusChanged", nil, "de", loc); ok {
		t.Error("only events with a text are pushed")
	}
}

func TestHTTPClient(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &got)
		auth = r.Header.Get("Authorization")
	}))
	defer srv.Close()
	c := &HTTPClient{HTTP: srv.Client()}
	err := c.Send(context.Background(), Config{Server: srv.URL, Topic: "dreamhub-x1", Token: "tk"}, Message{Title: "Ü", Body: "b", Priority: 4})
	if err != nil || got["topic"] != "dreamhub-x1" || got["title"] != "Ü" || got["priority"] != float64(4) || auth != "Bearer tk" {
		t.Errorf("published %v auth %q err %v", got, auth, err)
	}
	if err := c.Send(context.Background(), Config{Server: srv.URL, Topic: "no/slash"}, Message{}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("topic syntax: %v", err)
	}
	if err := c.Send(context.Background(), Config{Server: srv.URL}, Message{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("no topic: %v", err)
	}
}
