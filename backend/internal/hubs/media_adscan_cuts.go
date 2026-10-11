package hubs

// Marking ad breaks in the cut list (#44, ROADMAP.md): writes the ticked
// breaks into the recording's .ts.cuts as IN/OUT pairs, so the box's own
// remote can jump past them ("jump to next marked position") — nothing in
// the recording itself changes, unlike trimming (media_trim.go). The
// format and the merge rule (keep the resume position, hand-made marks and
// any existing cut, unite overlapping regions) are confirmed on the
// reference box (docs/ADSCAN_PLAN.md, internal/adscan/sidecar.go) and only
// ever applied through a plan, per PRINCIPLES.md.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/adscan"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Error codes (i18n keys under "error.media").
const (
	CodeMarkInvalid = "mark_invalid_blocks"
	CodeMarkNoAP    = "mark_no_access_points"
)

// markPlan previews writing ticked ad breaks into the recording's cut
// list: POST {path, blocks: [{start, end}, ...]} — the same shape trimPlan
// takes, so the UI can offer this next to "Cut out the ads".
func (h *Media) markPlan(w http.ResponseWriter, r *http.Request) {
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
			httpx.WriteError(w, r, http.StatusBadRequest, CodeMarkInvalid, "every block must lie inside the recording")
			return
		}
	}
	if _, err := h.FS.Lstat(p + ".ap"); err != nil {
		httpx.WriteError(w, r, http.StatusConflict, CodeMarkNoAP, "the recording has no access points (.ts.ap) to mark against")
		return
	}
	if !h.checkNotInUse(w, r, []string{p}) {
		return
	}
	base := recfs.Base(p)
	total := 0.0
	for _, b := range req.Blocks {
		total += b.End - b.Start
	}
	changes := []plan.Change{
		{Type: "recording.mark", After: map[string]any{"path": p, "blocks": len(req.Blocks), "seconds": total}},
	}
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "media.adscan.mark",
		Changes:     changes,
		Summary:     fmt.Sprintf("Marked %d ad break(s), %.0f s, in %s's cut list", len(req.Blocks), total, base),
		Detail:      map[string]any{"path": p, "blocks": req.Blocks},
		Fingerprint: h.filesFingerprint([]string{p, p + ".cuts"}),
		Current:     func(context.Context) (string, error) { return h.filesFingerprint([]string{p, p + ".cuts"}), nil },
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			if err := h.inUseFailure(ctx, []string{p}); err != nil {
				return err
			}
			if err := h.writeAdCuts(ctx, p, req.Blocks); err != nil {
				return err
			}
			res.Marked = true
			if raw, err := json.Marshal(res); err == nil {
				_ = h.Store.SaveAdScanResult(context.WithoutCancel(ctx), p, h.serviceRef(p), raw)
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

// writeAdCuts reads the recording's access points and any existing cut
// list, turns blocks (seconds) into stream PTS, unites them with whatever
// was already cut (hand-made cuts, the resume position, markers all kept),
// and writes the result back. Confirmed live 2026-10-09
// (spikes/adscan/phase0.sh): a boundary snaps to the keyframe at or before
// it — exact to the second, not the frame, which is enough for a jump
// point (trimming, not marking, is where frame accuracy matters).
func (h *Media) writeAdCuts(ctx context.Context, p string, blocks []adscan.Span) error {
	apRaw, err := h.FS.ReadFile(p + ".ap")
	if err != nil {
		return err
	}
	aps, err := adscan.ParseAP(apRaw)
	if err != nil {
		return err
	}
	spans := make([]adscan.PTSSpan, 0, len(blocks))
	for _, b := range blocks {
		start, ok := adscan.APAt(aps, b.Start)
		if !ok {
			continue
		}
		end, ok := adscan.APAt(aps, b.End)
		if !ok {
			continue
		}
		spans = append(spans, adscan.PTSSpan{Start: uint64(start.PTS), End: uint64(end.PTS)})
	}
	var existing []adscan.Cut
	hadFile := true
	cutsRaw, err := h.FS.ReadFile(p + ".cuts")
	switch {
	case errors.Is(err, fs.ErrNotExist):
		hadFile = false
	case err != nil:
		return err
	default:
		if existing, err = adscan.ParseCuts(cutsRaw); err != nil {
			return err
		}
	}
	merged := adscan.EncodeCuts(adscan.MergeAds(existing, spans))
	// WriteFile only replaces a file that's already there (it keeps its
	// mode); most recordings have no .ts.cuts at all until something —
	// Enigma2 itself, CutListEditor, or this — makes one.
	if hadFile {
		return h.FS.WriteFile(p+".cuts", merged)
	}
	return h.FS.Create(p+".cuts", merged)
}
