package hubs

import (
	"context"
	"encoding/json"
	"path"

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

// Items lists every installed curated app with its config directory's
// real size on disk — "always visible" before the admin ever downloads
// anything (docs/APPSHUB_PLAN.md's running "sensible default, always
// visible, always changeable" rule). A free-search install has no
// config either way (App4 is curated-only), so nothing is left out here
// that App4 could have produced.
func (h *Apps) Items(ctx context.Context) ([]BackupItem, error) {
	if h.Docker == nil {
		return nil, nil
	}
	containers, err := h.Docker.List(ctx)
	if err != nil {
		return nil, nil // a degraded Docker connector means an empty section, not a failed backup
	}
	out := make([]BackupItem, 0, len(containers))
	for _, c := range containers {
		appID := c.AppID()
		name := c.Image
		if a, ok := appcatalog.ByID(appID); ok {
			name = a.Name
		}
		size, _ := h.configFS().Size(h.configDir(appID))
		out = append(out, BackupItem{ID: appID, Name: name, SizeBytes: size, DefaultIncluded: true})
	}
	return out, nil
}

func (h *Apps) configFS() configfs.FS {
	if h.ConfigFS != nil {
		return h.ConfigFS
	}
	return configfs.OS{}
}

func (h *Apps) configDir(appID string) string {
	return path.Join(h.StorageRoot, appID)
}

// appBackup is one app's own entry inside the "apps" section's payload
// — Archive is a base64 gzip+tar of its whole config directory (Go's
// JSON encoding already base64-encodes a []byte field).
type appBackup struct {
	Archive []byte `json:"archive"`
}

// Export archives exactly itemIDs' config directories (every installed
// app's, if itemIDs is nil — "?sections=apps" in system.go, the whole-
// section case the UI uses today).
func (h *Apps) Export(ctx context.Context, itemIDs []string) (json.RawMessage, error) {
	if h.Docker == nil {
		return json.Marshal(map[string]appBackup{})
	}
	containers, err := h.Docker.List(ctx)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, id := range itemIDs {
		want[id] = true
	}
	out := map[string]appBackup{}
	for _, c := range containers {
		appID := c.AppID()
		if itemIDs != nil && !want[appID] {
			continue
		}
		archive, err := h.configFS().Archive(h.configDir(appID))
		if err != nil {
			return nil, err
		}
		out[appID] = appBackup{Archive: archive}
	}
	return json.Marshal(out)
}

// Restore writes each app's own archived config directory back —
// restoring an app whose own container isn't installed any more just
// recreates its config directory for when it's reinstalled; App4's
// Create always mounts HostDir as-is, without caring whether it was
// empty or already had content.
func (h *Apps) Restore(ctx context.Context, data json.RawMessage) error {
	var in map[string]appBackup
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	for appID, b := range in {
		if err := h.configFS().Restore(h.configDir(appID), b.Archive); err != nil {
			return err
		}
	}
	return nil
}
