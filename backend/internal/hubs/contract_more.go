package hubs

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// The hub contract's search providers and view sources for the hubs that
// didn't have them yet (ROADMAP.md, Foundation: "remaining parts of the
// hub contract"): DiscoveryHub's picks, SystemHub's change log, AdminHub's
// services, RadioHub's programmes. ControlHub has no content of its own to
// find — its "results" would be actions, and search only ever opens pages.

// Sources of the views below.
const (
	SourceDiscoveryPicks = "discovery.picks"
	SourceRadioNow       = "radio.now"
)

const maxContractResults = 8

func matches(q string, texts ...string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return false
	}
	for _, t := range texts {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}

// --- DiscoveryHub: the day's saved picks -------------------------------------

// picks are the signed-in user's saved picks still to come.
func (h *Discovery) picks(ctx context.Context) []pickView {
	user, ok := httpx.User(ctx)
	if !ok || h.Store == nil {
		return nil
	}
	data, _, found, err := h.Store.DiscoveryPicks(ctx, user.ID)
	if err != nil || !found {
		return nil
	}
	var p savedPicks
	if json.Unmarshal([]byte(data), &p) != nil {
		return nil
	}
	return h.views(ctx, p.Recommendations)
}

// Search finds the day's picks by title, channel or the AI's reason.
func (h *Discovery) Search(ctx context.Context, query string) ([]hub.SearchResult, error) {
	var out []hub.SearchResult
	for _, p := range h.picks(ctx) {
		if !matches(query, p.Title, p.ServiceName, p.Reason) {
			continue
		}
		out = append(out, hub.SearchResult{Title: p.Title, Subtitle: p.ServiceName, Path: "/discovery"})
		if len(out) == maxContractResults {
			break
		}
	}
	return out, nil
}

func (h *Discovery) ViewSources() []hub.ViewSource {
	return []hub.ViewSource{{
		ID:      SourceDiscoveryPicks,
		Label:   "dashboard.source.discoveryPicks",
		Filters: []string{hub.FilterText, hub.FilterWithinDays, hub.FilterFromTime, hub.FilterWarnOnly},
		Sorts:   []string{hub.SortSoonest},
	}}
}

// View lists the saved picks; Warn marks the ones without a timer yet
// ("only open picks" via the warn-only filter).
func (h *Discovery) View(ctx context.Context, sourceID string, q hub.ViewQuery, now time.Time) ([]hub.ViewItem, error) {
	out := []hub.ViewItem{}
	for _, p := range h.picks(ctx) {
		out = append(out, hub.ViewItem{Title: p.Title, Subtitle: p.ServiceName, Time: p.Start, End: p.Start + p.Duration, Warn: !p.HasTimer, Path: "/discovery"})
	}
	return out, nil
}

// --- SystemHub: the change log -----------------------------------------------

// Search finds change log entries by what changed; the result opens the
// change log filtered to the query.
func (h *System) Search(ctx context.Context, query string) ([]hub.SearchResult, error) {
	if h.Store == nil {
		return nil, nil
	}
	entries, err := h.Store.Changes(ctx, 300)
	if err != nil {
		return nil, err
	}
	var out []hub.SearchResult
	for _, e := range entries {
		if !matches(query, e.Summary, e.Action) {
			continue
		}
		out = append(out, hub.SearchResult{Title: e.Summary, Subtitle: e.At.Format(time.DateOnly),
			Path: "/device?" + url.Values{"changes": {strings.TrimSpace(query)}}.Encode()})
		if len(out) == maxContractResults {
			break
		}
	}
	return out, nil
}

// --- AdminHub: the box's services --------------------------------------------

// Search finds services by unit name or description (AdminHub is admin
// only, so the registry asks it only for admins).
func (h *Admin) Search(ctx context.Context, query string) ([]hub.SearchResult, error) {
	if h.Systemd == nil {
		return nil, nil
	}
	units, err := h.Systemd.Status(ctx, h.services())
	if err != nil {
		return nil, err
	}
	var out []hub.SearchResult
	for _, u := range units {
		name := strings.TrimSuffix(u.Name, ".service")
		if matches(query, name, u.Description) {
			out = append(out, hub.SearchResult{Title: name, Subtitle: u.Description, Path: "/device?tab=admin"})
		}
	}
	return out, nil
}

// --- DeviceHub: SystemHub and AdminHub share one nav entry -------------------

// DeviceSearch combines System's and Admin's own search results under the
// one hub ID DeviceHub registers as (the registry's search index holds one
// provider per hub ID, hub.go's AddSearchProvider).
type DeviceSearch struct {
	System *System
	Admin  *Admin
}

func (d DeviceSearch) Search(ctx context.Context, query string) ([]hub.SearchResult, error) {
	out, err := d.System.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	more, err := d.Admin.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	out = append(out, more...)
	if len(out) > maxContractResults {
		out = out[:maxContractResults]
	}
	return out, nil
}

// --- RadioHub: what's on the radio -------------------------------------------

func (h *Radio) ViewSources() []hub.ViewSource {
	return []hub.ViewSource{{
		ID:      SourceRadioNow,
		Label:   "dashboard.source.radioNow",
		Filters: []string{hub.FilterText},
		Sorts:   []string{hub.SortSoonest},
	}}
}

// View is what runs now on the radio stations of the first radio bouquet
// that has any — else of all radio stations, as on the reference box with
// its empty favourites. Stations without programme information are left
// out: a tile of station names alone says nothing.
func (h *Radio) View(ctx context.Context, sourceID string, q hub.ViewQuery, now time.Time) ([]hub.ViewItem, error) {
	bouquets, err := h.radioBouquets(ctx)
	if err != nil {
		return nil, err
	}
	for _, b := range bouquets {
		services, err := h.WC.Services(ctx, b.ServiceReference)
		if err != nil || !hasServices(services) {
			continue
		}
		nowNext, err := h.WC.EPGNowNext(ctx, b.ServiceReference)
		if err != nil {
			return nil, err
		}
		out := []hub.ViewItem{}
		for _, st := range mergeStations(services, nowNext, b.ServiceReference == webcontrol.RadioAll) {
			if st.Now == nil {
				continue
			}
			out = append(out, hub.ViewItem{Title: st.Now.Title, Subtitle: st.ServiceName, Time: st.Now.Start, End: st.Now.Start + st.Now.Duration,
				Path: "/radio?" + url.Values{"bouquet": {b.ServiceReference}, "station": {st.ServiceReference}}.Encode()})
		}
		return out, nil
	}
	return []hub.ViewItem{}, nil
}
