package hubs

// Ad detection (docs/ADSCAN_PLAN.md, #44, phase 1): MediaHub finds the ad
// breaks of recordings in the background queue (internal/jobs), by the
// channel's logo (internal/adscan). It only finds — the result is a
// suggestion; writing it into the recording's cut list comes later, as a
// plan to confirm, once the cut-list format is confirmed on the box.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/adscan"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/jobs"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
)

// JobAdScan is the ad-detection job kind.
const JobAdScan = "media.adscan"

// Error codes (i18n keys under "error.media").
const (
	CodeAdScanUnavailable = "adscan_unavailable"
	CodeAdScanNotAnalysed = "adscan_not_analysed"
)

type adScanParams struct {
	Path string `json:"path"`
}

// RegisterJobs registers MediaHub's job kinds on the queue.
func (h *Media) RegisterJobs(q *jobs.Service) {
	h.Jobs = q
	q.Register(jobs.Kind{Name: JobAdScan, Run: h.runAdScan, Resumable: true})
	// Not resumable: a cut interrupted halfway starts over (its partial
	// file is removed), and only when asked again.
	q.Register(jobs.Kind{Name: JobTrim, Run: h.runTrim, Abandon: h.abandonTrim})
}

// adScanAvailable: an Opener (FFmpeg found, or mock mode), the queue, and
// the recordings on this file system.
func (h *Media) adScanAvailable(ctx context.Context, roots []string) bool {
	return h.AdScan != nil && h.Jobs != nil && onBox(ctx, h.FS, roots)
}

// startAdScan queues one analysis per recording: POST {paths: [...]}.
// Analysing changes nothing, so there is no plan; a recording already
// queued or running isn't queued twice.
func (h *Media) startAdScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Paths) == 0 || len(req.Paths) > maxPathsPerPlan {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "paths must hold 1–200 recordings")
		return
	}
	roots, err := h.roots(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	if !h.adScanAvailable(r.Context(), roots) {
		httpx.WriteError(w, r, http.StatusConflict, CodeAdScanUnavailable, "ad detection needs FFmpeg and DreamHub running on the box")
		return
	}
	busy, err := h.inUseSet(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not check whether the recordings are in use")
		return
	}
	type queued struct {
		Path     string `json:"path"`
		JobID    int64  `json:"jobId,omitempty"`
		Existing bool   `json:"existing,omitempty"`
		Reason   string `json:"reason,omitempty"` // why it wasn't queued
	}
	out := make([]queued, 0, len(req.Paths))
	me, _ := httpx.User(r.Context())
	for _, raw := range req.Paths {
		p, err := recfs.CleanIn(h.FS, raw, roots)
		if err != nil || !strings.HasSuffix(p, ".ts") {
			out = append(out, queued{Path: raw, Reason: "not_a_recording"})
			continue
		}
		// A recording still being written has no end yet.
		if busy[p] == "recording" {
			out = append(out, queued{Path: p, Reason: CodeMediaInUse})
			continue
		}
		j, existing, err := h.Jobs.Enqueue(r.Context(), JobAdScan, p, me.ID, adScanParams{Path: p})
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not queue the analysis")
			return
		}
		out = append(out, queued{Path: p, JobID: j.ID, Existing: existing})
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"items": out})
}

// adScanResult answers GET ?path=: the recording's last analysis.
func (h *Media) adScanResult(w http.ResponseWriter, r *http.Request) {
	// The box's movie list may report "//media/…"; results are kept under
	// the clean path.
	p := path.Clean("/" + strings.TrimLeft(r.URL.Query().Get("path"), "/"))
	res, ok, err := h.Store.AdScanResult(r.Context(), p)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load the analysis")
		return
	}
	if !ok {
		httpx.WriteError(w, r, http.StatusNotFound, CodeAdScanNotAnalysed, "this recording hasn't been analysed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"path": res.Path, "serviceRef": res.ServiceRef, "analyzedAt": res.AnalyzedAt, "result": res.Result,
	})
}

// runAdScan is the job: open the recording, reuse the channel's logo if
// one was learned, find the breaks, keep the result.
func (h *Media) runAdScan(ctx context.Context, run *jobs.Run) error {
	var p adScanParams
	if err := run.Params(&p); err != nil {
		return jobs.Fail("invalid_params", err)
	}
	if h.AdScan == nil {
		return jobs.Fail(CodeAdScanUnavailable, nil)
	}
	// The recording may have been renamed, trashed or started being
	// recorded again since it was queued.
	if _, err := h.FS.Lstat(p.Path); err != nil {
		return jobs.Fail("not_found", err)
	}
	if why, err := h.inUse(ctx, p.Path); err == nil && why == "recording" {
		return jobs.Fail(CodeMediaInUse, nil)
	}
	sref := h.serviceRef(p.Path)

	var known *adscan.Mask
	if sref != "" {
		if l, ok, err := h.Store.AdScanLogo(ctx, sref); err == nil && ok {
			var m adscan.Mask
			if json.Unmarshal(l.Mask, &m) == nil {
				known = &m
			}
		}
	}

	src, err := h.AdScan.Open(p.Path)
	if err != nil {
		return jobs.Fail("adscan_open_failed", err)
	}
	defer src.Close()
	res, mask, err := adscan.Analyze(ctx, src, known, adscan.Options{
		Wait: run.Wait,
		Progress: func(done, total int) {
			run.Progress(int64(done), int64(total), "jobs.media.adscan.progress", map[string]string{"name": recfs.Base(p.Path)})
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, adscan.ErrNoFrame) {
			return jobs.Fail("adscan_no_frames", err)
		}
		return jobs.Fail("adscan_failed", err)
	}
	// A learned mask becomes the channel's, unless it is weak and a good
	// one is already known.
	if res.Learned && sref != "" && (!mask.Weak() || known == nil || known.Weak()) {
		if raw, err := json.Marshal(mask); err == nil {
			if err := h.Store.SaveAdScanLogo(ctx, sref, raw, p.Path); err != nil {
				return err
			}
		}
	}
	raw, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return h.Store.SaveAdScanResult(ctx, p.Path, sref, raw)
}

// serviceRef reads the channel from the recording's .ts.meta (line 1), the
// key a logo is learned under; "" when there's none (then nothing is
// reused or kept per channel).
func (h *Media) serviceRef(tsPath string) string {
	data, err := h.FS.ReadFile(tsPath + ".meta")
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(line)
}

// adSummary is what the recording list shows of an analysis: how many
// breaks, how long, how many uncertain.
type adSummary struct {
	Blocks    int     `json:"blocks"`
	Seconds   float64 `json:"seconds"`
	Uncertain int     `json:"uncertain,omitempty"`
	Trimmed   bool    `json:"trimmed,omitempty"`
	Marked    bool    `json:"marked,omitempty"`
}

func (h *Media) adSummary(ctx context.Context, tsPath string) *adSummary {
	if h.Store == nil {
		return nil
	}
	stored, ok, err := h.Store.AdScanResult(ctx, tsPath)
	if err != nil || !ok {
		return nil
	}
	var res adscan.Result
	if json.Unmarshal(stored.Result, &res) != nil {
		return nil
	}
	sum := &adSummary{Blocks: len(res.Blocks), Trimmed: res.Trimmed, Marked: res.Marked}
	for _, b := range res.Blocks {
		sum.Seconds += b.End - b.Start
		if b.Uncertain {
			sum.Uncertain++
		}
	}
	return sum
}

// followMoves keeps ad-detection results with their recordings when they
// are renamed, trashed (x.ts → x.ts.trash) or restored. Best effort: a
// result lost here only means analysing again.
func (h *Media) followMoves(ctx context.Context, moves []recfs.Move) {
	if h.Store == nil {
		return
	}
	for _, m := range moves {
		if strings.HasSuffix(m.From, ".ts") || strings.HasSuffix(m.From, ".ts"+recfs.TrashSuffix) {
			_ = h.Store.MoveAdScanResult(context.WithoutCancel(ctx), m.From, m.To)
		}
	}
}

// forgetResults drops the results of recordings deleted for good.
func (h *Media) forgetResults(ctx context.Context, files []string) {
	if h.Store == nil {
		return
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".ts"+recfs.TrashSuffix) || strings.HasSuffix(f, ".ts") {
			_ = h.Store.DeleteAdScanResult(context.WithoutCancel(ctx), f)
		}
	}
}
