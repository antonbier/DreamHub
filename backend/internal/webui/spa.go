package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// FS is the embedded frontend rooted at dist/, so "index.html" sits at its
// root rather than at "dist/index.html".
func FS() fs.FS {
	sub, err := fs.Sub(Dist, "dist")
	if err != nil {
		// "dist" is embedded above by a compile-time directive; this can't
		// fail at runtime.
		panic(err)
	}
	return sub
}

// Handler serves the embedded frontend with an SPA fallback: a request for
// a path that isn't a real static file gets index.html instead, so
// client-side routes (e.g. /timers) work on a direct load or a refresh —
// vue-router's createWebHistory() needs this on the server side.
//
// The fallback only applies to route-shaped paths (no file extension on
// the last segment, e.g. /timers). A path that looks like an asset request
// (e.g. /assets/does-not-exist.js) is left alone and 404s normally,
// instead of silently returning a 200 HTML page for a broken reference.
func Handler() http.Handler { return HandlerFS(FS()) }

// HandlerFS is Handler over an arbitrary fs.FS, so tests can exercise the
// fallback logic without depending on a real frontend build.
func HandlerFS(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		switch p {
		case "sw.js", "index.html":
			// The service worker and the shell must be revalidated on every
			// load, or an installed app would keep running an old build.
			w.Header().Set("Cache-Control", "no-cache")
		case "manifest.webmanifest":
			// Not in Go's built-in MIME table; the box has no /etc/mime.types.
			w.Header().Set("Content-Type", "application/manifest+json")
		}
		looksLikeRoute := !strings.Contains(path.Base(p), ".")
		if looksLikeRoute {
			if _, err := fs.Stat(fsys, p); err != nil {
				r2 := new(http.Request)
				*r2 = *r
				r2.URL.Path = "/"
				r = r2
				w.Header().Set("Cache-Control", "no-cache")
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}
