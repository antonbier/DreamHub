package hubs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/naming"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/titles"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Batch rename: many recordings renamed by the user's naming scheme
// (media_naming.go) in one go — the "rename this whole season" case. Still
// preview and confirm (PRINCIPLES.md): the plan lists every recording
// before → after, and what it leaves alone and why; one confirm applies
// all of it, all or nothing (a failure rolls every recording back).

type batchRenameRequest struct {
	Paths []string `json:"paths"`
	// Source of the fields: "metadata" (the recording's own .ts.meta and
	// folder names, default) or "tmdb" (metadata, completed by TMDB: a
	// film's year, an episode's title — only on an exact title match).
	Source string `json:"source"`
	// Folders also sorts recordings into folders by the folder schemes
	// (Settings → Recordings), relative to their recording folder;
	// missing folders are created on apply.
	Folders bool `json:"folders"`
}

// Batch item states.
const (
	BatchRename    = "rename"
	BatchUnchanged = "unchanged" // already named by the scheme
	BatchSkipped   = "skipped"   // see Reason
)

type batchItem struct {
	Path   string `json:"path"`
	Before string `json:"before"` // base name, without .ts
	After  string `json:"after,omitempty"`
	// Folder is the new folder relative to the recording folder, set when
	// the recording moves ("FBI - Most Wanted/Staffel 05").
	Folder string        `json:"folder,omitempty"`
	Files  int           `json:"files,omitempty"`
	Fields naming.Fields `json:"fields"`
	Status string        `json:"status"`
	// Reason for a skip: an error code under error.media (target_exists,
	// duplicate_target, invalid_name, media_in_use, not_a_recording).
	Reason string `json:"reason,omitempty"`
}

// maxBatchTMDB caps parallel TMDB lookups, and batchTMDBTimeout the whole
// batch's, so 200 recordings don't hang the preview.
const (
	maxBatchTMDB     = 4
	batchTMDBTimeout = 15 * time.Second
)

func (h *Media) planBatchRename(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	var req batchRenameRequest
	if err := httpx.DecodeJSON(r, &req); err != nil || len(req.Paths) == 0 || len(req.Paths) > maxPathsPerPlan ||
		(req.Source != "" && req.Source != "metadata" && req.Source != "tmdb") {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", fmt.Sprintf(`expected {"paths": [...] (1–%d), "source": "metadata"|"tmdb"}`, maxPathsPerPlan))
		return
	}
	roots, ok := h.requireFileOps(w, r)
	if !ok {
		return
	}

	var tmdbKey, tmdbLang string
	if req.Source == "tmdb" {
		v, err := h.Settings.Values(ctx, user)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read settings")
			return
		}
		if tmdbKey = v.String(connector.KeyTMDBAPIKey); tmdbKey == "" || h.TMDB == nil {
			httpx.WriteError(w, r, http.StatusConflict, "tmdb_not_configured", "configure a TMDB API key in Settings first")
			return
		}
		tmdbLang = tmdbLanguage(v)
	}

	inUse, err := h.inUseChecker(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not check whether the recordings are in use")
		return
	}
	tpls := h.templates(ctx)
	channels := map[string]map[string]string{} // folder → file → channel

	items := make([]batchItem, 0, len(req.Paths))
	seen := map[string]bool{}
	for _, raw := range req.Paths {
		p, err := recfs.CleanIn(h.FS, raw, roots)
		if err == nil {
			_, err = recfs.Files(h.FS, p)
		}
		if err != nil {
			items = append(items, batchItem{Path: raw, Before: recfs.Base(raw), Status: BatchSkipped, Reason: recfsCode(err)})
			continue
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		meta := h.readMeta(p, roots)
		items = append(items, batchItem{Path: p, Before: recfs.Base(p), Fields: naming.FromMetadata(meta.title, meta.description, meta.folders)})
	}

	if req.Source == "tmdb" {
		h.completeFromTMDB(ctx, tmdbKey, tmdbLang, items)
	}

	// Names, then everything that would make a rename go wrong.
	var moves []recfs.Move
	var changes []plan.Change
	var renamed []string
	targets := map[string]bool{}
	for i := range items {
		it := &items[i]
		if it.Status == BatchSkipped {
			continue
		}
		dir := path.Dir(it.Path)
		if channels[dir] == nil {
			channels[dir] = h.channelsIn(ctx, dir)
		}
		extra := h.extraFor(it.Path, channels[dir])
		name := tpls.Render(it.Fields, extra)
		targetDir := dir
		if req.Folders {
			if folder := tpls.Folder(it.Fields, extra); folder != "" {
				root := rootOf(it.Path, roots)
				if _, err := recfs.Clean(path.Join(root, folder, "x"), roots); err == nil {
					targetDir = path.Join(root, folder)
				}
			}
		}
		if targetDir != dir {
			it.Folder = strings.TrimPrefix(targetDir, rootOf(it.Path, roots)+"/")
		}
		switch {
		case !it.Fields.Valid() || name == "":
			it.Status, it.Reason = BatchSkipped, recfs.ErrInvalidName.Error()
			continue
		case name == it.Before && targetDir == dir:
			it.Status, it.After = BatchUnchanged, name
			continue
		case targets[path.Join(targetDir, name)]:
			it.Status, it.After, it.Reason = BatchSkipped, name, "duplicate_target"
			continue
		}
		if why := inUse(it.Path); why != "" {
			it.Status, it.Reason = BatchSkipped, CodeMediaInUse
			continue
		}
		ms, err := recfs.PlanRenameTo(h.FS, it.Path, targetDir, name)
		if err != nil {
			it.Status, it.After, it.Reason = BatchSkipped, name, recfsCode(err)
			continue
		}
		targets[path.Join(targetDir, name)] = true
		it.Status, it.After, it.Files = BatchRename, name, len(ms)
		moves = append(moves, ms...)
		renamed = append(renamed, it.Path)
		changes = append(changes, plan.Change{Type: "recording.rename", Before: it.Path, After: ms[len(ms)-1].To})
	}

	// Folders the moves need: listed in the preview, created on apply. A
	// path through a link is refused outright.
	dirs, err := recfs.MissingDirs(h.FS, moves)
	if err != nil {
		writeRecfsError(w, r, err)
		return
	}
	for _, d := range dirs {
		changes = append(changes, plan.Change{Type: "folder.create", After: d})
	}

	out := map[string]any{"items": items}
	if len(moves) == 0 {
		httpx.WriteJSON(w, http.StatusOK, out) // nothing to do: no plan
		return
	}
	out["folders"] = dirs
	fp := h.movesFingerprint(moves)
	current, _ := fp(ctx)
	out["plan"] = h.Plans.Create(user, plan.Draft{
		Action:      "media.batchRename",
		Changes:     changes,
		Summary:     fmt.Sprintf("Renamed %d recording(s) by the naming scheme (%d files, %d new folders)", len(renamed), len(moves), len(dirs)),
		Detail:      map[string]any{"moves": moves},
		Fingerprint: current,
		Current:     fp,
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			if err := h.inUseFailure(ctx, renamed); err != nil {
				return err
			}
			defer h.refreshIndex(context.WithoutCancel(ctx))
			// Everything at once: a failure halfway renames every recording
			// back and removes the folders made for them.
			dirs, err := recfs.MissingDirs(h.FS, moves)
			if err != nil {
				return err
			}
			return recfs.ApplyInto(h.FS, dirs, moves)
		},
	})
	httpx.WriteJSON(w, http.StatusOK, out)
}

// completeFromTMDB fills in what the metadata lacks — a film's year, an
// episode's title — from TMDB, only on an exact title match (a wrong year
// in a file name is worse than none). Lookups run a few at a time within
// a time budget; whatever doesn't answer in time keeps its metadata.
func (h *Media) completeFromTMDB(ctx context.Context, key, lang string, items []batchItem) {
	ctx, cancel := context.WithTimeout(ctx, batchTMDBTimeout)
	defer cancel()
	sem := make(chan struct{}, maxBatchTMDB)
	var wg sync.WaitGroup
	for i := range items {
		it := &items[i]
		if it.Status == BatchSkipped || it.Fields.Title == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			results, err := h.TMDB.Search(ctx, key, it.Fields.Title, lang)
			if err != nil {
				return
			}
			want := titles.Normalize(it.Fields.Title)
			for _, res := range results {
				if titles.Normalize(res.Title) != want && titles.Normalize(res.OriginalTitle) != want {
					continue
				}
				if it.Fields.IsEpisode() && res.Kind == "tv" {
					if it.Fields.EpisodeTitle == "" {
						if t, err := h.TMDB.EpisodeTitle(ctx, key, res.ID, it.Fields.Season, it.Fields.Episode, lang); err == nil {
							it.Fields.EpisodeTitle = t
						}
					}
					return
				}
				if !it.Fields.IsEpisode() && res.Kind == "movie" {
					if it.Fields.Year == 0 {
						it.Fields.Year = res.Year
					}
					return
				}
			}
		}()
	}
	wg.Wait()
}

// inUseChecker reads the box's timers and what's playing once, then
// answers "recording"/"playing"/"" per path — a batch of 200 would
// otherwise cost 400 requests to the box.
func (h *Media) inUseChecker(ctx context.Context) (func(string) string, error) {
	timers, err := h.WC.TimerList(ctx)
	if err != nil {
		return nil, err
	}
	playing, err := h.WC.CurrentServiceReference(ctx)
	if err != nil {
		return nil, err
	}
	writing := map[string]bool{}
	for _, t := range timers {
		if t.State == webcontrol.TimerRunning && !t.Disabled && !t.JustPlay && t.Filename != "" {
			writing[path.Clean("/"+strings.TrimLeft(t.Filename, "/"))] = true
		}
	}
	played := ""
	if i := strings.Index(playing, ":/"); i >= 0 {
		played = path.Clean(playing[i+1:])
	}
	return func(p string) string {
		switch {
		case writing[p]:
			return "recording"
		case p == played:
			return "playing"
		}
		return ""
	}, nil
}

// rootOf is the recording folder p lies in.
func rootOf(p string, roots []string) string {
	for _, r := range roots {
		r = path.Clean(r)
		if strings.HasPrefix(p, r+"/") {
			return r
		}
	}
	return path.Dir(p)
}

// recfsCode is a recfs error as the API's error code.
func recfsCode(err error) string {
	for _, e := range []error{recfs.ErrNotARecording, recfs.ErrOutsideFolders, recfs.ErrInvalidName, recfs.ErrSameName, recfs.ErrTargetExists} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}
	return recfs.ErrNotARecording.Error()
}
