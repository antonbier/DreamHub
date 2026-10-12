package hubs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/dockersetup"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// "Set up Docker" in AppsHub's Apps tab (internal/dockersetup): the steps
// still missing are the preview; applying downloads, verifies and starts
// everything. Takes a minute or two (an 80 MB download on the box).

type dockerSetupRun struct {
	At  time.Time `json:"at"`
	OK  bool      `json:"ok"`
	Log string    `json:"log"`
}

// dockerStatus answers GET /api/v1/apps/docker.
func (h *Apps) dockerStatus(w http.ResponseWriter, r *http.Request) {
	if h.Setup == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_setup_unavailable", "setting Docker up isn't available on this system")
		return
	}
	st, err := h.Setup.Status(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "docker_setup_failed", "could not read Docker's state")
		return
	}
	h.setupMu.Lock()
	last := h.lastSetup
	h.setupMu.Unlock()
	var lastRun any
	if !last.At.IsZero() {
		lastRun = last
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"state": st, "version": dockersetup.Version, "lastRun": lastRun})
}

// planDockerSetup answers POST /api/v1/apps/docker/setup/plan.
func (h *Apps) planDockerSetup(w http.ResponseWriter, r *http.Request) {
	if h.Setup == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_setup_unavailable", "setting Docker up isn't available on this system")
		return
	}
	ctx := r.Context()
	st, err := h.Setup.Status(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "docker_setup_failed", "could not read Docker's state")
		return
	}
	if !st.Supported {
		httpx.WriteError(w, r, http.StatusConflict, "docker_unsupported", "there's no Docker build for this box's processor")
		return
	}
	steps, err := h.Setup.Plan(ctx)
	switch {
	case errors.Is(err, dockersetup.ErrNoMatchingOverlay):
		httpx.WriteError(w, r, http.StatusConflict, "docker_no_overlay", "no overlay kernel module matches this box's kernel")
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusBadGateway, "docker_setup_failed", "could not work out what's missing")
		return
	}
	if len(steps) == 0 {
		httpx.WriteError(w, r, http.StatusConflict, "nothing_to_do", "Docker is already set up and running")
		return
	}
	if st.Version == "" && st.FreeBytes < dockersetup.NeededBytes {
		httpx.WriteError(w, r, http.StatusConflict, "disk_full", fmt.Sprintf("/data has %d MB free; setting Docker up needs about %d MB", st.FreeBytes>>20, dockersetup.NeededBytes>>20))
		return
	}

	changes := make([]plan.Change, 0, len(steps))
	kinds := make([]string, 0, len(steps))
	for _, s := range steps {
		changes = append(changes, plan.Change{Type: "docker." + s.Kind, After: s.Detail})
		kinds = append(kinds, s.Kind)
	}
	user, _ := httpx.User(ctx)
	summary := "Set up Docker " + dockersetup.Version + " (" + strings.Join(kinds, ", ") + ")"
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "apps.docker.setup",
		Changes:     changes,
		Summary:     summary,
		Detail:      map[string]any{"steps": steps},
		Fingerprint: strings.Join(kinds, ","),
		Current: func(ctx context.Context) (string, error) {
			now, err := h.Setup.Plan(ctx)
			if err != nil {
				return "", err
			}
			k := make([]string, 0, len(now))
			for _, s := range now {
				k = append(k, s.Kind)
			}
			return strings.Join(k, ","), nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			log, runErr := h.Setup.Apply(ctx, steps)
			h.setupMu.Lock()
			h.lastSetup = dockerSetupRun{At: h.now(), OK: runErr == nil, Log: log}
			h.setupMu.Unlock()
			prefix := ""
			if runErr != nil {
				prefix = "Failed: "
			}
			_ = h.Store.AppendChange(context.WithoutCancel(ctx), store.ChangeEntry{UserID: user.ID, Action: "apps.docker.setup", Summary: prefix + summary})
			if runErr != nil {
				return &plan.Failure{Status: http.StatusBadGateway, Code: "docker_setup_failed", Message: "setting Docker up failed; see the log", Err: runErr}
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl, "steps": steps})
}
