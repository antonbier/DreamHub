package hubs

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/hub"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Guide is GuideHub: a TV-magazine-style timeline grid, per bouquet and time
// window — several events per channel, not just now/next, so the frontend
// can render an actual schedule grid (switchable bouquet and window).
type Guide struct {
	WC  webcontrol.Client
	Now func() time.Time
	// PiconDirs are searched for channel logos (guide_views.go); nil
	// means DefaultPiconDirs.
	PiconDirs []string
}

// maxGuideResults caps the search provider's answer; the palette shows a
// handful per hub anyway.
const maxGuideResults = 10

// Search implements hub.SearchProvider over the box's own EPG title search
// (/web/epgsearch), upcoming and running programmes only, soonest first.
// A result links into the grid at that programme's start (?at=).
func (h *Guide) Search(ctx context.Context, query string) ([]hub.SearchResult, error) {
	// A short query makes the box serialize its whole EPG (ARCHITECTURE.md,
	// "/web/epgsearch has no result cap"), blocking enigma2 for seconds.
	if len([]rune(strings.TrimSpace(query))) < webcontrol.MinEPGSearchQuery {
		return nil, nil
	}
	events, err := h.WC.EPGSearch(ctx, query)
	if err != nil {
		return nil, err
	}
	now := h.Now().Unix()
	upcoming := events[:0]
	for _, e := range events {
		if e.Start+e.Duration > now {
			upcoming = append(upcoming, e)
		}
	}
	sort.SliceStable(upcoming, func(i, j int) bool { return upcoming[i].Start < upcoming[j].Start })
	if len(upcoming) > maxGuideResults {
		upcoming = upcoming[:maxGuideResults]
	}
	out := make([]hub.SearchResult, 0, len(upcoming))
	for _, e := range upcoming {
		out = append(out, hub.SearchResult{Title: e.Title, Subtitle: e.ServiceName, Path: "/guide?at=" + strconv.FormatInt(e.Start, 10)})
	}
	return out, nil
}

// Mount registers GuideHub's routes.
func (h *Guide) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/guide/bouquets", h.bouquets)
	mux.HandleFunc("GET /api/v1/guide/grid", h.grid)
	mux.HandleFunc("GET /api/v1/guide/nownext", h.nowNext)
	mux.HandleFunc("GET /api/v1/guide/picon", h.picon)
}

func (h *Guide) bouquets(w http.ResponseWriter, r *http.Request) {
	bouquets, err := h.WC.Bouquets(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"bouquets": bouquets})
}

// ChannelSchedule is one channel's events within the requested window.
type ChannelSchedule struct {
	ServiceReference string                `json:"serviceReference"`
	ServiceName      string                `json:"serviceName"`
	Events           []webcontrol.EPGEvent `json:"events"`
}

// groupByChannel turns epgmulti's flat event list into one entry per
// channel, preserving each channel's first-seen order and each channel's
// own chronological event order (epgmulti already returns events in start
// order per channel — confirmed live, ARCHITECTURE.md).
func groupByChannel(events []webcontrol.EPGEvent) []ChannelSchedule {
	var order []string
	byChannel := map[string]*ChannelSchedule{}
	for i := range events {
		e := events[i]
		ch, ok := byChannel[e.ServiceReference]
		if !ok {
			ch = &ChannelSchedule{ServiceReference: e.ServiceReference, ServiceName: e.ServiceName}
			byChannel[e.ServiceReference] = ch
			order = append(order, e.ServiceReference)
		}
		ch.Events = append(ch.Events, e)
	}
	out := make([]ChannelSchedule, 0, len(order))
	for _, ref := range order {
		out = append(out, *byChannel[ref])
	}
	return out
}

func (h *Guide) grid(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	bouquet := q.Get("bouquet")
	start, errStart := strconv.ParseInt(q.Get("start"), 10, 64)
	end, errEnd := strconv.ParseInt(q.Get("end"), 10, 64)
	if bouquet == "" || errStart != nil || errEnd != nil || end <= start {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_query", "bouquet, start and end (Unix seconds, end after start) are required")
		return
	}
	events, err := h.WC.EPGMulti(r.Context(), bouquet, start, end)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	// ?service= narrows the answer to one channel (the channel view): the
	// box is still asked for the bouquet, but the browser gets one row.
	if svc := q.Get("service"); svc != "" {
		kept := events[:0]
		for _, e := range events {
			if e.ServiceReference == svc {
				kept = append(kept, e)
			}
		}
		events = kept
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"channels": groupByChannel(events)})
}
