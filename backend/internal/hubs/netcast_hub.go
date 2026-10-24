package hubs

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/iptv"
	"github.com/antonbier/DreamHub/backend/internal/netcast"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Netcast is the page of its own for the Gemini Netcast plugin's sources
// (docs/NETCAST_PLAN.md, "A page of its own"): browse a source, play a
// channel on the box now, or hand the list to the channel list's IPTV tab to
// put channels into a bouquet. It owns no data: sources, lists and the
// kept sessions are ChannelHub's (channels_netcast*.go), reached through the
// same Channels value, so a list read here has the token the IPTV tab's plan
// needs.
type Netcast struct {
	Channels *Channels
	// VavooProxy makes sure the Vavoo addon's proxy answers before a Vavoo
	// channel is played (internal/vavooproxy); nil does nothing. A failure
	// does not stop the play — the box may have a proxy of its own — but is
	// reported to the page.
	VavooProxy func(ctx context.Context) error

	mu      sync.Mutex
	present map[string]presence // per box

	guide netcastGuideState
	logos netcastLogos
}

type presence struct {
	ok bool
	at time.Time
}

// presenceTTL is how long "this box has Netcast (or not)" is believed.
const presenceTTL = time.Minute

// Present says whether the box of ctx has Netcast: its source files are
// there. The page is left out of the navigation on a box without (a Solo2).
// A box that cannot be asked counts as having it — nothing is hidden on a
// guess — and that answer is not kept.
func (h *Netcast) Present(ctx context.Context) bool {
	rd, ok := h.Channels.WC.(webcontrol.BoxFileReader)
	if !ok {
		return false
	}
	key := box.Key(ctx)
	h.mu.Lock()
	p, seen := h.present[key]
	h.mu.Unlock()
	if seen && time.Since(p.at) < presenceTTL {
		return p.ok
	}
	view, err := readNetcast(ctx, rd)
	if err != nil {
		return true
	}
	h.mu.Lock()
	if h.present == nil {
		h.present = map[string]presence{}
	}
	h.present[key] = presence{ok: view.Available, at: time.Now()}
	h.mu.Unlock()
	return view.Available
}

// Mount registers the routes under /api/v1/netcast.
func (h *Netcast) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/netcast/sources", h.sources)
	mux.HandleFunc("POST /api/v1/netcast/read", h.read)
	mux.HandleFunc("POST /api/v1/netcast/play", h.play)
	mux.HandleFunc("POST /api/v1/netcast/guide", h.guideHandler)
	mux.HandleFunc("GET /api/v1/netcast/logo", h.logo)
}

// Perms: reading a list is for everyone who may see the hub (it only keeps
// the list for the session); playing switches the TV, which is ControlHub's
// `control`.
func (h *Netcast) Perms() map[string]perm.Perm {
	return map[string]perm.Perm{
		"POST /api/v1/netcast/read":  perm.Watch,
		"POST /api/v1/netcast/guide": perm.Watch,
		"POST /api/v1/netcast/play":  perm.Control,
	}
}

// play switches the box to a channel of a list read with /netcast/read: the
// reference is built the way an entry of a bouquet would be (the plugin's own
// kind for a Netcast entry, so it keeps resolving the address), without
// touching any bouquet. The address and any login stay on the server.
func (h *Netcast) play(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token   string `json:"token"`
		ID      *int   `json:"id"`
		Variant *int   `json:"variant"` // another stream of the channel (Vavoo)
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || body.Token == "" || body.ID == nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"token","id"}`)
		return
	}
	user, _ := httpx.User(r.Context())
	s, ok := h.Channels.iptvSession(body.Token, user.ID)
	if !ok {
		httpx.WriteError(w, r, http.StatusNotFound, CodeIPTVSessionGone, "that list is no longer kept; open the source again")
		return
	}
	if *body.ID < 0 || *body.ID >= len(s.pl.Entries) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "unknown channel")
		return
	}
	e := s.pl.Entries[*body.ID]
	ref := iptv.EntryRef(e)
	if body.Variant != nil {
		if *body.Variant < 0 || *body.Variant >= len(e.Alts) {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "unknown variant")
			return
		}
		ref = iptv.Ref(e.Alts[*body.Variant].URL)
	}
	out := map[string]any{"played": e.Name}
	if h.VavooProxy != nil && strings.HasPrefix(iptv.URLPart(ref), iptv.URLPart(iptv.Ref(netcastVavooProxy))) {
		if err := h.VavooProxy(r.Context()); err != nil {
			out["warning"] = "vavoo_proxy"
		}
	}
	if err := h.Channels.WC.Zap(r.Context(), ref); err != nil {
		writeControlError(w, r, "zap", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// NetcastSourceView is a source as the page lists it, with what can be done
// with it: State is "ready" (channels can be listed now), "fetch" (Netcast has
// not downloaded the list yet; DreamHub loads it from the address the source
// has) or "waiting" (no list yet and none DreamHub may fetch: a login, or a
// service Netcast loads itself), "unsupported" (no channel list DreamHub can
// use: Vavoo, films and series).
type NetcastSourceView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Desc      string `json:"desc,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	Host      string `json:"host,omitempty"`
	HasLogin  bool   `json:"hasLogin,omitempty"`
	State     string `json:"state"`
	// NoBouquet: the channels play from this page only (Vavoo's go through a
	// proxy of the plugin), so there is no "put into a bouquet".
	NoBouquet bool `json:"noBouquet,omitempty"`
}

// netcastApis are the sources whose channel lists the plugin caches in a form
// DreamHub reads (channels_netcast_lists.go).
var netcastApis = []string{netcastSamsungID, netcastPlutoID}

func (h *Netcast) sources(w http.ResponseWriter, r *http.Request) {
	out := struct {
		Available bool                `json:"available"`
		Sources   []NetcastSourceView `json:"sources"`
	}{Sources: []NetcastSourceView{}}
	rd, ok := h.Channels.WC.(webcontrol.BoxFileReader)
	if !ok {
		httpx.WriteJSON(w, http.StatusOK, out)
		return
	}
	view, err := readNetcast(r.Context(), rd)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	out.Available = view.Available
	// What the plugin has cached: asked file by file (a listing of the
	// folder shows media files only on a Dreambox). Reading one byte more
	// than allowed tells "there" from "not there" without downloading it; a
	// box that cannot be asked counts as having it, and the read says otherwise.
	ctx := r.Context()
	has := func(name string) bool {
		_, err := rd.ReadBoxFileMax(ctx, path.Join(netcastCacheDir, name), 1)
		return err == nil || !errors.Is(err, webcontrol.ErrNotFound)
	}
	for _, f := range view.Files {
		if f.Kind != "server" {
			continue
		}
		for _, s := range f.Sources {
			if s.ID == "" {
				continue
			}
			out.Sources = append(out.Sources, NetcastSourceView{ID: s.ID, Name: s.Name, Desc: s.Desc, MediaType: s.MediaType,
				Host: iptv.Host(s.Address), HasLogin: s.HasLogin, State: netcastSourceState(s, has), NoBouquet: s.ID == netcastVavooID})
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func netcastSourceState(s netcast.Source, has func(name string) bool) string {
	if s.ID == netcastVavooID {
		if has(netcastVavooFile) {
			return "ready"
		}
		return "waiting"
	}
	for _, id := range netcastApis {
		if s.ID == id {
			if id == netcastPlutoID {
				if has("epgChannelList_netcastserver_" + id + ".json") {
					return "ready"
				}
			} else if has("netcastserver_" + id) {
				return "ready"
			}
			return "waiting"
		}
	}
	if strings.HasSuffix(s.ID, "api") || strings.HasPrefix(s.ID, "vavoo") || !strings.HasPrefix(s.MediaType, "playlist") {
		return "unsupported"
	}
	switch {
	case has("playlist_netcastserver_" + s.ID):
		return "ready"
	case s.Fetch != "":
		return "fetch"
	}
	return "waiting"
}

func (h *Netcast) read(w http.ResponseWriter, r *http.Request) {
	h.Channels.openNetcastList(w, r, true)
}
