package hubs

import (
	"context"
	"net/http"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Media is MediaHub: listing recordings and the disk-space overview.
// Rename/delete (ROADMAP.md, "MediaHub") aren't built yet — WebControl's API
// has no file-operation endpoints (ARCHITECTURE.md), so those need their own
// filesystem-backed component, still to be designed.
type Media struct {
	WC webcontrol.Client
}

// Mount registers MediaHub's routes.
func (h *Media) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/media/movies", h.list)
	mux.HandleFunc("GET /api/v1/media/disk", h.disk)
}

// isRecording reports whether a movielist entry is an actual Dreambox
// recording rather than other media manually placed in the same folder
// (ROADMAP.md, "MediaHub": confirmed live against a real .mkv file sitting
// alongside real recordings on the reference device). Enigma2 recordings are
// always .ts; anything else was not made by this box's tuner.
func isRecording(filename string) bool {
	return strings.HasSuffix(strings.ToLower(filename), ".ts")
}

// MovieView is a movielist entry as MediaHub serves it, with the
// recording/other-media distinction resolved server-side so every client
// doesn't have to reimplement the filename check.
type MovieView struct {
	webcontrol.Movie
	IsRecording bool `json:"isRecording"`
}

// Search implements hub.SearchProvider: a case-insensitive substring match
// over each recording's title.
func (h *Media) Search(ctx context.Context, query string) ([]hub.SearchResult, error) {
	movies, err := h.WC.MovieList(ctx)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var out []hub.SearchResult
	for _, m := range movies {
		if strings.Contains(strings.ToLower(m.Title), q) {
			out = append(out, hub.SearchResult{Title: m.Title, Subtitle: m.ServiceName, Path: "/media"})
		}
	}
	return out, nil
}

func (h *Media) list(w http.ResponseWriter, r *http.Request) {
	movies, err := h.WC.MovieList(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	views := make([]MovieView, len(movies))
	for i, m := range movies {
		views[i] = MovieView{Movie: m, IsRecording: isRecording(m.Filename)}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"movies": views})
}

// Disk is one storage device's usage, as MediaHub reports it.
type Disk struct {
	Model        string   `json:"model"`
	CapacityText string   `json:"capacityText"`
	FreeText     string   `json:"freeText"`
	UsedPercent  *float64 `json:"usedPercent,omitempty"`
}

func (h *Media) disk(w http.ResponseWriter, r *http.Request) {
	info, err := h.WC.DeviceInfo(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	disks := make([]Disk, 0, len(info.HDDs))
	for _, hd := range info.HDDs {
		d := Disk{Model: hd.Model, CapacityText: hd.Capacity, FreeText: hd.Free}
		if cap, ok := webcontrol.ParseSize(hd.Capacity); ok && cap > 0 {
			if free, ok := webcontrol.ParseSize(hd.Free); ok {
				pct := 100 * float64(cap-free) / float64(cap)
				d.UsedPercent = &pct
			}
		}
		disks = append(disks, d)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"disks": disks})
}
