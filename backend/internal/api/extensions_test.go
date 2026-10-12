package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

type extensionCard struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Widgets []struct {
		Type        string `json:"type"`
		Label       string `json:"label"`
		Value       string `json:"value"`
		ActionID    string `json:"actionId"`
		ButtonLabel string `json:"buttonLabel"`
	} `json:"widgets"`
}

func (e *env) listExtensions(t *testing.T) (int, []extensionCard) {
	t.Helper()
	resp, b := e.do(t, "GET", "/api/v1/extensions", "", nil)
	var v struct {
		Extensions []extensionCard `json:"extensions"`
	}
	json.Unmarshal(b, &v)
	return resp.StatusCode, v.Extensions
}

// Mock mode pre-seeds one demo extension (app.go) so Extension Hub has
// something to show without a real extension process.
func TestExtensions_MockDemoIsListed(t *testing.T) {
	e := newEnv(t)
	code, cards := e.listExtensions(t)
	if code != http.StatusOK || len(cards) != 1 {
		t.Fatalf("list: %d %+v", code, cards)
	}
	card := cards[0]
	if card.ID != "demo.socket" || card.Status != "available" {
		t.Fatalf("card: %+v", card)
	}
	var action string
	for _, w := range card.Widgets {
		if w.Type == "action" {
			action = w.ActionID
		}
	}
	if action == "" {
		t.Fatalf("expected an action widget, got %+v", card.Widgets)
	}
}

func TestExtensions_ActionGoesThroughPreviewAndConfirm(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "POST", "/api/v1/extensions/demo.socket/actions/toggle/plan", "", nil)
	var p struct {
		Plan struct {
			ID      string `json:"id"`
			Summary string `json:"summary"`
		} `json:"plan"`
	}
	json.Unmarshal(b, &p)
	if resp.StatusCode != http.StatusOK || p.Plan.ID == "" || p.Plan.Summary == "" {
		t.Fatalf("plan: %d %+v %s", resp.StatusCode, p, b)
	}

	// The widget must now report "off" and the opposite action.
	_, cards := e.listExtensions(t)
	before := cards[0].Widgets[0].Value

	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}

	_, cards = e.listExtensions(t)
	after := cards[0].Widgets[0].Value
	if after == before {
		t.Fatalf("expected the socket's status to flip: before %q, after %q", before, after)
	}
}

func TestExtensions_StalePreviewIsRefused(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "POST", "/api/v1/extensions/demo.socket/actions/toggle/plan", "", nil)
	var p1 struct {
		Plan struct{ ID string } `json:"plan"`
	}
	json.Unmarshal(b, &p1)

	// A second preview (and apply) changes the extension's own state
	// fingerprint out from under the first.
	_, b = e.do(t, "POST", "/api/v1/extensions/demo.socket/actions/toggle/plan", "", nil)
	var p2 struct {
		Plan struct{ ID string } `json:"plan"`
	}
	json.Unmarshal(b, &p2)
	e.do(t, "POST", "/api/v1/plans/"+p2.Plan.ID+"/apply", "", nil)

	resp, _ := e.do(t, "POST", "/api/v1/plans/"+p1.Plan.ID+"/apply", "", nil)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("the first preview must be stale after the second was applied")
	}
}

func TestExtensions_RegisterNeedsLoopback(t *testing.T) {
	e := newEnv(t)
	// The test server itself is reached over 127.0.0.1, so this succeeds —
	// a real cross-network request would be rejected the same way
	// Plex.Stream rejects one (hubs/plex_play.go).
	resp, _ := e.do(t, "POST", "/api/v1/extensions/register", `{"id":"new.ext","name":"New","schemaVersion":1,"baseUrl":"http://127.0.0.1:9"}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register: %d", resp.StatusCode)
	}
	_, cards := e.listExtensions(t)
	if len(cards) != 2 {
		t.Fatalf("expected the newly registered extension to show up, got %+v", cards)
	}
}

func TestExtensions_UnsupportedSchemaVersionDegrades(t *testing.T) {
	e := newEnv(t)
	e.do(t, "POST", "/api/v1/extensions/register", `{"id":"future.ext","name":"Future","schemaVersion":99,"baseUrl":"http://127.0.0.1:9"}`, nil)
	_, cards := e.listExtensions(t)
	var card extensionCard
	for _, c := range cards {
		if c.ID == "future.ext" {
			card = c
		}
	}
	if card.ID == "" || card.Status != "unavailable" {
		t.Fatalf("expected future.ext unavailable, got %+v", cards)
	}
}

func TestExtensions_RegisterRejectsANonLoopbackBaseURL(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "POST", "/api/v1/extensions/register", `{"id":"new.ext","name":"New","schemaVersion":1,"baseUrl":"http://192.168.1.50:9"}`, nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusBadRequest || code != "invalid_body" {
		t.Fatalf("expected 400 invalid_body for a non-loopback baseUrl, got %d %s", resp.StatusCode, b)
	}
}
