package hubs

import (
	"net/http"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Guide is GuideHub: a "what's on now/next" grid, per bouquet. A full
// timeline grid EPG is still open (ROADMAP.md); this is the real now/next
// data every timeline view would build on.
type Guide struct {
	WC webcontrol.Client
}

// Mount registers GuideHub's routes.
func (h *Guide) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/guide/bouquets", h.bouquets)
	mux.HandleFunc("GET /api/v1/guide/now", h.now)
}

func (h *Guide) bouquets(w http.ResponseWriter, r *http.Request) {
	bouquets, err := h.WC.Bouquets(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"bouquets": bouquets})
}

// Channel is one channel's now/next in the guide, per ChannelEvents below.
type ChannelEvents struct {
	ServiceReference string               `json:"serviceReference"`
	ServiceName      string               `json:"serviceName"`
	Now              *webcontrol.EPGEvent `json:"now,omitempty"`
	Next             *webcontrol.EPGEvent `json:"next,omitempty"`
}

// groupNowNext turns epgnownext's flat, chronological event list (two
// entries per channel: now, then next — confirmed live, ARCHITECTURE.md)
// into one entry per channel, preserving each channel's first-seen order.
func groupNowNext(events []webcontrol.EPGEvent) []ChannelEvents {
	var order []string
	byChannel := map[string]*ChannelEvents{}
	for i := range events {
		e := &events[i]
		ch, ok := byChannel[e.ServiceReference]
		if !ok {
			ch = &ChannelEvents{ServiceReference: e.ServiceReference, ServiceName: e.ServiceName}
			byChannel[e.ServiceReference] = ch
			order = append(order, e.ServiceReference)
		}
		switch {
		case ch.Now == nil:
			ch.Now = e
		case ch.Next == nil:
			ch.Next = e
		}
	}
	out := make([]ChannelEvents, 0, len(order))
	for _, ref := range order {
		out = append(out, *byChannel[ref])
	}
	return out
}

func (h *Guide) now(w http.ResponseWriter, r *http.Request) {
	bouquet := r.URL.Query().Get("bouquet")
	if bouquet == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_query", "bouquet is required")
		return
	}
	events, err := h.WC.EPGNowNext(r.Context(), bouquet)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"channels": groupNowNext(events)})
}
