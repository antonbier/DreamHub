// Package appcatalog is AppsHub's app model and its curated list
// (docs/APPSHUB_PLAN.md, App3: "Curated list, verified multi-arch, no AI
// and no ongoing manual curation of taste"; App12: "Docker apps defined as
// Compose, with a form").
//
// Since App12 an app is a docker-compose.yml with DreamHub's `x-dreamhub`
// extension (internal/compose). The curated apps live as embedded files in
// apps/*.yml; the same model is built from a compose file an admin pastes
// (a "custom" app) — Resolve is the one place a compose file plus the
// person's form values becomes an App. The list is plain, hand-maintained
// project data, never AI-generated or AI-kept-current (PRINCIPLES.md, "AI
// is always optional"), deliberately lean: name, category, image, and which
// architectures were actually checked against Docker Hub's own API, nothing
// requiring ongoing subjective judgment.
//
// A service's `x-dreamhub.arches` is a point-in-time fact, not a live
// check: verified against https://hub.docker.com/v2/repositories/<image>/
// tags/<tag> on 2026-10-12/13. A maintainer can change an image's supported
// platforms at any time; the install flow re-verifies through
// dockerhub.Manifest before ever pulling, the same lazy, on-click check
// free search results get — this list is a useful default, not a promise.
//
// Every app runs on the box's own network (App7), so each one's ports are
// the box's ports — an app whose port can't be moved off one the box
// already uses (80 for WebControl, 8080 for DreamHub) is moved through the
// image's own documented setting (an environment variable bound to the web
// port field), or left out. Left out on purpose: Sonarr/Radarr/Prowlarr/
// Bazarr (the owner doesn't want *arr apps), Heimdall (always binds 80 and
// 443, no setting to move it), WireGuard (needs NET_ADMIN and a kernel
// module), a reverse proxy (needs 80/443, and 80 is WebControl's) and a
// Plex server (the box can't transcode in software).
//
// Storage layout (the compose volume sources, resolved by internal/compose):
// "." is a container's own folder — <root>/<id> for the main service,
// <root>/<id>.<service> for a helper — and "./name" (or a named volume) is
// <root>/<id>.name, shared by the app.
package appcatalog

import (
	"embed"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/compose"
	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
)

// Port is one more port an app binds on the box besides its web port —
// all of them are checked before installing, since on the host network
// two apps (or an app and the box) can't share one.
type Port struct {
	Port int
	UDP  bool
	// Local: the app binds it on 127.0.0.1 only (a database next to its
	// app), so it's checked there and never reachable from the LAN.
	Local bool
}

// Service is a helper container of a multi-container app (e.g. Nextcloud's
// database) — same fields as App's own container, each its own storage
// directory (StorageRoot/<app id>.<service name>). All containers of an
// app share the box's network, so they reach each other on 127.0.0.1.
type Service struct {
	// Name is the helper's role, lowercase letters and digits.
	Name       string
	Image      string
	Tag        string
	Arches     []dockerhub.Arch
	ConfigPath string
	Env        map[string]string
	Cmd        []string
	Ports      []Port
	MemoryMB   int
	// Mounts are all the container's volumes, ConfigPath's included.
	Mounts   []compose.Volume
	Elevated compose.Elevated
}

// App is one app with its form values applied.
type App struct {
	// ID is stable, used as the dreamhub.app label value once installed
	// (internal/dockerd.AppLabel) — lowercase letters and digits.
	ID       string
	Name     string
	Category string
	// Description is plain English text, like a fetched programme
	// synopsis elsewhere in this codebase — data, not UI chrome, so it
	// isn't a locale key (CLAUDE.md's "no hardcoded user-facing strings"
	// is about this app's own interface, not third-party content it
	// displays); DescriptionText/NoteText hold it per language. Note is the
	// same kind of text: what to know before or right after installing it
	// on this box ("" if nothing).
	Description     string
	Note            string
	DescriptionText compose.Text
	NoteText        compose.Text
	// Icon names one of the UI's bundled icons ("" for the category's).
	Icon string
	// Image is the "namespace/repo" to pull (official images as
	// "library/<name>", which Docker pulls the same way). Tag is the tag to
	// follow, "" meaning "latest".
	Image string
	Tag   string
	// Arches are the platforms confirmed in Docker Hub's own manifest for
	// Image's tag — see the package doc. Empty: not checked (a custom app).
	Arches []dockerhub.Arch
	// ContainerPort is the app's web UI port — on the host network it's also
	// the box's port. 0 means the app has no web UI to open. WebPath is the
	// path to open.
	ContainerPort int
	WebPath       string
	// PortVar names the form field holding the web port; "" means the port
	// is fixed (the image has no setting for it).
	PortVar string
	// ConfigPath is where the main container's own folder (StorageRoot/<id>)
	// is mounted, DataPath where its bulk data folder (StorageRoot/<id>.data)
	// is — the latter never in a backup by default (docs/APPSHUB_PLAN.md:
	// "configuration only, never bulk volume data automatically"). Both are
	// views of Mounts.
	ConfigPath string
	DataPath   string
	// Mounts are all the main container's volumes.
	Mounts []compose.Volume
	// LargeConfig: the config directory holds the app's own code as well
	// (Nextcloud copies itself there), so it's opt-in in a backup too.
	LargeConfig bool
	// Env is fixed per app plus what the form set. A value
	// "{{secret:<name>}}" becomes a random password generated once at
	// install and kept next to the app's storage (StorageRoot/<id>.secrets.
	// json, outside every container's mount); the same name means the same
	// value across the app's services.
	Env map[string]string
	// Cmd replaces the image's CMD (its ENTRYPOINT stays).
	Cmd []string
	// Ports are the app's other ports (see Port).
	Ports []Port
	// MemoryMB is the memory limit for this app's own container; 0 when the
	// compose file sets none (the installer picks a default).
	MemoryMB int
	// Services are the app's helper containers, installed, started,
	// stopped and removed together with it.
	Services []Service
	// Elevated is what the main container asks for beyond a plain one.
	Elevated compose.Elevated
	// Notes are things worth knowing from reading the compose file.
	Notes []compose.Issue
	// Custom: pasted by an admin, not in the curated list.
	Custom bool
	// File is the compose file this was built from; Values the form values
	// applied to it.
	File   *compose.File
	Values map[string]string
}

// Categories, in the fixed order the frontend shows them.
const (
	CategoryNetwork   = "network"
	CategoryCloud     = "cloud"
	CategoryHome      = "home"
	CategoryDownloads = "downloads"
	CategoryOther     = "other"
)

//go:embed apps/*.yml
var appFiles embed.FS

// order is the order the curated apps are shown in (within a category).
var order = []string{"adguardhome", "pihole", "nextcloud", "vaultwarden", "syncthing", "homeassistant", "qbittorrent"}

// Apps is the curated list with every field at its default — see the
// package doc for why it stays short and who maintains it.
var Apps = mustLoad()

func mustLoad() []App {
	var out []App
	for _, id := range order {
		data, err := appFiles.ReadFile("apps/" + id + ".yml")
		if err != nil {
			panic("appcatalog: " + err.Error())
		}
		f, err := compose.Parse(data)
		if err != nil {
			panic("appcatalog: apps/" + id + ".yml: " + err.Error())
		}
		app, err := Resolve(id, f, nil, false)
		if err != nil {
			panic("appcatalog: apps/" + id + ".yml: " + err.Error())
		}
		out = append(out, app)
	}
	return out
}

// ByID returns the curated entry with that id, and whether it exists.
func ByID(id string) (App, bool) {
	for _, a := range Apps {
		if a.ID == id {
			return a, true
		}
	}
	return App{}, false
}

// FileIDs lists the ids of the embedded files (tests check it against the
// list order).
func FileIDs() []string {
	entries, _ := appFiles.ReadDir("apps")
	var ids []string
	for _, e := range entries {
		ids = append(ids, strings.TrimSuffix(e.Name(), ".yml"))
	}
	sort.Strings(ids)
	return ids
}

var serviceNameChars = regexp.MustCompile(`[^a-z0-9]+`)

// helperName turns a compose service name into a helper container's role:
// lowercase letters and digits, at most 20.
func helperName(s string) string {
	n := serviceNameChars.ReplaceAllString(strings.ToLower(s), "")
	if len(n) > 20 {
		n = n[:20]
	}
	return n
}

func archesOf(in []string) []dockerhub.Arch {
	var out []dockerhub.Arch
	for _, s := range in {
		a, v, _ := strings.Cut(s, "/")
		out = append(out, dockerhub.Arch{Architecture: a, Variant: v})
	}
	return out
}

func envOf(in []compose.Pair) map[string]string {
	if len(in) == 0 {
		return nil
	}
	m := make(map[string]string, len(in))
	for _, p := range in {
		m[p.Key] = p.Value
	}
	return m
}

func portsOf(in []compose.Port, skip func(compose.Port) bool) []Port {
	var out []Port
	for _, p := range in {
		if skip != nil && skip(p) {
			continue
		}
		out = append(out, Port{Port: p.Port, UDP: p.UDP, Local: p.Local})
	}
	return out
}

// Resolve builds an App from a compose file and the form's values (a name
// → value map; a missing or empty one means the file's default). id is the
// app's id: the file name for a curated app, chosen by the installer for a
// custom one.
func Resolve(id string, f *compose.File, values map[string]string, custom bool) (App, error) {
	r, err := f.Resolve(values)
	if err != nil {
		return App{}, err
	}
	var main *compose.RService
	for i := range r.Services {
		if r.Services[i].Name == r.Main {
			main = &r.Services[i]
		}
	}
	app := App{
		ID: id, Name: f.Ext.Name, Category: f.Ext.Category, Icon: f.Ext.Icon,
		DescriptionText: f.Ext.Description, NoteText: f.Ext.Note,
		Description: f.Ext.Description.Get("en"), Note: f.Ext.Note.Get("en"),
		ContainerPort: r.WebPort, WebPath: r.WebPath, LargeConfig: f.Ext.LargeConfig,
		Image: main.Image, Tag: main.Tag, Arches: archesOf(main.Arches),
		Env: envOf(main.Env), Cmd: main.Command, MemoryMB: main.MemoryMB,
		Mounts: main.Volumes, Elevated: main.Elevated,
		Notes: r.Notes, Custom: custom, File: f, Values: values,
	}
	if app.Name == "" {
		app.Name = f.Name
	}
	if app.Name == "" {
		app.Name = path.Base(main.Image)
	}
	if app.Category == "" {
		app.Category = CategoryOther
	}
	if custom {
		app.Category = CategoryOther
	}
	if r.WebPort != 0 {
		app.PortVar = webPortVar(f)
	}
	for _, v := range main.Volumes {
		if v.Own() && app.ConfigPath == "" {
			app.ConfigPath = v.Target
		}
		if v.Name == "data" && app.DataPath == "" {
			app.DataPath = v.Target
		}
	}
	webPort := func(p compose.Port) bool { return p.Port == r.WebPort && !p.UDP && !p.Local }
	app.Ports = portsOf(main.Ports, webPort)

	names := map[string]bool{}
	for _, v := range main.Volumes {
		if v.Name != "" {
			names[v.Name] = true
		}
	}
	for _, s := range r.Services {
		if s.Name == r.Main {
			continue
		}
		hn := helperName(s.Name)
		if hn == "" || hn == "secrets" || names[hn] {
			return App{}, compose.Issues{{Code: "service_name", Service: s.Name, Detail: s.Name,
				Message: fmt.Sprintf("the service name %q can't be used as a helper container name (taken or empty after removing punctuation)", s.Name)}}
		}
		for _, v := range s.Volumes {
			if v.Name != "" {
				if helperName(v.Name) == hn {
					return App{}, compose.Issues{{Code: "volume_invalid", Service: s.Name, Detail: v.Name,
						Message: fmt.Sprintf("the folder name %q clashes with the service %q", v.Name, s.Name)}}
				}
				names[v.Name] = true
			}
		}
		names[hn] = true
		sv := Service{
			Name: hn, Image: s.Image, Tag: s.Tag, Arches: archesOf(s.Arches),
			Env: envOf(s.Env), Cmd: s.Command, Ports: portsOf(s.Ports, nil), MemoryMB: s.MemoryMB,
			Mounts: s.Volumes, Elevated: s.Elevated,
		}
		for _, v := range s.Volumes {
			if v.Own() {
				sv.ConfigPath = v.Target
				break
			}
		}
		app.Services = append(app.Services, sv)
	}
	// Folder names of the app's shared folders must not collide with a
	// helper's role either (both become <id>.<name> on disk).
	for _, sv := range app.Services {
		for _, v := range app.Mounts {
			if v.Name == sv.Name {
				return App{}, compose.Issues{{Code: "volume_invalid", Service: sv.Name, Detail: v.Name,
					Message: fmt.Sprintf("the folder name %q clashes with the service %q", v.Name, sv.Name)}}
			}
		}
	}
	return app, nil
}

// webPortVar is the first variable the web port is written with.
func webPortVar(f *compose.File) string {
	for _, fl := range f.Fields("en") {
		if fl.WebPort {
			return fl.Name
		}
	}
	return ""
}

// Fields are the install form's inputs, labels in lang.
func (a App) Fields(lang string) []compose.Field {
	if a.File == nil {
		return nil
	}
	return a.File.Fields(lang)
}

// WithValues rebuilds a with form values applied (name → value).
func (a App) WithValues(values map[string]string) (App, error) {
	return Resolve(a.ID, a.File, values, a.Custom)
}

// Localized returns the description and note in lang (English as fallback).
func (a App) Localized(lang string) (description, note string) {
	return a.DescriptionText.Get(lang), a.NoteText.Get(lang)
}

// PortEditable reports whether the web port can be moved at install.
func (a App) PortEditable() bool { return a.ContainerPort != 0 && a.PortVar != "" }

// WithPort returns a copy of a whose web port is port: the web port field
// set. The original (shared catalog data) is never changed.
func (a App) WithPort(port int) App {
	if !a.PortEditable() {
		return a
	}
	values := map[string]string{}
	for k, v := range a.Values {
		values[k] = v
	}
	values[a.PortVar] = strconv.Itoa(port)
	moved, err := a.WithValues(values)
	if err != nil {
		return a
	}
	return moved
}

// SupportsArch reports whether every container of a was verified for box
// — dockerhub.Supports against the curated Arches, rather than a fresh
// API call. A container without recorded architectures (a custom app's)
// is not known to be unsupported; see ArchUnknown.
func (a App) SupportsArch(box dockerhub.Arch) bool {
	if len(a.Arches) > 0 && !dockerhub.Supports(a.Arches, box) {
		return false
	}
	for _, s := range a.Services {
		if len(s.Arches) > 0 && !dockerhub.Supports(s.Arches, box) {
			return false
		}
	}
	return true
}

// ArchUnknown reports whether any container has no recorded architectures.
func (a App) ArchUnknown() bool {
	if len(a.Arches) == 0 {
		return true
	}
	for _, s := range a.Services {
		if len(s.Arches) == 0 {
			return true
		}
	}
	return false
}

// ImageRef is Image:Tag, with "latest" for an empty Tag.
func ImageRef(image, tag string) string {
	if tag == "" {
		tag = "latest"
	}
	return image + ":" + tag
}

// TotalMemoryMB is the memory limit of all of a's containers together.
func (a App) TotalMemoryMB() int {
	total := a.MemoryMB
	for _, s := range a.Services {
		total += s.MemoryMB
	}
	return total
}

var secretRef = regexp.MustCompile(`^\{\{secret:([a-z0-9]+)\}\}$`)

// SecretNames lists the distinct "{{secret:<name>}}" names a's containers
// use, sorted.
func (a App) SecretNames() []string {
	seen := map[string]bool{}
	collect := func(env map[string]string) {
		for _, v := range env {
			if m := secretRef.FindStringSubmatch(v); m != nil {
				seen[m[1]] = true
			}
		}
	}
	collect(a.Env)
	for _, s := range a.Services {
		collect(s.Env)
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ExpandEnv returns env with every "{{secret:<name>}}" value replaced by
// secrets[name].
func ExpandEnv(env map[string]string, secrets map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		if m := secretRef.FindStringSubmatch(v); m != nil {
			v = secrets[m[1]]
		}
		out[k] = v
	}
	return out
}
