package hubs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// similarBox answers SimilarEvents only.
type similarBox struct {
	webcontrol.Client
	events []webcontrol.EPGEvent
	err    error
}

func (b similarBox) SimilarEvents(context.Context, string, int64) ([]webcontrol.EPGEvent, error) {
	return b.events, b.err
}

func getSimilar(t *testing.T, wc webcontrol.Client, query string) (int, map[string]json.RawMessage) {
	t.Helper()
	now := time.Unix(1000, 0)
	h := &Guide{WC: wc, Now: func() time.Time { return now }}
	rec := httptest.NewRecorder()
	h.similar(rec, httptest.NewRequest(http.MethodGet, "/api/v1/guide/similar?"+query, nil))
	var body map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func TestGuideSimilar_UpcomingSoonestFirst(t *testing.T) {
	box := similarBox{events: []webcontrol.EPGEvent{
		{ID: 3, Start: 5000, Duration: 900, Title: "later"},
		{ID: 1, Start: 100, Duration: 300, Title: "over"},
		{ID: 2, Start: 900, Duration: 300, Title: "running"},
	}}
	code, body := getSimilar(t, box, "ref=1:0:19:283D&event=9")
	var events []webcontrol.EPGEvent
	json.Unmarshal(body["events"], &events)
	if code != http.StatusOK || string(body["available"]) != "true" || len(events) != 2 || events[0].ID != 2 || events[1].ID != 3 {
		t.Errorf("%d %s", code, fmt.Sprint(body))
	}
}

func TestGuideSimilar_BoxWithoutTheCall(t *testing.T) {
	code, body := getSimilar(t, similarBox{err: fmt.Errorf("x: %w", webcontrol.ErrNotFound)}, "ref=r&event=9")
	if code != http.StatusOK || string(body["available"]) != "false" || string(body["events"]) != "[]" {
		t.Errorf("%d %s", code, fmt.Sprint(body))
	}
}

func TestGuideSimilar_NeedsRefAndEvent(t *testing.T) {
	for _, q := range []string{"", "ref=r", "event=9", "ref=r&event=x", "ref=r&event=0"} {
		if code, _ := getSimilar(t, similarBox{}, q); code != http.StatusBadRequest {
			t.Errorf("%q: %d", q, code)
		}
	}
}
