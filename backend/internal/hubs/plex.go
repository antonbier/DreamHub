package hubs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/enrich"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/plex"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Plex is PlexHub, "Plex Mediathek": the films and series of a Plex
// server (internal/plex), their watched state (read and set in Plex
// itself), and films/episodes played on the box straight from the
// server's file — no Plex client on the box involved.
//
// Plex servers are often not always on (CLAUDE.md). Everything read is
// kept in the store (plex_cache) and served from there while the server
// doesn't answer; then nothing can be played or marked, and the answer
// says so (online: false, syncedAt).
type Plex struct {
	Client   plex.Client
	Settings *settings.Service
	Store    *store.Store
	Plans    *plan.Service
	WC       webcontrol.Client
	Images   *enrich.ImageCache
	// User reads the (global) Plex settings for the background sync.
	User store.User
	// StreamBase is how the box reaches DreamHub itself, e.g.
	// "http://127.0.0.1:8080" — set only when DreamHub runs on the box;
	// empty means films can't be played there.
	StreamBase string
	Now        func() time.Time
	Logger     *slog.Logger

	mu       sync.Mutex
	lastTry  time.Time
	lastErr  error
	server   plex.Server
	syncing  bool
	pins     map[int64]*pinState
	tickets  map[string]ticket
	clientID string
	// active is the address in use when it isn't plex.url: a fallback
	// that answered when plex.url didn't (failover).
	active  string
	probing bool
}

// Error codes.
const (
	CodePlexNotConfigured = "plex_not_configured"
	CodePlexOffline       = "plex_offline"
	CodePlexUnauthorized  = "plex_unauthorized"
	CodePlexNotFound      = "plex_not_found"
	CodePlexBadAnswer     = "plex_bad_answer"
)

// offlineBackoff: after a failed request, further reads go straight to the
// cache for this long instead of waiting for the server each time.
const offlineBackoff = time.Minute

// Perms are PlexHub's route permissions beyond the hub's own (internal/perm).
func (h *Plex) Perms() map[string]perm.Perm {
	return map[string]perm.Perm{
		"POST /api/v1/plex/items/{key}/play":         perm.PlexUse,
		"POST /api/v1/plex/items/{key}/watched":      perm.PlexUse,
		"POST /api/v1/plex/items/{key}/watched/plan": perm.PlexUse,
		"POST /api/v1/plex/sync":                     perm.PlexUse,
		"PUT /api/v1/plex/libraries":                 perm.SettingsGlobal,
		"POST /api/v1/plex/login":                    perm.SettingsGlobal,
		"GET /api/v1/plex/login/{id}":                perm.SettingsGlobal,
		"POST /api/v1/plex/login/{id}/use":           perm.SettingsGlobal,
	}
}

// syncEvery is the background sync's period while the server answers.
const syncEvery = 15 * time.Minute

func (h *Plex) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/plex/status", h.status)
	mux.HandleFunc("POST /api/v1/plex/sync", h.syncNow)
	mux.HandleFunc("GET /api/v1/plex/libraries", h.libraries)
	mux.HandleFunc("PUT /api/v1/plex/libraries", h.selectLibraries)
	mux.HandleFunc("GET /api/v1/plex/libraries/{id}/items", h.items)
	mux.HandleFunc("GET /api/v1/plex/items/{key}", h.item)
	mux.HandleFunc("GET /api/v1/plex/ondeck", h.onDeck)
	mux.HandleFunc("GET /api/v1/plex/image", h.image)
	mux.HandleFunc("POST /api/v1/plex/items/{key}/watched", h.setWatched)
	mux.HandleFunc("POST /api/v1/plex/items/{key}/watched/plan", h.planWatched)
	mux.HandleFunc("POST /api/v1/plex/items/{key}/play", h.play)
	mux.HandleFunc("POST /api/v1/plex/login", h.loginStart)
	mux.HandleFunc("GET /api/v1/plex/login/{id}", h.loginCheck)
	mux.HandleFunc("POST /api/v1/plex/login/{id}/use", h.loginUse)
}

// config is what the Plex settings say right now.
type plexConfig struct {
	conn plex.Conn
	// addresses: plex.url, then the fallbacks — conn.URL is one of them.
	addresses []string
	libraries map[string]bool // nil: every film and series library
	playOnBox bool
}

func (c plexConfig) configured() bool { return c.conn.URL != "" && c.conn.Token != "" }

func (h *Plex) config(ctx context.Context) (plexConfig, error) {
	v, err := h.Settings.Values(ctx, h.User)
	if err != nil {
		return plexConfig{}, err
	}
	c := plexConfig{
		conn:      plex.Conn{URL: strings.TrimRight(v.String(connector.KeyPlexURL), "/"), Token: v.String(connector.KeyPlexToken)},
		playOnBox: v.Bool(connector.KeyPlexPlayOnBox),
	}
	if c.conn.URL != "" {
		c.addresses = []string{c.conn.URL}
		for _, u := range connector.PlexFallbackURLs(v.String(connector.KeyPlexFallbackURLs)) {
			if !contains(c.addresses, u) {
				c.addresses = append(c.addresses, u)
			}
		}
	}
	h.mu.Lock()
	if h.active != "" && contains(c.addresses, h.active) {
		c.conn.URL = h.active
	}
	h.mu.Unlock()
	if ids := strings.TrimSpace(v.String(connector.KeyPlexLibraries)); ids != "" {
		c.libraries = map[string]bool{}
		for _, id := range strings.Split(ids, ",") {
			if id = strings.TrimSpace(id); id != "" {
				c.libraries[id] = true
			}
		}
	}
	return c, nil
}

// --- online state -------------------------------------------------------

// note records how the last request to the server went.
func (h *Plex) note(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastTry = h.Now()
	// Only "no connection" and "token refused" mean offline. Anything else
	// — not found, an answer DreamHub can't read, a server error — came
	// from a server that's there; it's logged, not taken for offline (that
	// would send every read to the cache for a minute).
	if err != nil && !errors.Is(err, plex.ErrUnreachable) && !errors.Is(err, plex.ErrUnauthorized) {
		if !errors.Is(err, plex.ErrNotFound) && h.Logger != nil {
			h.Logger.Warn("plex answer not usable", "err", err)
		}
		err = nil
	}
	h.lastErr = err
}

// failover looks for an address the server answers at when the one in use
// doesn't: plex.url first, then the fallbacks — the server's LAN address
// next to one behind a reverse proxy, say (on the reference box, the
// public address stopped answering the box for an hour after a reboot
// while the server itself was fine). An address only counts when the same
// server answers there. It returns the address now in use.
func (h *Plex) failover(ctx context.Context, c plexConfig) (string, bool) {
	var known string
	h.cacheGet(ctx, "machine", &known)
	for _, u := range c.addresses {
		if u == c.conn.URL {
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		id, err := h.Client.Identity(pctx, plex.Conn{URL: u, Token: c.conn.Token})
		cancel()
		if err != nil || (known != "" && id.MachineIdentifier != known) {
			continue
		}
		h.mu.Lock()
		h.active = u
		h.lastErr, h.lastTry = nil, time.Time{}
		h.mu.Unlock()
		if h.Logger != nil {
			h.Logger.Info("plex: switched address", "from", c.conn.URL, "to", u)
		}
		return u, true
	}
	return "", false
}

// failoverSoon runs failover in the background after a read found the
// server unreachable, once at a time; the read itself answers from the
// cache meanwhile.
func (h *Plex) failoverSoon(c plexConfig) {
	if len(c.addresses) < 2 {
		return
	}
	h.mu.Lock()
	if h.probing {
		h.mu.Unlock()
		return
	}
	h.probing = true
	h.mu.Unlock()
	go func() {
		defer func() {
			h.mu.Lock()
			h.probing = false
			h.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		h.failover(ctx, c)
	}()
}

// worthTrying reports whether to ask the server now, or go straight to the
// cache because it just failed.
func (h *Plex) worthTrying() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastErr == nil || h.Now().Sub(h.lastTry) > offlineBackoff
}

func (h *Plex) online() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.lastTry.IsZero() && h.lastErr == nil
}

// errState names the last failure for clients: "", "unreachable" or
// "unauthorized".
func (h *Plex) errState() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.lastErr == nil:
		return ""
	case errors.Is(h.lastErr, plex.ErrUnauthorized):
		return "unauthorized"
	default:
		return "unreachable"
	}
}

// --- cache --------------------------------------------------------------

func (h *Plex) cacheGet(ctx context.Context, key string, out any) (int64, bool) {
	data, at, ok, err := h.Store.PlexCache(ctx, key)
	if err != nil || !ok || json.Unmarshal([]byte(data), out) != nil {
		return 0, false
	}
	return at, true
}

func (h *Plex) cachePut(ctx context.Context, key string, v any) int64 {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	at := h.Now().Unix()
	if err := h.Store.PutPlexCache(ctx, key, string(b), at); err != nil && h.Logger != nil {
		h.Logger.Warn("plex cache write failed", "key", key, "err", err)
	}
	return at
}

// read answers from the server when it's worth trying (and caches the
// answer under key), else — or when it fails — from the cache. syncedAt
// is when the data was read from the server.
func read[T any](ctx context.Context, h *Plex, c plexConfig, key string, fetch func(context.Context) (T, error)) (data T, syncedAt int64, fresh bool, err error) {
	if h.worthTrying() {
		fctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		v, ferr := fetch(fctx)
		cancel()
		h.note(ferr)
		if ferr == nil {
			return v, h.cachePut(ctx, key, v), true, nil
		}
		if errors.Is(ferr, plex.ErrNotFound) {
			return data, 0, false, ferr
		}
		if errors.Is(ferr, plex.ErrUnreachable) {
			h.failoverSoon(c)
		}
		err = ferr
	}
	if at, ok := h.cacheGet(ctx, key, &data); ok {
		return data, at, false, nil
	}
	if err == nil {
		err = plex.ErrUnreachable
	}
	return data, 0, false, err
}

// --- sync ---------------------------------------------------------------

// Sync reads the libraries, every chosen library's films/series and
// "continue watching" into the cache. A different server than last time
// (another machine identifier) starts the cache over.
func (h *Plex) Sync(ctx context.Context) error {
	c, err := h.config(ctx)
	if err != nil {
		return err
	}
	if !c.configured() {
		return errNotConfigured
	}
	// After a restart: introduce DreamHub to Plex under its saved
	// identifier, not anonymously.
	_, _ = h.ensureClientID(ctx)
	h.mu.Lock()
	if h.syncing {
		h.mu.Unlock()
		return nil
	}
	h.syncing = true
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.syncing = false
		h.mu.Unlock()
	}()

	id, err := h.Client.Identity(ctx, c.conn)
	if errors.Is(err, plex.ErrUnreachable) {
		if u, ok := h.failover(ctx, c); ok {
			c.conn.URL = u
			id, err = h.Client.Identity(ctx, c.conn)
		}
	}
	if err != nil {
		h.note(err)
		return err
	}
	var known string
	if _, ok := h.cacheGet(ctx, "machine", &known); ok && known != id.MachineIdentifier {
		if err := h.Store.ClearPlexCache(ctx); err != nil {
			return err
		}
	}
	h.cachePut(ctx, "machine", id.MachineIdentifier)
	srv, err := h.Client.Server(ctx, c.conn)
	h.note(err)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.server = srv
	h.mu.Unlock()
	h.cachePut(ctx, "server", srv)

	libs, err := h.Client.Libraries(ctx, c.conn)
	h.note(err)
	if err != nil {
		return err
	}
	h.cachePut(ctx, "libraries", libs)
	for _, l := range libs {
		if !shown(l, c) {
			continue
		}
		items, err := h.Client.Items(ctx, c.conn, l.ID)
		h.note(err)
		if err != nil {
			return err
		}
		h.cachePut(ctx, "items:"+l.ID, items)
	}
	deck, err := h.Client.OnDeck(ctx, c.conn)
	h.note(err)
	if err == nil {
		h.cachePut(ctx, "ondeck", deck)
	}
	return err
}

var errNotConfigured = errors.New("plex: not configured")

// shown reports whether a library is shown: films and series only, and
// only the chosen ones when there's a choice.
func shown(l plex.Library, c plexConfig) bool {
	if l.Type != "movie" && l.Type != "show" {
		return false
	}
	return c.libraries == nil || c.libraries[l.ID]
}

// Watch syncs shortly after start and then every syncEvery, as long as
// Plex is set up. Reading only — nothing in Plex changes.
func (h *Plex) Watch(ctx context.Context) {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if err := h.Sync(ctx); err != nil && !errors.Is(err, errNotConfigured) && h.Logger != nil {
			h.Logger.Info("plex sync failed", "err", err)
		}
		timer.Reset(syncEvery)
	}
}

// --- read endpoints -----------------------------------------------------

// meta is what every read answer says about where its data came from.
type plexMeta struct {
	Online   bool  `json:"online"`
	SyncedAt int64 `json:"syncedAt,omitempty"` // Unix seconds
}

func (h *Plex) meta(syncedAt int64) plexMeta { return plexMeta{Online: h.online(), SyncedAt: syncedAt} }

// requireConfig answers 409 plex_not_configured when Plex isn't set up.
func (h *Plex) requireConfig(w http.ResponseWriter, r *http.Request) (plexConfig, bool) {
	c, err := h.config(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read settings")
		return c, false
	}
	if !c.configured() {
		httpx.WriteError(w, r, http.StatusConflict, CodePlexNotConfigured, "Plex isn't set up (Settings → Services → Plex)")
		return c, false
	}
	return c, true
}

func (h *Plex) writeReadError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, plex.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, CodePlexNotFound, "no such item on the Plex server")
	case errors.Is(err, plex.ErrUnauthorized):
		httpx.WriteError(w, r, http.StatusBadGateway, CodePlexUnauthorized, "the Plex server rejected the token")
	case errors.Is(err, plex.ErrUnreachable):
		httpx.WriteError(w, r, http.StatusBadGateway, CodePlexOffline, "the Plex server doesn't answer and nothing is cached yet")
	default:
		httpx.WriteError(w, r, http.StatusBadGateway, CodePlexBadAnswer, "the Plex server's answer couldn't be used: "+err.Error())
	}
}

func (h *Plex) status(w http.ResponseWriter, r *http.Request) {
	c, err := h.config(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read settings")
		return
	}
	out := map[string]any{"configured": c.configured(), "canPlay": c.playOnBox && h.StreamBase != ""}
	if c.configured() {
		// A cheap live check when it's worth one, so the page opens with
		// the server's real state.
		if h.worthTrying() {
			sctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
			srv, err := h.Client.Server(sctx, c.conn)
			cancel()
			h.note(err)
			if err == nil {
				h.mu.Lock()
				h.server = srv
				h.mu.Unlock()
				h.cachePut(r.Context(), "server", srv)
			}
		}
		var srv plex.Server
		h.cacheGet(r.Context(), "server", &srv)
		_, syncedAt, _, _ := h.Store.PlexCache(r.Context(), "libraries")
		h.mu.Lock()
		out["syncing"] = h.syncing
		h.mu.Unlock()
		out["online"] = h.online()
		out["error"] = h.errState()
		out["server"] = srv
		out["syncedAt"] = syncedAt
		out["url"] = c.conn.URL
		var machine string
		if _, ok := h.cacheGet(r.Context(), "machine", &machine); ok {
			out["machineId"] = machine // with url: the link to open an item in Plex itself
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Plex) syncNow(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireConfig(w, r); !ok {
		return
	}
	// A manual sync tries the server even right after a failure.
	h.mu.Lock()
	h.lastErr = nil
	h.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := h.Sync(ctx); err != nil {
		h.writeReadError(w, r, err)
		return
	}
	h.status(w, r)
}

// libraryView is a library as the hub lists it.
type libraryView struct {
	plex.Library
	Count    int  `json:"count"`
	Selected bool `json:"selected"`
}

func (h *Plex) libraries(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireConfig(w, r)
	if !ok {
		return
	}
	libs, at, _, err := read(r.Context(), h, c, "libraries", func(ctx context.Context) ([]plex.Library, error) {
		return h.Client.Libraries(ctx, c.conn)
	})
	if err != nil {
		h.writeReadError(w, r, err)
		return
	}
	out := []libraryView{}
	for _, l := range libs {
		if l.Type != "movie" && l.Type != "show" {
			continue
		}
		v := libraryView{Library: l, Selected: shown(l, c)}
		var items []plex.Item
		if _, ok := h.cacheGet(r.Context(), "items:"+l.ID, &items); ok {
			v.Count = len(items)
		}
		out = append(out, v)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"libraries": out, "meta": h.meta(at)})
}

// selectLibraries saves which libraries are shown — DreamHub's own
// setting, so directly, like any other setting (and logged).
func (h *Plex) selectLibraries(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Selected []string `json:"selected"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || len(body.Selected) == 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"selected": [library IDs, at least one]}`)
		return
	}
	var libs []plex.Library
	h.cacheGet(r.Context(), "libraries", &libs)
	all := true
	for _, l := range libs {
		if (l.Type == "movie" || l.Type == "show") && !contains(body.Selected, l.ID) {
			all = false
		}
	}
	value := strings.Join(body.Selected, ",")
	if all {
		value = "" // every library, including ones added later
	}
	raw, _ := json.Marshal(value)
	user, _ := httpx.User(r.Context())
	if err := h.Settings.Update(r.Context(), user, map[string]json.RawMessage{connector.KeyPlexLibraries: raw}); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	_ = h.Store.AppendChange(r.Context(), store.ChangeEntry{UserID: user.ID, Action: "plex.libraries", Summary: "Plex libraries shown: " + strings.Join(body.Selected, ", ")})
	go h.Sync(context.WithoutCancel(r.Context()))
	h.libraries(w, r)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// itemView is an item with its watched state worked out.
type itemView struct {
	plex.Item
	Watched bool `json:"watched"`
}

func views(items []plex.Item) []itemView {
	out := make([]itemView, len(items))
	for i, it := range items {
		out[i] = itemView{Item: it, Watched: it.Watched()}
	}
	return out
}

func (h *Plex) items(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireConfig(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	items, at, _, err := read(r.Context(), h, c, "items:"+id, func(ctx context.Context) ([]plex.Item, error) {
		return h.Client.Items(ctx, c.conn, id)
	})
	if err != nil {
		h.writeReadError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views(items), "meta": h.meta(at)})
}

// item is one film, series, season or episode; a series lists its
// seasons, a season its episodes.
func (h *Plex) item(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireConfig(w, r)
	if !ok {
		return
	}
	key := r.PathValue("key")
	it, at, _, err := read(r.Context(), h, c, "item:"+key, func(ctx context.Context) (plex.Item, error) {
		return h.Client.Item(ctx, c.conn, key)
	})
	if err != nil {
		h.writeReadError(w, r, err)
		return
	}
	out := map[string]any{"item": itemView{Item: it, Watched: it.Watched()}}
	if it.Type == "show" || it.Type == "season" {
		children, cat, _, err := read(r.Context(), h, c, "children:"+key, func(ctx context.Context) ([]plex.Item, error) {
			return h.Client.Children(ctx, c.conn, key)
		})
		if err == nil {
			out["children"] = views(children)
			at = min(at, cat)
		} else {
			out["children"] = []itemView{}
		}
	}
	out["meta"] = h.meta(at)
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Plex) onDeck(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireConfig(w, r)
	if !ok {
		return
	}
	deck, at, _, err := read(r.Context(), h, c, "ondeck", func(ctx context.Context) ([]plex.Item, error) {
		return h.Client.OnDeck(ctx, c.conn)
	})
	if err != nil {
		h.writeReadError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views(deck), "meta": h.meta(at)})
}

// imagePath is the only kind of path the image proxy passes on: an item's
// own pictures on the Plex server.
var imagePath = regexp.MustCompile(`^/library/metadata/\d+/(thumb|art|banner|clearLogo)(/\d+)?$`)

// imageWidths are the sizes the frontend asks for.
var imageWidths = map[int]bool{120: true, 240: true, 360: true, 720: true, 1280: true}

// image serves an item's picture, scaled by the server, from DreamHub's
// disk cache once fetched — the browser never talks to Plex, and pictures
// stay visible while the server is off. The path carries Plex's update
// stamp, so a cached picture never goes stale.
func (h *Plex) image(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	width, _ := strconv.Atoi(r.URL.Query().Get("w"))
	if !imagePath.MatchString(path) || !imageWidths[width] {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "not a Plex picture")
		return
	}
	height := width * 3 / 2 // posters
	switch {
	case strings.Contains(path, "/art"):
		height = width * 9 / 16
	case strings.Contains(path, "/clearLogo"):
		height = width * 3 / 8 // logos are wide; the server keeps the ratio
	}
	size := "w" + strconv.Itoa(width)
	if h.Images != nil {
		if b, ctype, ok := h.Images.Get(size, path); ok {
			writeImage(w, b, ctype)
			return
		}
	}
	c, ok := h.requireConfig(w, r)
	if !ok {
		return
	}
	if !h.worthTrying() {
		httpx.WriteError(w, r, http.StatusNotFound, CodePlexOffline, "the Plex server is off and the picture isn't cached")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	b, _, err := h.Client.Image(ctx, c.conn, path, width, height)
	if err != nil {
		if !errors.Is(err, plex.ErrNotFound) {
			h.note(err)
		}
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such picture")
		return
	}
	if h.Images != nil {
		h.Images.Put(size, path, b)
	}
	writeImage(w, b, http.DetectContentType(b))
}

func writeImage(w http.ResponseWriter, b []byte, ctype string) {
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "private, max-age=604800")
	w.Write(b)
}

// Search implements hub.SearchProvider over the cached films and series of
// the shown libraries — works while the server is off too.
func (h *Plex) Search(ctx context.Context, query string) ([]hub.SearchResult, error) {
	c, err := h.config(ctx)
	if err != nil || !c.configured() {
		return nil, err
	}
	var libs []plex.Library
	h.cacheGet(ctx, "libraries", &libs)
	q := strings.ToLower(query)
	var out []hub.SearchResult
	for _, l := range libs {
		if !shown(l, c) {
			continue
		}
		var items []plex.Item
		h.cacheGet(ctx, "items:"+l.ID, &items)
		for _, it := range items {
			if !strings.Contains(strings.ToLower(it.Title), q) {
				continue
			}
			sub := l.Title
			if it.Year > 0 {
				sub += " · " + strconv.Itoa(it.Year)
			}
			out = append(out, hub.SearchResult{Title: it.Title, Subtitle: sub, Path: "/plex?item=" + it.RatingKey})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	if len(out) > 20 {
		out = out[:20]
	}
	return out, nil
}

// InLibrary returns a check for whether a film with exactly this title is
// already in a shown Plex library (#32, "already in Plex" next to
// broadcasts in GuideHub/DiscoveryHub) — cached, like Search, so it works
// while the server is off too. Exact match only, the same rule MediaHub's
// own enrichment lookup uses; a broadcast's EPG title and Plex's rarely
// agree on anything looser (subtitles, year suffixes) without false
// positives. The cache is read once per call, not once per title: a guide
// grid asks for every broadcast, and decoding the libraries per broadcast
// made a 26-channel grid take 4 s on the reference box. A nil Plex (not
// set up) never matches.
func (h *Plex) InLibrary(ctx context.Context) func(title string) bool {
	never := func(string) bool { return false }
	if h == nil {
		return never
	}
	c, err := h.config(ctx)
	if err != nil || !c.configured() {
		return never
	}
	titles := map[string]bool{}
	var libs []plex.Library
	h.cacheGet(ctx, "libraries", &libs)
	for _, l := range libs {
		if l.Type != "movie" || !shown(l, c) {
			continue
		}
		var items []plex.Item
		h.cacheGet(ctx, "items:"+l.ID, &items)
		for _, it := range items {
			titles[strings.ToLower(it.Title)] = true
		}
	}
	return func(title string) bool { return title != "" && titles[strings.ToLower(title)] }
}

func jsonMarshal(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// randomHex returns n random bytes as hex.
func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
