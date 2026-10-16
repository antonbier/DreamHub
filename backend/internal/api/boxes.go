package api

import (
	"net/http"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
)

// BoxParam is the query parameter that names the box a request is about
// (docs/MULTIDEVICE_PLAN.md). A query parameter, not a header, because
// EventSource, <img> and <video src> can't send headers. Omitted means
// the first box, so clients from before multi-box support keep working.
const BoxParam = "box"

// localOnlyPrefixes are the routes that administer the machine DreamHub
// runs on (services, packages, Docker, the terminal). They have no meaning
// for another box until that box runs DreamHub itself (docs/MULTIDEVICE_PLAN.md,
// "Basic" and "Full").
var localOnlyPrefixes = []string{"/api/v1/admin", "/api/v1/apps"}

// withBox resolves the request's box and puts it into the context:
// everything below reads it from there (internal/box). An unknown box is
// a 404; a box other than DreamHub's own machine is refused on the routes
// that administer that machine.
func withBox(d Deps, next http.Handler) http.Handler {
	if d.Boxes == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get(BoxParam)
		if id == "" {
			id = d.Boxes.DefaultID()
		}
		e, ok := d.Boxes.Get(id)
		if !ok {
			httpx.WriteError(w, r, http.StatusNotFound, "box_not_found", "no such box")
			return
		}
		ctx := box.With(r.Context(), e.ID)
		if !e.Local {
			ctx = box.WithRemote(ctx)
			for _, p := range localOnlyPrefixes {
				if r.URL.Path == p || strings.HasPrefix(r.URL.Path, p+"/") {
					httpx.WriteError(w, r, http.StatusConflict, "box_not_full", "this needs DreamHub running on that box")
					return
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
