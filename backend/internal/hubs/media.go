package hubs

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/adscan"
	"github.com/antonbier/DreamHub/backend/internal/ai"
	"github.com/antonbier/DreamHub/backend/internal/enrich"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	"github.com/antonbier/DreamHub/backend/internal/jobs"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/tmdb"
	"github.com/antonbier/DreamHub/backend/internal/trim"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Media is MediaHub: browsing recordings folder by folder, the disk-space
// overview, and file operations (rename, trash, restore, empty — see
// media_files.go and internal/recfs).
type Media struct {
	WC    webcontrol.Client
	Plans *plan.Service
	// FS is the file system the recordings live on: the real one on the box,
	// the mock's in-memory one in mock mode. File operations are only
	// offered when the box's recording folders exist on it
	// (recfs.Available) — off the box they're unavailable, not broken.
	FS recfs.FS
	// ReadOnly refuses to apply file changes (previews still work) — the same
	// switch as WebControl writes (DREAMHUB_ENABLE_DEVICE_WRITES=0), so a
	// read-only trial run on a real box can't touch recordings either.
	ReadOnly bool

	// Name suggestions (media_suggest.go): settings for the keys and the
	// language, the store for the AI budget, and the two optional
	// external sources.
	Settings *settings.Service
	Store    *store.Store
	AI       ai.Client
	TMDB     tmdb.Client
	Now      func() time.Time
	// Enrich finds the covers "Save covers" writes (media_cover_save.go);
	// nil when TMDB isn't available at all.
	Enrich *enrich.Service

	// AdScan opens recordings for ad detection (media_adscan.go): FFmpeg on
	// the box, synthetic frames in mock mode; nil when FFmpeg is missing.
	AdScan adscan.Opener
	// Trim cuts ad breaks out (media_trim.go): FFmpeg on the box, a
	// stand-in in mock mode; nil when FFmpeg is missing.
	Trim trim.Cutter
	// Jobs is the background queue analyses and cuts run in (RegisterJobs).
	Jobs *jobs.Service

	// index is every recording below the recording folders, read from the
	// file system on the box (media_index.go).
	index recIndex
}

// Perms are MediaHub's route permissions beyond the hub's own (internal/perm).
func (h *Media) Perms() map[string]perm.Perm {
	return map[string]perm.Perm{
		"POST /api/v1/media/adscan":             perm.RecordingsManage,
		"POST /api/v1/media/trim/plan":          perm.RecordingsManage,
		"POST /api/v1/media/covers/plan":        perm.RecordingsManage,
		"POST /api/v1/media/rename/batch/plan":  perm.RecordingsManage,
		"POST /api/v1/media/rename/plan":        perm.RecordingsManage,
		"POST /api/v1/media/rename/suggest":     perm.RecordingsManage,
		"POST /api/v1/media/trash/empty/plan":   perm.RecordingsManage,
		"POST /api/v1/media/trash/plan":         perm.RecordingsManage,
		"POST /api/v1/media/trash/restore/plan": perm.RecordingsManage,
	}
}

// Mount registers MediaHub's routes.
func (h *Media) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/media/movies", h.list)
	mux.HandleFunc("GET /api/v1/media/disk", h.disk)
	mux.HandleFunc("GET /api/v1/media/cover", h.cover)
	mux.HandleFunc("POST /api/v1/media/covers/plan", h.planSaveCovers)
	mux.HandleFunc("POST /api/v1/media/rename/plan", h.planRename)
	mux.HandleFunc("POST /api/v1/media/rename/batch/plan", h.planBatchRename)
	mux.HandleFunc("POST /api/v1/media/rename/suggest", h.suggest)
	mux.HandleFunc("POST /api/v1/media/trash/plan", h.planTrash)
	mux.HandleFunc("GET /api/v1/media/trash", h.trashList)
	mux.HandleFunc("POST /api/v1/media/trash/restore/plan", h.planRestore)
	mux.HandleFunc("POST /api/v1/media/trash/empty/plan", h.planEmpty)
	mux.HandleFunc("POST /api/v1/media/adscan", h.startAdScan)
	mux.HandleFunc("GET /api/v1/media/adscan", h.adScanResult)
	mux.HandleFunc("POST /api/v1/media/trim/plan", h.trimPlan)
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
	// Cover/Backdrop: cover art already next to the recording on disk
	// (media_cover.go), served by GET /api/v1/media/cover. Only known when
	// DreamHub runs on the box.
	Cover    bool `json:"cover,omitempty"`
	Backdrop bool `json:"backdrop,omitempty"`
	// InUse: "recording" or "playing" while the box writes or plays the
	// file — the list says so up front instead of only refusing a rename
	// or delete later (media_files.go). Best effort: empty when the timer
	// list can't be read.
	InUse string `json:"inUse,omitempty"`
	// Ads is the last ad detection's summary (media_adscan.go), when the
	// recording was analysed.
	Ads *adSummary `json:"ads,omitempty"`
}

// Search implements hub.SearchProvider: a case-insensitive substring match
// over each recording's title.
// Search implements hub.SearchProvider: recordings whose title contains
// query, across every folder when DreamHub runs on the box (the recording
// index), else the top folder /web/movielist answers for. A result opens
// its folder.
func (h *Media) Search(ctx context.Context, query string) ([]hub.SearchResult, error) {
	q := strings.ToLower(query)
	var out []hub.SearchResult
	if entries, ok := h.recordingIndex(ctx); ok {
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Title), q) {
				out = append(out, hub.SearchResult{Title: e.Title, Subtitle: folderLabel(e.Folder), Path: "/media?dir=" + url.QueryEscape(e.Folder)})
				if len(out) == maxMediaResults {
					break
				}
			}
		}
		return out, nil
	}
	movies, err := h.WC.MovieList(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range movies {
		if strings.Contains(strings.ToLower(m.Title), q) {
			out = append(out, hub.SearchResult{Title: m.Title, Subtitle: m.ServiceName, Path: "/media"})
		}
	}
	return out, nil
}

// maxMediaResults caps the search provider's answer over the whole index.
const maxMediaResults = 20

// folderLabel is a folder's own name for a search result's subtitle.
func folderLabel(folder string) string {
	return path.Base(strings.TrimSuffix(folder, "/"))
}

// folderView is one subfolder in a listing.
type folderView struct {
	Path string `json:"path"` // absolute, trailing slash
	Name string `json:"name"`
}

// list answers one folder: ?dir= (a recording folder or anything below
// one), default the box's first recording folder. Folder by folder rather
// than the whole tree: walking every folder through WebControl takes ~40 s
// on the reference box (173 folders) and blocks enigma2 meanwhile
// (ARCHITECTURE.md).
func (h *Media) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	roots, err := h.roots(ctx)
	if err != nil || len(roots) == 0 {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	root := path.Clean(roots[0])
	dir := root
	if q := r.URL.Query().Get("dir"); q != "" {
		dir = path.Clean("/" + strings.TrimLeft(q, "/"))
		if !insideOrAt(dir, roots) {
			httpx.WriteError(w, r, http.StatusBadRequest, recfs.ErrOutsideFolders.Error(), "not a recording folder")
			return
		}
	}
	folder, err := h.WC.MovieDir(ctx, dir+"/")
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	onBox := recfs.Available(h.FS, roots)
	busy, _ := h.inUseSet(ctx)
	views := make([]MovieView, len(folder.Movies))
	for i, m := range folder.Movies {
		views[i] = MovieView{Movie: m, IsRecording: isRecording(m.Filename)}
		if views[i].IsRecording {
			views[i].InUse = busy[path.Clean("/"+strings.TrimLeft(m.Filename, "/"))]
		}
		if onBox && views[i].IsRecording {
			if p, err := recfs.CleanIn(h.FS, m.Filename, roots); err == nil {
				poster, backdrop := recfs.Covers(h.FS, p)
				views[i].Cover, views[i].Backdrop = poster != "", backdrop != ""
				views[i].Ads = h.adSummary(ctx, p)
			}
		}
	}
	subfolders := make([]folderView, 0, len(folder.Subfolders))
	for _, sf := range folder.Subfolders {
		name := path.Base(path.Clean(sf))
		// The old AMS trash folder isn't a place recordings live.
		if name == "trashcan" && path.Dir(path.Clean(sf)) == root {
			continue
		}
		subfolders = append(subfolders, folderView{Path: path.Clean(sf) + "/", Name: name})
	}
	sort.Slice(subfolders, func(i, j int) bool { return strings.ToLower(subfolders[i].Name) < strings.ToLower(subfolders[j].Name) })
	out := map[string]any{
		"dir":        dir + "/",
		"root":       root + "/",
		"subfolders": subfolders,
		"movies":     views,
		"fileOps":    h.fileOps(roots),
		// Whether "Detect ads" can run here (FFmpeg, on the box or mock).
		"adScan": h.adScanAvailable(roots),
	}
	if dir != root {
		out["parent"] = path.Dir(dir) + "/"
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// insideOrAt reports whether dir is one of the roots or below one.
func insideOrAt(dir string, roots []string) bool {
	for _, r := range roots {
		r = path.Clean("/" + strings.TrimLeft(r, "/"))
		if dir == r || strings.HasPrefix(dir, r+"/") {
			return true
		}
	}
	return false
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
