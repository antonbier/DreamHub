package serienrecorder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Real response shapes, captured live 2026-09-24 (ARCHITECTURE.md, #10).
const infoJSON = `{"payload":{"apiVersion":"2.12","dbVersion":"4.6.0","srVersion":"4.8.0-beta","stbType":"one"},"status":true}`
const markersJSON = `{"payload":{"markers":[{"active":false,"channels":"Alle","fsid":"and-just-like-that","id":9,"info":"USA 2021-2025","name":"And Just Like That...","numberOfRecords":1,"wlid":"42185"}]},"status":true}`
const timerJSON = `{"payload":{"timer":[{"active":true,"channel":"RTL Television","episode":"8615","fsID":"gute-zeiten-schlechte-zeiten","season":"0","series":"Gute Zeiten, schlechte Zeiten","serviceRef":"1:0:1:2EE3:441:1:C00000:0:0:0:","startTime":1790319120,"title":"Fehler der Vergangenheit"}]},"status":true}`

func TestInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/serienrecorderapi/info" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(infoJSON))
	}))
	defer srv.Close()

	info, err := NewHTTPClient(srv.URL).Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.APIVersion != "2.12" || info.SRVersion != "4.8.0-beta" {
		t.Errorf("unexpected info: %+v", info)
	}
}

func TestMarkers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(markersJSON))
	}))
	defer srv.Close()

	markers, err := NewHTTPClient(srv.URL).Markers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 1 || markers[0].Name != "And Just Like That..." || markers[0].FSID != "and-just-like-that" || markers[0].WLID != "42185" {
		t.Errorf("unexpected markers: %+v", markers)
	}
}

func TestUpcomingEpisodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("remaining") != "1" {
			t.Errorf("expected remaining=1, got %s", r.URL.RawQuery)
		}
		w.Write([]byte(timerJSON))
	}))
	defer srv.Close()

	episodes, err := NewHTTPClient(srv.URL).UpcomingEpisodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 1 || episodes[0].Series != "Gute Zeiten, schlechte Zeiten" || episodes[0].FSID != "gute-zeiten-schlechte-zeiten" {
		t.Errorf("unexpected episodes: %+v", episodes)
	}
}

func TestMarkers_SurfacesTheAPIsHTMLErrorPage(t *testing.T) {
	// A missing/invalid required parameter returns an HTML error page, not
	// JSON (confirmed live, ARCHITECTURE.md) — must surface as an error, not
	// a panic or a zero-value success.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><title>Processing Failed</title></head><body><b>Processing Failed</b></body></html>`))
	}))
	defer srv.Close()

	if _, err := NewHTTPClient(srv.URL).Markers(context.Background()); err == nil {
		t.Fatal("expected an error for a non-JSON response")
	}
}
