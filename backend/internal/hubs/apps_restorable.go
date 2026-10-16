package hubs

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
)

// This file is AppsHub's App15 (docs/APPSHUB_PLAN.md): the app list in a
// backup, and bringing apps back from it. A backup can carry, besides the
// apps' folders (apps_backup.go), what was chosen for each app — its
// definition (<root>/<id>.app.json, App13) and its generated passwords
// (<id>.secrets.json). Restoring writes those files next to the folders;
// an app whose files are there but whose containers are not is "restorable"
// and is installed again through the ordinary install plan. Removing an app
// leaves the same files behind, so the same list also offers an app that was
// removed on purpose.

const appListSectionID = "appList"

// appListSection is the backup section that carries the app list.
type appListSection struct{ h *Apps }

// AppList is the backup section for the app list.
func (h *Apps) AppList() BackupSection { return appListSection{h} }

func (appListSection) ID() string { return appListSectionID }

// appListEntry is one app's entry in the section's payload.
type appListEntry struct {
	Root       string          `json:"root"`
	Definition json.RawMessage `json:"definition"`
	// Secrets are the passwords DreamHub generated for the app: its database
	// was set up with them, so the restored folders need the same ones.
	Secrets json.RawMessage `json:"secrets,omitempty"`
}

// listed gathers what the backup carries per installed app. An app installed
// before its choices were stored is listed with the catalog defaults; a
// custom app without stored choices cannot be rebuilt and is left out.
func (s appListSection) listed(ctx context.Context) (map[string]appListEntry, map[string]string, error) {
	h := s.h
	entries, names := map[string]appListEntry{}, map[string]string{}
	if h.Docker == nil {
		return entries, names, nil
	}
	containers, err := h.Docker.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, g := range groupApps(containers) {
		root := g.root(h.StorageRoot)
		d, ok := h.loadDefinition(root, g.appID)
		if !ok {
			if _, curated := appcatalog.ByID(g.appID); !curated {
				continue
			}
			d = definitionOf(installBody{AppID: g.appID})
		}
		def, err := json.Marshal(d)
		if err != nil {
			continue
		}
		e := appListEntry{Root: root, Definition: def}
		if sec, err := h.configFS().ReadFile(secretsFile(root, g.appID)); err == nil && json.Valid(sec) {
			e.Secrets = sec
		}
		entries[g.appID] = e
		names[g.appID] = groupName(g, g.all()[0].Image)
	}
	return entries, names, nil
}

func (s appListSection) Items(ctx context.Context) ([]BackupItem, error) {
	entries, names, err := s.listed(ctx)
	if err != nil {
		return []BackupItem{}, nil // like the folders: a degraded Docker means an empty section
	}
	out := []BackupItem{}
	for id, e := range entries {
		out = append(out, BackupItem{ID: id, Name: names[id], SizeBytes: int64(len(e.Definition) + len(e.Secrets)), DefaultIncluded: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s appListSection) Export(ctx context.Context, itemIDs []string) (json.RawMessage, error) {
	entries, _, err := s.listed(ctx)
	if err != nil {
		return nil, err
	}
	if itemIDs != nil {
		want := map[string]bool{}
		for _, id := range itemIDs {
			want[id] = true
		}
		for id := range entries {
			if !want[id] {
				delete(entries, id)
			}
		}
	}
	return json.Marshal(entries)
}

// Restore writes the stored files for every app that is not installed now;
// one that is installed keeps what it has. The ids come from an uploaded
// file: they must look like app ids, and the root is only used when it is
// one of the places apps live — otherwise the default root takes its place.
func (s appListSection) Restore(ctx context.Context, data json.RawMessage) error {
	h := s.h
	var in map[string]appListEntry
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	var containers []dockerd.Container
	if h.Docker != nil {
		var err error
		if containers, err = h.Docker.List(ctx); err != nil {
			return err
		}
	}
	installed := map[string]bool{}
	for _, g := range groupApps(containers) {
		installed[g.appID] = true
	}
	known := h.restoreRoots(ctx, containers)
	for id, e := range in {
		if !validAppID.MatchString(id) || installed[id] {
			continue
		}
		var d appDefinition
		if json.Unmarshal(e.Definition, &d) != nil || d.Version != 1 {
			continue
		}
		root := h.defaultRoot(ctx)
		if known[e.Root] {
			root = e.Root
		}
		h.saveDefinition(root, id, d)
		if len(e.Secrets) > 0 && json.Valid(e.Secrets) {
			if err := h.configFS().WriteFile(secretsFile(root, id), e.Secrets); err != nil {
				return err
			}
		}
	}
	return nil
}

// validAppID: the id of a curated app, or of a custom one ("my" + letters).
var validAppID = regexp.MustCompile(`^[a-z0-9]+$`)

// restoreRoots are the places an app of a backup may be put back.
func (h *Apps) restoreRoots(ctx context.Context, containers []dockerd.Container) map[string]bool {
	out := map[string]bool{}
	for _, r := range h.appRoots(ctx, containers) {
		out[r] = true
	}
	for _, c := range h.storageChoices(ctx) {
		out[c.Path] = true
	}
	return out
}

// --- GET /api/v1/apps/restorable ------------------------------------------------

type restorableApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Root is the storage it was in; Custom marks an app of the person's own.
	Root   string `json:"root"`
	Custom bool   `json:"custom,omitempty"`
}

// findRestorable lists the apps whose stored choices are on the box but
// which are not installed.
func (h *Apps) findRestorable(ctx context.Context, containers []dockerd.Container) []restorableApp {
	installed := map[string]bool{}
	for _, g := range groupApps(containers) {
		installed[g.appID] = true
	}
	roots := []string{}
	for r := range h.restoreRoots(ctx, containers) {
		roots = append(roots, r)
	}
	sort.Strings(roots)
	seen := map[string]bool{}
	out := []restorableApp{}
	for _, root := range roots {
		names, err := h.configFS().List(root)
		if err != nil {
			continue
		}
		for _, n := range names {
			id, ok := strings.CutSuffix(n, ".app.json")
			if !ok || installed[id] || seen[id] || !validAppID.MatchString(id) {
				continue
			}
			d, ok := h.loadDefinition(root, id)
			if !ok {
				continue
			}
			ra := restorableApp{ID: id, Root: root}
			if d.Custom != nil {
				ra.Custom, ra.Name = true, d.Custom.Name
			} else if a, ok := appcatalog.ByID(d.AppID); ok {
				ra.Name = a.Name
			} else {
				continue
			}
			seen[id] = true
			out = append(out, ra)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (h *Apps) restorable(w http.ResponseWriter, r *http.Request) {
	var containers []dockerd.Container
	if h.Docker != nil {
		var err error
		if containers, err = h.Docker.List(r.Context()); err != nil {
			writeDockerError(w, r, err, "could not look up the apps")
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"apps": h.findRestorable(r.Context(), containers)})
}

// planRestoreApp answers POST /api/v1/apps/restorable/{id}/plan: the install
// plan of a stored app, with what was chosen for it.
func (h *Apps) planRestoreApp(w http.ResponseWriter, r *http.Request) {
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
	id := r.PathValue("id")
	for _, ra := range h.findRestorable(ctx, containers) {
		if ra.ID != id {
			continue
		}
		d, _ := h.loadDefinition(ra.Root, id)
		body := d.body(id)
		body.Storage = ra.Root
		h.installPlan(w, r, body)
		return
	}
	httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no stored app with that id")
}
