package hubs

import (
	"context"
	"encoding/json"
	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/logosearch"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/iptv"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Channels is ChannelHub (docs/CHANNELHUB_PLAN.md): the box's channel list
// for the phone — bouquets, every channel the box knows with "new since"
// and "in no bouquet", a channel check, and the picon sets. This file is
// the reading side (C1, C4); editing, through the optional BouquetEditor
// plugin, is in channels_edit.go.
type Channels struct {
	WC       webcontrol.Client
	Plans    *plan.Service
	Store    *store.Store
	Settings *settings.Service
	// User reads the global settings (the chosen picon set).
	User store.User
	// PiconDirs: as Guide's; nil means DefaultPiconDirs.
	PiconDirs []string
	// LogoDir is where hand-chosen channel logos are kept (channel_logos.go);
	// "" turns choosing one off. Logos finds one online; nil hides that.
	LogoDir string
	Logos   logosearch.Source
	Now     func() time.Time
	// ProviderPause is the wait between two provider reads of the daily
	// background refresh, so a box that answers one request at a time (every
	// enigma2) can serve the people using it in between; 0 means none
	// (tests). A refresh a request waits for is not slowed down by it.
	ProviderPause time.Duration

	// Boxes lists the box IDs the daily refresh visits; nil means just the
	// first box.
	Boxes func() []string

	// IPTV reads playlists from an address (channels_iptv.go); nil means a
	// default one.
	IPTV *iptv.Fetcher

	mu          sync.Mutex
	refreshedAt map[string]time.Time // per box

	iptvMu   sync.Mutex
	iptvSess []*iptvSession // playlists read so far, kept for a while
}

// KeyPiconSet is the picon set an admin chose for DreamHub's logos (a
// subfolder's name of a picon pool); "" keeps the default order.
const KeyPiconSet = "channels.piconSet"

// ChannelsSettingsSection is ChannelHub's own setting, edited from the hub
// itself (hidden in the generic settings form).
func ChannelsSettingsSection() settings.Section {
	return settings.Section{
		ID: "channels", Label: "settings.channels", Tab: settings.TabHubs,
		Fields: []settings.Field{
			{Key: KeyPiconSet, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.channels.piconSet", Default: "", Hidden: true},
		},
	}
}

// channelsRefreshEvery: how old the channel snapshot may get before
// opening ChannelHub reads the box's list again.
const channelsRefreshEvery = time.Hour

// Mount registers ChannelHub's routes.
func (h *Channels) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/channels/bouquets", h.bouquets)
	mux.HandleFunc("GET /api/v1/channels/bouquet", h.bouquet)
	mux.HandleFunc("GET /api/v1/channels/all", h.all)
	mux.HandleFunc("GET /api/v1/channels/check", h.check)
	mux.HandleFunc("GET /api/v1/channels/picons", h.picons)
	mux.HandleFunc("PUT /api/v1/channels/picons", h.choosePicons)
	mux.HandleFunc("GET /api/v1/channels/picon", h.channelPicon)
	mux.HandleFunc("GET /api/v1/channels/logo/sets", h.logoSets)
	mux.HandleFunc("GET /api/v1/channels/logo/custom", h.customLogos)
	mux.HandleFunc("GET /api/v1/channels/logo/search", h.searchLogos)
	mux.HandleFunc("GET /api/v1/channels/logo/preview", h.previewLogo)
	mux.HandleFunc("PUT /api/v1/channels/logo", h.chooseLogo)
	mux.HandleFunc("POST /api/v1/channels/logo/upload", h.uploadLogo)
	mux.HandleFunc("POST /api/v1/channels/logo/online", h.fetchLogo)
	mux.HandleFunc("DELETE /api/v1/channels/logo", h.resetLogo)
	mux.HandleFunc("POST /api/v1/channels/bouquet/plan", h.planBouquet)
	mux.HandleFunc("POST /api/v1/channels/bouquets/plan", h.planBouquets)
	mux.HandleFunc("POST /api/v1/channels/sync/plan", h.planSync)
	mux.HandleFunc("POST /api/v1/channels/iptv/read", h.readIPTV)
	mux.HandleFunc("POST /api/v1/channels/iptv/upload", h.uploadIPTV)
	mux.HandleFunc("POST /api/v1/channels/iptv/plan", h.planIPTV)
	mux.HandleFunc("GET /api/v1/channels/backups", h.backups)
	mux.HandleFunc("POST /api/v1/channels/backups/restore/plan", h.planRestore)
}

// Perms are ChannelHub's route permissions beyond the hub's own.
func (h *Channels) Perms() map[string]perm.Perm {
	return map[string]perm.Perm{
		"PUT /api/v1/channels/picons":                perm.ChannelsManage,
		"PUT /api/v1/channels/logo":                  perm.ChannelsManage,
		"POST /api/v1/channels/logo/upload":          perm.ChannelsManage,
		"POST /api/v1/channels/logo/online":          perm.ChannelsManage,
		"DELETE /api/v1/channels/logo":               perm.ChannelsManage,
		"GET /api/v1/channels/logo/search":           perm.ChannelsManage,
		"GET /api/v1/channels/logo/preview":          perm.ChannelsManage,
		"POST /api/v1/channels/bouquet/plan":         perm.ChannelsManage,
		"POST /api/v1/channels/bouquets/plan":        perm.ChannelsManage,
		"POST /api/v1/channels/sync/plan":            perm.ChannelsManage,
		"POST /api/v1/channels/iptv/read":            perm.ChannelsManage,
		"POST /api/v1/channels/iptv/upload":          perm.ChannelsManage,
		"POST /api/v1/channels/iptv/plan":            perm.ChannelsManage,
		"POST /api/v1/channels/backups/restore/plan": perm.ChannelsManage,
	}
}

// Init applies the saved picon set; called once at startup.
func (h *Channels) Init(ctx context.Context) {
	SetLogoDir(h.LogoDir)
	if v, err := h.Settings.Values(ctx, h.User); err == nil {
		SetPreferredPiconSet(v.String(KeyPiconSet))
	}
}

// pacedRefresh marks the background refresh in a context: it leaves room for
// other requests between its own; one a person waits for does not.
type pacedRefresh struct{}

// Watch refreshes the channel snapshot once a day, so "new since" also
// knows channels from a scan nobody looked at in ChannelHub.
func (h *Channels) Watch(ctx context.Context, logger *slog.Logger) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			ids := []string{""}
			if h.Boxes != nil {
				ids = h.Boxes()
			}
			for _, id := range ids {
				bctx := ctx
				if id != "" {
					bctx = box.With(ctx, id)
				}
				if err := h.refresh(context.WithValue(bctx, pacedRefresh{}, true)); err != nil && logger != nil {
					logger.Warn("channel snapshot refresh failed", "box", id, "error", err)
				}
			}
			timer.Reset(24 * time.Hour)
		}
	}
}

// --- bouquets ---------------------------------------------------------------

// BouquetSummary is one bouquet in the list.
type BouquetSummary struct {
	Ref   string `json:"ref"`
	Name  string `json:"name"`
	Kind  string `json:"kind"` // "tv" or "radio"
	Count int    `json:"count"`
}

// allBouquets lists the TV bouquets, then the radio ones.
func (h *Channels) allBouquets(ctx context.Context) ([]BouquetSummary, error) {
	tv, err := h.WC.Bouquets(ctx)
	if err != nil {
		return nil, err
	}
	radio, _ := h.WC.Services(ctx, webcontrol.RadioRoot) // a box may have no radio bouquets
	var out []BouquetSummary
	for kind, list := range map[string][]webcontrol.Service{"tv": tv, "radio": radio} {
		for _, b := range list {
			if webcontrol.IsMarker(b.ServiceReference) {
				continue
			}
			out = append(out, BouquetSummary{Ref: b.ServiceReference, Name: b.ServiceName, Kind: kind})
		}
	}
	// TV first, each in the box's own order.
	order := map[string]int{}
	for i, b := range append(append([]webcontrol.Service{}, tv...), radio...) {
		order[b.ServiceReference] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind == "tv"
		}
		return order[out[i].Ref] < order[out[j].Ref]
	})
	return out, nil
}

func (h *Channels) bouquets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := h.allBouquets(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	for i := range list {
		if services, err := h.WC.Services(ctx, list[i].Ref); err == nil {
			list[i].Count = countChannels(services)
		}
	}
	editor := false
	if p, ok := h.WC.(webcontrol.BouquetEditorProber); ok {
		editor = p.BouquetEditorAvailable(ctx)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"bouquets": list, "editor": editor})
}

func countChannels(services []webcontrol.Service) int {
	n := 0
	for _, s := range services {
		if !webcontrol.IsMarker(s.ServiceReference) {
			n++
		}
	}
	return n
}

// BouquetEntry is one line of a bouquet: a channel or a marker (heading).
type BouquetEntry struct {
	Ref     string `json:"ref"`
	Name    string `json:"name"`
	Marker  bool   `json:"marker,omitempty"`
	Quality string `json:"quality,omitempty"`
	Picon   bool   `json:"picon"`
	// Now is the programme on now, when the box has EPG for the channel.
	Now *webcontrol.EPGEvent `json:"now,omitempty"`
}

func (h *Channels) bouquet(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_query", "ref is required")
		return
	}
	ctx := r.Context()
	services, err := h.WC.Services(ctx, ref)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	now := map[string]*webcontrol.EPGEvent{}
	if hasServices(services) { // see Radio.stations: an empty bouquet breaks epgnownext
		nn, _ := h.WC.EPGNowNext(ctx, ref)
		for _, n := range nn {
			if n.Now != nil {
				now[channelKey(n.ServiceReference)] = n.Now
			}
		}
	}
	dirs := piconSearchDirs(h.piconDirs())
	entries := make([]BouquetEntry, 0, len(services))
	for _, s := range services {
		e := BouquetEntry{Ref: s.ServiceReference, Name: s.ServiceName}
		if webcontrol.IsMarker(s.ServiceReference) {
			e.Marker = true
		} else {
			e.Quality = webcontrol.Quality(s.ServiceReference)
			e.Picon = hasLogo(dirs, s.ServiceReference, s.ServiceName)
			e.Now = now[channelKey(s.ServiceReference)]
		}
		entries = append(entries, e)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// channelKey identifies a channel by its reference's first ten fields —
// how the box itself matches a bouquet entry to a channel (a bouquet may
// add a name or URL after them).
func channelKey(ref string) string {
	parts := strings.Split(ref, ":")
	if len(parts) > 10 {
		parts = parts[:10]
	}
	return strings.ToUpper(strings.Join(parts, ":"))
}

// --- every channel ------------------------------------------------------------

// refresh reads every channel the box knows, with its provider, into the
// snapshot.
func (h *Channels) refresh(ctx context.Context) error {
	now := h.now()
	var seen []store.SeenChannel
	for _, kind := range []struct {
		name      string
		all       string
		providers string
	}{{"tv", webcontrol.TVAll, webcontrol.TVProviders}, {"radio", webcontrol.RadioAll, webcontrol.RadioProviders}} {
		all, err := h.WC.Services(ctx, kind.all)
		if err != nil {
			return err
		}
		provider := map[string]string{}
		if providers, err := h.WC.Services(ctx, kind.providers); err == nil {
			failures := 0
			for _, p := range providers {
				list, err := h.WC.Services(ctx, p.ServiceReference)
				if err != nil {
					// A box that stops answering is not asked a hundred more
					// times (a VU+ Solo2 with a full satellite list went away
					// under this, 2026-10-21): the names are kept, the
					// providers of the rest are left out until the next run.
					if failures++; failures >= 3 || ctx.Err() != nil {
						break
					}
					continue
				}
				failures = 0
				for _, s := range list {
					provider[channelKey(s.ServiceReference)] = p.ServiceName
				}
				if h.ProviderPause > 0 && ctx.Value(pacedRefresh{}) != nil {
					select {
					case <-ctx.Done():
					case <-time.After(h.ProviderPause):
					}
				}
			}
		}
		for _, s := range all {
			if webcontrol.IsMarker(s.ServiceReference) || unnamedService(s.ServiceName) {
				continue
			}
			seen = append(seen, store.SeenChannel{ServiceRef: s.ServiceReference, Name: s.ServiceName,
				Provider: provider[channelKey(s.ServiceReference)], Kind: kind.name})
		}
	}
	if err := h.Store.RecordChannels(ctx, seen, now.Unix()); err != nil {
		return err
	}
	h.mu.Lock()
	if h.refreshedAt == nil {
		h.refreshedAt = map[string]time.Time{}
	}
	h.refreshedAt[box.Key(ctx)] = now
	h.mu.Unlock()
	return nil
}

func (h *Channels) refreshIfStale(ctx context.Context, force bool) error {
	h.mu.Lock()
	stale := force || h.now().Sub(h.refreshedAt[box.Key(ctx)]) > channelsRefreshEvery
	h.mu.Unlock()
	if !stale {
		return nil
	}
	return h.refresh(ctx)
}

// ChannelView is one channel the box knows.
type ChannelView struct {
	Ref       string `json:"ref"`
	Name      string `json:"name"`
	Provider  string `json:"provider,omitempty"`
	Kind      string `json:"kind"`
	Quality   string `json:"quality,omitempty"`
	FirstSeen int64  `json:"firstSeen"`
	// New: not in DreamHub's first look at the box — appeared since.
	New bool `json:"new,omitempty"`
	// Gone: the box listed it once and doesn't anymore.
	Gone     bool     `json:"gone,omitempty"`
	Bouquets []string `json:"bouquets"`
}

// membership maps a channel key to the names of the bouquets it's in.
func (h *Channels) membership(ctx context.Context) (map[string][]string, error) {
	list, err := h.allBouquets(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, b := range list {
		services, err := h.WC.Services(ctx, b.Ref)
		if err != nil {
			continue
		}
		for _, s := range services {
			if !webcontrol.IsMarker(s.ServiceReference) {
				k := channelKey(s.ServiceReference)
				if !contains(out[k], b.Name) {
					out[k] = append(out[k], b.Name)
				}
			}
		}
	}
	return out, nil
}

func (h *Channels) all(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.refreshIfStale(ctx, r.URL.Query().Get("refresh") == "1"); err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not read the box's channel list")
		return
	}
	snapshot, latest, err := h.Store.SeenChannels(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read the channel snapshot")
		return
	}
	in, err := h.membership(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	// "Since": when DreamHub first looked — what "new" is measured against.
	var since, refreshed int64
	for _, c := range snapshot {
		if c.FirstLook == 1 && (since == 0 || c.FirstSeen < since) {
			since = c.FirstSeen
		}
		if c.LastSeen > refreshed {
			refreshed = c.LastSeen
		}
	}
	out := make([]ChannelView, 0, len(snapshot))
	for _, c := range snapshot {
		b := in[channelKey(c.ServiceRef)]
		if b == nil {
			b = []string{}
		}
		out = append(out, ChannelView{Ref: c.ServiceRef, Name: c.Name, Provider: c.Provider, Kind: c.Kind,
			Quality: webcontrol.Quality(c.ServiceRef), FirstSeen: c.FirstSeen,
			New: c.FirstLook > 1, Gone: c.LastLook < latest, Bouquets: b})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"channels": out, "refreshedAt": refreshed, "since": since})
}

// --- the channel check --------------------------------------------------------

// ChannelIssue is one thing in a bouquet worth a look.
type ChannelIssue struct {
	// Kind: "duplicate" (twice in one bouquet), "gone" (the box no longer
	// knows it), "noEpg" (no programme now — TV only), "noPicon".
	Kind       string `json:"kind"`
	BouquetRef string `json:"bouquetRef"`
	Bouquet    string `json:"bouquet"`
	Ref        string `json:"ref"`
	Name       string `json:"name"`
}

func (h *Channels) check(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := h.allBouquets(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	known := map[string]bool{}
	if err := h.refreshIfStale(ctx, false); err == nil {
		if snapshot, latest, err := h.Store.SeenChannels(ctx); err == nil {
			for _, c := range snapshot {
				if c.LastLook == latest {
					known[channelKey(c.ServiceRef)] = true
				}
			}
		}
	}
	dirs := piconSearchDirs(h.piconDirs())
	issues := []ChannelIssue{}
	for _, b := range list {
		services, err := h.WC.Services(ctx, b.Ref)
		if err != nil {
			continue
		}
		withNow := map[string]bool{}
		if b.Kind == "tv" && hasServices(services) {
			nn, _ := h.WC.EPGNowNext(ctx, b.Ref)
			for _, n := range nn {
				if n.Now != nil {
					withNow[channelKey(n.ServiceReference)] = true
				}
			}
		}
		count := map[string]int{}
		for _, s := range services {
			if webcontrol.IsMarker(s.ServiceReference) {
				continue
			}
			k := channelKey(s.ServiceReference)
			count[k]++
			add := func(kind string) {
				issues = append(issues, ChannelIssue{Kind: kind, BouquetRef: b.Ref, Bouquet: b.Name, Ref: s.ServiceReference, Name: s.ServiceName})
			}
			switch {
			case count[k] == 2:
				add("duplicate")
			case count[k] > 2:
				// Reported once.
			case len(known) > 0 && !known[k]:
				add("gone")
			default:
				if b.Kind == "tv" && !withNow[k] {
					add("noEpg")
				}
				if !hasLogo(dirs, s.ServiceReference, s.ServiceName) {
					add("noPicon")
				}
			}
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"issues": issues})
}

// --- picons ---------------------------------------------------------------------

func (h *Channels) piconDirs() []string {
	if h.PiconDirs != nil {
		return h.PiconDirs
	}
	return DefaultPiconDirs
}

func (h *Channels) picon(w http.ResponseWriter, r *http.Request) { servePicon(w, r, h.PiconDirs) }

// hasLogo: a hand-chosen logo, or one of dirs' picons.
func hasLogo(dirs []string, ref, name string) bool {
	return hasLogoOverride(ref) || hasPiconIn(dirs, ref, name)
}

// hasPiconIn reports whether one of dirs has a picon file for a channel —
// only whether the file is there, for counting many at once.
func hasPiconIn(dirs []string, ref, name string) bool {
	for _, file := range piconNames(ref, name) {
		for _, dir := range dirs {
			if fi, err := os.Lstat(filepath.Join(dir, file)); err == nil && fi.Mode().IsRegular() && fi.Size() > 0 {
				return true
			}
		}
	}
	return false
}

// PiconSet is one installed picon set and how many of the bouquets'
// channels it has a logo for.
type PiconSet struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Covered int    `json:"covered"`
}

// piconSets lists the sets: each pool's subfolders, plus a pool itself
// when logos lie directly in it.
func (h *Channels) piconSets() []PiconSet {
	var out []PiconSet
	for _, d := range h.piconDirs() {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		direct := false
		for _, e := range entries {
			if e.IsDir() {
				out = append(out, PiconSet{Name: e.Name(), Path: filepath.Join(d, e.Name())})
			} else if strings.HasSuffix(strings.ToLower(e.Name()), ".png") {
				direct = true
			}
		}
		if direct {
			out = append(out, PiconSet{Name: filepath.Base(d), Path: d})
		}
	}
	return out
}

func (h *Channels) picons(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := h.allBouquets(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	channels := map[string]webcontrol.Service{}
	for _, b := range list {
		services, err := h.WC.Services(ctx, b.Ref)
		if err != nil {
			continue
		}
		for _, s := range services {
			if !webcontrol.IsMarker(s.ServiceReference) {
				channels[channelKey(s.ServiceReference)] = s
			}
		}
	}
	sets := h.piconSets()
	for i := range sets {
		for _, s := range channels {
			if hasPiconIn([]string{sets[i].Path}, s.ServiceReference, s.ServiceName) {
				sets[i].Covered++
			}
		}
	}
	sort.SliceStable(sets, func(i, j int) bool { return sets[i].Covered > sets[j].Covered })
	if sets == nil {
		sets = []PiconSet{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"sets": sets, "total": len(channels), "chosen": preferredPicon()})
}

// choosePicons saves the set DreamHub takes logos from first — DreamHub's
// own setting, saved directly like any other.
func (h *Channels) choosePicons(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Set string `json:"set"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"set": "<name>"} ("" for automatic)`)
		return
	}
	if body.Set != "" {
		found := false
		for _, s := range h.piconSets() {
			found = found || s.Name == body.Set
		}
		if !found {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no picon set with this name")
			return
		}
	}
	user, _ := httpx.User(r.Context())
	raw, _ := json.Marshal(body.Set)
	if err := h.Settings.Update(r.Context(), user, map[string]json.RawMessage{KeyPiconSet: raw}); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not save the choice")
		return
	}
	SetPreferredPiconSet(body.Set)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"chosen": body.Set})
}

func (h *Channels) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}
