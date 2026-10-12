package hubs

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path"
	"sort"
	"strconv"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// This file is AppsHub's App4 (docs/APPSHUB_PLAN.md, "Install/run as a
// plan"): installing, removing, starting and stopping a Docker app
// through internal/dockerd's isolated helper (App2) — the first code that
// actually calls it. Scoped deliberately narrow for this first slice:
// only the curated catalog (internal/appcatalog) can be installed, not
// yet a free-search image (App3 can only check compatibility for one;
// installing it needs the admin to tell DreamHub its web port and config
// path, which the catalog already knows and free search doesn't — a
// later, separate form, not built here). Every app's storage goes on the
// recording HDD by default and there's no UI yet to choose flash
// instead — see the plan's "Storage" decision.

// defaultMemoryLimitMB is used when the client doesn't ask for a
// different one — generous enough for any of the curated apps' own
// documented minimums, small enough that several could run at once on
// the reference box's 2 GB.
const defaultMemoryLimitMB = 256

// memoryReserveBytes is held back from meminfo's own MemAvailable before
// any app may claim it — a safety margin for whatever enigma2/DreamHub
// need next, not the app's own request (docs/APPSHUB_PLAN.md: "refuse...
// never let the box become unstable").
const memoryReserveBytes = 128 << 20

// diskFullPercent mirrors events.poller's own disk.aboveThreshold line
// (internal/events/poller.go) — the same box, the same risk, so the same
// number, not a separately invented one.
const diskFullPercent = 90.0

// knownPorts names the box's own other services for a friendlier
// conflict message (ARCHITECTURE.md) — a port missing from this map
// still refuses the same way, just without naming what's on it.
var knownPorts = map[int]string{
	80: "WebControl", 8080: "DreamHub", 4200: "Terminal (shellinaboxd)",
	139: "Samba", 445: "Samba",
}

// portBindable reports whether port is free to bind on this host right
// now — a real attempt, not just "not in our own known list"
// (docs/APPSHUB_PLAN.md: "combine it with an actual bind attempt").
func portBindable(port int) bool {
	l, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// containerTZ is the box's own configured system time zone, passed
// through to a container's TZ env var (docs/APPSHUB_PLAN.md: "TZ
// pre-filled from the box's own configured time zone"). Go's time.Local
// already reflects /etc/localtime on a correctly set up Linux box, so
// nothing beyond reading it is needed; "Local" means no zoneinfo link
// was found, which UTC is the honest fallback for.
func containerTZ() string {
	if name := time.Local.String(); name != "" && name != "Local" {
		return name
	}
	return "UTC"
}

type installedApp struct {
	ID       string `json:"id"`
	Name     string `json:"name"` // the catalog's name, or the raw image if unknown
	Image    string `json:"image"`
	State    string `json:"state"`
	HostPort int    `json:"hostPort,omitempty"`
	// Upgradable is App5's update check (apps_watch.go): the registry's
	// current digest for this image:tag no longer matches what's
	// actually running. Always computed live, never stored, same as
	// HostPort above.
	Upgradable bool `json:"upgradable,omitempty"`
}

func (h *Apps) listInstalled(w http.ResponseWriter, r *http.Request) {
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	containers, err := h.Docker.List(r.Context())
	if err != nil {
		writeDockerError(w, r, err, "could not list installed apps")
		return
	}
	out := make([]installedApp, 0, len(containers))
	for _, c := range containers {
		name := c.Image
		if a, ok := appcatalog.ByID(c.AppID()); ok {
			name = a.Name
		}
		out = append(out, installedApp{
			ID: c.ID, Name: name, Image: c.Image, State: c.State, HostPort: c.HostPort,
			Upgradable: h.imageUpgradable(r.Context(), c),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"apps": out})
}

func installedFingerprint(containers []dockerd.Container, appID string) string {
	for _, c := range containers {
		if c.AppID() == appID {
			return "installed:" + c.ID
		}
	}
	return "not-installed"
}

func (h *Apps) planInstall(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AppID         string `json:"appId"`
		HostPort      int    `json:"hostPort"`
		MemoryLimitMB int    `json:"memoryLimitMB"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || body.AppID == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"appId": "..."}`)
		return
	}
	app, ok := appcatalog.ByID(body.AppID)
	if !ok {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such app in the curated catalog")
		return
	}
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	ctx := r.Context()
	containers, err := h.Docker.List(ctx)
	if err != nil {
		writeDockerError(w, r, err, "could not check installed apps")
		return
	}
	if installedFingerprint(containers, app.ID) != "not-installed" {
		httpx.WriteError(w, r, http.StatusConflict, "already_installed", app.Name+" is already installed")
		return
	}

	memoryLimitMB := body.MemoryLimitMB
	if memoryLimitMB <= 0 {
		memoryLimitMB = defaultMemoryLimitMB
	}
	if problem := h.checkResourceBudget(ctx, int64(memoryLimitMB)<<20); problem != "" {
		httpx.WriteError(w, r, http.StatusConflict, "insufficient_memory", problem)
		return
	}
	if problem := h.checkDiskSpace(ctx); problem != "" {
		httpx.WriteError(w, r, http.StatusConflict, "disk_full", problem)
		return
	}

	hostPort, portErr := h.resolvePort(app, body.HostPort)
	if portErr != "" {
		httpx.WriteError(w, r, http.StatusConflict, "port_in_use", portErr)
		return
	}

	hostDir := path.Join(h.StorageRoot, app.ID)
	tz := containerTZ()
	spec := dockerd.ContainerSpec{
		AppID: app.ID, Image: app.Image + ":latest",
		Env:      map[string]string{"TZ": tz},
		HostPort: hostPort, ContainerPort: app.ContainerPort,
		HostDir: hostDir, ContainerDir: app.ConfigPath,
		MemoryLimitBytes: int64(memoryLimitMB) << 20,
		HostNetwork:      true,
	}

	summary := fmt.Sprintf("Install %s (%s), up to %d MB RAM, storage at %s", app.Name, spec.Image, memoryLimitMB, hostDir)
	if hostPort != 0 {
		summary = fmt.Sprintf("%s, reachable on port %d", summary, hostPort)
	}

	user, _ := httpx.User(ctx)
	pl := h.Plans.Create(user, plan.Draft{
		Action: "apps.install",
		Changes: []plan.Change{{Type: "apps.install", After: map[string]any{
			"appId": app.ID, "image": spec.Image, "hostPort": hostPort, "memoryLimitMB": memoryLimitMB, "storage": hostDir, "tz": tz,
		}}},
		Summary:     summary,
		Detail:      map[string]any{"appId": app.ID},
		Fingerprint: "not-installed",
		Current: func(ctx context.Context) (string, error) {
			list, err := h.Docker.List(ctx)
			if err != nil {
				return "", err
			}
			return installedFingerprint(list, app.ID), nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return &plan.Failure{Status: http.StatusConflict, Code: "writes_disabled", Message: "device writes are disabled"}
			}
			if _, err := h.Docker.Create(ctx, spec); err != nil {
				return &plan.Failure{Status: http.StatusBadGateway, Code: "install_failed", Message: "the app could not be installed", Err: err}
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

// checkResourceBudget returns a non-empty, user-facing reason if
// requested wouldn't fit in meminfo's current MemAvailable, minus
// memoryReserveBytes — "" means it fits (or the check can't run, e.g. no
// Mem configured, which refuses rather than silently skips: see
// planInstall calling this before ever reaching Execute).
func (h *Apps) checkResourceBudget(ctx context.Context, requested int64) string {
	if h.Mem == nil {
		return "the memory budget couldn't be checked"
	}
	info, err := h.Mem.Read(ctx)
	if err != nil {
		return "the memory budget couldn't be checked"
	}
	headroom := info.AvailableBytes - memoryReserveBytes
	if requested > headroom {
		return fmt.Sprintf("not enough free memory: %d MB requested, about %d MB free", requested>>20, headroom>>20)
	}
	return ""
}

func (h *Apps) checkDiskSpace(ctx context.Context) string {
	if h.WC == nil {
		return "the recording disk's free space couldn't be checked"
	}
	info, err := h.WC.DeviceInfo(ctx)
	if err != nil {
		return "the recording disk's free space couldn't be checked"
	}
	for _, hd := range info.HDDs {
		cap, ok := webcontrol.ParseSize(hd.Capacity)
		if !ok || cap <= 0 {
			continue
		}
		free, ok := webcontrol.ParseSize(hd.Free)
		if !ok {
			continue
		}
		if pct := 100 * float64(cap-free) / float64(cap); pct >= diskFullPercent {
			return "the recording disk is almost full"
		}
	}
	return ""
}

// resolvePort decides the host port App4 asks Docker to publish.
// explicit (from the request body) is honored exactly or refused — an
// admin's own choice is never silently overridden; a 0 (no explicit
// choice) falls back to the app's own documented ContainerPort and tries
// a few ports upward from there if that one's taken. An app with no web
// UI (ContainerPort == 0, e.g. wireguard) gets no port at all.
// resolvePort: apps run on the box's own network (docker run --network
// host — the box's kernel has no veth/br_netfilter for Docker's bridge,
// ARCHITECTURE.md, "Box checks for 0.4.0"), so an app listens on its own
// port and it can't be remapped; a conflict is refused, naming who has
// the port when it's one of the box's own services.
func (h *Apps) resolvePort(app appcatalog.App, explicit int) (port int, problem string) {
	if app.ContainerPort == 0 {
		return 0, ""
	}
	p := app.ContainerPort
	if explicit != 0 && explicit != p {
		return 0, fmt.Sprintf("%s always uses port %d on this box (apps share the box's network)", app.Name, p)
	}
	if portBindable(p) {
		return p, ""
	}
	if name, known := knownPorts[p]; known {
		return 0, fmt.Sprintf("port %d is already used by %s", p, name)
	}
	return 0, fmt.Sprintf("port %d is already in use", p)
}

func (h *Apps) planRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	ctx := r.Context()
	containers, err := h.Docker.List(ctx)
	if err != nil {
		writeDockerError(w, r, err, "could not look up the app")
		return
	}
	var found *dockerd.Container
	for i := range containers {
		if containers[i].ID == id {
			found = &containers[i]
			break
		}
	}
	if found == nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such installed app")
		return
	}
	name := found.Image
	if a, ok := appcatalog.ByID(found.AppID()); ok {
		name = a.Name
	}

	user, _ := httpx.User(ctx)
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "apps.remove",
		Changes:     []plan.Change{{Type: "apps.remove", Before: map[string]any{"id": id, "name": name}}},
		Summary:     "Remove " + name + " (its stored config at " + path.Join(h.StorageRoot, found.AppID()) + " is kept)",
		Detail:      map[string]any{"id": id, "appId": found.AppID()},
		Fingerprint: "installed:" + found.State,
		Current: func(ctx context.Context) (string, error) {
			list, err := h.Docker.List(ctx)
			if err != nil {
				return "", err
			}
			for _, c := range list {
				if c.ID == id {
					return "installed:" + c.State, nil
				}
			}
			return "removed", nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return &plan.Failure{Status: http.StatusConflict, Code: "writes_disabled", Message: "device writes are disabled"}
			}
			if err := h.Docker.Remove(ctx, id); err != nil {
				return &plan.Failure{Status: http.StatusBadGateway, Code: "remove_failed", Message: "the app could not be removed", Err: err}
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

// startInstalled/stopInstalled are direct actions, not a plan — the same
// bar ControlHub's own zap/power actions use: reversible, expected,
// low-risk compared to install (pulls and runs new software) or remove
// (deletes a container), which do go through preview and confirm above.
func (h *Apps) startInstalled(w http.ResponseWriter, r *http.Request) {
	h.setRunning(w, r, true)
}

func (h *Apps) stopInstalled(w http.ResponseWriter, r *http.Request) {
	h.setRunning(w, r, false)
}

func (h *Apps) setRunning(w http.ResponseWriter, r *http.Request, running bool) {
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	if h.ReadOnly {
		httpx.WriteError(w, r, http.StatusConflict, "writes_disabled", "device writes are disabled")
		return
	}
	id := r.PathValue("id")
	var err error
	if running {
		err = h.Docker.Start(r.Context(), id)
	} else {
		err = h.Docker.Stop(r.Context(), id)
	}
	if err == dockerd.ErrNotFound {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such installed app")
		return
	}
	if err != nil {
		writeDockerError(w, r, err, "the request failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeDockerError answers a failed helper call. An unreachable helper or
// daemon (Docker not installed on this box, or the helper not running) is
// its own code, docker_unavailable, so the Apps tab can say so plainly
// and keep the catalog usable, instead of a generic failure.
func writeDockerError(w http.ResponseWriter, r *http.Request, err error, msg string) {
	if errors.Is(err, dockerd.ErrUnavailable) {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "Docker isn't reachable on this box")
		return
	}
	httpx.WriteError(w, r, http.StatusBadGateway, "docker_failed", msg)
}
