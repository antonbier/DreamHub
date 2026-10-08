package hubs

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// visibleBouquets is the one place the bouquet lists the everyday hubs
// serve (guide, live TV, radio) pass through for the request's profile
// (docs/CHANNELHUB_PLAN.md, C5). A profile with no bouquets set — today
// every profile — sees all of them; one with a list sees only those, and
// not radio's "all stations" either. Prepared for a kids role: nothing
// sets the list yet.
func visibleBouquets(ctx context.Context, list []webcontrol.Service) []webcontrol.Service {
	u, ok := httpx.User(ctx)
	if !ok || len(u.Bouquets) == 0 {
		return list
	}
	allowed := map[string]bool{}
	for _, b := range u.Bouquets {
		allowed[b] = true
	}
	out := []webcontrol.Service{}
	for _, b := range list {
		if allowed[b.ServiceReference] {
			out = append(out, b)
		}
	}
	return out
}
