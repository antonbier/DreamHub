package hubs

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Radio is RadioHub (CONCEPT.md, v1.1): the box's radio stations with
// what's on now and next, which one the box is playing, and the radio
// recordings coming up. Listening happens on the box ("Play on the box" is
// ControlHub's zap, called by the frontend); playback in the browser
// belongs to StreamHub — radio is often MP2 audio, which browsers don't
// play reliably. Recording a programme goes through TimerHub's plan/apply
// like everywhere else.
type Radio struct {
	WC  webcontrol.Client
	Now func() time.Time
	// PiconDirs: as Guide's; nil means DefaultPiconDirs.
	PiconDirs []string
}

// Mount registers RadioHub's routes.
func (h *Radio) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/radio/bouquets", h.bouquets)
	mux.HandleFunc("GET /api/v1/radio/stations", h.stations)
	mux.HandleFunc("GET /api/v1/radio/timers", h.timers)
	mux.HandleFunc("GET /api/v1/radio/picon", h.picon)
}

// radioBouquets is the box's radio bouquet list without headings.
func (h *Radio) radioBouquets(ctx context.Context) ([]webcontrol.Service, error) {
	list, err := h.WC.Services(ctx, webcontrol.RadioRoot)
	if err != nil {
		return nil, err
	}
	out := []webcontrol.Service{}
	for _, b := range list {
		if !webcontrol.IsMarker(b.ServiceReference) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (h *Radio) bouquets(w http.ResponseWriter, r *http.Request) {
	list, err := h.radioBouquets(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"bouquets": list})
}

// Station is one radio station with its now/next (nil without EPG —
// many stations send none).
type Station struct {
	ServiceReference string               `json:"serviceReference"`
	ServiceName      string               `json:"serviceName"`
	Now              *webcontrol.EPGEvent `json:"now"`
	Next             *webcontrol.EPGEvent `json:"next"`
}

// mergeStations takes the bouquet's own list for order and completeness
// (epgnownext may skip a station without EPG) and adds now/next from the
// box's one-call epgnownext. Headings are dropped.
func mergeStations(services []webcontrol.Service, nowNext []webcontrol.NowNext) []Station {
	byRef := map[string]webcontrol.NowNext{}
	for _, n := range nowNext {
		byRef[n.ServiceReference] = n
	}
	out := []Station{}
	for _, s := range services {
		if webcontrol.IsMarker(s.ServiceReference) {
			continue
		}
		st := Station{ServiceReference: s.ServiceReference, ServiceName: strings.TrimSpace(s.ServiceName)}
		if n, ok := byRef[s.ServiceReference]; ok {
			st.Now, st.Next = n.Now, n.Next
		}
		out = append(out, st)
	}
	return out
}

func (h *Radio) stations(w http.ResponseWriter, r *http.Request) {
	bouquet := r.URL.Query().Get("bouquet")
	if bouquet == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_query", "bouquet is required")
		return
	}
	ctx := r.Context()
	services, err := h.WC.Services(ctx, bouquet)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	// Without EPG the list is still worth showing: names, Play on the box.
	nowNext, _ := h.WC.EPGNowNext(ctx, bouquet)
	// What the box plays, to mark it; not knowing just marks nothing.
	playing, _ := h.WC.CurrentServiceReference(ctx)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"stations": mergeStations(services, nowNext), "playing": playing})
}

// radioTimers are the timers on radio stations that haven't ended yet,
// soonest first.
func radioTimers(all []webcontrol.Timer, now int64) []webcontrol.Timer {
	out := []webcontrol.Timer{}
	for _, t := range all {
		if webcontrol.IsRadioService(t.ServiceReference) && t.End > now {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Begin < out[j].Begin })
	return out
}

func (h *Radio) timers(w http.ResponseWriter, r *http.Request) {
	all, err := h.WC.TimerList(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"timers": radioTimers(all, h.Now().Unix())})
}

func (h *Radio) picon(w http.ResponseWriter, r *http.Request) { servePicon(w, r, h.PiconDirs) }

// maxRadioResults caps the search provider's answer.
const maxRadioResults = 8

// Search implements hub.SearchProvider: stations by name across the radio
// bouquets; a result opens RadioHub at that station.
func (h *Radio) Search(ctx context.Context, query string) ([]hub.SearchResult, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	bouquets, err := h.radioBouquets(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []hub.SearchResult
	for _, b := range bouquets {
		services, err := h.WC.Services(ctx, b.ServiceReference)
		if err != nil {
			return nil, err
		}
		for _, s := range services {
			name := strings.TrimSpace(s.ServiceName)
			if webcontrol.IsMarker(s.ServiceReference) || seen[s.ServiceReference] || !strings.Contains(strings.ToLower(name), q) {
				continue
			}
			seen[s.ServiceReference] = true
			out = append(out, hub.SearchResult{Title: name, Subtitle: b.ServiceName,
				Path: "/radio?" + url.Values{"bouquet": {b.ServiceReference}, "station": {s.ServiceReference}}.Encode()})
			if len(out) == maxRadioResults {
				return out, nil
			}
		}
	}
	return out, nil
}
