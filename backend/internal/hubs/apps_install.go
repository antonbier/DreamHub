package hubs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
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

// portFree reports whether p is free to bind on this host right now — a
// real attempt, not just "not in our own known list" (docs/APPSHUB_PLAN.md:
// "combine it with an actual bind attempt"); a Local port is tried on
// 127.0.0.1 only, the way the app itself binds it.
func portFree(p appcatalog.Port) bool {
	host := ""
	if p.Local {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(p.Port))
	if p.UDP {
		c, err := net.ListenPacket("udp", addr)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	}
	l, err := net.Listen("tcp", addr)
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
	// ID is the app's main container's id — what start/stop/remove/logs/
	// console are addressed by; start/stop/remove act on every container
	// of the app.
	ID       string `json:"id"`
	AppID    string `json:"appId"`
	Name     string `json:"name"` // the catalog's name, or the raw image if unknown
	Image    string `json:"image"`
	State    string `json:"state"`
	HostPort int    `json:"hostPort,omitempty"`
	// Upgradable is App5's update check (apps_watch.go): the registry's
	// current digest for one of the app's images no longer matches what's
	// actually running. Always computed live, never stored, same as
	// HostPort above.
	Upgradable bool `json:"upgradable,omitempty"`
	// Services are the app's helper containers (e.g. Nextcloud's
	// database), each with its own logs.
	Services []installedService `json:"services,omitempty"`
}

type installedService struct {
	ID      string `json:"id"`
	Service string `json:"service"`
	Image   string `json:"image"`
	State   string `json:"state"`
}

// appGroup is one installed app: its main container and its helpers.
type appGroup struct {
	appID    string
	main     *dockerd.Container
	services []dockerd.Container
}

// all is every container of the app, the main one first.
func (g appGroup) all() []dockerd.Container {
	var out []dockerd.Container
	if g.main != nil {
		out = append(out, *g.main)
	}
	return append(out, g.services...)
}

// groupApps groups containers by AppID, sorted by AppID.
func groupApps(containers []dockerd.Container) []appGroup {
	byID := map[string]*appGroup{}
	var order []string
	for i := range containers {
		c := containers[i]
		g, ok := byID[c.AppID()]
		if !ok {
			g = &appGroup{appID: c.AppID()}
			byID[c.AppID()] = g
			order = append(order, c.AppID())
		}
		if c.Service() == "" {
			g.main = &c
		} else {
			g.services = append(g.services, c)
		}
	}
	sort.Strings(order)
	out := make([]appGroup, 0, len(order))
	for _, id := range order {
		g := byID[id]
		sort.Slice(g.services, func(i, j int) bool { return g.services[i].Service() < g.services[j].Service() })
		out = append(out, *g)
	}
	return out
}

// findApp returns the group one of whose containers has id.
func findApp(containers []dockerd.Container, id string) (appGroup, bool) {
	for _, g := range groupApps(containers) {
		for _, c := range g.all() {
			if c.ID == id {
				return g, true
			}
		}
	}
	return appGroup{}, false
}

func appName(appID, fallback string) string {
	if a, ok := appcatalog.ByID(appID); ok {
		return a.Name
	}
	return fallback
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
	groups := groupApps(containers)
	out := make([]installedApp, 0, len(groups))
	for _, g := range groups {
		all := g.all()
		head := all[0] // the main container, or a helper left over without it
		row := installedApp{
			ID: head.ID, AppID: g.appID, Name: appName(g.appID, head.Image), Image: head.Image,
			State: head.State, HostPort: head.HostPort,
		}
		for _, c := range all {
			if h.imageUpgradable(r.Context(), c) {
				row.Upgradable = true
			}
		}
		for _, c := range g.services {
			row.Services = append(row.Services, installedService{ID: c.ID, Service: c.Service(), Image: c.Image, State: c.State})
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"apps": out})
}

func installedFingerprint(containers []dockerd.Container, appID string) string {
	var ids []string
	for _, c := range containers {
		if c.AppID() == appID {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		return "not-installed"
	}
	sort.Strings(ids)
	return "installed:" + strings.Join(ids, ",")
}

// Storage paths of an app, all under StorageRoot and all computed here,
// never taken from the image or a request: its own config directory, one
// per helper, its bulk data directory, and its generated passwords (in no
// container's mount).
func (h *Apps) configDir(appID string) string { return path.Join(h.StorageRoot, appID) }
func (h *Apps) serviceDir(appID, service string) string {
	return path.Join(h.StorageRoot, appID+"."+service)
}
func (h *Apps) dataDir(appID string) string { return path.Join(h.StorageRoot, appID+".data") }
func (h *Apps) secretsFile(appID string) string {
	return path.Join(h.StorageRoot, appID+".secrets.json")
}

// appSpecs builds the containers of app, helpers first (the app expects
// its database to be there), with env still holding "{{secret:…}}"
// placeholders — Execute fills them in.
func (h *Apps) appSpecs(app appcatalog.App, memoryLimitMB int, tz string) []dockerd.ContainerSpec {
	var specs []dockerd.ContainerSpec
	withTZ := func(env map[string]string) map[string]string {
		out := map[string]string{"TZ": tz}
		for k, v := range env {
			out[k] = v
		}
		return out
	}
	for _, s := range app.Services {
		specs = append(specs, dockerd.ContainerSpec{
			AppID: app.ID, Service: s.Name, Image: appcatalog.ImageRef(s.Image, s.Tag),
			Env: withTZ(s.Env), Cmd: s.Cmd,
			HostDir: h.serviceDir(app.ID, s.Name), ContainerDir: s.ConfigPath,
			MemoryLimitBytes: int64(s.MemoryMB) << 20, HostNetwork: true,
		})
	}
	main := dockerd.ContainerSpec{
		AppID: app.ID, Image: appcatalog.ImageRef(app.Image, app.Tag),
		Env: withTZ(app.Env), Cmd: app.Cmd,
		HostPort: app.ContainerPort, ContainerPort: app.ContainerPort,
		HostDir: h.configDir(app.ID), ContainerDir: app.ConfigPath,
		MemoryLimitBytes: int64(memoryLimitMB) << 20, HostNetwork: true,
	}
	if app.DataPath != "" {
		main.DataHostDir, main.DataContainerDir = h.dataDir(app.ID), app.DataPath
	}
	return append(specs, main)
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
		memoryLimitMB = app.MemoryMB
	}
	if memoryLimitMB <= 0 {
		memoryLimitMB = defaultMemoryLimitMB
	}
	totalMB := memoryLimitMB + app.TotalMemoryMB() - app.MemoryMB
	if problem := h.checkResourceBudget(ctx, int64(totalMB)<<20); problem != "" {
		httpx.WriteError(w, r, http.StatusConflict, "insufficient_memory", problem)
		return
	}
	if problem := h.checkDiskSpace(ctx); problem != "" {
		httpx.WriteError(w, r, http.StatusConflict, "disk_full", problem)
		return
	}
	if problem := h.checkPorts(app, body.HostPort); problem != "" {
		httpx.WriteError(w, r, http.StatusConflict, "port_in_use", problem)
		return
	}

	tz := containerTZ()
	specs := h.appSpecs(app, memoryLimitMB, tz)
	main := specs[len(specs)-1]

	summary := fmt.Sprintf("Install %s (%s)", app.Name, main.Image)
	for _, s := range specs[:len(specs)-1] {
		summary += fmt.Sprintf(" with its %s (%s)", s.Service, s.Image)
	}
	summary += fmt.Sprintf(", up to %d MB RAM, storage at %s", totalMB, main.HostDir)
	if main.DataHostDir != "" {
		summary += fmt.Sprintf(", data at %s", main.DataHostDir)
	}
	if app.ContainerPort != 0 {
		summary = fmt.Sprintf("%s, reachable on port %d", summary, app.ContainerPort)
	}

	images := make([]string, 0, len(specs))
	for _, s := range specs {
		images = append(images, s.Image)
	}
	user, _ := httpx.User(ctx)
	pl := h.Plans.Create(user, plan.Draft{
		Action: "apps.install",
		Changes: []plan.Change{{Type: "apps.install", After: map[string]any{
			"appId": app.ID, "images": images, "hostPort": app.ContainerPort, "memoryLimitMB": totalMB, "storage": main.HostDir, "tz": tz,
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
			secrets, err := h.appSecrets(app)
			if err != nil {
				return &plan.Failure{Status: http.StatusInternalServerError, Code: "install_failed", Message: "the app's passwords could not be stored", Err: err}
			}
			var created []string
			for _, spec := range specs {
				spec.Env = appcatalog.ExpandEnv(spec.Env, secrets)
				c, err := h.Docker.Create(ctx, spec)
				if err != nil {
					// Nothing half-installed stays behind: the helpers
					// already created go again (their storage is kept).
					for _, id := range created {
						_ = h.Docker.Remove(context.WithoutCancel(ctx), id)
					}
					return &plan.Failure{Status: http.StatusBadGateway, Code: "install_failed", Message: "the app could not be installed", Err: err}
				}
				created = append(created, c.ID)
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

// appSecrets returns app's generated passwords, creating and storing the
// missing ones — kept across a remove and reinstall (an update), since
// the app's database was set up with them.
func (h *Apps) appSecrets(app appcatalog.App) (map[string]string, error) {
	names := app.SecretNames()
	secrets := map[string]string{}
	if len(names) == 0 {
		return secrets, nil
	}
	file := h.secretsFile(app.ID)
	if data, err := h.configFS().ReadFile(file); err == nil {
		if err := json.Unmarshal(data, &secrets); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	changed := false
	for _, n := range names {
		if secrets[n] != "" {
			continue
		}
		b := make([]byte, 18)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		secrets[n] = hex.EncodeToString(b)
		changed = true
	}
	if changed {
		data, err := json.Marshal(secrets)
		if err != nil {
			return nil, err
		}
		if err := h.configFS().WriteFile(file, data); err != nil {
			return nil, err
		}
	}
	return secrets, nil
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

// checkPorts: apps run on the box's own network (docker run --network
// host — the box's kernel has no veth/br_netfilter for Docker's bridge,
// ARCHITECTURE.md, "Box checks for 0.4.0"), so an app listens on its own
// ports and they can't be remapped. Every one of them — the web port, the
// app's other ports, its helpers' — is tried for real; a taken one is
// refused, naming who has it when it's one of the box's own services. An
// explicit hostPort other than the app's own is refused too: an admin's
// choice is never silently overridden.
func (h *Apps) checkPorts(app appcatalog.App, explicit int) string {
	if explicit != 0 && explicit != app.ContainerPort {
		if app.ContainerPort == 0 {
			return fmt.Sprintf("%s has no web port", app.Name)
		}
		return fmt.Sprintf("%s always uses port %d on this box (apps share the box's network)", app.Name, app.ContainerPort)
	}
	var ports []appcatalog.Port
	if app.ContainerPort != 0 {
		ports = append(ports, appcatalog.Port{Port: app.ContainerPort})
	}
	ports = append(ports, app.Ports...)
	for _, s := range app.Services {
		ports = append(ports, s.Ports...)
	}
	free := h.PortFree
	if free == nil {
		free = portFree
	}
	for _, p := range ports {
		if free(p) {
			continue
		}
		proto := "TCP"
		if p.UDP {
			proto = "UDP"
		}
		if name, known := knownPorts[p.Port]; known && !p.UDP {
			return fmt.Sprintf("port %d is already used by %s", p.Port, name)
		}
		return fmt.Sprintf("port %d (%s) is already in use", p.Port, proto)
	}
	return ""
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
	g, found := findApp(containers, id)
	if !found {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such installed app")
		return
	}
	all := g.all()
	name := appName(g.appID, all[0].Image)
	kept := []string{h.configDir(g.appID)}
	var ids []string
	for _, c := range all {
		ids = append(ids, c.ID)
		if c.Service() != "" {
			kept = append(kept, h.serviceDir(g.appID, c.Service()))
		}
	}
	summary := "Remove " + name
	if len(g.services) > 0 {
		summary += fmt.Sprintf(" and its %d helper container(s)", len(g.services))
	}
	summary += " (its stored config at " + strings.Join(kept, ", ") + " is kept)"

	user, _ := httpx.User(ctx)
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "apps.remove",
		Changes:     []plan.Change{{Type: "apps.remove", Before: map[string]any{"id": id, "name": name, "containers": ids}}},
		Summary:     summary,
		Detail:      map[string]any{"id": id, "appId": g.appID},
		Fingerprint: installedFingerprint(containers, g.appID),
		Current: func(ctx context.Context) (string, error) {
			list, err := h.Docker.List(ctx)
			if err != nil {
				return "", err
			}
			return installedFingerprint(list, g.appID), nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return &plan.Failure{Status: http.StatusConflict, Code: "writes_disabled", Message: "device writes are disabled"}
			}
			// The app first, then what it depends on.
			for _, cid := range ids {
				if err := h.Docker.Remove(ctx, cid); err != nil && !errors.Is(err, dockerd.ErrNotFound) {
					return &plan.Failure{Status: http.StatusBadGateway, Code: "remove_failed", Message: "the app could not be removed", Err: err}
				}
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
// Both act on every container of the app: start brings helpers up first,
// stop takes the app down first.
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
	containers, err := h.Docker.List(r.Context())
	if err != nil {
		writeDockerError(w, r, err, "the request failed")
		return
	}
	g, found := findApp(containers, r.PathValue("id"))
	if !found {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such installed app")
		return
	}
	order := g.all()
	if running {
		slices.Reverse(order)
	}
	for _, c := range order {
		if running {
			err = h.Docker.Start(r.Context(), c.ID)
		} else {
			err = h.Docker.Stop(r.Context(), c.ID)
		}
		if err == dockerd.ErrNotFound {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such installed app")
			return
		}
		if err != nil {
			writeDockerError(w, r, err, "the request failed")
			return
		}
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
