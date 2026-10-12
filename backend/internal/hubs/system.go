package hubs

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/dashboard"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// System is SystemHub: the change log, and backup/restore of DreamHub's own
// state (docs/archive/V1_PLAN.md, WP5). Device info and hub diagnostics are core
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
	// Sections are the optional, additional parts a backup can carry
	// beyond settings and the dashboard layout (docs/APPSHUB_PLAN.md,
	// App6, "backup is a general mechanism from the start") — AppsHub's
	// installed app configs is the first and so far only one. Nil or
	// empty: backups behave exactly as before App6.
	Sections []BackupSection
}

// BackupSection is one extra, opt-in part of a backup file — a hub that
// has something worth backing up beyond settings implements this and is
// wired into System.Sections (app.go), rather than System growing
// hub-specific knowledge for each one.
type BackupSection interface {
	// ID is this section's own key in backupFile.Sections, and the
	// ?sections= query value that includes it.
	ID() string
	// Items lists what this section could include right now, each with
	// its own real size, so an admin sees what they're about to
	// download before they do — the plan's own "sensible default,
	// always visible, always changeable" rule, applied here too.
	Items(ctx context.Context) ([]BackupItem, error)
	// Export returns this section's payload for exactly itemIDs, or for
	// every default-included item if itemIDs is nil (the whole-section
	// "?sections=apps" case the UI uses today — see the plan's own
	// "default per app: configuration only" decision, which Items'
	// DefaultIncluded already reflects).
	Export(ctx context.Context, itemIDs []string) (json.RawMessage, error)
	// Restore writes back a previously exported payload. Called from
	// inside the overall restore plan's own Execute, strictly after
	// settings have already been written — a section can assume any
	// setting it might care about is already in its restored state.
	Restore(ctx context.Context, data json.RawMessage) error
}

// BackupItem is one thing a BackupSection could include.
type BackupItem struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	SizeBytes       int64  `json:"sizeBytes"`
	DefaultIncluded bool   `json:"defaultIncluded"`
}

func (h *System) section(id string) (BackupSection, bool) {
	for _, s := range h.Sections {
		if s.ID() == id {
			return s, true
		}
	}
	return nil, false
}

func (h *System) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// Mount registers SystemHub's routes.
func (h *System) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/changes", h.changes)
	mux.HandleFunc("GET /api/v1/system/backup", h.backup)
	mux.HandleFunc("GET /api/v1/system/backup/sections", h.backupSections)
	mux.HandleFunc("POST /api/v1/system/backup/restore", h.planRestore)
	if h.SelfTest != nil {
		mux.HandleFunc("GET /api/v1/system/selftest", adminOnly(h.SelfTest.handleLast))
		mux.HandleFunc("POST /api/v1/system/selftest", adminOnly(h.SelfTest.handleRun))
		mux.HandleFunc("GET /api/v1/system/selftest/download", adminOnly(h.SelfTest.handleDownload))
	}
}

type changeView struct {
	ID     int64  `json:"id"`
	At     string `json:"at"` // UTC, RFC 3339
	UserID int64  `json:"userId,omitempty"`
	// UserName is the profile's name now; empty for the system's own
	// entries and for deleted profiles.
	UserName string `json:"userName,omitempty"`
	Action   string `json:"action"`
	Summary  string `json:"summary"`
	Detail   any    `json:"detail"`
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
	// ?user=<profile id>, or 0 for what DreamHub noticed itself.
	user := int64(-1)
	if v := r.URL.Query().Get("user"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_query", "user must be a profile id, or 0 for the system")
			return
		}
		user = n
	}
	entries, err := h.Store.ChangesBy(r.Context(), limit, user)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load the change log")
		return
	}
	names := map[int64]string{}
	if users, err := h.Store.Users(r.Context()); err == nil {
		for _, u := range users {
			names[u.ID] = u.Name
		}
	}
	out := make([]changeView, 0, len(entries))
	for _, e := range entries {
		out = append(out, changeView{
			ID: e.ID, At: e.At.UTC().Format("2006-01-02T15:04:05Z"), UserID: e.UserID, UserName: names[e.UserID],
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
// they're easy to re-enter once (docs/archive/V1_PLAN.md, WP5).
type backupFile struct {
	Version    int              `json:"version"`
	ExportedAt string           `json:"exportedAt"`
	Settings   map[string]any   `json:"settings"`
	Dashboard  dashboard.Layout `json:"dashboard"`
	// Sections is App6's own addition — keyed by BackupSection.ID,
	// omitted entirely for a plain settings-only backup (an older file
	// without it restores exactly as it always did).
	Sections map[string]json.RawMessage `json:"sections,omitempty"`
}

// backupSectionsView answers GET /api/v1/system/backup/sections: every
// registered section's own items, so the UI can show what's available
// and its size before the admin decides to include it.
func (h *System) backupSections(w http.ResponseWriter, r *http.Request) {
	type sectionView struct {
		ID    string       `json:"id"`
		Items []BackupItem `json:"items"`
	}
	out := make([]sectionView, 0, len(h.Sections))
	for _, s := range h.Sections {
		items, err := s.Items(r.Context())
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not list "+s.ID()+"'s backup items")
			return
		}
		out = append(out, sectionView{ID: s.ID(), Items: items})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"sections": out})
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

	// ?sections=apps,otherSection — each named, registered section's own
	// default-included items (App6). An unknown name is ignored rather
	// than refused: a backup download must never fail outright over one
	// bad query value.
	if q := r.URL.Query().Get("sections"); q != "" {
		out.Sections = map[string]json.RawMessage{}
		for _, id := range strings.Split(q, ",") {
			sec, ok := h.section(strings.TrimSpace(id))
			if !ok {
				continue
			}
			data, err := sec.Export(ctx, nil)
			if err != nil {
				httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not export "+sec.ID())
				return
			}
			out.Sections[sec.ID()] = data
		}
	}

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

	// App6: a section present in the uploaded file but not registered
	// here (an older DreamHub, or the owning feature removed) is simply
	// skipped — restoring settings must never fail over it.
	type pendingSection struct {
		sec  BackupSection
		data json.RawMessage
	}
	var pendingSections []pendingSection
	for id, data := range in.Sections {
		sec, ok := h.section(id)
		if !ok {
			continue
		}
		pendingSections = append(pendingSections, pendingSection{sec: sec, data: data})
		changes = append(changes, plan.Change{Type: "backup.section." + id, After: id})
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
			// Sections restore last, after settings are already in
			// effect (BackupSection's own doc comment: a section can
			// assume any setting it cares about is already restored).
			for _, p := range pendingSections {
				if err := p.sec.Restore(ctx, p.data); err != nil {
					return &plan.Failure{Status: http.StatusBadGateway, Code: "section_restore_failed",
						Message: "restoring " + p.sec.ID() + " failed", Err: err}
				}
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}
