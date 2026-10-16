package hubs

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/perm"
)

// This file is App15's "save on the box" (docs/APPSHUB_PLAN.md): the same
// backup as the download, written to BackupDir (the recording disk by
// default) instead of through the browser — an app's data can be far too big
// to carry through a phone. A stored file is listed, downloaded, restored
// from (the normal preview and confirm) or deleted; nothing else.

// storedBackupName is the only shape of file name the endpoints accept, so a
// request can never name a path.
var storedBackupName = regexp.MustCompile(`^dreamhub-backup-[0-9]{8}-[0-9]{6}\.json$`)

type storedBackup struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	CreatedAt string `json:"createdAt"`
}

// saveBackup answers POST /api/v1/system/backup/save[?sections=…]: it builds
// the backup like the download and keeps it in BackupDir.
func (h *System) saveBackup(w http.ResponseWriter, r *http.Request) {
	out, ok := h.build(w, r)
	if !ok {
		return
	}
	data, err := json.Marshal(out)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not write the backup")
		return
	}
	name := "dreamhub-backup-" + h.now().UTC().Format("20060102-150405") + ".json"
	if err := os.MkdirAll(h.BackupDir, 0o700); err != nil {
		httpx.WriteError(w, r, http.StatusConflict, "backup_dir_unavailable", "the place for backups on the box isn't available")
		return
	}
	// Written under another name first: a full disk leaves no half file.
	tmp, err := os.CreateTemp(h.BackupDir, ".backup-*")
	if err == nil {
		_, err = tmp.Write(data)
		if cerr := tmp.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = os.Rename(tmp.Name(), filepath.Join(h.BackupDir, name))
		}
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInsufficientStorage, "backup_write_failed", "the backup could not be written (is the disk full?)")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"file": storedBackup{Name: name, SizeBytes: int64(len(data)), CreatedAt: out.ExportedAt}})
}

// listBackups answers GET /api/v1/system/backup/files, newest first.
func (h *System) listBackups(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(h.BackupDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read the backup folder")
		return
	}
	out := []storedBackup{}
	for _, e := range entries {
		if !e.Type().IsRegular() || !storedBackupName.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, storedBackup{Name: e.Name(), SizeBytes: info.Size(), CreatedAt: info.ModTime().UTC().Format(time.RFC3339)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"dir": h.BackupDir, "files": out})
}

// storedPath is the file a request names, or "" (answered) when it is not a
// stored backup.
func (h *System) storedPath(w http.ResponseWriter, r *http.Request) string {
	name := r.PathValue("name")
	if !storedBackupName.MatchString(name) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such stored backup")
		return ""
	}
	return filepath.Join(h.BackupDir, name)
}

func (h *System) downloadBackup(w http.ResponseWriter, r *http.Request) {
	p := h.storedPath(w, r)
	if p == "" {
		return
	}
	if _, err := os.Stat(p); err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such stored backup")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("name")+`"`)
	w.Header().Set("Content-Type", "application/json")
	http.ServeFile(w, r, p)
}

func (h *System) deleteBackup(w http.ResponseWriter, r *http.Request) {
	p := h.storedPath(w, r)
	if p == "" {
		return
	}
	if err := os.Remove(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such stored backup")
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not delete the backup")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// planRestoreStored previews restoring a stored file.
func (h *System) planRestoreStored(w http.ResponseWriter, r *http.Request) {
	p := h.storedPath(w, r)
	if p == "" {
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such stored backup")
		return
	}
	var in backupFile
	if json.Unmarshal(data, &in) != nil || in.Version != backupVersion {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_backup", "not a DreamHub backup file, or from an incompatible version")
		return
	}
	h.restorePlan(w, r, in)
}

// Perms: a backup carries the apps' folders and passwords, so reading,
// saving and restoring one is for the admin; the change log stays as it was.
func (h *System) Perms() map[string]perm.Perm {
	m := map[string]perm.Perm{
		"GET /api/v1/system/backup":          perm.System,
		"GET /api/v1/system/backup/sections": perm.System,
	}
	if h.BackupDir != "" {
		m["GET /api/v1/system/backup/files"] = perm.System
		m["GET /api/v1/system/backup/files/{name}"] = perm.System
	}
	return m
}
