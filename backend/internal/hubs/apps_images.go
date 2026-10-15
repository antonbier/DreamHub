package hubs

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// Cleaning up unused images (docs/APPSHUB_PLAN.md, App13, "Docker
// settings"): an image stays on the disk after its app is removed or updated.
// The list and the removal both go through the helper's two fixed calls
// (dockerd.Client.UnusedImages / PruneImages): the person sees what would go
// and how much it frees, then confirms a plan.

type unusedImagesView struct {
	Images         []dockerd.Image `json:"images"`
	ReclaimedBytes int64           `json:"reclaimableBytes"`
	// Kept are the previous versions kept for "Roll back"; the clean-up
	// never removes them, they have their own line (and plan) below it.
	Kept      []dockerd.KeptImage `json:"kept"`
	KeptBytes int64               `json:"keptBytes"`
}

func totalBytes(images []dockerd.Image) int64 {
	var n int64
	for _, i := range images {
		n += i.SizeBytes
	}
	return n
}

// unusedImages answers GET /api/v1/apps/docker/images.
func (h *Apps) unusedImages(w http.ResponseWriter, r *http.Request) {
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	images, err := h.Docker.UnusedImages(r.Context())
	if err != nil {
		writeDockerError(w, r, err, "could not list the images")
		return
	}
	if images == nil {
		images = []dockerd.Image{}
	}
	kept, err := h.Docker.KeptImages(r.Context())
	if err != nil || kept == nil {
		kept = []dockerd.KeptImage{}
	}
	var keptBytes int64
	for _, k := range kept {
		keptBytes += k.SizeBytes
	}
	httpx.WriteJSON(w, http.StatusOK, unusedImagesView{Images: images, ReclaimedBytes: totalBytes(images), Kept: kept, KeptBytes: keptBytes})
}

func imagesFingerprint(images []dockerd.Image) string {
	ids := make([]string, 0, len(images))
	for _, i := range images {
		ids = append(ids, i.ID)
	}
	sort.Strings(ids)
	return "images:" + strings.Join(ids, ",")
}

// planPruneImages answers POST /api/v1/apps/docker/images/prune/plan.
func (h *Apps) planPruneImages(w http.ResponseWriter, r *http.Request) {
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	ctx := r.Context()
	images, err := h.Docker.UnusedImages(ctx)
	if err != nil {
		writeDockerError(w, r, err, "could not list the images")
		return
	}
	if len(images) == 0 {
		httpx.WriteError(w, r, http.StatusConflict, "nothing_to_do", "there are no unused images")
		return
	}
	size := totalBytes(images)
	user, _ := httpx.User(ctx)
	pl := h.Plans.Create(user, plan.Draft{
		Action:  "apps.images.prune",
		Changes: []plan.Change{{Type: "apps.images.prune", Before: map[string]any{"images": images, "reclaimableBytes": size}}},
		Summary: fmt.Sprintf("Remove %d unused Docker image(s), about %d MB", len(images), size>>20),
		Detail:  map[string]any{"images": len(images), "reclaimableBytes": size},
		// The list the person saw; if an install or removal changed it since,
		// they look again.
		Fingerprint: imagesFingerprint(images),
		Current: func(ctx context.Context) (string, error) {
			list, err := h.Docker.UnusedImages(ctx)
			if err != nil {
				return "", err
			}
			return imagesFingerprint(list), nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return &plan.Failure{Status: http.StatusConflict, Code: "writes_disabled", Message: "device writes are disabled"}
			}
			res, err := h.Docker.PruneImages(ctx)
			if err != nil {
				return &plan.Failure{Status: http.StatusBadGateway, Code: "prune_failed", Message: "the images could not be removed", Err: err}
			}
			if h.Store != nil {
				_ = h.Store.AppendChange(context.WithoutCancel(ctx), store.ChangeEntry{UserID: user.ID, Action: "apps.images.pruned",
					Summary: fmt.Sprintf("Removed %d unused Docker image(s), %d MB freed", res.Removed, res.ReclaimedBytes>>20)})
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

func keptFingerprint(kept []dockerd.KeptImage) string {
	ids := make([]string, 0, len(kept))
	for _, k := range kept {
		ids = append(ids, k.AppID+"."+k.Service+"="+k.ImageID)
	}
	sort.Strings(ids)
	return "kept:" + strings.Join(ids, ",")
}

// planDropKept answers POST /api/v1/apps/docker/images/kept/plan: give up the
// previous versions kept for "Roll back" (every app's). The ordinary clean-up
// leaves them alone on purpose; this is the one place they go, and an app
// then has no "Roll back" until its next update.
func (h *Apps) planDropKept(w http.ResponseWriter, r *http.Request) {
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	ctx := r.Context()
	kept, err := h.Docker.KeptImages(ctx)
	if err != nil {
		writeDockerError(w, r, err, "could not list the images")
		return
	}
	if len(kept) == 0 {
		httpx.WriteError(w, r, http.StatusConflict, "nothing_to_do", "no previous versions are kept")
		return
	}
	var size int64
	for _, k := range kept {
		size += k.SizeBytes
	}
	user, _ := httpx.User(ctx)
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "apps.images.dropKept",
		Changes:     []plan.Change{{Type: "apps.images.dropKept", Before: map[string]any{"kept": kept, "keptBytes": size}}},
		Summary:     fmt.Sprintf("Remove the %d kept previous version(s) of apps, about %d MB; they can no longer be rolled back", len(kept), size>>20),
		Detail:      map[string]any{"images": len(kept), "bytes": size},
		Fingerprint: keptFingerprint(kept),
		Current: func(ctx context.Context) (string, error) {
			list, err := h.Docker.KeptImages(ctx)
			if err != nil {
				return "", err
			}
			return keptFingerprint(list), nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return &plan.Failure{Status: http.StatusConflict, Code: "writes_disabled", Message: "device writes are disabled"}
			}
			if _, err := h.Docker.DropKeptImages(ctx, ""); err != nil {
				return &plan.Failure{Status: http.StatusBadGateway, Code: "prune_failed", Message: "the images could not be removed", Err: err}
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}
