package hubs

import (
	"net/http"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/ai"
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Discovery is DiscoveryHub: AI recommendations from a bouquet's EPG
// (ROADMAP.md). The AI only proposes — "create timer" is a plain frontend
// call to TimerHub's own POST /api/v1/timers/plan, not something this hub
// builds itself (CONCEPT.md: "When DiscoveryHub creates a timer, it calls
// TimerHub's API").
type Discovery struct {
	WC       webcontrol.Client
	AI       ai.Client
	Store    *store.Store
	Settings *settings.Service
	Now      func() time.Time
}

// Mount registers DiscoveryHub's routes.
func (h *Discovery) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/discovery/status", h.status)
	mux.HandleFunc("POST /api/v1/discovery/recommendations", h.generate)
}

// window is how far ahead "the day's EPG" (ROADMAP.md) looks from now.
const window = 20 * time.Hour

func (h *Discovery) today() string { return h.Now().UTC().Format("2006-01-02") }

func (h *Discovery) status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	v, err := h.Settings.Values(ctx, user)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read settings")
		return
	}
	used, err := h.Store.AIUsageToday(ctx, h.today())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read AI usage")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"usedToday": used, "dailyLimit": v.Int(connector.KeyAIDailyLimit)})
}

type generateRequest struct {
	Bouquet string `json:"bouquet"`
}

func (h *Discovery) generate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := httpx.User(ctx)

	var req generateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil || strings.TrimSpace(req.Bouquet) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "bouquet is required")
		return
	}

	v, err := h.Settings.Values(ctx, user)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read settings")
		return
	}
	apiKey := v.String(connector.KeyOpenAIAPIKey)
	if apiKey == "" {
		httpx.WriteError(w, r, http.StatusConflict, "ai_not_configured", "configure an OpenAI API key in Settings first")
		return
	}
	limit := v.Int(connector.KeyAIDailyLimit)
	today := h.today()
	used, err := h.Store.AIUsageToday(ctx, today)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read AI usage")
		return
	}
	if int64(used) >= limit {
		httpx.WriteError(w, r, http.StatusTooManyRequests, "ai_budget_exceeded", "today's AI recommendation budget is used up")
		return
	}

	now := h.Now()
	events, err := h.WC.EPGMulti(ctx, req.Bouquet, now.Unix(), now.Add(window).Unix())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	items := make([]ai.EPGItem, len(events))
	for i, e := range events {
		items[i] = ai.EPGItem{
			Title: e.Title, Description: e.Description, ServiceReference: e.ServiceReference,
			ServiceName: e.ServiceName, Start: e.Start, Duration: e.Duration,
		}
	}

	// Reserve the budget slot before the real call: a slow/failed call still
	// counts, rather than letting a retry-happy client exceed the daily cap.
	used, err = h.Store.IncrementAIUsage(ctx, today)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not record AI usage")
		return
	}

	recs, err := h.AI.Recommend(ctx, apiKey, v.String(connector.KeyOpenAIModel), items)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "ai_unavailable", "could not get recommendations")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"recommendations": recs, "usedToday": used, "dailyLimit": limit})
}
