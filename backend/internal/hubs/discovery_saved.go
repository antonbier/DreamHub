package hubs

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/antonbier/DreamHub/backend/internal/ai"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// The day's recommendations are kept per user (store: discovery_picks):
// generating costs an AI call, so leaving the page, reloading or opening
// DreamHub on another device must not lose them. GET answers the last
// set, without the broadcasts that have ended since, each flagged when a
// timer now covers it (one created from here, or any other). A new
// generation replaces it.

type savedPicks struct {
	Bouquet         string              `json:"bouquet"`
	GeneratedAt     int64               `json:"generatedAt"` // Unix seconds
	Recommendations []ai.Recommendation `json:"recommendations"`
	Taste           *tasteCountsView    `json:"taste,omitempty"`
}

// pickView is a recommendation as served: with whether a timer covers it.
type pickView struct {
	ai.Recommendation
	HasTimer bool `json:"hasTimer"`
}

func (h *Discovery) savePicks(ctx context.Context, userID int64, p savedPicks) {
	if b, err := json.Marshal(p); err == nil {
		_ = h.Store.PutDiscoveryPicks(context.WithoutCancel(ctx), userID, string(b), p.GeneratedAt)
	}
}

// views drops ended broadcasts and flags covered ones.
func (h *Discovery) views(ctx context.Context, recs []ai.Recommendation) []pickView {
	timers, _ := h.WC.TimerList(ctx) // no flags is better than no picks
	now := h.Now().Unix()
	out := make([]pickView, 0, len(recs))
	for _, r := range recs {
		if r.Start+r.Duration <= now {
			continue
		}
		e := webcontrol.EPGEvent{ServiceReference: r.ServiceReference, Start: r.Start, Duration: r.Duration}
		out = append(out, pickView{Recommendation: r, HasTimer: coveredByTimer(e, timers)})
	}
	return out
}

func (h *Discovery) saved(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	data, _, ok, err := h.Store.DiscoveryPicks(ctx, user.ID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read saved recommendations")
		return
	}
	var p savedPicks
	if !ok || json.Unmarshal([]byte(data), &p) != nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"saved": nil})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"saved": map[string]any{
		"bouquet": p.Bouquet, "generatedAt": p.GeneratedAt, "taste": p.Taste,
		"recommendations": h.views(ctx, p.Recommendations),
	}})
}
