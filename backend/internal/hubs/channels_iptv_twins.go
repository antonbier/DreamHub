package hubs

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/iptv"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// A guide for an internet channel (docs/NETCAST_PLAN.md, "Linking a guide",
// way A): its reference carries the identity of a channel of the box that has
// events — "Das Erste" from the list gets the guide of "Das Erste HD" — and
// enigma2 shows that channel's programme. Nothing is guessed silently:
//
//	remembered — a match the person made before
//	sure       — the names are the same (quality suffixes aside) and one channel fits
//	candidates — similar names; the person looks and decides
//
// and a channel can be matched by hand with a search over every channel of
// the box, which shows what each candidate is showing now.

const iptvMaxTwinIDs = 5000

type twinService struct {
	ref  string
	name string
	key  string
	hd   bool
}

type iptvTwinView struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
	HD   bool   `json:"hd,omitempty"`
}

func (s twinService) view() iptvTwinView { return iptvTwinView{Ref: s.ref, Name: s.name, HD: s.hd} }

// twinKey makes names comparable for matching a list's channel with the
// box's: "Das Erste HD" and "Das Erste" are the same station.
func twinKey(name string) string {
	n := normName(name)
	for trimmed := true; trimmed; {
		trimmed = false
		for _, suffix := range []string{"uhd", "fhd", "hd", "sd", "4k"} {
			if len(n) > len(suffix)+2 && strings.HasSuffix(n, suffix) {
				n, trimmed = n[:len(n)-len(suffix)], true
			}
		}
	}
	return n
}

// twinServices lists the box's channels that can lend their guide, by name.
func (h *Channels) twinServices(ctx context.Context) ([]twinService, map[string]twinService, error) {
	tc, err := h.knownChannels(ctx)
	if err != nil {
		return nil, nil, err
	}
	var all []twinService
	byKey := map[string]twinService{}
	for _, s := range tc.byKey {
		if !strings.HasPrefix(s.ServiceReference, "1:0:") {
			continue // headings, bouquets and internet channels themselves
		}
		q := webcontrol.Quality(s.ServiceReference)
		ts := twinService{ref: s.ServiceReference, name: s.ServiceName, key: twinKey(s.ServiceName), hd: q == "hd" || q == "uhd"}
		all = append(all, ts)
		byKey[channelKey(s.ServiceReference)] = ts
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].name != all[j].name {
			return strings.ToLower(all[i].name) < strings.ToLower(all[j].name)
		}
		return all[i].ref < all[j].ref
	})
	return all, byKey, nil
}

type iptvTwinSuggestion struct {
	ID         int            `json:"id"`
	Remembered *iptvTwinView  `json:"remembered,omitempty"`
	Sure       *iptvTwinView  `json:"sure,omitempty"`
	Candidates []iptvTwinView `json:"candidates"`
}

// suggestTwin works out what to offer for one channel of the list.
func suggestTwin(id int, e iptv.Entry, all []twinService, byKey map[string]twinService, remembered map[string]store.IPTVTwin) iptvTwinSuggestion {
	out := iptvTwinSuggestion{ID: id, Candidates: []iptvTwinView{}}
	if t, ok := remembered[iptvIdentHex(iptvKey(e))]; ok {
		if s, ok := byKey[channelKey(t.ServiceRef)]; ok {
			v := s.view()
			out.Remembered = &v
		}
	}
	key := twinKey(e.Name)
	if len(key) < 2 {
		return out
	}
	var exact, partial []twinService
	for _, s := range all {
		switch {
		case s.key == key:
			exact = append(exact, s)
		case len(key) >= 4 && len(s.key) >= 4 && (strings.Contains(s.key, key) || strings.Contains(key, s.key)):
			partial = append(partial, s)
		}
	}
	switch {
	case len(exact) == 1:
		v := exact[0].view()
		out.Sure = &v
	case len(exact) > 1:
		var hd []twinService
		for _, s := range exact {
			if s.hd {
				hd = append(hd, s)
			}
		}
		if len(hd) == 1 {
			v := hd[0].view()
			out.Sure = &v
		}
	}
	sort.SliceStable(partial, func(i, j int) bool { return len(partial[i].key) < len(partial[j].key) })
	for _, s := range append(exact, partial...) {
		if len(out.Candidates) == 5 {
			break
		}
		out.Candidates = append(out.Candidates, s.view())
	}
	return out
}

func (h *Channels) suggestIPTVTwins(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
		IDs   []int  `json:"ids"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"token", "ids": [...]}`)
		return
	}
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	sess, found := h.iptvSession(req.Token, user.ID)
	if !found {
		httpx.WriteError(w, r, http.StatusGone, CodeIPTVSessionGone, "the list was read too long ago; read it again")
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > iptvMaxTwinIDs {
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "nothing was suggested", map[string]httpx.FieldError{"ids": {Code: "invalid"}})
		return
	}
	for _, id := range req.IDs {
		if id < 0 || id >= len(sess.pl.Entries) {
			httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "nothing was suggested", map[string]httpx.FieldError{"ids": {Code: "invalid"}})
			return
		}
	}
	all, byKey, err := h.twinServices(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	remembered, err := h.Store.IPTVTwins(ctx)
	if err != nil {
		slog.Warn("could not read the remembered guide matches", "err", err)
	}
	out := make([]iptvTwinSuggestion, 0, len(req.IDs))
	for _, id := range req.IDs {
		out = append(out, suggestTwin(id, sess.pl.Entries[id], all, byKey, remembered))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"suggestions": out})
}

type iptvTwinHit struct {
	iptvTwinView
	Now *iptvNow `json:"now"`
}

type iptvNow struct {
	Title string `json:"title"`
	Start int64  `json:"start"` // Unix seconds, UTC
	End   int64  `json:"end"`
}

// searchIPTVTwins finds a channel of the box by name, with what it shows now,
// so the person can see whether it is the right station.
func (h *Channels) searchIPTVTwins(w http.ResponseWriter, r *http.Request) {
	q := normName(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": []iptvTwinHit{}})
		return
	}
	ctx := r.Context()
	all, _, err := h.twinServices(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	var hits []twinService
	for _, s := range all {
		if strings.Contains(normName(s.name), q) {
			hits = append(hits, s)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		pi, pj := strings.HasPrefix(normName(hits[i].name), q), strings.HasPrefix(normName(hits[j].name), q)
		if pi != pj {
			return pi
		}
		return len(hits[i].name) < len(hits[j].name)
	})
	if len(hits) > 20 {
		hits = hits[:20]
	}
	now := h.nowOnBox(ctx, hits)
	out := make([]iptvTwinHit, 0, len(hits))
	for _, s := range hits {
		out = append(out, iptvTwinHit{iptvTwinView: s.view(), Now: now[channelKey(s.ref)]})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": out})
}

// nowOnBox reads what the given channels show now: one call per kind (TV,
// radio), only for the kinds among them. A box without a guide answers
// nothing; the hits are shown without it.
func (h *Channels) nowOnBox(ctx context.Context, svcs []twinService) map[string]*iptvNow {
	out := map[string]*iptvNow{}
	var tv, radio bool
	for _, s := range svcs {
		if parts := strings.Split(s.ref, ":"); len(parts) > 2 && (parts[2] == "2" || strings.EqualFold(parts[2], "A")) {
			radio = true
		} else {
			tv = true
		}
	}
	for _, root := range []struct {
		ref string
		use bool
	}{{webcontrol.TVAll, tv}, {webcontrol.RadioAll, radio}} {
		if !root.use {
			continue
		}
		list, err := h.WC.EPGNowNext(ctx, root.ref)
		if err != nil {
			continue
		}
		for _, n := range list {
			if n.Now != nil && n.Now.Title != "" {
				out[channelKey(n.ServiceReference)] = &iptvNow{Title: n.Now.Title, Start: n.Now.Start, End: n.Now.Start + n.Now.Duration}
			}
		}
	}
	return out
}

// rememberTwins keeps the guide matches that were just used.
func (h *Channels) rememberTwins(ctx context.Context, twins map[string]store.IPTVTwin) {
	if len(twins) == 0 {
		return
	}
	if err := h.Store.SaveIPTVTwins(ctx, twins); err != nil {
		slog.Warn("could not remember the guide matches", "err", err)
	}
}
