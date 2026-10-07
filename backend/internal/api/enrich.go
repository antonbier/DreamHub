package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/enrich"
	"github.com/antonbier/DreamHub/backend/internal/fanart"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/tmdb"
)

// enrichLookupBody is up to enrich.MaxBatch titles to look up at once — a
// screenful of rows.
type enrichLookupBody struct {
	Items []enrich.Query `json:"items"`
}

// handleEnrichLookup answers each title with what enrichment knows about
// it, or null. Never an error for "not configured" or "switched off":
// then it's {"enabled": false} and every result is null, so a screen just
// shows no pictures instead of an error. Read-only despite the POST (a
// batch of titles, not a query string).
func handleEnrichLookup(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body enrichLookupBody
		if err := httpx.DecodeJSON(r, &body); err != nil || len(body.Items) > enrich.MaxBatch {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"items": [{"title", "kind"}]}, at most 40`)
			return
		}
		results := make([]*enrich.Info, len(body.Items))
		v, err := d.Settings.Values(r.Context(), current(r))
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load settings")
			return
		}
		key := v.String(connector.KeyTMDBAPIKey)
		if d.Enrich == nil || key == "" || !v.Bool(connector.KeyTMDBEnrich) {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"enabled": false, "results": results})
			return
		}
		results = d.Enrich.Lookup(r.Context(), key, enrich.Language(v.String(settings.KeyLanguage)), body.Items)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"enabled": true, "results": results})
	}
}

// handleEnrichImage serves one poster or backdrop, from DreamHub's disk
// cache or fetched once from TMDB — the browser never contacts TMDB itself
// (PRINCIPLES.md). Cached images are still served with enrichment
// switched off or the key removed; only new fetches stop.
func handleEnrichImage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		size, file := r.PathValue("size"), r.PathValue("file")
		path := "/" + file
		if d.Enrich == nil || strings.Contains(file, "/") || !tmdb.ValidImage(size, path) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such image")
			return
		}
		b, ct, err := d.Enrich.Image(r.Context(), size, path)
		switch {
		case errors.Is(err, tmdb.ErrInvalidImage):
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such image")
			return
		case err != nil:
			httpx.WriteError(w, r, http.StatusBadGateway, "image_unavailable", "the image could not be fetched")
			return
		}
		w.Header().Set("Content-Type", ct)
		// Image paths are content-addressed by TMDB: the same path is always
		// the same picture.
		w.Header().Set("Cache-Control", "private, max-age=2592000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Write(b)
	}
}

// handleEnrichExtras answers ?kind=movie|tv&id=<TMDB ID> with what further
// databases add (enrich.Extras: Rotten Tomatoes, IMDb, Metascore from
// OMDb; a logo from fanart.tv) — one title, for a detail view. Like
// lookup, never an error for "not configured": {"extras": null}.
func handleEnrichExtras(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind := r.URL.Query().Get("kind")
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if (kind != "movie" && kind != "tv") || err != nil || id <= 0 {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_query", "expected ?kind=movie|tv&id=<TMDB ID>")
			return
		}
		v, err := d.Settings.Values(r.Context(), current(r))
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load settings")
			return
		}
		keys := enrich.ExtrasKeys{TMDB: v.String(connector.KeyTMDBAPIKey), OMDb: v.String(connector.KeyOMDbAPIKey), Fanart: v.String(connector.KeyFanartAPIKey)}
		if d.Enrich == nil || !v.Bool(connector.KeyTMDBEnrich) || (keys.OMDb == "" && keys.Fanart == "") {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"extras": nil})
			return
		}
		x := d.Enrich.Extras(r.Context(), keys, enrich.Language(v.String(settings.KeyLanguage)), kind, id)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"extras": x})
	}
}

// handleEnrichLogo serves one fanart.tv logo (?path=, as Extras.Logo
// names it), from DreamHub's disk cache or fetched once — the browser
// never contacts fanart.tv.
func handleEnrichLogo(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if d.Enrich == nil || !fanart.ValidImage(path) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such image")
			return
		}
		b, ct, err := d.Enrich.Logo(r.Context(), path)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadGateway, "image_unavailable", "the image could not be fetched")
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "private, max-age=2592000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Write(b)
	}
}
