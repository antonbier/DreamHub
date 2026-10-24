package hubs

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/iptv"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// The programme shown next to a channel on the Netcast page: "now" and "next",
// from two places, the first that knows wins.
//
//  1. The box itself, for a channel that is already in one of its bouquets:
//     the guide the box holds under that entry (Netcast's guide loader, or a
//     twin's). One call per bouquet that holds internet entries, kept for a
//     minute.
//  2. The XMLTV files the person has set up in Netcast (`netcastepg_*.json`),
//     loaded by DreamHub from the same addresses (never one with a login),
//     matched by tvg-id and then by name. A file is streamed, only the hours
//     around now are kept, and it is loaded again after two hours — and only
//     while someone looks at the page. DreamHub asks no streaming service
//     for a guide.

const (
	netcastGuideTTL    = 2 * time.Hour
	netcastGuideBefore = time.Hour
	netcastGuideAhead  = 12 * time.Hour
	netcastGuideMaxIDs = 400
	netcastGuideLoad   = 4 * time.Minute
	netcastBoxGuideTTL = time.Minute
)

type guideSource struct {
	sched   *iptv.Schedule
	loading bool
	failed  bool
	at      time.Time
}

type boxGuide struct {
	byURL map[string]webcontrol.NowNext // escaped stream address -> now/next
	at    time.Time
}

type netcastGuideState struct {
	mu      sync.Mutex
	sources map[string]*guideSource // by address
	urls    map[string]urlCache     // per box: the guide addresses in Netcast's files
	box     map[string]boxGuide     // per box
	load    chan struct{}           // one file is loaded at a time
}

type urlCache struct {
	list []string
	at   time.Time
}

// GuideProgramme is one programme as the page gets it; times are Unix seconds, UTC.
type GuideProgramme struct {
	Title string `json:"title"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
}

// GuideNowNext is what runs on a channel now and after it.
type GuideNowNext struct {
	Now  *GuideProgramme `json:"now,omitempty"`
	Next *GuideProgramme `json:"next,omitempty"`
}

func gp(p *iptv.Programme) *GuideProgramme {
	if p == nil {
		return nil
	}
	return &GuideProgramme{Title: p.Title, Start: p.Start.Unix(), End: p.End.Unix()}
}

func gpEvent(e *webcontrol.EPGEvent) *GuideProgramme {
	if e == nil || e.Title == "" || e.Duration <= 0 {
		return nil
	}
	return &GuideProgramme{Title: e.Title, Start: e.Start, End: e.Start + e.Duration}
}

// guideFetcher is a fetcher for guide files: the same safeguards as the
// channel list's, but time for a big file.
func (h *Netcast) guideFetcher() *iptv.Fetcher {
	f := &iptv.Fetcher{Timeout: netcastGuideLoad}
	if base := h.Channels.IPTV; base != nil {
		f.AllowLoopback, f.UserAgent = base.AllowLoopback, base.UserAgent
	}
	return f
}

// guideURLs are the XMLTV addresses of the box's Netcast guide sources that
// DreamHub may load.
func (h *Netcast) guideURLs(ctx context.Context) []string {
	rd, ok := h.Channels.WC.(webcontrol.BoxFileReader)
	if !ok {
		return nil
	}
	key := box.Key(ctx)
	st := &h.guide
	st.mu.Lock()
	c, seen := st.urls[key]
	st.mu.Unlock()
	if seen && time.Since(c.at) < time.Minute {
		return c.list
	}
	view, err := readNetcast(ctx, rd)
	if err != nil {
		return c.list
	}
	var list []string
	for _, f := range view.Files {
		if f.Kind != "epg" {
			continue
		}
		for _, s := range f.Sources {
			if s.Fetch != "" {
				list = append(list, s.Fetch)
			}
		}
	}
	st.mu.Lock()
	if st.urls == nil {
		st.urls = map[string]urlCache{}
	}
	st.urls[key] = urlCache{list: list, at: time.Now()}
	st.mu.Unlock()
	return list
}

// ensureGuide starts loading the files that are not there or are old, and
// returns the ones that are loaded and whether any is still loading.
func (h *Netcast) ensureGuide(urls []string) (loaded []*iptv.Schedule, loading bool) {
	st := &h.guide
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.sources == nil {
		st.sources = map[string]*guideSource{}
		st.load = make(chan struct{}, 1)
	}
	now := h.Channels.now()
	for _, u := range urls {
		s := st.sources[u]
		if s == nil {
			s = &guideSource{}
			st.sources[u] = s
		}
		stale := s.sched == nil || now.Sub(s.at) > netcastGuideTTL
		if stale && !s.loading && (!s.failed || now.Sub(s.at) > 10*time.Minute) {
			s.loading = true
			go h.loadGuide(u, s)
		}
		if s.sched != nil {
			loaded = append(loaded, s.sched)
		}
		if s.loading && s.sched == nil {
			loading = true
		}
	}
	return loaded, loading
}

func (h *Netcast) loadGuide(u string, s *guideSource) {
	st := &h.guide
	st.load <- struct{}{} // one at a time: a guide file is big
	defer func() { <-st.load }()
	ctx, cancel := context.WithTimeout(context.Background(), netcastGuideLoad)
	defer cancel()
	now := h.Channels.now()
	sched, err := h.guideFetcher().ReadSchedule(ctx, u, now.Add(-netcastGuideBefore), now.Add(netcastGuideAhead))
	st.mu.Lock()
	defer st.mu.Unlock()
	s.loading = false
	s.at = h.Channels.now()
	if err != nil {
		s.failed = true
		return
	}
	s.failed = false
	s.sched = sched
}

// boxGuideIndex is what the box's guide says on every internet entry that is
// in a bouquet, by the entry's stream address.
func (h *Netcast) boxGuideIndex(ctx context.Context) map[string]webcontrol.NowNext {
	key := box.Key(ctx)
	st := &h.guide
	st.mu.Lock()
	b, seen := st.box[key]
	st.mu.Unlock()
	if seen && time.Since(b.at) < netcastBoxGuideTTL {
		return b.byURL
	}
	idx := map[string]webcontrol.NowNext{}
	bouquets, err := h.Channels.WC.Bouquets(ctx)
	if err != nil {
		return b.byURL
	}
	for _, bq := range bouquets {
		svcs, err := h.Channels.WC.Services(ctx, bq.ServiceReference)
		if err != nil {
			continue
		}
		has := false
		for _, s := range svcs {
			if k, _ := iptv.StreamKind(s.ServiceReference); k != "" {
				has = true
				break
			}
		}
		if !has {
			continue
		}
		nn, err := h.Channels.WC.EPGNowNext(ctx, bq.ServiceReference)
		if err != nil {
			continue
		}
		for _, n := range nn {
			if k, _ := iptv.StreamKind(n.ServiceReference); k == "" {
				continue
			}
			if u := iptv.URLPart(n.ServiceReference); u != "" && (n.Now != nil || n.Next != nil) {
				idx[u] = n
			}
		}
	}
	st.mu.Lock()
	if st.box == nil {
		st.box = map[string]boxGuide{}
	}
	st.box[key] = boxGuide{byURL: idx, at: time.Now()}
	st.mu.Unlock()
	return idx
}

// guide answers with what runs on the asked channels of a read list.
func (h *Netcast) guideHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
		IDs   []int  `json:"ids"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || body.Token == "" || len(body.IDs) > netcastGuideMaxIDs {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"token","ids"} with at most `+strconv.Itoa(netcastGuideMaxIDs)+` ids`)
		return
	}
	user, _ := httpx.User(r.Context())
	s, ok := h.Channels.iptvSession(body.Token, user.ID)
	if !ok {
		httpx.WriteError(w, r, http.StatusNotFound, CodeIPTVSessionGone, "that list is no longer kept; open the source again")
		return
	}
	ctx := r.Context()
	boxIdx := h.boxGuideIndex(ctx)
	urls := h.guideURLs(ctx)
	scheds, loading := h.ensureGuide(urls)
	now := h.Channels.now()
	out := map[string]GuideNowNext{}
	for _, id := range body.IDs {
		if id < 0 || id >= len(s.pl.Entries) {
			continue
		}
		e := s.pl.Entries[id]
		var g GuideNowNext
		if n, ok := boxIdx[iptv.URLPart(iptv.EntryRef(e))]; ok {
			g = GuideNowNext{Now: gpEvent(n.Now), Next: gpEvent(n.Next)}
		}
		if g.Now == nil {
			for _, sc := range scheds {
				if a, b := sc.Find(e.TvgID, e.Name, now); a != nil || b != nil {
					g = GuideNowNext{Now: gp(a), Next: gp(b)}
					break
				}
			}
		}
		if g.Now != nil || g.Next != nil {
			out[strconv.Itoa(id)] = g
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"programmes": out, "loading": loading, "sources": len(urls)})
}

// --- logos ----------------------------------------------------------------------------

// A logo address comes from a list the person set up; the browser must not
// load it itself (nothing external at runtime), so the backend fetches it,
// as a small PNG, and keeps a few hundred for a while.

const (
	netcastLogoKeep = 600
	netcastLogoTTL  = 6 * time.Hour
)

type cachedLogo struct {
	png []byte // nil: this logo cannot be had
	at  time.Time
}

type netcastLogos struct {
	mu  sync.Mutex
	m   map[string]cachedLogo
	sem chan struct{}
}

func (h *Netcast) logo(w http.ResponseWriter, r *http.Request) {
	user, _ := httpx.User(r.Context())
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	s, ok := h.Channels.iptvSession(r.URL.Query().Get("token"), user.ID)
	if err != nil || !ok || id < 0 || id >= len(s.pl.Entries) {
		http.NotFound(w, r)
		return
	}
	addr := s.pl.Entries[id].Logo
	if u, err := url.Parse(addr); addr == "" || err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		http.NotFound(w, r)
		return
	}
	png := h.logoPNG(r.Context(), addr)
	if png == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(png)
}

func (h *Netcast) logoPNG(ctx context.Context, addr string) []byte {
	l := &h.logos
	l.mu.Lock()
	if l.m == nil {
		l.m = map[string]cachedLogo{}
		l.sem = make(chan struct{}, iptvLogoParallel)
	}
	c, ok := l.m[addr]
	l.mu.Unlock()
	if ok && time.Since(c.at) < netcastLogoTTL {
		return c.png
	}
	select {
	case l.sem <- struct{}{}:
		defer func() { <-l.sem }()
	case <-ctx.Done():
		return nil
	}
	fctx, cancel := context.WithTimeout(ctx, iptvLogoEach)
	defer cancel()
	var png []byte
	if data, err := h.Channels.fetcher().FetchLimited(fctx, addr, iptvLogoMax); err == nil {
		png, _ = normalizeLogo(data)
	} else if ctx.Err() != nil {
		return nil // the page went away: not a reason to remember a failure
	}
	l.mu.Lock()
	if len(l.m) >= netcastLogoKeep {
		keys := make([]string, 0, len(l.m))
		for k := range l.m {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return l.m[keys[i]].at.Before(l.m[keys[j]].at) })
		for _, k := range keys[:len(keys)/2] {
			delete(l.m, k)
		}
	}
	l.m[addr] = cachedLogo{png: png, at: time.Now()}
	l.mu.Unlock()
	return png
}
