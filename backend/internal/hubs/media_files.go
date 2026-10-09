package hubs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// MediaHub's file operations (docs/V1_PLAN.md, WP4): rename, trash, restore,
// empty — each a plan (PRINCIPLES.md, "Preview and confirm"), each refused
// while the recording is being written or played ("never touch a file
// that's currently being recorded or played"), checked at preview *and*
// again at apply. The rules for which files belong to a recording and how
// the trash works live in internal/recfs.

// maxPathsPerPlan bounds one bulk request.
const maxPathsPerPlan = 200

// Error codes (i18n keys under "error.media"; recfs's own codes join them).
const (
	CodeFileOpsUnavailable = "file_ops_unavailable"
	CodeMediaInUse         = "media_in_use"
)

// fileOpsState tells the client whether file operations work here, and if
// not, why — off the box they're unavailable, not broken.
type fileOpsState struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"` // i18n key under "media.fileOps"
}

func (h *Media) fileOps(roots []string) fileOpsState {
	if recfs.Available(h.FS, roots) {
		return fileOpsState{Available: true}
	}
	return fileOpsState{Reason: "notOnBox"}
}

// roots returns the box's recording folders.
func (h *Media) roots(ctx context.Context) ([]string, error) {
	return h.WC.Locations(ctx)
}

// requireFileOps loads the roots and answers the request itself when file
// operations aren't possible.
func (h *Media) requireFileOps(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	roots, err := h.roots(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return nil, false
	}
	if !recfs.Available(h.FS, roots) {
		httpx.WriteError(w, r, http.StatusConflict, CodeFileOpsUnavailable, "file operations need DreamHub to run on the box itself")
		return nil, false
	}
	return roots, true
}

// inUse reports why tsPath must not be touched right now: "recording" (a
// running timer writes it) or "playing" (the box plays it), else "".
func (h *Media) inUse(ctx context.Context, tsPath string) (string, error) {
	busy, err := h.inUseSet(ctx)
	if err != nil {
		return "", err
	}
	return busy[tsPath], nil
}

// inUseSet maps every .ts path the box is writing or playing right now to
// the reason, "recording" or "playing" — two WebControl calls, however many
// paths are checked against it.
func (h *Media) inUseSet(ctx context.Context) (map[string]string, error) {
	timers, err := h.WC.TimerList(ctx)
	if err != nil {
		return nil, err
	}
	busy := map[string]string{}
	for _, t := range timers {
		if t.State == webcontrol.TimerRunning && !t.Disabled && !t.JustPlay && t.Filename != "" {
			busy[path.Clean("/"+strings.TrimLeft(t.Filename, "/"))] = "recording"
		}
	}
	playing, err := h.WC.CurrentServiceReference(ctx)
	if err != nil {
		return nil, err
	}
	// A recording's reference carries its path after the ten numeric fields:
	// "1:0:0:0:0:0:0:0:0:0:/media/hdd/movie/x.ts".
	if i := strings.Index(playing, ":/"); i >= 0 {
		if p := path.Clean(playing[i+1:]); busy[p] == "" {
			busy[p] = "playing"
		}
	}
	return busy, nil
}

// checkNotInUse answers the request when a path is in use (or can't be
// checked) and reports whether to continue.
func (h *Media) checkNotInUse(w http.ResponseWriter, r *http.Request, paths []string) bool {
	for _, p := range paths {
		why, err := h.inUse(r.Context(), p)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not check whether the recording is in use")
			return false
		}
		if why != "" {
			httpx.WriteErrorDetails(w, r, http.StatusConflict, CodeMediaInUse, "the recording is being recorded or played right now", map[string]any{"path": p, "reason": why})
			return false
		}
	}
	return true
}

// inUseFailure is the apply-time version of checkNotInUse.
func (h *Media) inUseFailure(ctx context.Context, paths []string) error {
	for _, p := range paths {
		why, err := h.inUse(ctx, p)
		if err != nil {
			return err
		}
		if why != "" {
			return &plan.Failure{Status: http.StatusConflict, Code: CodeMediaInUse, Message: "the recording started being recorded or played since the preview; nothing was changed", Details: map[string]any{"path": p, "reason": why}}
		}
	}
	return nil
}

// writeRecfsError maps recfs's errors onto the API's error envelope.
func writeRecfsError(w http.ResponseWriter, r *http.Request, err error) {
	status := map[error]int{
		recfs.ErrNotARecording:  http.StatusNotFound,
		recfs.ErrNotInTrash:     http.StatusNotFound,
		recfs.ErrOutsideFolders: http.StatusBadRequest,
		recfs.ErrInvalidName:    http.StatusBadRequest,
		recfs.ErrSameName:       http.StatusBadRequest,
		recfs.ErrInvalidTitle:   http.StatusBadRequest,
		recfs.ErrNoMeta:         http.StatusConflict,
		recfs.ErrTargetExists:   http.StatusConflict,
		recfs.ErrRestoreBlocked: http.StatusConflict,
	}
	for e, code := range status {
		if errors.Is(err, e) {
			httpx.WriteError(w, r, code, e.Error(), "the file operation isn't possible")
			return
		}
	}
	httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "file operation failed")
}

// filesFingerprint identifies the current state of a set of files (names
// and sizes), so a plan is rejected as stale when they changed.
func (h *Media) filesFingerprint(files []string) string {
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	sum := sha256.New()
	for _, f := range sorted {
		size := int64(-1)
		if fi, err := h.FS.Lstat(f); err == nil {
			size = fi.Size()
		}
		fmt.Fprintf(sum, "%s\x00%d\n", f, size)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func (h *Media) movesFingerprint(moves []recfs.Move) func(ctx context.Context) (string, error) {
	var files []string
	for _, m := range moves {
		files = append(files, m.From, m.To)
	}
	return func(context.Context) (string, error) { return h.filesFingerprint(files), nil }
}

// --- rename -----------------------------------------------------------------

type renameRequest struct {
	Path    string `json:"path"`    // the recording's .ts file, as listed
	NewName string `json:"newName"` // new base name, without ".ts"
	// Title, when set, also changes the title in the recording's .ts.meta —
	// what the box's own movie list shows. With NewName unchanged, only the
	// title changes.
	Title *string `json:"title,omitempty"`
}

func (h *Media) planRename(w http.ResponseWriter, r *http.Request) {
	user, _ := httpx.User(r.Context())
	var req renameRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"path", "newName"}`)
		return
	}
	roots, ok := h.requireFileOps(w, r)
	if !ok {
		return
	}
	p, err := recfs.CleanIn(h.FS, req.Path, roots)
	if err != nil {
		writeRecfsError(w, r, err)
		return
	}
	var title, oldTitle string
	if req.Title != nil {
		if title, err = recfs.ValidTitle(*req.Title); err != nil {
			writeRecfsError(w, r, err)
			return
		}
		if fi, err := h.FS.Lstat(p + ".meta"); err != nil || !fi.Mode().IsRegular() {
			writeRecfsError(w, r, recfs.ErrNoMeta)
			return
		}
		oldTitle = recfs.MetaTitle(h.FS, p)
		if title == oldTitle {
			title = "" // unchanged: nothing to write
		}
	}
	moves, err := recfs.PlanRename(h.FS, p, req.NewName)
	if errors.Is(err, recfs.ErrSameName) && title != "" {
		moves, err = nil, nil // only the title changes
	}
	if err != nil {
		writeRecfsError(w, r, err)
		return
	}
	if !h.checkNotInUse(w, r, []string{p}) {
		return
	}
	changes := make([]plan.Change, 0, len(moves)+1)
	for _, m := range moves {
		changes = append(changes, plan.Change{Type: "file.rename", Before: m.From, After: m.To})
	}
	if title != "" {
		changes = append(changes, plan.Change{Type: "meta.title", Before: oldTitle, After: title})
	}
	// The title is part of what the preview promised: changed since, the
	// plan is stale like a moved file would make it.
	movesFP := h.movesFingerprint(moves)
	fp := func(ctx context.Context) (string, error) {
		f, err := movesFP(ctx)
		return f + "|" + recfs.MetaTitle(h.FS, p), err
	}
	current, _ := fp(r.Context())
	newTS := p
	if len(moves) > 0 {
		newTS = moves[len(moves)-1].To
	}
	summary := fmt.Sprintf("Renamed recording: %s → %s (%d files)", path.Base(p), path.Base(newTS), len(moves))
	if len(moves) == 0 {
		summary = fmt.Sprintf("Changed a recording's title: %s → %s", oldTitle, title)
	} else if title != "" {
		summary += fmt.Sprintf(", title %s → %s", oldTitle, title)
	}
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "media.rename",
		Changes:     changes,
		Summary:     summary,
		Detail:      map[string]any{"moves": moves, "title": title},
		Fingerprint: current,
		Current:     fp,
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			if err := h.inUseFailure(ctx, []string{p}); err != nil {
				return err
			}
			defer h.refreshIndex(context.WithoutCancel(ctx))
			// The title first, on the file as it's named now; if the renames
			// then fail, the old content goes back: all or nothing.
			var oldMeta []byte
			if title != "" {
				var err error
				if oldMeta, err = recfs.SetMetaTitle(h.FS, p, title); err != nil {
					return err
				}
			}
			if err := recfs.Apply(h.FS, moves); err != nil {
				if oldMeta != nil {
					_ = h.FS.WriteFile(p+".meta", oldMeta)
				}
				return err
			}
			h.followMoves(ctx, moves)
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

// --- trash --------------------------------------------------------------------

type pathsRequest struct {
	Paths []string `json:"paths"`
}

func (h *Media) decodePaths(w http.ResponseWriter, r *http.Request, roots []string) ([]string, bool) {
	var req pathsRequest
	if err := httpx.DecodeJSON(r, &req); err != nil || len(req.Paths) == 0 || len(req.Paths) > maxPathsPerPlan {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", fmt.Sprintf(`expected {"paths": [...]} with 1–%d entries`, maxPathsPerPlan))
		return nil, false
	}
	out := make([]string, 0, len(req.Paths))
	seen := map[string]bool{}
	for _, raw := range req.Paths {
		p, err := recfs.CleanIn(h.FS, raw, roots)
		if err != nil {
			writeRecfsError(w, r, err)
			return nil, false
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, true
}

func (h *Media) planTrash(w http.ResponseWriter, r *http.Request) {
	user, _ := httpx.User(r.Context())
	roots, ok := h.requireFileOps(w, r)
	if !ok {
		return
	}
	paths, ok := h.decodePaths(w, r, roots)
	if !ok {
		return
	}
	moves := make([]recfs.Move, 0, len(paths))
	for _, p := range paths {
		m, err := recfs.PlanTrash(h.FS, p)
		if err != nil {
			writeRecfsError(w, r, err)
			return
		}
		moves = append(moves, m)
	}
	if !h.checkNotInUse(w, r, paths) {
		return
	}
	changes := make([]plan.Change, 0, len(moves))
	for _, m := range moves {
		changes = append(changes, plan.Change{Type: "recording.trash", Before: m.From, After: m.To})
	}
	fp := h.movesFingerprint(moves)
	current, _ := fp(r.Context())
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "media.trash",
		Changes:     changes,
		Summary:     fmt.Sprintf("Moved %d recording(s) to the trash", len(moves)),
		Detail:      map[string]any{"moves": moves},
		Fingerprint: current,
		Current:     fp,
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			if err := h.inUseFailure(ctx, paths); err != nil {
				return err
			}
			err := recfs.Apply(h.FS, moves)
			if err == nil {
				h.followMoves(ctx, moves)
			}
			h.refreshIndex(context.WithoutCancel(ctx))
			return err
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

func (h *Media) trashList(w http.ResponseWriter, r *http.Request) {
	roots, ok := h.requireFileOps(w, r)
	if !ok {
		return
	}
	items, err := recfs.ListTrash(h.FS, roots)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read the trash")
		return
	}
	if items == nil {
		items = []recfs.TrashItem{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Media) planRestore(w http.ResponseWriter, r *http.Request) {
	user, _ := httpx.User(r.Context())
	roots, ok := h.requireFileOps(w, r)
	if !ok {
		return
	}
	paths, ok := h.decodePaths(w, r, roots)
	if !ok {
		return
	}
	moves := make([]recfs.Move, 0, len(paths))
	changes := make([]plan.Change, 0, len(paths))
	for _, p := range paths {
		m, err := recfs.PlanRestore(h.FS, p)
		if err != nil {
			writeRecfsError(w, r, err)
			return
		}
		moves = append(moves, m)
		changes = append(changes, plan.Change{Type: "recording.restore", Before: m.From, After: m.To})
	}
	fp := h.movesFingerprint(moves)
	current, _ := fp(r.Context())
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "media.restore",
		Changes:     changes,
		Summary:     fmt.Sprintf("Restored %d recording(s) from the trash", len(moves)),
		Detail:      map[string]any{"moves": moves},
		Fingerprint: current,
		Current:     fp,
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			err := recfs.Apply(h.FS, moves)
			if err == nil {
				h.followMoves(ctx, moves)
			}
			h.refreshIndex(context.WithoutCancel(ctx))
			return err
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

// planEmpty previews deleting trashed recordings for good — the only real
// delete MediaHub does. The plan lists every file and the total size.
func (h *Media) planEmpty(w http.ResponseWriter, r *http.Request) {
	user, _ := httpx.User(r.Context())
	roots, ok := h.requireFileOps(w, r)
	if !ok {
		return
	}
	paths, ok := h.decodePaths(w, r, roots)
	if !ok {
		return
	}
	var files []string
	var size int64
	changes := []plan.Change{}
	for _, p := range paths {
		del, err := recfs.PlanEmpty(h.FS, p)
		if err != nil {
			writeRecfsError(w, r, err)
			return
		}
		for _, f := range del {
			if fi, err := h.FS.Lstat(f); err == nil {
				size += fi.Size()
			}
			changes = append(changes, plan.Change{Type: "file.delete", Before: f})
		}
		files = append(files, del...)
	}
	current := h.filesFingerprint(files)
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "media.emptyTrash",
		Changes:     changes,
		Summary:     fmt.Sprintf("Deleted %d recording(s) from the trash for good (%d files, %d bytes)", len(paths), len(files), size),
		Detail:      map[string]any{"files": files, "bytes": size},
		Fingerprint: current,
		Current:     func(context.Context) (string, error) { return h.filesFingerprint(files), nil },
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			err := recfs.Remove(h.FS, files)
			if err == nil {
				h.forgetResults(ctx, files)
			}
			h.refreshIndex(context.WithoutCancel(ctx))
			return err
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl, "bytes": size})
}
