package hubs

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/boxsettings"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// BoxSettings shows the receiver's own setup menus (docs/BOXSETTINGS_PLAN.md,
// S1): read-only, on DeviceHub's admin side. The box is read once and
// kept for boxSettingsKeep, so opening a section or searching does not ask
// the box again — it answers one request at a time and the whole list is
// 18 sections.
type BoxSettings struct {
	WC  webcontrol.Client
	Now func() time.Time

	mu    sync.Mutex
	cache map[string]cachedCatalog // per box
}

type cachedCatalog struct {
	at  time.Time
	cat *boxsettings.Catalog
}

// boxSettingsKeep is how long a read of the box's setup menus is reused.
const boxSettingsKeep = 10 * time.Minute

// maxSettingsHits caps one search.
const maxSettingsHits = 60

// Mount registers the routes.
func (h *BoxSettings) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/boxsettings", h.overview)
	mux.HandleFunc("GET /api/v1/boxsettings/sections/{key}", h.section)
	mux.HandleFunc("GET /api/v1/boxsettings/search", h.search)
}

func (h *BoxSettings) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// errBoxSettingsUnsupported: the box's client or the box itself has no
// setup menus to read; errBoxSettingsNeedsPlugin: a Dreambox whose DreamHub
// plugin is missing or too old to serve them (S2).
var (
	errBoxSettingsUnsupported = errors.New("boxsettings: not supported by this box")
	errBoxSettingsNeedsPlugin = errors.New("boxsettings: the box's DreamHub plugin has no settings route")
)

// unavailable maps what a read of the box failed with to one of the two
// "cannot show" errors, or nil for a real failure.
func unavailable(err error) error {
	switch {
	case errors.Is(err, webcontrol.ErrNeedsPlugin):
		return errBoxSettingsNeedsPlugin
	case errors.Is(err, webcontrol.ErrNotFound):
		return errBoxSettingsUnsupported
	}
	return nil
}

// catalog reads the box's setup menus, or reuses the last read.
func (h *BoxSettings) catalog(ctx context.Context, force bool) (*boxsettings.Catalog, error) {
	reader, ok := h.WC.(webcontrol.BoxSettingsReader)
	if !ok {
		return nil, errBoxSettingsUnsupported
	}
	key := box.Key(ctx)
	h.mu.Lock()
	defer h.mu.Unlock() // one read at a time: a second caller reuses the first's
	if c, ok := h.cache[key]; ok && !force && h.now().Sub(c.at) < boxSettingsKeep {
		return c.cat, nil
	}
	sections, err := reader.ConfigSections(ctx)
	if err != nil {
		if u := unavailable(err); u != nil {
			return nil, u
		}
		return nil, err
	}
	items := make(map[string][]webcontrol.ConfigItem, len(sections))
	for _, s := range sections {
		list, err := reader.ConfigItems(ctx, s.Key)
		if err != nil {
			if u := unavailable(err); u != nil {
				return nil, u
			}
			return nil, err
		}
		items[s.Key] = list
	}
	// The setup files add level, restart and help; without them the list
	// is still shown, just without those.
	setup, err := reader.SetupDefinition(ctx)
	if err != nil && !errors.Is(err, webcontrol.ErrNotFound) {
		setup = nil
	}
	cat := boxsettings.Build(sections, items, setup)
	if h.cache == nil {
		h.cache = map[string]cachedCatalog{}
	}
	h.cache[key] = cachedCatalog{at: h.now(), cat: cat}
	return cat, nil
}

// load answers the error cases the same way for every route; ok is false
// when it has already written the answer.
func (h *BoxSettings) load(w http.ResponseWriter, r *http.Request, unsupported func(reason string)) (*boxsettings.Catalog, bool) {
	cat, err := h.catalog(r.Context(), r.URL.Query().Get("refresh") == "1")
	switch {
	case errors.Is(err, errBoxSettingsUnsupported):
		unsupported("unsupported")
		return nil, false
	case errors.Is(err, errBoxSettingsNeedsPlugin):
		unsupported("plugin")
		return nil, false
	case err != nil:
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach the box")
		return nil, false
	}
	return cat, true
}

// overview: {available, writes, boxLevel, boxLevelName, sections}.
// available is false for a box that cannot show its setup menus; reason
// says why: "plugin" (a Dreambox whose DreamHub plugin has no settings route)
// or "unsupported".
func (h *BoxSettings) overview(w http.ResponseWriter, r *http.Request) {
	cat, ok := h.load(w, r, func(reason string) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"available": false, "reason": reason, "writes": false, "sections": []boxsettings.SectionInfo{}})
	})
	if !ok {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"available":    true,
		"writes":       false,
		"boxLevel":     cat.BoxLevel,
		"boxLevelName": cat.BoxLevelName,
		"sections":     cat.Sections(),
	})
}

func (h *BoxSettings) section(w http.ResponseWriter, r *http.Request) {
	cat, ok := h.load(w, r, func(string) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "this box has no setup menus to show")
	})
	if !ok {
		return
	}
	info, items, found := cat.Section(r.PathValue("key"))
	if !found {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such section")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"section": info, "items": items})
}

func (h *BoxSettings) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "q is required")
		return
	}
	cat, ok := h.load(w, r, func(reason string) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"available": false, "reason": reason, "hits": []boxsettings.Hit{}})
	})
	if !ok {
		return
	}
	hits := cat.Search(q, maxSettingsHits)
	if hits == nil {
		hits = []boxsettings.Hit{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"available": true, "hits": hits})
}
