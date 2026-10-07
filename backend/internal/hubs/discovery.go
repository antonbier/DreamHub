package hubs

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/ai"
	"github.com/antonbier/DreamHub/backend/internal/autotimer"
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
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
	WC webcontrol.Client
	// SR and AT are optional taste sources (SerienRecorder's followed
	// series, AutoTimer's search terms): nil, or failing because the plugin
	// isn't installed, just leaves that part of the taste profile empty.
	SR       serienrecorder.Client
	AT       autotimer.Client
	AI       ai.Client
	Store    *store.Store
	Settings *settings.Service
	Now      func() time.Time
	// Loc is the box's zone, for "the evening" (candidates); nil: the
	// process's own, which on the box is the box's.
	Loc *time.Location
	// Plex flags recommendations already in the library (#32); nil (Plex
	// not set up) just means the flag is never set.
	Plex *Plex
}

// Perms are DiscoveryHub's route permissions beyond the hub's own (internal/perm).
func (h *Discovery) Perms() map[string]perm.Perm {
	return map[string]perm.Perm{
		"POST /api/v1/discovery/recommendations": perm.Discovery,
	}
}

// Mount registers DiscoveryHub's routes.
func (h *Discovery) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/discovery/status", h.status)
	mux.HandleFunc("GET /api/v1/discovery/taste", h.getTaste)
	mux.HandleFunc("GET /api/v1/discovery/recommendations", h.saved)
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
	// UseTaste sends the user's taste profile (taste.go) along. Off
	// unless asked for: it's the user's own viewing data going to an
	// external AI provider.
	UseTaste bool `json:"useTaste"`
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
	// Broadcasts a timer already covers aren't worth recommending — the
	// user has them.
	timers, _ := h.WC.TimerList(ctx)
	items := make([]ai.EPGItem, 0, len(events))
	for _, e := range events {
		if coveredByTimer(e, timers) {
			continue
		}
		items = append(items, ai.EPGItem{
			Title: e.Title, Description: e.Description, ServiceReference: e.ServiceReference,
			ServiceName: e.ServiceName, Start: e.Start, Duration: e.Duration,
		})
	}
	items = candidates(items, ai.MaxItems, h.Loc)
	var taste ai.Taste
	if req.UseTaste {
		taste = h.taste(ctx)
	}

	// Reserve the budget slot before the real call: a slow/failed call still
	// counts, rather than letting a retry-happy client exceed the daily cap.
	used, err = h.Store.IncrementAIUsage(ctx, today)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not record AI usage")
		return
	}

	recs, err := h.AI.Recommend(ctx, apiKey, v.String(connector.KeyOpenAIModel), items, taste)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "ai_unavailable", "could not get recommendations")
		return
	}
	counts := tasteCounts(taste)
	saved := savedPicks{Bouquet: req.Bouquet, GeneratedAt: now.Unix(), Recommendations: recs}
	if req.UseTaste {
		saved.Taste = &counts
	}
	h.savePicks(ctx, user.ID, saved)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"recommendations": h.views(ctx, recs), "usedToday": used, "dailyLimit": limit, "taste": counts,
		"bouquet": req.Bouquet, "generatedAt": saved.GeneratedAt,
	})
}

// candidates picks at most n broadcasts for the model, fairly: the box
// lists a bouquet's EPG channel by channel, so cutting the list at n
// dropped the bouquet's last channels entirely. Instead the channels take
// turns, and within a channel prime time (20:00–23:00 on the box's clock)
// comes first, then the rest of the evening, then the day. The result is
// in time order.
func candidates(items []ai.EPGItem, n int, loc *time.Location) []ai.EPGItem {
	if len(items) <= n {
		return items
	}
	if loc == nil {
		loc = time.Local
	}
	rank := func(it ai.EPGItem) int {
		switch h := time.Unix(it.Start, 0).In(loc).Hour(); {
		case h >= 20 && h < 23:
			return 0
		case h >= 18:
			return 1
		}
		return 2
	}
	var order []string
	byChannel := map[string][]ai.EPGItem{}
	for _, it := range items {
		if _, ok := byChannel[it.ServiceReference]; !ok {
			order = append(order, it.ServiceReference)
		}
		byChannel[it.ServiceReference] = append(byChannel[it.ServiceReference], it)
	}
	for _, ch := range order {
		list := byChannel[ch]
		sort.SliceStable(list, func(i, j int) bool {
			if ri, rj := rank(list[i]), rank(list[j]); ri != rj {
				return ri < rj
			}
			return list[i].Start < list[j].Start
		})
	}
	out := make([]ai.EPGItem, 0, n)
	for round := 0; len(out) < n; round++ {
		added := false
		for _, ch := range order {
			if list := byChannel[ch]; round < len(list) && len(out) < n {
				out = append(out, list[round])
				added = true
			}
		}
		if !added {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}
