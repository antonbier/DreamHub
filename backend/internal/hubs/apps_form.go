package hubs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/compose"
	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/unraid"
)

// This file is AppsHub's App12 (docs/APPSHUB_PLAN.md, "Docker apps defined
// as Compose, with a form"): the install form's server side. One request
// shape serves the curated catalog and the admin's own containers: a
// compose file plus the person's values for its fields, extra rows
// (environment variables, ports, paths) and storage choice, turned into the
// same install plan. Everything an admin can type ends up in a
// dockerd.ContainerSpec that the helper checks again (dockerd.ValidateSpec):
// the rules below are what the person sees first, the helper's are what
// holds if this process is ever wrong.

const (
	maxValueLen    = 4096
	maxExtraRows   = 32
	maxComposeText = compose.MaxSize
)

// installBody is POST /api/v1/apps/install/plan.
type installBody struct {
	// AppID names a curated app; Custom carries an admin's own compose
	// file. Exactly one of them.
	AppID  string      `json:"appId"`
	Custom *customBody `json:"custom"`
	// HostPort is the web port (kept for older clients; Values holds it as
	// the web port field now).
	HostPort      int               `json:"hostPort"`
	MemoryLimitMB int               `json:"memoryLimitMB"`
	Storage       string            `json:"storage"`
	Values        map[string]string `json:"values"`
	Extra         *extraBody        `json:"extra"`
	// ConfirmElevated is the person's explicit extra confirmation for
	// privileged mode, capabilities, devices and host PID/IPC.
	ConfirmElevated bool `json:"confirmElevated"`
}

type customBody struct {
	Compose string `json:"compose"`
	// Name is what the app is called in the list; it also seeds its id.
	Name string `json:"name"`
}

// extraBody are the advanced view's free rows. They apply to the app's main
// container.
type extraBody struct {
	Env   []extraEnv  `json:"env"`
	Ports []extraPort `json:"ports"`
	Paths []extraPath `json:"paths"`
}

type extraEnv struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type extraPort struct {
	Port int  `json:"port"`
	UDP  bool `json:"udp"`
}

type extraPath struct {
	Host      string `json:"host"`
	Container string `json:"container"`
	ReadOnly  bool   `json:"readOnly"`
}

// formError is a refusal with a stable code the UI translates.
type formError struct {
	status  int
	code    string
	message string
	details any
}

func (e *formError) write(w http.ResponseWriter, r *http.Request) {
	httpx.WriteErrorDetails(w, r, e.status, e.code, e.message, e.details)
}

func issuesError(err error) *formError {
	var is compose.Issues
	if errors.As(err, &is) {
		msg := is.Error()
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return &formError{http.StatusUnprocessableEntity, "compose_invalid", msg, map[string]any{"issues": is}}
	}
	return &formError{http.StatusUnprocessableEntity, "compose_invalid", err.Error(), nil}
}

// elevatedItem is one thing a container asks for beyond a plain one.
type elevatedItem struct {
	Service string `json:"service,omitempty"`
	// Kind is privileged, capability, device, pid or ipc.
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

func elevatedItems(service string, e compose.Elevated) []elevatedItem {
	var out []elevatedItem
	if e.Privileged {
		out = append(out, elevatedItem{service, "privileged", ""})
	}
	for _, c := range e.CapAdd {
		out = append(out, elevatedItem{service, "capability", c})
	}
	for _, d := range e.Devices {
		out = append(out, elevatedItem{service, "device", d})
	}
	if e.PidHost {
		out = append(out, elevatedItem{service, "pid", "host"})
	}
	if e.IpcHost {
		out = append(out, elevatedItem{service, "ipc", "host"})
	}
	return out
}

// mountPolicy is the rule set for host paths: /media and the app's own
// folders, never DreamHub's own data.
func (h *Apps) mountPolicy() dockerd.MountPolicy {
	if h.Policy != nil {
		return *h.Policy
	}
	p := dockerd.DefaultPolicy()
	if h.DataDir != "" {
		p.Protected = []string{h.DataDir}
	}
	return p
}

var customIDChars = regexp.MustCompile(`[^a-z0-9]+`)

// customAppID is the id of an admin's own app: "my" + the name's letters
// and digits. The prefix keeps it clear of the curated ids (also checked).
func customAppID(name string) string {
	n := customIDChars.ReplaceAllString(strings.ToLower(name), "")
	if n == "" {
		n = "app"
	}
	id := "my" + n
	if len(id) > 20 {
		id = id[:20]
	}
	return id
}

// installPrep is a validated install, ready to preview and apply.
type installPrep struct {
	app      appcatalog.App
	root     string
	mainMB   int
	totalMB  int
	tz       string
	specs    []dockerd.ContainerSpec // helpers first, the main container last
	elevated []elevatedItem
	warnings []status.Problem
	// secretValues are the form values of secret fields: masked in the
	// preview wherever they appear.
	secretValues map[string]bool
}

// resolveApp turns the request into an app: the compose file, the form
// values applied and the extra rows added. It does not look at Docker, the
// disk or the box's ports — prepareInstall does, parseCustom doesn't.
func (h *Apps) resolveApp(body installBody) (appcatalog.App, map[string]bool, *formError) {
	var (
		f      *compose.File
		id     string
		custom bool
	)
	switch {
	case body.AppID != "" && body.Custom != nil, body.AppID == "" && body.Custom == nil:
		return appcatalog.App{}, nil, &formError{http.StatusBadRequest, "invalid_body", `expected {"appId": "..."} or {"custom": {...}}`, nil}
	case body.AppID != "":
		cat, ok := appcatalog.ByID(body.AppID)
		if !ok {
			return appcatalog.App{}, nil, &formError{http.StatusNotFound, "not_found", "no such app in the curated catalog", nil}
		}
		f, id = cat.File, cat.ID
	default:
		if len(body.Custom.Compose) > maxComposeText {
			return appcatalog.App{}, nil, &formError{http.StatusRequestEntityTooLarge, "compose_invalid", "the compose file is too large", nil}
		}
		parsed, err := compose.Parse([]byte(body.Custom.Compose))
		if err != nil {
			return appcatalog.App{}, nil, issuesError(err)
		}
		f, custom = parsed, true
		name := strings.TrimSpace(body.Custom.Name)
		if name == "" {
			name = firstNonEmpty(f.Ext.Name, f.Name, path.Base(f.MainService().Image))
		}
		id = customAppID(name)
		if _, taken := appcatalog.ByID(id); taken {
			id += "x"
		}
	}

	fields := f.Fields("en")
	values, ferr := cleanValues(fields, body.Values)
	if ferr != nil {
		return appcatalog.App{}, nil, ferr
	}
	secret := map[string]bool{}
	for _, fl := range fields {
		if fl.Secret && values[fl.Name] != "" {
			secret[values[fl.Name]] = true
		}
	}

	app, err := appcatalog.Resolve(id, f, values, custom)
	if err != nil {
		return appcatalog.App{}, nil, issuesError(err)
	}
	if custom {
		if n := strings.TrimSpace(body.Custom.Name); n != "" {
			app.Name = n
		}
		if len(app.Name) > 60 || !dockerd.ValidName(app.Name) {
			return appcatalog.App{}, nil, &formError{http.StatusBadRequest, "name_invalid", "the name must be at most 60 characters", nil}
		}
	}

	// The older single hostPort: the web port field.
	if body.HostPort != 0 && body.HostPort != app.ContainerPort {
		if !app.PortEditable() {
			return appcatalog.App{}, nil, &formError{http.StatusConflict, "port_fixed", fmt.Sprintf("%s always uses port %d on this box", app.Name, app.ContainerPort), nil}
		}
		if body.HostPort < 1024 || body.HostPort > 65535 || boxPorts[body.HostPort] {
			return appcatalog.App{}, nil, &formError{http.StatusBadRequest, "port_invalid", "pick a port between 1024 and 65535 that the box doesn't use itself", nil}
		}
		values[app.PortVar] = strconv.Itoa(body.HostPort)
		if app, err = appcatalog.Resolve(id, f, values, custom); err != nil {
			return appcatalog.App{}, nil, issuesError(err)
		}
		if custom && strings.TrimSpace(body.Custom.Name) != "" {
			app.Name = strings.TrimSpace(body.Custom.Name)
		}
	}

	if body.Extra != nil {
		if ferr := applyExtras(&app, body.Extra); ferr != nil {
			return appcatalog.App{}, nil, ferr
		}
		for _, e := range body.Extra.Env {
			if looksSecret(e.Key) {
				secret[e.Value] = true
			}
		}
	}
	delete(secret, "")
	return app, secret, nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

var (
	extraEnvKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	secretKey   = regexp.MustCompile(`(?i)(PASS|SECRET|TOKEN|KEY)`)
)

func looksSecret(key string) bool { return secretKey.MatchString(key) }

// cleanValues checks the form values against the fields and drops empty
// ones (an empty value means "the default").
func cleanValues(fields []compose.Field, in map[string]string) (map[string]string, *formError) {
	byName := map[string]compose.Field{}
	for _, f := range fields {
		byName[f.Name] = f
	}
	out := map[string]string{}
	for name, v := range in {
		f, ok := byName[name]
		if !ok {
			return nil, &formError{http.StatusBadRequest, "unknown_field", "there is no field " + name, map[string]any{"field": name}}
		}
		if len(v) > maxValueLen || strings.ContainsRune(v, 0) {
			return nil, &formError{http.StatusBadRequest, "value_invalid", "the value of " + name + " is not usable", map[string]any{"field": name}}
		}
		if v == "" {
			continue
		}
		if f.Type == "port" {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			min := 1
			if f.WebPort {
				min = 1024
			}
			if err != nil || n < min || n > 65535 || boxPorts[n] {
				return nil, &formError{http.StatusBadRequest, "port_invalid", "pick a port between 1024 and 65535 that the box doesn't use itself", map[string]any{"field": name}}
			}
			v = strconv.Itoa(n)
		}
		out[name] = v
	}
	return out, nil
}

// applyExtras adds the advanced rows to the app's main container.
func applyExtras(app *appcatalog.App, x *extraBody) *formError {
	if len(x.Env) > maxExtraRows || len(x.Ports) > maxExtraRows || len(x.Paths) > maxExtraRows {
		return &formError{http.StatusBadRequest, "too_many_rows", "too many extra rows", nil}
	}
	env := map[string]string{}
	for k, v := range app.Env {
		env[k] = v
	}
	for _, e := range x.Env {
		if !extraEnvKey.MatchString(e.Key) || len(e.Value) > maxValueLen || strings.ContainsRune(e.Value, 0) {
			return &formError{http.StatusBadRequest, "extra_env_invalid", "the variable " + e.Key + " is not usable", map[string]any{"key": e.Key}}
		}
		env[e.Key] = e.Value
	}
	app.Env = env
	for _, p := range x.Ports {
		if p.Port < 1 || p.Port > 65535 || boxPorts[p.Port] {
			return &formError{http.StatusBadRequest, "port_invalid", fmt.Sprintf("port %d can't be used", p.Port), map[string]any{"port": p.Port}}
		}
		app.Ports = append(app.Ports, appcatalog.Port{Port: p.Port, UDP: p.UDP})
	}
	for _, p := range x.Paths {
		if !strings.HasPrefix(p.Container, "/") || path.Clean(p.Container) != p.Container || strings.ContainsAny(p.Container, ":\x00\n\r,") || len(p.Container) > 512 {
			return &formError{http.StatusBadRequest, "extra_path_invalid", "the path in the container must be a clean absolute path", map[string]any{"path": p.Container}}
		}
		v := compose.Volume{Target: p.Container, ReadOnly: p.ReadOnly}
		switch {
		case strings.HasPrefix(p.Host, "/"):
			v.HostPath = p.Host
		case strings.HasPrefix(p.Host, "./") && len(p.Host) > 2:
			name := p.Host[2:]
			if len(name) > 20 || strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789") != "" {
				return &formError{http.StatusBadRequest, "extra_path_invalid", "a folder name is 1-20 lowercase letters and digits", map[string]any{"path": p.Host}}
			}
			v.Name = name
		default:
			return &formError{http.StatusBadRequest, "extra_path_invalid", "the folder on the box must be an absolute path (or ./name for the app's own storage)", map[string]any{"path": p.Host}}
		}
		for _, m := range app.Mounts {
			if m.Target == v.Target {
				return &formError{http.StatusBadRequest, "extra_path_invalid", "that path in the container is already mounted", map[string]any{"path": p.Container}}
			}
		}
		app.Mounts = append(app.Mounts, v)
	}
	return nil
}

// hostDirOf is the folder on the box a volume of a container is.
func hostDirOf(root, appID, service string, v compose.Volume) string {
	switch {
	case v.HostPath != "":
		return v.HostPath
	case v.Own() && service == "":
		return configDir(root, appID)
	case v.Own():
		return serviceDir(root, appID, service)
	}
	return path.Join(root, appID+"."+v.Name)
}

// checkMounts applies the mount rules to every volume of every container.
func (h *Apps) checkMounts(app appcatalog.App, root string) *formError {
	policy := h.mountPolicy()
	check := func(service string, vols []compose.Volume) *formError {
		for _, v := range vols {
			host := hostDirOf(root, app.ID, service, v)
			if err := policy.CheckMount(root, app.ID, host); err != nil {
				se, _ := dockerd.IsRefusal(err)
				code, msg, p := "mount_refused", err.Error(), host
				if se != nil {
					msg, p = se.Message, se.Path
					code = "mount_refused"
				}
				return &formError{http.StatusUnprocessableEntity, code, msg, map[string]any{
					"service": service, "path": p, "reason": reasonOf(se), "container": v.Target,
					"issues": compose.Issues{{Code: "mount_refused", Service: service, Key: p, Detail: reasonOf(se), Message: msg}},
				}}
			}
		}
		return nil
	}
	if ferr := check("", app.Mounts); ferr != nil {
		return ferr
	}
	for _, s := range app.Services {
		if ferr := check(s.Name, s.Mounts); ferr != nil {
			return ferr
		}
	}
	return nil
}

func reasonOf(se *dockerd.SpecError) string {
	if se == nil {
		return ""
	}
	return se.Code
}

// allElevated lists everything every container of app asks for.
func allElevated(app appcatalog.App) []elevatedItem {
	items := elevatedItems("", app.Elevated)
	for _, s := range app.Services {
		items = append(items, elevatedItems(s.Name, s.Elevated)...)
	}
	return items
}

// chooseRoot validates the storage choice (the same places GET
// /api/v1/apps/storage lists).
func (h *Apps) chooseRoot(ctx context.Context, storage string) (string, *formError) {
	if storage == "" || storage == h.StorageRoot {
		return h.StorageRoot, nil
	}
	for _, c := range h.storageChoices(ctx) {
		if c.Path == storage {
			if c.FreeBytes < minStorageFreeBytes {
				return "", &formError{http.StatusConflict, "disk_full", "there is not enough free space at that location", nil}
			}
			return c.Path, nil
		}
	}
	return "", &formError{http.StatusBadRequest, "storage_invalid", "that isn't one of the places apps can be stored", nil}
}

// prepareInstall validates a whole install request against this box:
// the mount rules, the elevated confirmation, memory, disk and ports.
func (h *Apps) prepareInstall(ctx context.Context, body installBody, installed []dockerd.Container) (*installPrep, *formError) {
	app, secret, ferr := h.resolveApp(body)
	if ferr != nil {
		return nil, ferr
	}
	if installedFingerprint(installed, app.ID) != "not-installed" {
		return nil, &formError{http.StatusConflict, "already_installed", app.Name + " is already installed", nil}
	}
	root, ferr := h.chooseRoot(ctx, body.Storage)
	if ferr != nil {
		return nil, ferr
	}
	// Refusals first: a person must see "this mount is refused" before
	// being asked to confirm anything.
	if ferr := h.checkMounts(app, root); ferr != nil {
		return nil, ferr
	}
	prep := &installPrep{app: app, root: root, tz: containerTZ(), secretValues: secret}
	prep.elevated = allElevated(app)
	for _, it := range prep.elevated {
		if it.Kind == "device" {
			if err := dockerd.CheckDevice(it.Value); err != nil {
				se, _ := dockerd.IsRefusal(err)
				return nil, &formError{http.StatusUnprocessableEntity, "device_refused", se.Message, map[string]any{
					"service": it.Service, "path": it.Value, "issues": compose.Issues{{Code: "device_refused", Service: it.Service, Key: it.Value, Message: se.Message}}}}
			}
		}
	}
	if len(prep.elevated) > 0 && !body.ConfirmElevated {
		return nil, &formError{http.StatusConflict, "confirmation_required",
			"this app asks for more than a plain container; it needs your explicit confirmation",
			map[string]any{"elevated": prep.elevated}}
	}

	// Far above any box's RAM: refuse it before the MB→bytes shifts below
	// can overflow and slip past the budget check.
	if body.MemoryLimitMB > 1<<20 {
		return nil, &formError{http.StatusConflict, "insufficient_memory", "that is more memory than the box has", nil}
	}
	prep.mainMB = body.MemoryLimitMB
	if prep.mainMB <= 0 {
		prep.mainMB = app.MemoryMB
	}
	if prep.mainMB <= 0 {
		prep.mainMB = defaultMemoryLimitMB
	}
	prep.totalMB = prep.mainMB
	for _, s := range app.Services {
		prep.totalMB += serviceMemoryMB(s)
	}
	if problem := h.checkResourceBudget(ctx, int64(prep.totalMB)<<20); problem != "" {
		return nil, &formError{http.StatusConflict, "insufficient_memory", problem, nil}
	}
	if problem := h.checkDiskSpace(ctx); problem != "" {
		return nil, &formError{http.StatusConflict, "disk_full", problem, nil}
	}
	if problem := h.checkPorts(app, 0); problem != "" {
		return nil, &formError{http.StatusConflict, "port_in_use", problem, nil}
	}
	if app.Custom {
		if ferr := h.checkCustomImages(ctx, app, prep); ferr != nil {
			return nil, ferr
		}
	}
	if len(prep.elevated) > 0 {
		prep.warnings = append(prep.warnings, status.Problem{Code: "apps.elevated", Params: map[string]any{"count": len(prep.elevated)}})
	}
	prep.specs = h.appSpecs(app, prep.mainMB, prep.tz, root, body.ConfirmElevated)
	return prep, nil
}

func serviceMemoryMB(s appcatalog.Service) int {
	if s.MemoryMB > 0 {
		return s.MemoryMB
	}
	return defaultMemoryLimitMB
}

// hubImage turns an image reference into the "namespace/repository" Docker
// Hub knows, or "" for one on another registry (checked only on Docker Hub).
func hubImage(image string) string {
	first, _, hasSlash := strings.Cut(image, "/")
	if hasSlash && (strings.ContainsAny(first, ".:") || first == "localhost") {
		return ""
	}
	if !hasSlash {
		image = "library/" + image
	}
	if !validDockerImage.MatchString(image) {
		return ""
	}
	return image
}

// checkCustomImages asks Docker Hub, best effort, whether each image of a
// custom app exists and has the box's architecture. Another registry, or
// Docker Hub not answering, is a warning — the pull itself decides then.
func (h *Apps) checkCustomImages(ctx context.Context, app appcatalog.App, prep *installPrep) *formError {
	if h.DockerHub == nil {
		return nil
	}
	type img struct{ image, tag string }
	imgs := []img{{app.Image, app.Tag}}
	for _, s := range app.Services {
		imgs = append(imgs, img{s.Image, s.Tag})
	}
	box := dockerhub.BoxArch()
	for _, i := range imgs {
		name := hubImage(i.image)
		if name == "" {
			prep.warnings = append(prep.warnings, status.Problem{Code: "apps.imageUnchecked", Params: map[string]any{"image": i.image}})
			continue
		}
		tag := i.tag
		if tag == "" {
			tag = "latest"
		}
		arches, err := h.DockerHub.Manifest(ctx, name, tag)
		switch {
		case errors.Is(err, dockerhub.ErrNotFound):
			return &formError{http.StatusConflict, "image_not_found", fmt.Sprintf("%s:%s wasn't found on Docker Hub", i.image, tag), map[string]any{"image": i.image}}
		case err != nil:
			prep.warnings = append(prep.warnings, status.Problem{Code: "apps.imageUnchecked", Params: map[string]any{"image": i.image}})
		case !dockerhub.Supports(arches, box):
			return &formError{http.StatusConflict, "arch_incompatible", fmt.Sprintf("%s has no build for this box (%s)", i.image, box.Architecture), map[string]any{"image": i.image}}
		}
	}
	return nil
}

// appSpecs builds the containers of app, helpers first (the app expects
// its database to be there), with env still holding "{{secret:…}}"
// placeholders — Execute fills them in.
func (h *Apps) appSpecs(app appcatalog.App, mainMB int, tz, root string, confirmed bool) []dockerd.ContainerSpec {
	var specs []dockerd.ContainerSpec
	withTZ := func(env map[string]string) map[string]string {
		out := map[string]string{"TZ": tz}
		for k, v := range env {
			out[k] = v
		}
		return out
	}
	mounts := func(service string, vols []compose.Volume) []dockerd.Mount {
		var out []dockerd.Mount
		for _, v := range vols {
			out = append(out, dockerd.Mount{HostDir: hostDirOf(root, app.ID, service, v), ContainerDir: v.Target, ReadOnly: v.ReadOnly})
		}
		return out
	}
	// The helper reads Confirmed only when something is elevated; it is
	// the person's checkbox, nothing else.
	elevated := func(e compose.Elevated) dockerd.Elevated {
		return dockerd.Elevated{Privileged: e.Privileged, CapAdd: e.CapAdd, Devices: e.Devices, PidHost: e.PidHost, IpcHost: e.IpcHost, Confirmed: confirmed}
	}
	for _, s := range app.Services {
		specs = append(specs, dockerd.ContainerSpec{
			AppID: app.ID, Service: s.Name, Image: appcatalog.ImageRef(s.Image, s.Tag),
			Env: withTZ(s.Env), Cmd: s.Cmd, Mounts: mounts(s.Name, s.Mounts), Elevated: elevated(s.Elevated),
			MemoryLimitBytes: int64(serviceMemoryMB(s)) << 20, HostNetwork: true, Root: root,
		})
	}
	main := dockerd.ContainerSpec{
		AppID: app.ID, Image: appcatalog.ImageRef(app.Image, app.Tag),
		Env: withTZ(app.Env), Cmd: app.Cmd,
		HostPort: app.ContainerPort, ContainerPort: app.ContainerPort,
		Mounts: mounts("", app.Mounts), Elevated: elevated(app.Elevated),
		MemoryLimitBytes: int64(mainMB) << 20, HostNetwork: true, Root: root,
	}
	if app.Custom {
		main.Name = app.Name
	}
	return append(specs, main)
}

// --- the preview -------------------------------------------------------

type previewPort struct {
	Port  int  `json:"port"`
	UDP   bool `json:"udp,omitempty"`
	Local bool `json:"local,omitempty"`
}

type previewMount struct {
	Host      string `json:"host"`
	Container string `json:"container"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

type previewEnv struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// Masked: a secret, shown as dots; Generated: made at install.
	Masked    bool `json:"masked,omitempty"`
	Generated bool `json:"generated,omitempty"`
}

type previewContainer struct {
	Service  string            `json:"service,omitempty"`
	Image    string            `json:"image"`
	MemoryMB int               `json:"memoryMB"`
	Ports    []previewPort     `json:"ports,omitempty"`
	Mounts   []previewMount    `json:"mounts,omitempty"`
	Env      []previewEnv      `json:"env,omitempty"`
	Command  []string          `json:"command,omitempty"`
	Elevated *compose.Elevated `json:"elevated,omitempty"`
}

// preview lists exactly what will be created. Secret values never appear.
func (p *installPrep) preview() []previewContainer {
	mask := func(env map[string]string) []previewEnv {
		keys := make([]string, 0, len(env))
		for k := range env {
			if k != "TZ" || env[k] != p.tz {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		out := make([]previewEnv, 0, len(keys))
		for _, k := range keys {
			v := env[k]
			pe := previewEnv{Key: k, Value: v}
			switch {
			case strings.HasPrefix(v, "{{secret:"):
				pe.Value, pe.Generated = "", true
			case p.secretValues[v] || looksSecret(k):
				pe.Value, pe.Masked = "", true
			}
			out = append(out, pe)
		}
		return out
	}
	build := func(service string, spec dockerd.ContainerSpec, ports []appcatalog.Port, el compose.Elevated) previewContainer {
		pc := previewContainer{Service: service, Image: spec.Image, MemoryMB: int(spec.MemoryLimitBytes >> 20), Env: mask(spec.Env), Command: spec.Cmd}
		for _, m := range spec.Mounts {
			pc.Mounts = append(pc.Mounts, previewMount{Host: m.HostDir, Container: m.ContainerDir, ReadOnly: m.ReadOnly})
		}
		for _, port := range ports {
			pc.Ports = append(pc.Ports, previewPort{Port: port.Port, UDP: port.UDP, Local: port.Local})
		}
		if el.Any() {
			e := el
			pc.Elevated = &e
		}
		return pc
	}
	var out []previewContainer
	for i, s := range p.app.Services {
		out = append(out, build(s.Name, p.specs[i], s.Ports, s.Elevated))
	}
	mainPorts := p.app.Ports
	if p.app.ContainerPort != 0 {
		mainPorts = append([]appcatalog.Port{{Port: p.app.ContainerPort}}, mainPorts...)
	}
	return append(out, build("", p.specs[len(p.specs)-1], mainPorts, p.app.Elevated))
}

// --- GET /api/v1/apps/custom/parse ---------------------------------------

type parseBody struct {
	// Kind is compose, image or unraid; Text the pasted text (a compose
	// file, an image name such as "ghcr.io/x/y:1", or an Unraid XML).
	Kind string `json:"kind"`
	Text string `json:"text"`
	Name string `json:"name"`
	Lang string `json:"lang"`
}

type parsedService struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// parsedApp is what the form is built from.
type parsedApp struct {
	// Compose is the normalized compose file (an image name or an Unraid
	// template becomes one) — sent back with the install request.
	Compose      string          `json:"compose"`
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Note         string          `json:"note,omitempty"`
	Fields       []compose.Field `json:"fields"`
	Services     []parsedService `json:"services"`
	Elevated     []elevatedItem  `json:"elevated"`
	Notes        []compose.Issue `json:"notes"`
	WebPort      int             `json:"webPort,omitempty"`
	PortEditable bool            `json:"portEditable,omitempty"`
	MemoryMB     int             `json:"memoryMB"`
}

// parseCustom builds the form for an admin's own container from a pasted
// compose file, an image name or an Unraid template. It changes nothing; a
// file that is refused (including by the mount rules, with the field
// defaults applied) is refused here already, so the person sees why before
// filling in anything.
func (h *Apps) parseCustom(w http.ResponseWriter, r *http.Request) {
	var body parseBody
	if err := httpx.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.Text) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"kind": "compose|image|unraid", "text": "..."}`)
		return
	}
	text := body.Text
	var extraNotes []compose.Issue
	switch body.Kind {
	case "compose":
	case "image":
		t, err := composeForImage(strings.TrimSpace(body.Text), body.Name)
		if err != nil {
			issuesError(err).write(w, r)
			return
		}
		text = t
	case "unraid":
		t, notes, err := unraid.Convert([]byte(body.Text))
		if err != nil {
			issuesError(err).write(w, r)
			return
		}
		text, extraNotes = t, notes
	default:
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "kind must be compose, image or unraid")
		return
	}
	if len(text) > maxComposeText {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "compose_invalid", "the compose file is too large")
		return
	}
	req := installBody{Custom: &customBody{Compose: text, Name: body.Name}}
	// Defaults only: required fields without one are what the form asks.
	f, err := compose.Parse([]byte(text))
	if err != nil {
		issuesError(err).write(w, r)
		return
	}
	preview := h.previewDefaults(f, req)
	if preview.err != nil {
		preview.err.write(w, r)
		return
	}
	lang := body.Lang
	if lang != "de" && lang != "it" {
		lang = "en"
	}
	app := preview.app
	out := parsedApp{
		Compose: text, ID: app.ID, Name: app.Name, Fields: f.Fields(lang), Elevated: allElevated(app),
		Notes: append(extraNotes, f.Notes...), WebPort: app.ContainerPort, PortEditable: app.PortEditable(),
		MemoryMB: app.TotalMemoryMB(),
	}
	out.Description, out.Note = app.Localized(lang)
	if out.Elevated == nil {
		out.Elevated = []elevatedItem{}
	}
	if out.Notes == nil {
		out.Notes = []compose.Issue{}
	}
	for _, s := range f.Services {
		out.Services = append(out.Services, parsedService{Name: s.Name, Image: s.Image})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"app": out})
}

type defaultsPreview struct {
	app appcatalog.App
	err *formError
}

// previewDefaults resolves f with only its defaults. Required fields that
// have none are filled with a placeholder just for this check, so the mount
// rules still run on the rest of the file.
func (h *Apps) previewDefaults(f *compose.File, req installBody) defaultsPreview {
	values := map[string]string{}
	for _, fl := range f.Fields("en") {
		if fl.Required && fl.Default == "" {
			switch fl.Type {
			case "port":
				values[fl.Name] = "1024"
			case "path":
				values[fl.Name] = "./placeholder"
			default:
				values[fl.Name] = "x"
			}
		}
	}
	req.Values = values
	app, _, ferr := h.resolveApp(req)
	if ferr != nil {
		if len(values) > 0 && ferr.code == "compose_invalid" {
			// A placeholder may be what the file can't take (a memory size,
			// an image tag): the form still opens, the plan checks the
			// real values.
			name := firstNonEmpty(req.Custom.Name, f.Ext.Name, f.Name, path.Base(f.MainService().Image))
			return defaultsPreview{app: appcatalog.App{ID: customAppID(name), Name: name, File: f, DescriptionText: f.Ext.Description, NoteText: f.Ext.Note}}
		}
		return defaultsPreview{err: ferr}
	}
	if ferr := h.checkMounts(app, h.StorageRoot); ferr != nil {
		return defaultsPreview{err: ferr}
	}
	return defaultsPreview{app: app}
}

// composeForImage is the compose file for "just an image": one service and
// an optional web port field.
func composeForImage(image, name string) (string, error) {
	img, tag, err := compose.SplitImage(image)
	if err != nil {
		return "", compose.Issues{{Code: "image_invalid", Detail: image, Message: err.Error()}}
	}
	ref := img
	if tag != "" {
		ref += ":" + tag
	}
	if name == "" {
		name = path.Base(img)
	}
	// Built from parts that were validated above: the image and the name go
	// through the YAML encoder in the parser's own tests (quoted).
	return "x-dreamhub:\n  name: " + strconv.Quote(name) + "\n  webui:\n    port: ${WEB_PORT:-0}\nservices:\n  app:\n    image: " + strconv.Quote(ref) + "\n", nil
}
