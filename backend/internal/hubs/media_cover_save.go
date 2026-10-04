package hubs

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/enrich"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/naming"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Saving covers: the posters and backdrops enrichment found (TMDB) written
// next to recordings that have none, under the names the box's other
// plugins already use ("<base>.jpg", "<base>.backdrop.jpg", media_cover.go)
// — so Enigma2's own movie list and every other plugin see them too.
//
// Never overwrites: a recording that already has a poster keeps it (and
// its backdrop is only added when missing), checked at preview and again
// at write time (recfs.Create refuses an existing file). Preview and
// confirm like every file operation; all or nothing.

// Cover sizes written to the box: a poster large enough for a detail
// screen, a backdrop for a full-width background.
const (
	coverPosterSize   = "w342"
	coverBackdropSize = "w780"
)

// Cover item states and reasons.
const (
	CoverWrite    = "write"
	CoverHasCover = "has_cover"
	CoverSkipped  = "skipped"
	CodeNoMatch   = "no_match"
)

type coverSaveRequest struct {
	Paths []string `json:"paths"`
}

type coverItem struct {
	Path   string   `json:"path"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	Reason string   `json:"reason,omitempty"`
	Files  []string `json:"files,omitempty"` // names to be written
	// Poster is the TMDB image path, for the preview's thumbnail.
	Poster   string `json:"poster,omitempty"`
	backdrop string // TMDB image path, when a backdrop is written
}

// coverWrite is one file the plan writes.
type coverWrite struct {
	To   string `json:"to"`
	Size string `json:"size"`
	From string `json:"from"` // TMDB image path
}

func (h *Media) planSaveCovers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	var req coverSaveRequest
	if err := httpx.DecodeJSON(r, &req); err != nil || len(req.Paths) == 0 || len(req.Paths) > maxPathsPerPlan {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", fmt.Sprintf(`expected {"paths": [...] (1–%d)}`, maxPathsPerPlan))
		return
	}
	roots, ok := h.requireFileOps(w, r)
	if !ok {
		return
	}
	v, err := h.Settings.Values(ctx, user)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read settings")
		return
	}
	key := v.String(connector.KeyTMDBAPIKey)
	if key == "" || h.Enrich == nil {
		httpx.WriteError(w, r, http.StatusConflict, "tmdb_not_configured", "configure a TMDB API key in Settings first")
		return
	}

	items := make([]coverItem, 0, len(req.Paths))
	var queries []enrich.Query
	var asked []int // item index per query
	seen := map[string]bool{}
	for _, raw := range req.Paths {
		p, err := recfs.CleanIn(h.FS, raw, roots)
		if err == nil {
			_, err = recfs.Files(h.FS, p)
		}
		if err != nil {
			items = append(items, coverItem{Path: raw, Title: recfs.Base(raw), Status: CoverSkipped, Reason: recfsCode(err)})
			continue
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		meta := h.readMeta(p, roots)
		f := naming.FromMetadata(meta.title, meta.description, meta.folders)
		it := coverItem{Path: p, Title: f.Title}
		if it.Title == "" {
			it.Title = meta.title
		}
		if poster, backdrop := recfs.Covers(h.FS, p); poster != "" && backdrop != "" {
			it.Status = CoverHasCover
		} else {
			kind := ""
			if f.IsEpisode() {
				kind = "tv"
			}
			queries = append(queries, enrich.Query{Title: it.Title, Kind: kind})
			asked = append(asked, len(items))
		}
		items = append(items, it)
	}

	lang := enrich.Language(v.String(settings.KeyLanguage))
	for start := 0; start < len(queries); start += enrich.MaxBatch {
		end := min(start+enrich.MaxBatch, len(queries))
		results := h.Enrich.Lookup(ctx, key, lang, queries[start:end])
		for j, info := range results {
			h.planCoverItem(&items[asked[start+j]], info)
		}
	}

	var writes []coverWrite
	var changes []plan.Change
	var targets []string
	for i := range items {
		it := &items[i]
		if it.Status != CoverWrite {
			continue
		}
		dir := path.Dir(it.Path)
		for _, name := range it.Files {
			cw := coverWrite{To: path.Join(dir, name), Size: coverPosterSize, From: it.Poster}
			if strings.HasSuffix(name, ".backdrop.jpg") {
				cw.Size, cw.From = coverBackdropSize, it.backdrop
			}
			writes = append(writes, cw)
			targets = append(targets, cw.To)
			changes = append(changes, plan.Change{Type: "file.create", After: cw.To})
		}
	}

	out := map[string]any{"items": items}
	if len(writes) == 0 {
		httpx.WriteJSON(w, http.StatusOK, out) // nothing to do: no plan
		return
	}
	fp := func(context.Context) (string, error) { return h.filesFingerprint(targets), nil }
	current, _ := fp(ctx)
	out["plan"] = h.Plans.Create(user, plan.Draft{
		Action:      "media.saveCovers",
		Changes:     changes,
		Summary:     fmt.Sprintf("Saved %d cover file(s) next to recordings", len(writes)),
		Detail:      map[string]any{"writes": writes},
		Fingerprint: current,
		Current:     fp,
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			return h.writeCovers(ctx, writes)
		},
	})
	httpx.WriteJSON(w, http.StatusOK, out)
}

// planCoverItem decides what to write for one recording from what
// enrichment found: the poster under the name a picture of its type gets
// ("<base>.jpg"/".png"), the backdrop as "<base>.backdrop.jpg" — each only
// when the recording has none yet.
func (h *Media) planCoverItem(it *coverItem, info *enrich.Info) {
	poster, backdrop := recfs.Covers(h.FS, it.Path)
	if info == nil || info.Poster == "" {
		if poster != "" {
			it.Status = CoverHasCover // only a backdrop was missing
		} else {
			it.Status, it.Reason = CoverSkipped, CodeNoMatch
		}
		return
	}
	base := recfs.Base(it.Path)
	it.Poster = info.Poster
	if poster == "" {
		ext := path.Ext(info.Poster)
		if ext != ".jpg" && ext != ".png" {
			ext = ".jpg"
		}
		it.Files = append(it.Files, base+ext)
	}
	// Enigma2's convention only knows a JPEG backdrop.
	if backdrop == "" && strings.HasSuffix(info.Backdrop, ".jpg") {
		it.Files = append(it.Files, base+".backdrop.jpg")
		it.backdrop = info.Backdrop
	}
	if len(it.Files) == 0 {
		it.Status = CoverHasCover
		return
	}
	it.Status = CoverWrite
}

// writeCovers fetches every picture first (through enrichment's image
// cache), checks it really is one, then creates the files — all or
// nothing: a failure removes the files this call created.
func (h *Media) writeCovers(ctx context.Context, writes []coverWrite) error {
	data := make([][]byte, len(writes))
	for i, cw := range writes {
		b, ct, err := h.Enrich.Image(ctx, cw.Size, cw.From)
		if err != nil {
			return err
		}
		want := "image/jpeg"
		if strings.HasSuffix(cw.To, ".png") {
			want = "image/png"
		}
		if http.DetectContentType(b) != want || (ct != "" && ct != want) {
			return fmt.Errorf("cover %s: not a %s", cw.From, want)
		}
		data[i] = b
	}
	var made []string
	for i, cw := range writes {
		if err := h.FS.Create(cw.To, data[i]); err != nil {
			for _, f := range made {
				_ = h.FS.Remove(f)
			}
			return err
		}
		made = append(made, cw.To)
	}
	return nil
}
