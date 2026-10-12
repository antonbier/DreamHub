package hubs

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/apt"
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
)

// AdminHub's package browser (docs/ADMINHUB_PLAN.md, A2, #47): the box's
// installed and upgradable packages, grouped by apt's own Section field,
// filterable. Read-only — install/remove as a plan is a later slice, once
// the box has confirmed `apt-get -s` simulate output and the generalized
// "don't restart a service while recording" check.

// listPackages answers GET /api/v1/admin/packages, filtered by:
//   - q: a case-insensitive substring of the name or description
//   - category: an exact match against Package.Category
//   - status: "upgradable" (only those with an update pending) or
//     "installed" (everything dpkg knows about, the default — there's no
//     "not installed" catalog yet, see docs/ADMINHUB_PLAN.md)
func (h *Admin) listPackages(w http.ResponseWriter, r *http.Request) {
	if h.Apt == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "apt_unavailable", "dpkg/apt aren't available on this system")
		return
	}
	pkgs, err := h.Apt.List(r.Context())
	if errors.Is(err, apt.ErrUnavailable) {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "apt_unavailable", "dpkg/apt aren't available on this system")
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "apt_failed", "could not read the package list")
		return
	}

	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	category := r.URL.Query().Get("category")
	status := r.URL.Query().Get("status")

	out := make([]apt.Package, 0, len(pkgs))
	categories := map[string]bool{}
	for _, p := range pkgs {
		categories[p.Category] = true
		if q != "" && !strings.Contains(strings.ToLower(p.Name), q) && !strings.Contains(strings.ToLower(p.Description), q) {
			continue
		}
		if category != "" && p.Category != category {
			continue
		}
		if status == "upgradable" && !p.Upgradable {
			continue
		}
		out = append(out, p)
	}

	cats := make([]string, 0, len(categories))
	for c := range categories {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"packages": out, "categories": cats})
}

// listFeeds answers GET /api/v1/admin/feeds: the box's configured apt
// feeds, each flagged known/added-by-hand (docs/ADMINHUB_PLAN.md, "Feeds,
// with a trust signal").
func (h *Admin) listFeeds(w http.ResponseWriter, r *http.Request) {
	if h.Feeds == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "apt_unavailable", "feeds aren't available on this system")
		return
	}
	feeds, err := h.Feeds.ListFeeds(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "apt_failed", "could not read the feed list")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"feeds": feeds})
}

// explainPackage answers POST /api/v1/admin/packages/explain: an on-demand
// "what does this do?" for one package, through the same opt-in AI
// connector and shared daily budget DiscoveryHub's own AI use already
// spends from (docs/ADMINHUB_PLAN.md, "AI explains a package, on
// request") — nothing sent unless this is actually called.
func (h *Admin) explainPackage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || body.Name == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"name": "<package>", "description": "..."}`)
		return
	}
	if h.AI == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "ai_unavailable", "AI explanations aren't available on this system")
		return
	}
	ctx := r.Context()
	user, _ := httpx.User(ctx)
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
	today := h.now().UTC().Format("2006-01-02")
	used, err := h.Store.AIUsageToday(ctx, today)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read AI usage")
		return
	}
	if int64(used) >= limit {
		httpx.WriteError(w, r, http.StatusTooManyRequests, "ai_budget_exceeded", "today's AI budget is used up")
		return
	}
	// Reserve the budget slot before the real call, like DiscoveryHub's
	// own generate handler: a slow/failed call still counts.
	if _, err := h.Store.IncrementAIUsage(ctx, today); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not record AI usage")
		return
	}
	explanation, err := h.AI.ExplainPackage(ctx, apiKey, v.String(connector.KeyOpenAIModel), body.Name, body.Description)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "ai_unavailable", "could not get an explanation")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"explanation": explanation})
}
