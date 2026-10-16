package hubs

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/apt"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/status"
)

// Downgrade to the previous version (docs/APPSHUB_PLAN.md, App14): offered
// only for a package whose feed still carries an older version, and done as a
// plan like every other package change — apt-get's simulate run lists
// everything that would go back, including what depends on it.

// packageVersions answers GET /api/v1/admin/packages/{name}/versions: the
// older versions of an installed package, newest first. Empty for the common
// case that the feeds only carry the latest — then the UI shows no downgrade.
func (h *Admin) packageVersions(w http.ResponseWriter, r *http.Request) {
	d, ok := h.Packages.(apt.Downgrader)
	if h.Packages == nil || !ok {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"versions": []string{}})
		return
	}
	name := r.PathValue("name")
	if name == selfPackage {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"versions": []string{}})
		return
	}
	versions, err := d.OlderVersions(r.Context(), name)
	switch {
	case errors.Is(err, apt.ErrInvalidName):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	case errors.Is(err, apt.ErrUnavailable):
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "apt_unavailable", "dpkg/apt aren't available on this system")
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusBadGateway, "apt_failed", "could not read the versions the feeds offer")
		return
	}
	if versions == nil {
		versions = []string{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

// planDowngrade answers POST /api/v1/admin/packages/downgrade/plan with
// {"name": "...", "version": "..."}; the version must be one the feeds offer
// as older than the installed one.
func (h *Admin) planDowngrade(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || body.Name == "" || body.Version == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"name": "<package>", "version": "<an older version>"}`)
		return
	}
	d, ok := h.Packages.(apt.Downgrader)
	if h.Packages == nil || !ok {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "apt_unavailable", "installing packages isn't available on this system")
		return
	}
	if err := apt.ValidNames(apt.ActionInstall, []string{body.Name}); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if !apt.ValidVersion(body.Version) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "that is not a package version")
		return
	}
	if body.Name == selfPackage {
		httpx.WriteError(w, r, http.StatusConflict, "package_self", "DreamHub can't change its own package; update or remove it with the box's own tools")
		return
	}
	ctx := r.Context()
	// Only a version the feeds really offer as older: nothing the person types
	// is handed to apt-get unchecked.
	older, err := d.OlderVersions(ctx, body.Name)
	if err != nil {
		h.writeSimulateError(w, r, err)
		return
	}
	offered := false
	for _, v := range older {
		offered = offered || v == body.Version
	}
	if !offered {
		httpx.WriteError(w, r, http.StatusConflict, "version_not_offered", "the feeds don't carry that older version")
		return
	}
	steps, err := d.SimulateDowngrade(ctx, body.Name, body.Version)
	if !h.writeSimulateError(w, r, err) {
		return
	}
	if len(steps) == 0 {
		httpx.WriteError(w, r, http.StatusConflict, "nothing_to_do", "apt has nothing to change for this")
		return
	}
	for _, s := range steps {
		if s.Name == selfPackage {
			httpx.WriteError(w, r, http.StatusConflict, "package_self", "DreamHub can't change its own package; update or remove it with the box's own tools")
			return
		}
	}

	warnings := packageWarnings(steps, []string{body.Name})
	if touchesEnigma2(steps) {
		if names := h.runningRecordings(ctx); len(names) > 0 {
			warnings = append(warnings, status.Problem{Code: CodePackageRecording, Params: map[string]any{"names": strings.Join(names, ", "), "count": len(names)}})
		}
	}
	changes := make([]plan.Change, 0, len(steps))
	for _, s := range steps {
		c := plan.Change{Type: "package." + s.Kind}
		if s.OldVersion != "" {
			c.Before = map[string]string{"name": s.Name, "version": s.OldVersion}
		}
		if s.NewVersion != "" {
			c.After = map[string]string{"name": s.Name, "version": s.NewVersion}
		}
		changes = append(changes, c)
	}
	user, _ := httpx.User(ctx)
	name, version := body.Name, body.Version
	summary := describeSteps(steps)
	pl := h.Plans.Create(ctx, user, plan.Draft{
		Action:      "package.downgrade",
		Changes:     changes,
		Warnings:    warnings,
		Summary:     summary,
		Detail:      map[string]any{"steps": steps},
		Fingerprint: stepsFingerprint(steps),
		Current: func(ctx context.Context) (string, error) {
			now, err := d.SimulateDowngrade(ctx, name, version)
			if err != nil {
				return "", err
			}
			return stepsFingerprint(now), nil
		},
		Execute: func(ctx context.Context) error {
			if f := h.applyPackages(ctx, user, "downgrade", steps, summary, func(ctx context.Context) (string, error) {
				return d.ApplyDowngrade(ctx, name, version)
			}); f != nil {
				return f
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl, "steps": steps})
}
