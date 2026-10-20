package hubs

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
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

	rt radioTextState // radio_text.go
}

// Mount registers RadioHub's routes.
func (h *Radio) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/radio/bouquets", h.bouquets)
	mux.HandleFunc("GET /api/v1/radio/stations", h.stations)
	mux.HandleFunc("GET /api/v1/radio/timers", h.timers)
	mux.HandleFunc("GET /api/v1/radio/picon", h.picon)
	mux.HandleFunc("GET /api/v1/radio/text", h.text)
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
	// Every radio service, last; no name — the screen calls it "All radio
	// stations" in its own language.
	return visibleBouquets(ctx, append(out, webcontrol.Service{ServiceReference: webcontrol.RadioAll})), nil
}

func (h *Radio) bouquets(w http.ResponseWriter, r *http.Request) {
	list, err := h.radioBouquets(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"bouquets": list, "all": webcontrol.RadioAll})
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
// box's one-call epgnownext. Headings are dropped. For the list of all
// radio services (all), unnamed data services go too, and a station on
// several transponders is shown once — the one with programme
// information, else the first.
func mergeStations(services []webcontrol.Service, nowNext []webcontrol.NowNext, all bool) []Station {
	byRef := map[string]webcontrol.NowNext{}
	for _, n := range nowNext {
		byRef[n.ServiceReference] = n
	}
	out := []Station{}
	byName := map[string]int{}
	for _, s := range services {
		if webcontrol.IsMarker(s.ServiceReference) {
			continue
		}
		st := Station{ServiceReference: s.ServiceReference, ServiceName: strings.TrimSpace(s.ServiceName)}
		if n, ok := byRef[s.ServiceReference]; ok {
			st.Now, st.Next = n.Now, n.Next
		}
		if all {
			if unnamedService(st.ServiceName) {
				continue
			}
			key := strings.ToLower(st.ServiceName)
			if i, seen := byName[key]; seen {
				if out[i].Now == nil && st.Now != nil {
					out[i] = st
				}
				continue
			}
			byName[key] = len(out)
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
	// An empty bouquet isn't asked at all: the box's /web/epgnownext fails
	// on one with a traceback in enigma2's log ("Processing Failed",
	// KeyError 'BouquetReference' — confirmed live 2026-10-04, the
	// reference box's empty radio favourites).
	var nowNext []webcontrol.NowNext
	if hasServices(services) {
		nowNext, _ = h.WC.EPGNowNext(ctx, bouquet)
	}
	// What the box plays, to mark it; not knowing just marks nothing.
	playing, _ := h.WC.CurrentServiceReference(ctx)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"stations": mergeStations(services, nowNext, bouquet == webcontrol.RadioAll), "playing": playing})
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
	// By name: the list of all radio services has a station once per
	// transponder.
	seen := map[string]bool{}
	var out []hub.SearchResult
	for _, b := range bouquets {
		services, err := h.WC.Services(ctx, b.ServiceReference)
		if err != nil {
			return nil, err
		}
		for _, s := range services {
			name := strings.TrimSpace(s.ServiceName)
			key := strings.ToLower(name)
			if webcontrol.IsMarker(s.ServiceReference) || unnamedService(name) || seen[key] || !strings.Contains(key, q) {
				continue
			}
			seen[key] = true
			out = append(out, hub.SearchResult{Title: name, Subtitle: b.ServiceName,
				Path: "/radio?" + url.Values{"bouquet": {b.ServiceReference}, "station": {s.ServiceReference}}.Encode()})
			if len(out) == maxRadioResults {
				return out, nil
			}
		}
	}
	return out, nil
}

// sidName is how Enigma2 names a service that sends no name of its own:
// frequency, polarisation and service ID ("10796V SID 0x3dae" — nine of
// them in the reference box's radio list).
var sidName = regexp.MustCompile(`^\d+[HVLR] SID 0x[0-9a-fA-F]+$`)

// unnamedService: a data service ("." on the reference box), an empty
// name, or one Enigma2 made up from the transponder.
func unnamedService(name string) bool {
	return strings.Trim(name, ". ") == "" || sidName.MatchString(name)
}

// hasServices: anything besides headings.
func hasServices(services []webcontrol.Service) bool {
	for _, s := range services {
		if !webcontrol.IsMarker(s.ServiceReference) {
			return true
		}
	}
	return false
}
