package hubs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
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

// planDockerUninstall answers POST /api/v1/apps/docker/uninstall/plan,
// body {"deleteData": false}. The plan lists what goes: the containers
// DreamHub created (first), the two services, the overlay-module loading
// and the Docker files — and, only when asked ("Also delete the app
// data?", default no), Docker's data root and the apps' storage folders.
// enigma2 and recordings are not touched; there is nothing to do once
// nothing is left, so a second uninstall is a 409.
func (h *Apps) planDockerUninstall(w http.ResponseWriter, r *http.Request) {
	if h.Setup == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_setup_unavailable", "removing Docker isn't available on this system")
		return
	}
	var body struct {
		DeleteData bool `json:"deleteData"`
	}
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(r, &body); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"deleteData": false}`)
			return
		}
	}
	ctx := r.Context()
	containers := h.listForUninstall(ctx)
	opts := dockersetup.RemoveOptions{DeleteData: body.DeleteData, AppRoots: h.appRoots(ctx, containers)}
	steps, err := h.Setup.PlanRemove(ctx, opts)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "docker_setup_failed", "could not work out what Docker has left")
		return
	}
	if len(containers) > 0 {
		steps = append([]dockersetup.Step{{Kind: "containers", Detail: map[string]string{"count": strconv.Itoa(len(containers))}}}, steps...)
	}
	if len(steps) == 0 {
		httpx.WriteError(w, r, http.StatusConflict, "nothing_to_do", "Docker is not installed")
		return
	}

	changes := make([]plan.Change, 0, len(steps))
	kinds := make([]string, 0, len(steps))
	for _, s := range steps {
		changes = append(changes, plan.Change{Type: "docker.uninstall." + s.Kind, Before: s.Detail})
		kinds = append(kinds, s.Kind)
	}
	fingerprint := func(containers []dockerd.Container, steps []dockersetup.Step) string {
		k := make([]string, 0, len(steps)+1)
		for _, s := range steps {
			k = append(k, s.Kind)
		}
		return fmt.Sprintf("%s|%d|%t", strings.Join(k, ","), len(containers), body.DeleteData)
	}
	user, _ := httpx.User(ctx)
	summary := "Uninstall Docker (" + strings.Join(kinds, ", ") + ")"
	if body.DeleteData {
		summary += " and delete the app data"
	} else {
		summary += ", keeping the app data"
	}
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "apps.docker.uninstall",
		Changes:     changes,
		Summary:     summary,
		Detail:      map[string]any{"steps": steps, "deleteData": body.DeleteData},
		Fingerprint: fingerprint(containers, steps),
		Current: func(ctx context.Context) (string, error) {
			cs := h.listForUninstall(ctx)
			o := dockersetup.RemoveOptions{DeleteData: body.DeleteData, AppRoots: h.appRoots(ctx, cs)}
			now, err := h.Setup.PlanRemove(ctx, o)
			if err != nil {
				return "", err
			}
			if len(cs) > 0 {
				now = append([]dockersetup.Step{{Kind: "containers"}}, now...)
			}
			return fingerprint(cs, now), nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			var log strings.Builder
			fail := func(runErr error) error {
				h.setupMu.Lock()
				h.lastSetup = dockerSetupRun{At: h.now(), OK: false, Log: log.String()}
				h.setupMu.Unlock()
				_ = h.Store.AppendChange(context.WithoutCancel(ctx), store.ChangeEntry{UserID: user.ID, Action: "apps.docker.uninstall", Summary: "Failed: " + summary})
				return &plan.Failure{Status: http.StatusBadGateway, Code: "docker_setup_failed", Message: "uninstalling Docker failed; see the log", Err: runErr}
			}
			// The containers first, so nothing is left running on files
			// that are about to go.
			for _, c := range h.listForUninstall(ctx) {
				if err := h.Docker.Remove(ctx, c.ID); err != nil && !errors.Is(err, dockerd.ErrNotFound) {
					fmt.Fprintf(&log, "FAILED: removing %s: %v\n", c.Name, err)
					return fail(err)
				}
				fmt.Fprintf(&log, "Removed container %s\n", c.Name)
			}
			now, err := h.Setup.PlanRemove(ctx, opts)
			if err != nil {
				return fail(err)
			}
			out, runErr := h.Setup.Remove(ctx, now, opts)
			log.WriteString(out)
			if runErr != nil {
				return fail(runErr)
			}
			h.setupMu.Lock()
			h.lastSetup = dockerSetupRun{At: h.now(), OK: true, Log: log.String()}
			h.setupMu.Unlock()
			_ = h.Store.AppendChange(context.WithoutCancel(ctx), store.ChangeEntry{UserID: user.ID, Action: "apps.docker.uninstall", Summary: summary})
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl, "steps": steps})
}

// listForUninstall: the containers DreamHub created. A helper that
// doesn't answer (Docker already stopped or gone) just means there are
// none to remove.
func (h *Apps) listForUninstall(ctx context.Context) []dockerd.Container {
	if h.Docker == nil {
		return nil
	}
	list, err := h.Docker.List(ctx)
	if err != nil {
		return nil
	}
	return list
}

// appRoots are the storage roots the apps live under: the default one and
// whatever the installed apps were given at install. Only valid roots.
func (h *Apps) appRoots(ctx context.Context, containers []dockerd.Container) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range append([]string{h.StorageRoot, h.defaultRoot(ctx)}, rootsOf(containers)...) {
		if r != "" && dockerd.ValidRoot(r) && !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

func rootsOf(containers []dockerd.Container) []string {
	var out []string
	for _, c := range containers {
		if r := c.Root(); r != "" {
			out = append(out, r)
		}
	}
	return out
}
