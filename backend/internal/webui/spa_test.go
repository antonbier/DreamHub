package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// A small fake build, independent of whether `npm run build` has actually
// run — real fixtures live in internal/webui/dist (see embed.go).
func fakeDist() fstest.MapFS {
	return fstest.MapFS{
		"index.html":            {Data: []byte("<html>shell</html>")},
		"assets/app-abc123.js":  {Data: []byte("console.log('app')")},
		"assets/app-abc123.css": {Data: []byte("body{}")},
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestHandler_ServesIndexAtRoot(t *testing.T) {
	rec := get(t, HandlerFS(fakeDist()), "/")
	if rec.Code != http.StatusOK || rec.Body.String() != "<html>shell</html>" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestHandler_ServesRealAsset(t *testing.T) {
	rec := get(t, HandlerFS(fakeDist()), "/assets/app-abc123.js")
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log('app')" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestHandler_FallsBackToIndexForClientRoutes(t *testing.T) {
	for _, p := range []string{"/timers", "/media", "/settings", "/discovery"} {
		rec := get(t, HandlerFS(fakeDist()), p)
		if rec.Code != http.StatusOK || rec.Body.String() != "<html>shell</html>" {
			t.Fatalf("%s: got %d %q, want the SPA shell (vue-router client routes must resolve server-side)", p, rec.Code, rec.Body.String())
		}
	}
}

func TestHandler_MissingAssetIs404NotFallback(t *testing.T) {
	// A broken/stale asset reference must 404, not silently return the SPA
	// shell with a 200 — see Handler's doc comment.
	rec := get(t, HandlerFS(fakeDist()), "/assets/does-not-exist.js")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}
