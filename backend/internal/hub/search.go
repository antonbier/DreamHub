package hub

import (
	"context"
	"sort"

	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/status"
)

// SearchResult is one match from a hub's search provider (CONCEPT.md,
// "Global search": "every hub can contribute a search provider; results
// are grouped by hub").
type SearchResult struct {
	HubID    string `json:"hubId"`
	HubLabel string `json:"hubLabel"` // i18n key, same as the hub's nav label
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Path     string `json:"path"`
}

// SearchProvider is implemented by a hub that can answer a search query
// over its own content (e.g. TimerHub searching timer names).
type SearchProvider interface {
	Search(ctx context.Context, query string) ([]SearchResult, error)
}

// AddSearchProvider registers p for hubID. Call after Add for that hub.
func (r *Registry) AddSearchProvider(hubID string, p SearchProvider) {
	r.search[hubID] = p
}

// Search queries every registered provider whose hub is enabled, visible to
// role and currently at least degraded (not unavailable) — matching what
// the sidebar itself would show. A provider that errors is skipped rather
// than failing the whole search (CONCEPT.md: hubs degrade gracefully).
// Results are grouped by hub, in the hub's own navigation order.
func (r *Registry) Search(ctx context.Context, v settings.Values, reports map[string]connector.Report, perms perm.Set, query string) []SearchResult {
	statuses := r.StatusesFor(ctx, v, reports)
	order := map[string]int{}
	for _, d := range r.hubs {
		order[d.ID] = d.Order
	}
	sort.SliceStable(statuses, func(i, j int) bool { return order[statuses[i].ID] < order[statuses[j].ID] })

	var out []SearchResult
	for _, s := range statuses {
		if !s.Enabled || !Visible(s.Perm, perms) || s.Level == status.Unavailable {
			continue
		}
		p, ok := r.search[s.ID]
		if !ok {
			continue
		}
		results, err := p.Search(ctx, query)
		if err != nil {
			continue
		}
		for i := range results {
			results[i].HubID = s.ID
			results[i].HubLabel = s.Label
		}
		out = append(out, results...)
	}
	return out
}
