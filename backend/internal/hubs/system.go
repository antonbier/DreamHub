package hubs

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/dashboard"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// System is SystemHub: the change log, and backup/restore of DreamHub's own
// state (docs/V1_PLAN.md, WP5). Device info and hub diagnostics are core
// endpoints (GET /api/v1/deviceinfo, /api/v1/hubs) the frontend already
// calls directly — nothing hub-specific to add for those.
type System struct {
	Store     *store.Store
	Settings  *settings.Service
	Dashboard *dashboard.Service
	Plans     *plan.Service
	Now       func() time.Time
	// SelfTest is the box self-test (selftest.go); nil leaves it out.
	SelfTest *SelfTest
}

func (h *System) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// Mount registers SystemHub's routes.
func (h *System) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/changes", h.changes)
	mux.HandleFunc("GET /api/v1/system/backup", h.backup)
	mux.HandleFunc("POST /api/v1/system/backup/restore", h.planRestore)
	if h.SelfTest != nil {
		mux.HandleFunc("GET /api/v1/system/selftest", adminOnly(h.SelfTest.handleLast))
		mux.HandleFunc("POST /api/v1/system/selftest", adminOnly(h.SelfTest.handleRun))
		mux.HandleFunc("GET /api/v1/system/selftest/download", adminOnly(h.SelfTest.handleDownload))
	}
}

type changeView struct {
	ID      int64  `json:"id"`
	At      string `json:"at"` // UTC, RFC 3339
	UserID  int64  `json:"userId,omitempty"`
	Action  string `json:"action"`
	Summary string `json:"summary"`
	Detail  any    `json:"detail"`
}

func (h *System) changes(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 500")
			return
		}
		limit = n
	}
	entries, err := h.Store.Changes(r.Context(), limit)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load the change log")
		return
	}
	out := make([]changeView, 0, len(entries))
	for _, e := range entries {
		out = append(out, changeView{
			ID: e.ID, At: e.At.UTC().Format("2006-01-02T15:04:05Z"), UserID: e.UserID,
			Action: e.Action, Summary: e.Summary, Detail: e.Detail,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"changes": out})
}

// backupVersion guards restoring a file from an incompatible future format.
const backupVersion = 1

// backupFile is DreamHub's own state as one downloadable file: every
// customized, non-secret setting plus the dashboard layout. Secrets and
// passwords (API keys, the admin password) are never included — restoring
// them from a file would mean the file itself could unlock the box, and
// they're easy to re-enter once (docs/V1_PLAN.md, WP5).
type backupFile struct {
	Version    int              `json:"version"`
	ExportedAt string           `json:"exportedAt"`
	Settings   map[string]any   `json:"settings"`
	Dashboard  dashboard.Layout `json:"dashboard"`
}

func (h *System) backup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, _ := httpx.User(ctx)
	sections, err := h.Settings.View(ctx, u)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read settings")
		return
	}
	out := backupFile{Version: backupVersion, ExportedAt: h.now().UTC().Format(time.RFC3339), Settings: map[string]any{}}
	for _, sec := range sections {
		for _, f := range sec.Fields {
			if f.Type == settings.TypeSecret || f.Type == settings.TypePassword || !f.HasValue {
				continue
			}
			out.Settings[f.Key] = f.Value
		}
	}
	layout, err := h.Dashboard.Get(ctx, u)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read the dashboard")
		return
	}
	out.Dashboard = layout

	w.Header().Set("Content-Disposition", `attachment; filename="dreamhub-backup.json"`)
	httpx.WriteJSON(w, http.StatusOK, out)
}

// sameJSON compares two decoded values by their JSON encoding, so a stored
// int64 and a float64 decoded from the uploaded file's JSON numbers compare
// equal when they represent the same value.
func sameJSON(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

// planRestore previews restoring a backup file: every setting and the
// dashboard layout it would actually change, following PRINCIPLES.md's
// preview-and-confirm — applying is the normal POST /api/v1/plans/{id}/apply.
func (h *System) planRestore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, _ := httpx.User(ctx)
	var in backupFile
	if err := httpx.DecodeJSON(r, &in); err != nil || in.Version != backupVersion {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_backup", "not a DreamHub backup file, or from an incompatible version")
		return
	}

	sections, err := h.Settings.View(ctx, u)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read settings")
		return
	}
	fieldType := map[string]settings.Type{}
	currentValue := map[string]any{}
	for _, sec := range sections {
		for _, f := range sec.Fields {
			fieldType[f.Key] = f.Type
			currentValue[f.Key] = f.Value
		}
	}

	raw := map[string]json.RawMessage{}
	var changes []plan.Change
	for key, val := range in.Settings {
		t, known := fieldType[key]
		if !known || t == settings.TypeSecret || t == settings.TypePassword {
			continue // unknown, or a secret the backup never carried
		}
		if enc, err := json.Marshal(val); err == nil {
			raw[key] = enc
		}
		if !sameJSON(currentValue[key], val) {
			changes = append(changes, plan.Change{Type: "settings." + key, Before: currentValue[key], After: val})
		}
	}

	currentLayout, err := h.Dashboard.Get(ctx, u)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read the dashboard")
		return
	}
	dashboardChanged := !sameJSON(currentLayout, in.Dashboard)
	if dashboardChanged {
		changes = append(changes, plan.Change{Type: "dashboard", Before: currentLayout, After: in.Dashboard})
	}

	pl := h.Plans.Create(u, plan.Draft{
		Action:  "system.restore",
		Changes: changes,
		Summary: "Restored DreamHub state from a backup file",
		Detail:  map[string]any{"exportedAt": in.ExportedAt},
		Execute: func(ctx context.Context) error {
			if len(raw) > 0 {
				if err := h.Settings.Update(ctx, u, raw); err != nil {
					if ve, ok := settings.IsValidation(err); ok {
						return &plan.Failure{Status: http.StatusBadRequest, Code: "restore_invalid",
							Message: "the backup contains settings that no longer validate", Details: ve.Fields}
					}
					return err
				}
			}
			if dashboardChanged {
				if _, err := h.Dashboard.Save(ctx, u, in.Dashboard); err != nil {
					return err
				}
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}
