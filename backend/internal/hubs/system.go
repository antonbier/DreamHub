package hubs

import (
	"net/http"
	"strconv"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// System is SystemHub. For now: the change log.
type System struct {
	Store *store.Store
}

// Mount registers SystemHub's routes.
func (h *System) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/changes", h.changes)
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
