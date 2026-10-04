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
		"sw.js":                 {Data: []byte("self.addEventListener('fetch', () => {})")},
		"manifest.webmanifest":  {Data: []byte(`{"name":"DreamHub"}`)},
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

// The installable app: the service worker and the shell are revalidated on
// every load (or an installed app keeps an old build), the manifest gets its
// own MIME type (absent from Go's table and from the box).
func TestHandler_InstallableAppHeaders(t *testing.T) {
	h := HandlerFS(fakeDist())
	for _, p := range []string{"/", "/timers", "/sw.js"} {
		if got := get(t, h, p).Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control %q, want no-cache", p, got)
		}
	}
	if got := get(t, h, "/assets/app-abc123.js").Header().Get("Cache-Control"); got != "" {
		t.Errorf("hashed asset: Cache-Control %q, want none", got)
	}
	rec := get(t, h, "/manifest.webmanifest")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("manifest: got %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}
