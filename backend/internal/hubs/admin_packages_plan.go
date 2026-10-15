package hubs

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/apt"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Install, upgrade and remove as a plan (docs/ADMINHUB_PLAN.md, A2, #47):
// apt-get's own simulate run is the preview — every package it would
// install, upgrade or remove, including the ones the admin didn't name —
// and the same command without -s is the apply.

const (
	// CodePackageAlsoRemoves: apt removes more than was asked for
	// (params: names, count) — on the box, removing one small skin
	// component takes two whole skins with it (ARCHITECTURE.md).
	CodePackageAlsoRemoves = "package.alsoRemoves"
	// CodePackageTouchesEnigma2: a TV plugin, skin or enigma2 itself
	// changes; it takes effect after enigma2 restarts, and some packages
	// restart it themselves.
	CodePackageTouchesEnigma2 = "package.touchesEnigma2"
	// CodePackageRecording: the box is recording right now (params:
	// names); applying waits until it's done.
	CodePackageRecording = "package.recordingRunning"
	// CodePackageNeededByHub: a package a DreamHub hub uses would be
	// removed (params: names, hubs) — e.g. ffmpeg, which AdScan and
	// trimming run.
	CodePackageNeededByHub = "package.neededByHub"
)

// selfPackage is DreamHub's own package. Changing it through DreamHub
// would restart dreamhub.service in the middle of the apt-get run that
// DreamHub itself started — systemd stops the whole service, apt-get
// included, and leaves dpkg half-done. So it's refused here; DreamHub is
// updated like any other package from the box's own tools.
const selfPackage = "dreamhub"

type packageRunResult struct {
	At     time.Time  `json:"at"`
	Action string     `json:"action"`
	Steps  []apt.Step `json:"steps"`
	OK     bool       `json:"ok"`
	Output string     `json:"output"`
}

// planPackages answers POST /api/v1/admin/packages/plan with
// {"action": "install"|"remove"|"upgrade", "packages": [...]} — an empty
// list with "upgrade" means everything upgradable.
func (h *Admin) planPackages(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action   apt.Action `json:"action"`
		Packages []string   `json:"packages"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"action": "install|remove|upgrade", "packages": ["<name>", ...]}`)
		return
	}
	if err := apt.ValidNames(body.Action, body.Packages); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if h.Packages == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "apt_unavailable", "installing packages isn't available on this system")
		return
	}
	ctx := r.Context()
	steps, err := h.Packages.Simulate(ctx, body.Action, body.Packages)
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

	warnings := packageWarnings(steps, body.Packages)
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
	action, names := body.Action, body.Packages
	summary := describeSteps(steps)
	pl := h.Plans.Create(user, plan.Draft{
		Action:   "package." + string(action),
		Changes:  changes,
		Warnings: warnings,
		Summary:  summary,
		// The steps carry each package's previous version, so a downgrade
		// is a concrete step instead of guesswork (docs/ADMINHUB_PLAN.md).
		Detail:      map[string]any{"steps": steps},
		Fingerprint: stepsFingerprint(steps),
		Current: func(ctx context.Context) (string, error) {
			now, err := h.Packages.Simulate(ctx, action, names)
			if err != nil {
				return "", err
			}
			return stepsFingerprint(now), nil
		},
		Execute: func(ctx context.Context) error {
			if f := h.applyPackages(ctx, user, string(action), steps, summary, func(ctx context.Context) (string, error) {
				return h.Packages.Apply(ctx, action, names)
			}); f != nil {
				return f
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl, "steps": steps})
}

// applyPackages runs a package change that was previewed: the same checks and
// bookkeeping whether a person confirmed it (the plan) or it is one step of
// the combined update (apps_updates.go). A nil result means it was done.
func (h *Admin) applyPackages(ctx context.Context, user store.User, action string, steps []apt.Step, summary string, run func(context.Context) (string, error)) *plan.Failure {
	if h.ReadOnly {
		return &plan.Failure{Status: http.StatusConflict, Code: "writes_disabled", Message: "device writes are disabled", Err: webcontrol.ErrWritesDisabled}
	}
	// Checked again at apply time: a timer may have started since the
	// preview. A package that only adds a console tool can't cut a
	// recording; one that touches enigma2 can.
	if touchesEnigma2(steps) {
		if names := h.runningRecordings(ctx); len(names) > 0 {
			return &plan.Failure{Status: http.StatusConflict, Code: "recording_running", Message: "the box is recording; try again when it's done"}
		}
	}
	output, runErr := run(ctx)
	res := packageRunResult{At: h.now(), Action: action, Steps: steps, OK: runErr == nil, Output: output}
	h.pkgMu.Lock()
	h.lastPackageRun = res
	h.pkgMu.Unlock()
	prefix := ""
	if runErr != nil {
		prefix = "Failed: "
	}
	_ = h.Store.AppendChange(context.WithoutCancel(ctx), store.ChangeEntry{
		UserID: user.ID, Action: "admin.package." + action, Summary: prefix + summary,
	})
	if runErr != nil {
		return &plan.Failure{Status: http.StatusBadGateway, Code: "package_apply_failed", Message: "apt-get reported an error; see its output", Details: map[string]any{"output": lastLinesOf(output, 12)}, Err: runErr}
	}
	return nil
}

// lastLinesOf keeps the last n lines of apt's output for a failure report.
func lastLinesOf(out string, n int) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// writeSimulateError answers a failed simulate run; it reports whether
// there was no error (and the caller should go on).
func (h *Admin) writeSimulateError(w http.ResponseWriter, r *http.Request, err error) bool {
	var simErr *apt.SimulateError
	switch {
	case err == nil:
		return true
	case errors.Is(err, apt.ErrUnavailable):
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "apt_unavailable", "dpkg/apt aren't available on this system")
	case errors.As(err, &simErr):
		httpx.WriteErrorDetails(w, r, http.StatusConflict, "apt_refused", simErr.Output, nil)
	default:
		httpx.WriteError(w, r, http.StatusBadGateway, "apt_failed", "apt couldn't work out the change")
	}
	return false
}

// lastPackageRunView answers GET /api/v1/admin/packages/last-run: what
// the most recent apply did and apt-get's output, kept in memory only.
func (h *Admin) lastPackageRunView(w http.ResponseWriter, r *http.Request) {
	h.pkgMu.Lock()
	res := h.lastPackageRun
	h.pkgMu.Unlock()
	if res.At.IsZero() {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"run": nil})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"run": res})
}

func packageWarnings(steps []apt.Step, asked []string) []status.Problem {
	var warnings []status.Problem
	var extra []string
	for _, s := range steps {
		if s.Kind == "remove" && !slices.Contains(asked, s.Name) {
			extra = append(extra, s.Name)
		}
	}
	if len(extra) > 0 {
		warnings = append(warnings, status.Problem{Code: CodePackageAlsoRemoves, Params: map[string]any{"names": strings.Join(extra, ", "), "count": len(extra)}})
	}
	var needed, hubs []string
	for _, s := range steps {
		if s.Kind != "remove" {
			continue
		}
		if by := packageNeeds[s.Name]; len(by) > 0 {
			needed = append(needed, s.Name)
			for _, h := range by {
				if !slices.Contains(hubs, h) {
					hubs = append(hubs, h)
				}
			}
		}
	}
	if len(needed) > 0 {
		warnings = append(warnings, status.Problem{Code: CodePackageNeededByHub, Params: map[string]any{"names": strings.Join(needed, ", "), "hubs": strings.Join(hubs, ", "), "count": len(needed)}})
	}
	if touchesEnigma2(steps) {
		warnings = append(warnings, status.Problem{Code: CodePackageTouchesEnigma2})
	}
	return warnings
}

// touchesEnigma2: enigma2 itself, its plugins, skins and skin components
// all carry the enigma2 prefix on DreamOS feeds.
func touchesEnigma2(steps []apt.Step) bool {
	for _, s := range steps {
		if strings.HasPrefix(s.Name, "enigma2") {
			return true
		}
	}
	return false
}

func describeSteps(steps []apt.Step) string {
	parts := make([]string, 0, len(steps))
	for _, s := range steps {
		switch s.Kind {
		case "install":
			parts = append(parts, "installed "+s.Name+" "+s.NewVersion)
		case "upgrade":
			parts = append(parts, "upgraded "+s.Name+" "+s.OldVersion+" → "+s.NewVersion)
		case "remove":
			parts = append(parts, "removed "+s.Name+" "+s.OldVersion)
		case "downgrade":
			parts = append(parts, "downgraded "+s.Name+" "+s.OldVersion+" → "+s.NewVersion)
		}
	}
	out := strings.Join(parts, ", ")
	if out != "" {
		out = strings.ToUpper(out[:1]) + out[1:]
	}
	return out
}

func stepsFingerprint(steps []apt.Step) string {
	var b strings.Builder
	for _, s := range steps {
		b.WriteString(s.Kind + " " + s.Name + " " + s.OldVersion + " " + s.NewVersion + "\n")
	}
	return b.String()
}
