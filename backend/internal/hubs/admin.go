package hubs

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/logging"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// Admin is AdminHub. For now: DreamHub's runtime log level. Log viewing
// (journald) follows. Every route requires the admin role.
type Admin struct {
	Store *store.Store
	Level *logging.Level
}

// Mount registers AdminHub's routes.
func (h *Admin) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/log-level", adminOnly(h.getLevel))
	mux.HandleFunc("PUT /api/v1/admin/log-level", adminOnly(h.putLevel))
}

// DefaultDebugMinutes is how long debug logging lasts unless specified.
const DefaultDebugMinutes = 60

func adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u, ok := httpx.User(r.Context()); !ok || u.Role != store.RoleAdmin {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "this requires the admin role")
			return
		}
		next(w, r)
	}
}

func (h *Admin) levelView() map[string]any {
	level, until := h.Level.Get()
	out := map[string]any{"level": level}
	if !until.IsZero() {
		out["until"] = until.Format(time.RFC3339)
	}
	return out
}

func (h *Admin) getLevel(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, h.levelView())
}

func (h *Admin) putLevel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Level   string `json:"level"`
		Minutes int    `json:"minutes"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"level": "debug"|"info", "minutes": 1-1440}`)
		return
	}
	switch body.Level {
	case "debug":
		if body.Minutes == 0 {
			body.Minutes = DefaultDebugMinutes
		}
		if body.Minutes < 1 || body.Minutes > 1440 {
			httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "minutes must be between 1 and 1440",
				map[string]httpx.FieldError{"minutes": {Code: "out_of_range", Params: map[string]any{"min": 1, "max": 1440}}})
			return
		}
		h.Level.SetDebug(time.Duration(body.Minutes) * time.Minute)
	case "info":
		h.Level.SetInfo()
	default:
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "level must be debug or info",
			map[string]httpx.FieldError{"level": {Code: "invalid_option", Params: map[string]any{"options": []string{"debug", "info"}}}})
		return
	}

	// DreamHub's own configuration: recorded, but no plan needed (it doesn't
	// touch recordings, timers or the system).
	u, _ := httpx.User(r.Context())
	view := h.levelView()
	detail, _ := json.Marshal(view)
	_ = h.Store.AppendChange(r.Context(), store.ChangeEntry{
		UserID: u.ID, Action: "admin.logLevel", Summary: "Set log level to " + body.Level, Detail: detail,
	})
	httpx.WriteJSON(w, http.StatusOK, view)
}
