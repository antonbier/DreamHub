package hubs

import (
	"net/http"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
)

// Cover art that's already on the box: another plugin (the reference box
// has "<base>.jpg" and "<base>.backdrop.jpg" next to some recordings,
// ARCHITECTURE.md #11) put it next to the recording, so DreamHub shows
// that instead of fetching its own — the data is kept once, by whoever
// already keeps it. TMDB (internal/enrich) is only the fallback for
// recordings without one — and "Save covers" (media_cover_save.go) can
// write that fallback next to them, never replacing an existing file.

// maxCoverBytes guards against a huge or bogus file.
const maxCoverBytes = 8 << 20

// cover serves ?path=<recording .ts>&kind=poster|backdrop.
func (h *Media) cover(w http.ResponseWriter, r *http.Request) {
	roots, err := h.roots(r.Context())
	if err != nil || !recfs.Available(h.FS, roots) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no cover art here")
		return
	}
	p, err := recfs.CleanIn(h.FS, r.URL.Query().Get("path"), roots)
	if err != nil || !strings.HasSuffix(p, ".ts") {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no cover art here")
		return
	}
	poster, backdrop := recfs.Covers(h.FS, p)
	file := poster
	if r.URL.Query().Get("kind") == "backdrop" {
		file = backdrop
	}
	if file == "" {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no cover art here")
		return
	}
	if fi, err := h.FS.Lstat(file); err != nil || fi.Size() > maxCoverBytes {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no cover art here")
		return
	}
	b, err := h.FS.ReadFile(file)
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no cover art here")
		return
	}
	// Only real pictures, by content — the name alone proves nothing.
	ct := http.DetectContentType(b)
	if ct != "image/jpeg" && ct != "image/png" {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no cover art here")
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(b)
}
