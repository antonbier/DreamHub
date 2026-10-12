package extension

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClient_Widgets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/widgets" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode([]Widget{{Type: WidgetStatus, Label: "Power", Value: "On", Tone: ToneGood}})
	}))
	defer srv.Close()

	c := HTTPClient{BaseURL: srv.URL}
	widgets, err := c.Widgets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(widgets) != 1 || widgets[0].Label != "Power" {
		t.Fatalf("widgets = %+v", widgets)
	}
}

func TestHTTPClient_PlanAndApply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/actions/toggle/plan":
			json.NewEncoder(w).Encode(ActionPreview{Summary: "Toggle it", Fingerprint: "fp1"})
		case r.Method == http.MethodPost && r.URL.Path == "/actions/toggle/apply":
			var body struct{ Fingerprint string }
			json.NewDecoder(r.Body).Decode(&body)
			if body.Fingerprint != "fp1" {
				w.WriteHeader(http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := HTTPClient{BaseURL: srv.URL}
	preview, err := c.PlanAction(context.Background(), "toggle")
	if err != nil || preview.Summary != "Toggle it" || preview.Fingerprint != "fp1" {
		t.Fatalf("preview = %+v, err = %v", preview, err)
	}
	if err := c.Apply(context.Background(), "toggle", "fp1"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := c.Apply(context.Background(), "toggle", "stale"); err != ErrStale {
		t.Fatalf("apply with stale fingerprint: %v, want ErrStale", err)
	}
}

func TestHTTPClient_UnreachableIsAnError(t *testing.T) {
	c := HTTPClient{BaseURL: "http://127.0.0.1:1"}
	if _, err := c.Widgets(context.Background()); err == nil {
		t.Fatal("expected an error for an unreachable extension")
	}
}
