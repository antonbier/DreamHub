package hubs

import (
	"context"
	"encoding/json"
	"path"
	"regexp"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/configfs"
)

// This file is AppsHub's App6 (docs/APPSHUB_PLAN.md, "Backup is
// granular, built as a general mechanism from the start"): Apps
// implements System's BackupSection interface (system.go), so the
// existing Backup & Restore screen grows a generic, opt-in "apps"
// section rather than AppsHub inventing its own separate backup screen.
// Scoped exactly to the plan's own decision: "default per app:
// configuration only, never bulk volume data automatically" — every
// installed curated app's config directory (its HostDir, App4) is
// backed up whole; there is no separate "large volume" to opt into yet
// because no curated app has a second, bigger bind mount today (only
// App4's single ConfigPath one). If one ever does, that's the point
// BackupItem's own per-item size and DefaultIncluded earn their keep —
// the mechanism already supports it, nothing here would need to change.

const appsBackupSectionID = "apps"

func (h *Apps) ID() string { return appsBackupSectionID }

// validBackupItem: an item id is an app id, optionally with one
// ".<service>" or ".data" suffix — exactly the name of its directory
// under StorageRoot. Checked on restore too, where the ids come from an
// uploaded file: nothing outside StorageRoot is ever written.
var validBackupItem = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9]+)?$`)

func (h *Apps) itemDir(id string) string { return path.Join(h.StorageRoot, id) }

// backupItems lists every storage directory of the installed apps: the
// app's own config (in by default, unless it holds the app's code too —
// LargeConfig), each helper's (a database: in by default), and the bulk
// data directory (never in by default).
func (h *Apps) backupItems(ctx context.Context) ([]BackupItem, error) {
	if h.Docker == nil {
		return []BackupItem{}, nil
	}
	containers, err := h.Docker.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []BackupItem
	add := func(id, name string, def bool) {
		size, _ := h.configFS().Size(h.itemDir(id))
		out = append(out, BackupItem{ID: id, Name: name, SizeBytes: size, DefaultIncluded: def})
	}
	for _, g := range groupApps(containers) {
		all := g.all()
		app, curated := appcatalog.ByID(g.appID)
		name := appName(g.appID, all[0].Image)
		add(g.appID, name, !curated || !app.LargeConfig)
		for _, c := range g.services {
			add(g.appID+"."+c.Service(), name+" ("+c.Service()+")", true)
		}
		if curated && app.DataPath != "" {
			add(g.appID+".data", name+" (data)", false)
		}
	}
	if out == nil {
		out = []BackupItem{}
	}
	return out, nil
}

// Items lists every installed app's storage directories with their real
// size on disk — "always visible" before the admin ever downloads
// anything (docs/APPSHUB_PLAN.md's running "sensible default, always
// visible, always changeable" rule).
func (h *Apps) Items(ctx context.Context) ([]BackupItem, error) {
	items, err := h.backupItems(ctx)
	if err != nil {
		return []BackupItem{}, nil // a degraded Docker connector means an empty section, not a failed backup
	}
	return items, nil
}

func (h *Apps) configFS() configfs.FS {
	if h.ConfigFS != nil {
		return h.ConfigFS
	}
	return configfs.OS{}
}

// appBackup is one item's entry inside the "apps" section's payload —
// Archive is a base64 gzip+tar of its whole directory (Go's JSON encoding
// already base64-encodes a []byte field).
type appBackup struct {
	Archive []byte `json:"archive"`
}

// Export archives exactly itemIDs' directories, or every default-included
// one if itemIDs is nil.
func (h *Apps) Export(ctx context.Context, itemIDs []string) (json.RawMessage, error) {
	items, err := h.backupItems(ctx)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, id := range itemIDs {
		want[id] = true
	}
	out := map[string]appBackup{}
	for _, it := range items {
		if itemIDs == nil && !it.DefaultIncluded || itemIDs != nil && !want[it.ID] {
			continue
		}
		archive, err := h.configFS().Archive(h.itemDir(it.ID))
		if err != nil {
			return nil, err
		}
		out[it.ID] = appBackup{Archive: archive}
	}
	return json.Marshal(out)
}

// Restore writes each archived directory back — restoring an app whose
// own container isn't installed any more just recreates its directories
// for when it's reinstalled; Create always mounts them as they are.
func (h *Apps) Restore(ctx context.Context, data json.RawMessage) error {
	var in map[string]appBackup
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	for id, b := range in {
		if !validBackupItem.MatchString(id) {
			continue
		}
		if err := h.configFS().Restore(h.itemDir(id), b.Archive); err != nil {
			return err
		}
	}
	return nil
}
