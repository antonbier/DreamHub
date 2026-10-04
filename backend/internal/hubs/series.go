package hubs

import (
	"context"
	"github.com/antonbier/DreamHub/backend/internal/dmi"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/tvmaze"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Series is SeriesHub: series tracked in SerienRecorder and their upcoming
// episodes, plus a per-series detail view (series_detail.go) that adds the
// series' recordings and further EPG broadcasts from WebControl.
type Series struct {
	SR serienrecorder.Client
	// WC is optional: without it the detail view still shows
	// SerienRecorder's own data and reports the rest as unavailable.
	WC webcontrol.Client
	// FS is the recording file system (on the box), for finding a series'
	// recordings in every folder without asking WebControl folder by
	// folder; nil or off the box, /web/movielist is used instead.
	FS recfs.FS
	// TVmaze (optional) supplies the episode guide (series_episodes.go),
	// cached in Store; Settings says whether it's switched on.
	TVmaze   tvmaze.Client
	Store    *store.Store
	Settings *settings.Service
	// DMI (optional, read-only) tells which episode a broadcast is
	// (series_broadcast_episodes.go).
	DMI *dmi.Reader
	Now func() time.Time
}

func (h *Series) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// Mount registers SeriesHub's routes.
func (h *Series) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/series/markers", h.markers)
	mux.HandleFunc("GET /api/v1/series/markers/{id}", h.detail)
	mux.HandleFunc("GET /api/v1/series/markers/{id}/cover", h.cover)
	mux.HandleFunc("GET /api/v1/series/upcoming", h.upcoming)
}

// Search implements hub.SearchProvider: a case-insensitive substring match
// over each tracked series' name.
func (h *Series) Search(ctx context.Context, query string) ([]hub.SearchResult, error) {
	markers, err := h.SR.Markers(ctx)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var out []hub.SearchResult
	for _, m := range markers {
		if strings.Contains(strings.ToLower(m.Name), q) {
			out = append(out, hub.SearchResult{Title: m.Name, Subtitle: m.Info, Path: "/series/" + strconv.Itoa(m.ID)})
		}
	}
	return out, nil
}

func (h *Series) markers(w http.ResponseWriter, r *http.Request) {
	markers, err := h.SR.Markers(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "serienrecorder_unavailable", "could not reach SerienRecorder")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"markers": markers})
}

func (h *Series) upcoming(w http.ResponseWriter, r *http.Request) {
	episodes, err := h.SR.UpcomingEpisodes(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "serienrecorder_unavailable", "could not reach SerienRecorder")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"episodes": episodes})
}
