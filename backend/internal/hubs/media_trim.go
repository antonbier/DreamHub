package hubs

// Trimming (#27, first part): cutting the ad breaks ad detection found out
// of a recording, losslessly (internal/trim). Preview and confirm as
// always; applying queues a background job (a cut copies gigabytes). The
// original is kept: renamed to "<name> - original" with all its files and
// moved to the trash, so it can be restored next to the cut version. The
// cut version takes the original name, with the .meta, .eit and cover
// copied over; the old index files (.ap, .sc, .cuts) stay with the
// original — their positions don't fit the new file, and Enigma2 plays a
// recording without them.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/adscan"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/jobs"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/trim"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// JobTrim is the trimming job kind.
const JobTrim = "media.trim"

// Error codes (i18n keys under "error.media").
const (
	CodeTrimUnavailable  = "trim_unavailable"
	CodeTrimInvalid      = "trim_invalid_blocks"
	CodeTrimNoSpace      = "trim_not_enough_space"
	CodeTrimOriginalName = "trim_original_exists"
)

// originalSuffix marks the kept original's name.
const originalSuffix = " - original"

// The files the cut version gets copied from the original: what the box
// shows about it (title, description, programme data, cover). Not the
// index files, whose byte and time positions belong to the old file.
var trimCopied = []string{".ts.meta", ".eit", ".jpg", ".png", ".backdrop.jpg"}

type trimParams struct {
	Path     string        `json:"path"`
	Keep     []adscan.Span `json:"keep"`
	Duration float64       `json:"duration"`
}

// trimPlan previews cutting blocks out of a recording:
// POST {path, blocks: [{start, end}]} — the blocks the user ticked of its
// last analysis.
func (h *Media) trimPlan(w http.ResponseWriter, r *http.Request) {
	user, _ := httpx.User(r.Context())
	var req struct {
		Path   string        `json:"path"`
		Blocks []adscan.Span `json:"blocks"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Path == "" || len(req.Blocks) == 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"path": "...", "blocks": [{"start", "end"}, ...]}`)
		return
	}
	roots, ok := h.requireFileOps(w, r)
	if !ok {
		return
	}
	if h.Trim == nil || h.Jobs == nil {
		httpx.WriteError(w, r, http.StatusConflict, CodeTrimUnavailable, "trimming needs FFmpeg on the box")
		return
	}
	p, err := recfs.CleanIn(h.FS, req.Path, roots)
	if err != nil || !strings.HasSuffix(p, ".ts") {
		writeRecfsError(w, r, recfs.ErrNotARecording)
		return
	}
	stored, found, err := h.Store.AdScanResult(r.Context(), p)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load the analysis")
		return
	}
	var res adscan.Result
	if !found || json.Unmarshal(stored.Result, &res) != nil {
		httpx.WriteError(w, r, http.StatusConflict, CodeAdScanNotAnalysed, "detect the ads first")
		return
	}
	for _, b := range req.Blocks {
		if !(b.Start >= 0 && b.End > b.Start && b.End <= res.Duration+1) {
			httpx.WriteError(w, r, http.StatusBadRequest, CodeTrimInvalid, "every block must lie inside the recording")
			return
		}
	}
	keep := trim.Keep(res.Duration, req.Blocks)
	if len(keep) == 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, CodeTrimInvalid, "nothing of the recording would be left")
		return
	}
	if !h.checkNotInUse(w, r, []string{p}) {
		return
	}
	// The original's new name must be free — also of an earlier cut's
	// original, whose files stay next to its trashed .ts.
	base := recfs.Base(p)
	origBase, ok := h.originalBase(path.Dir(p), base)
	if !ok {
		httpx.WriteError(w, r, http.StatusConflict, CodeTrimOriginalName, "no free name for keeping the original next to the recording")
		return
	}
	// The cut version is written next to the original first.
	var size int64
	if fi, err := h.FS.Lstat(p); err == nil {
		size = fi.Size()
	}
	newSize := int64(float64(size) * trim.Length(keep) / max(res.Duration, 1))
	if freer, ok := h.FS.(recfs.Freer); ok {
		if free, err := freer.Free(path.Dir(p)); err == nil && free < uint64(newSize)+256<<20 {
			httpx.WriteErrorDetails(w, r, http.StatusConflict, CodeTrimNoSpace, "not enough free space for the cut version", map[string]any{"needed": newSize, "free": free})
			return
		}
	}

	cut := res.Duration - trim.Length(keep)
	changes := []plan.Change{
		{Type: "recording.trim", Before: map[string]any{"path": p, "duration": res.Duration, "size": size},
			After: map[string]any{"path": p, "duration": trim.Length(keep), "size": newSize, "removed": cut, "blocks": len(req.Blocks)}},
		{Type: "recording.trash", Before: p, After: path.Join(path.Dir(p), origBase+".ts"+recfs.TrashSuffix)},
	}
	pl := h.Plans.Create(r.Context(), user, plan.Draft{
		Action:      "media.trim",
		Changes:     changes,
		Summary:     fmt.Sprintf("Cut %d ad break(s), %.0f s, out of %s; the original went to the trash", len(req.Blocks), cut, base),
		Detail:      map[string]any{"path": p, "keep": keep},
		Fingerprint: h.filesFingerprint([]string{p}),
		Current:     func(context.Context) (string, error) { return h.filesFingerprint([]string{p}), nil },
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			if err := h.inUseFailure(ctx, []string{p}); err != nil {
				return err
			}
			_, _, err := h.Jobs.Enqueue(ctx, JobTrim, p, user.ID, trimParams{Path: p, Keep: keep, Duration: res.Duration})
			return err
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl, "keep": keep})
}

// runTrim is the job: cut into a hidden file next to the original, then
// swap — the original renamed and trashed, the cut version in its place.
func (h *Media) runTrim(ctx context.Context, run *jobs.Run) error {
	var p trimParams
	if err := run.Params(&p); err != nil {
		return jobs.Fail("invalid_params", err)
	}
	if h.Trim == nil {
		return jobs.Fail(CodeTrimUnavailable, nil)
	}
	if _, err := h.FS.Lstat(p.Path); err != nil {
		return jobs.Fail("not_found", err)
	}
	if why, err := h.inUse(ctx, p.Path); err == nil && why != "" {
		return jobs.Fail(CodeMediaInUse, nil)
	}
	dir, base := path.Dir(p.Path), recfs.Base(p.Path)
	tmp := trimTemp(dir, base)
	_ = h.FS.Remove(tmp) // a leftover from an interrupted run

	name := map[string]string{"name": base}
	run.Progress(0, 100, "jobs.media.trim.progress", name)
	err := h.Trim.Cut(ctx, p.Path, tmp, p.Keep, p.Duration, func(share float64) {
		run.Progress(int64(share*100), 100, "jobs.media.trim.progress", name)
	})
	if err != nil {
		_ = h.FS.Remove(tmp)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return jobs.Fail("trim_failed", err)
	}
	if err := h.swapTrimmed(ctx, p.Path, tmp); err != nil {
		_ = h.FS.Remove(tmp)
		return err
	}
	h.refreshIndex(context.WithoutCancel(ctx))

	// The cut version has no ads left; say so instead of showing the old
	// breaks against the new length.
	if raw, err := json.Marshal(adscan.Result{Duration: trim.Length(p.Keep), Blocks: []adscan.Block{}, Trimmed: true}); err == nil {
		_ = h.Store.SaveAdScanResult(context.WithoutCancel(ctx), p.Path, h.serviceRef(p.Path), raw)
	}
	return nil
}

// swapTrimmed puts the cut file tmp in place of the recording at p: the
// original's files are renamed to "<name> - original", the shown sidecars
// copied back to p's name, tmp renamed to p, then the original trashed.
// Up to the rename of tmp, a failure puts everything back.
func (h *Media) swapTrimmed(ctx context.Context, p, tmp string) error {
	// Nothing may have started recording or playing it during the cut.
	if why, err := h.inUse(ctx, p); err == nil && why != "" {
		return jobs.Fail(CodeMediaInUse, nil)
	}
	dir, base := path.Dir(p), recfs.Base(p)
	origBase, ok := h.originalBase(dir, base)
	if !ok {
		return jobs.Fail(CodeTrimOriginalName, nil)
	}
	moves, err := recfs.PlanRename(h.FS, p, origBase)
	if err != nil {
		return jobs.Fail(CodeTrimOriginalName, err)
	}
	if err := recfs.Apply(h.FS, moves); err != nil {
		return jobs.Fail("trim_swap_failed", err)
	}
	h.followMoves(ctx, moves)
	undo := func() {
		back := make([]recfs.Move, len(moves))
		for i, m := range moves {
			back[len(moves)-1-i] = recfs.Move{From: m.To, To: m.From}
		}
		_ = recfs.Apply(h.FS, back)
		h.followMoves(ctx, back)
	}

	var copied []string
	for _, ext := range trimCopied {
		data, err := h.FS.ReadFile(path.Join(dir, origBase+ext))
		if err != nil {
			continue // the original has no such file
		}
		to := path.Join(dir, base+ext)
		if err := h.FS.Create(to, data); err != nil {
			for _, c := range copied {
				_ = h.FS.Remove(c)
			}
			undo()
			return jobs.Fail("trim_swap_failed", err)
		}
		copied = append(copied, to)
	}
	if err := h.FS.Rename(tmp, p); err != nil {
		for _, c := range copied {
			_ = h.FS.Remove(c)
		}
		undo()
		return jobs.Fail("trim_swap_failed", err)
	}

	// The original into the trash. Should that fail, it stays next to the
	// cut version under its "- original" name — nothing is lost.
	origTs := path.Join(dir, origBase+".ts")
	if m, err := recfs.PlanTrash(h.FS, origTs); err == nil {
		if err := recfs.Apply(h.FS, []recfs.Move{m}); err == nil {
			h.followMoves(ctx, []recfs.Move{m})
		}
	}
	return nil
}

// trimTemp is where a cut is written first: next to the recording (same
// file system, so the final rename is instant), hidden and not ending in
// .ts, so the box's lists ignore it meanwhile.
func trimTemp(dir, base string) string {
	return path.Join(dir, "."+base+".dreamhub-trim")
}

// abandonTrim removes the half-written cut of a job a restart interrupted.
func (h *Media) abandonTrim(_ context.Context, j store.Job) {
	var p trimParams
	if json.Unmarshal(j.Params, &p) != nil || p.Path == "" {
		return
	}
	tmp := trimTemp(path.Dir(p.Path), recfs.Base(p.Path))
	_ = h.FS.Remove(tmp)
	_ = h.FS.Remove(tmp + ".list")
}

// originalBase finds a name for the kept original: "<name> - original",
// else "<name> - original 2" … — none of whose files may exist yet (an
// earlier cut's original keeps its sidecars next to its trashed .ts).
func (h *Media) originalBase(dir, base string) (string, bool) {
	for n := 1; n <= 9; n++ {
		cand := base + originalSuffix
		if n > 1 {
			cand += fmt.Sprintf(" %d", n)
		}
		if _, err := recfs.ValidName(cand); err != nil {
			return "", false
		}
		free := true
		for _, ext := range []string{".ts", ".ts" + recfs.TrashSuffix, ".ts.meta", ".ts.ap", ".ts.sc", ".ts.cuts", ".eit", ".jpg", ".backdrop.jpg"} {
			if _, err := h.FS.Lstat(path.Join(dir, cand+ext)); err == nil {
				free = false
				break
			}
		}
		if free {
			return cand, true
		}
	}
	return "", false
}
