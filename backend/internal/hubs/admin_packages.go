package hubs

import (
	"errors"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/apt"
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	"github.com/antonbier/DreamHub/backend/internal/pkginfo"
)

// AdminHub's package browser (docs/ADMINHUB_PLAN.md, A2, #47): the box's
// installed and upgradable packages, grouped by apt's own Section field,
// filterable. Install, upgrade and remove go through a plan
// (admin_packages_plan.go).

// maxPackageRows caps one answer of the package list.
const maxPackageRows = 400

// packageNeeds: which hubs need which OS package (hub.Descriptor.Packages).
var packageNeeds = hub.PackageNeeds(hub.V1())

// packageRow is one package in the list, plus the hubs that need it and
// the resolved information (internal/pkginfo: curated text, else the
// package's own description, else nothing) — no AI involved.
type packageRow struct {
	apt.Package
	NeededBy []string `json:"neededBy,omitempty"`
	// Info is what the Packages tab shows: description, category (a stable
	// id plus its label in the asked language) and where the text came from.
	Info pkginfo.Resolved `json:"info"`
	// Missing: a package a hub needs that isn't installed (only in the
	// "needed" view, which shows those too, with an install button).
	Missing bool `json:"missing,omitempty"`
}

// listPackages answers GET /api/v1/admin/packages, filtered by:
//   - q: a case-insensitive substring of the name or description
//   - category: a pkginfo category id ("extensions", "skins", …)
//   - lang: de, en or it — the language of descriptions and category labels
//     (default en)
//   - status: "upgradable" (only those with an update pending),
//     "available" (the feeds' packages that aren't installed) or
//     "installed" (everything dpkg knows about, the default) or "needed"
//     (the packages a DreamHub hub uses, installed or not)
func (h *Admin) listPackages(w http.ResponseWriter, r *http.Request) {
	if h.Apt == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "apt_unavailable", "dpkg/apt aren't available on this system")
		return
	}
	status := r.URL.Query().Get("status")
	var pkgs []apt.Package
	var err error
	if status == "available" {
		// The "not installed" catalog: what the enabled feeds offer.
		if h.Packages == nil {
			httpx.WriteError(w, r, http.StatusServiceUnavailable, "apt_unavailable", "dpkg/apt aren't available on this system")
			return
		}
		pkgs, err = h.Packages.Available(r.Context())
	} else {
		pkgs, err = h.Apt.List(r.Context())
	}
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
	lang := pkginfo.Lang(r.URL.Query().Get("lang"))

	if status == "needed" {
		pkgs = h.withMissingNeeds(r, pkgs)
	}

	out := make([]packageRow, 0, len(pkgs))
	categories := map[string]bool{}
	for _, p := range pkgs {
		info := pkginfo.Resolve(p.Name, p.Description, lang)
		needed := mergeNeeds(packageNeeds[p.Name], info.NeededBy)
		info.NeededBy = needed
		if q != "" && !strings.Contains(strings.ToLower(p.Name), q) &&
			!strings.Contains(strings.ToLower(p.Description), q) &&
			!strings.Contains(strings.ToLower(info.Description), q) {
			continue
		}
		if status == "upgradable" && !p.Upgradable {
			continue
		}
		if status == "needed" && len(needed) == 0 {
			continue
		}
		// The category chips show what the current view holds, so the
		// category itself is applied after they are collected.
		categories[info.Category] = true
		if category != "" && info.Category != category {
			continue
		}
		out = append(out, packageRow{Package: p, NeededBy: needed, Info: info, Missing: status == "needed" && p.Version == ""})
	}

	// The catalog runs to thousands of packages on a real box; without a
	// search term only the first ones are sent.
	truncated := false
	if len(out) > maxPackageRows {
		out, truncated = out[:maxPackageRows], true
	}

	// Chips in the order of pkginfo.Categories; only those with packages.
	cats := make([]categoryChip, 0, len(categories))
	for _, c := range pkginfo.Categories {
		if categories[c.ID] {
			cats = append(cats, categoryChip{ID: c.ID, Label: c.Label[lang]})
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"packages": out, "categories": cats, "truncated": truncated})
}

type categoryChip struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// mergeNeeds joins the hubs that declare a package (hub.Descriptor.Packages)
// with the curated entry's, without duplicates.
func mergeNeeds(a, b []string) []string {
	var out []string
	for _, id := range append(append([]string(nil), a...), b...) {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// withMissingNeeds adds a row (version empty) for every package a hub
// needs that isn't in installed — taken from the feeds' catalog when it's
// there, so its description shows, otherwise just the name.
func (h *Admin) withMissingNeeds(r *http.Request, installed []apt.Package) []apt.Package {
	have := map[string]bool{}
	for _, p := range installed {
		have[p.Name] = true
	}
	var missing []string
	for name := range packageNeeds {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return installed
	}
	sort.Strings(missing)
	feed := map[string]apt.Package{}
	if h.Packages != nil {
		if avail, err := h.Packages.Available(r.Context()); err == nil {
			for _, p := range avail {
				feed[p.Name] = p
			}
		}
	}
	out := append([]apt.Package(nil), installed...)
	for _, name := range missing {
		p, ok := feed[name]
		if !ok {
			p = apt.Package{Name: name}
		}
		p.Version, p.Upgradable, p.AvailableVersion = "", false, ""
		out = append(out, p)
	}
	return out
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
