// Package dashboard implements the personal dashboard (CONCEPT.md,
// "Dashboard (decided: tiles)"): a per-user layout of tiles, saved directly
// rather than through preview-and-confirm — like DreamHub's own settings,
// it's the app's own state, not a device change (PRINCIPLES.md, "Preview
// and confirm").
//
// Two kinds of tile: the built-in ones from a fixed catalog (see catalog
// below), one of each, and custom tiles — saved views with criteria over a
// hub's view source (CONCEPT.md: source + filters + sort + limit +
// display; internal/hub/view.go), any number up to MaxCustomTiles.
package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/antonbier/DreamHub/backend/internal/hub"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// Tile is one tile on the dashboard. For a built-in tile, Label is always
// the catalog's current value for Type — never trust or store a
// caller-supplied label. A custom tile has Type "custom", its own ID and
// Custom; its title is the user's own text, not an i18n key.
type Tile struct {
	Type   string  `json:"type"`
	ID     string  `json:"id,omitempty"`
	Label  string  `json:"label"`
	Width  int     `json:"width"`
	Hidden bool    `json:"hidden"`
	Custom *Custom `json:"custom,omitempty"`
}

// TypeCustom is the Type of every custom tile.
const TypeCustom = "custom"

// MaxCustomTiles caps custom tiles per dashboard.
const MaxCustomTiles = 12

// Custom is a custom tile's saved view.
type Custom struct {
	Title   string        `json:"title"`
	Source  string        `json:"source"`
	Query   hub.ViewQuery `json:"query"`
	Display string        `json:"display"` // "list" or "count"
}

// key identifies a tile within a layout: its type for built-ins, its ID
// for custom tiles.
func (t Tile) key() string {
	if t.Type == TypeCustom {
		return "custom:" + t.ID
	}
	return t.Type
}

// Layout is a dashboard's tiles, in display order.
type Layout struct {
	Tiles []Tile `json:"tiles"`
}

type tileType struct {
	ID           string
	Label        string
	DefaultWidth int
	// First puts the tile at the top when a stored layout is loaded without
	// it (an upgrade added it), instead of at the end. A layout being saved
	// without it still gets it at the end, keeping the caller's order.
	First bool
	// Hidden: offered in edit mode but not shown until switched on — a
	// cover row isn't part of the default layout (CONCEPT.md, DESIGN.md).
	Hidden bool
}

// catalog is the fixed v1 set of built-in tile types, each backed by data
// an existing endpoint already provides — no new data sources, only the
// layout (this type, its width, whether it's hidden, its position) is new.
var catalog = []tileType{
	// Backed by ControlHub's GET /api/v1/control/current.
	{ID: "control.nowPlaying", Label: "dashboard.tile.nowPlaying", DefaultWidth: 2, First: true},
	{ID: "timers.upcoming", Label: "dashboard.tile.timersUpcoming", DefaultWidth: 2},
	{ID: "hubs.status", Label: "dashboard.tile.hubsStatus", DefaultWidth: 2},
	{ID: "media.diskSpace", Label: "dashboard.tile.mediaDiskSpace", DefaultWidth: 1},
	{ID: "series.upcoming", Label: "dashboard.tile.seriesUpcoming", DefaultWidth: 2},
	// Backed by GET /api/v1/timers' per-timer conflict estimate.
	{ID: "timers.conflicts", Label: "dashboard.tile.timersConflicts", DefaultWidth: 2},
	// Backed by PlexHub's GET /api/v1/plex/ondeck: a cover row, so hidden
	// until switched on.
	{ID: "plex.continue", Label: "dashboard.tile.plexContinue", DefaultWidth: 4, Hidden: true},
	// Backed by StreamHub's GET /api/v1/stream/channels: channels to start
	// in the browser; hidden until switched on.
	{ID: "stream.channels", Label: "dashboard.tile.liveTv", DefaultWidth: 2, Hidden: true},
}

func catalogIndex() map[string]tileType {
	idx := make(map[string]tileType, len(catalog))
	for _, t := range catalog {
		idx[t.ID] = t
	}
	return idx
}

// ErrInvalidTile is returned by Save for an unknown tile type, a duplicate
// type, or a width outside 1-4.
var ErrInvalidTile = errors.New("dashboard: invalid tile")

// Service reads and writes dashboard layouts.
type Service struct {
	store *store.Store
	// KnownSource validates a custom tile's source (hub.Registry's
	// KnownSource); nil accepts any non-empty source.
	KnownSource func(string) bool
}

// NewService returns a Service.
func NewService(st *store.Store) *Service { return &Service{store: st} }

// dashboard returns the user's one v1 dashboard (the data model supports
// several per user — CONCEPT.md — v1 just doesn't build a switcher yet).
func (s *Service) dashboard(ctx context.Context, u store.User) (store.Dashboard, error) {
	ds, err := s.store.Dashboards(ctx, u.ID)
	if err != nil {
		return store.Dashboard{}, err
	}
	if len(ds) == 0 {
		// EnsureDefaultUser always creates one; this would mean the user
		// row exists without it, which shouldn't happen outside a bug.
		return store.Dashboard{}, store.ErrNotFound
	}
	return ds[0], nil
}

// reconcile drops tiles of a type no longer in the catalog (a removed
// feature), clamps an out-of-range width back to the type's default, and
// appends any catalog type missing from the saved layout (a newly added
// tile type after an upgrade) — so the layout degrades and grows gracefully
// instead of ever silently losing a tile forever.
func reconcile(l Layout, onLoad bool) Layout {
	idx := catalogIndex()
	seen := make(map[string]bool, len(l.Tiles))
	out := Layout{Tiles: []Tile{}}
	for _, t := range l.Tiles {
		if t.Type == TypeCustom {
			if t.ID == "" || t.Custom == nil || seen[t.key()] {
				continue
			}
			seen[t.key()] = true
			if t.Width < 1 || t.Width > 4 {
				t.Width = 2
			}
			out.Tiles = append(out.Tiles, Tile{Type: TypeCustom, ID: t.ID, Width: t.Width, Hidden: t.Hidden, Custom: t.Custom})
			continue
		}
		ct, ok := idx[t.Type]
		if !ok || seen[t.Type] {
			continue
		}
		seen[t.Type] = true
		width := t.Width
		if width < 1 || width > 4 {
			width = ct.DefaultWidth
		}
		out.Tiles = append(out.Tiles, Tile{Type: ct.ID, Label: ct.Label, Width: width, Hidden: t.Hidden})
	}
	var first []Tile
	for _, ct := range catalog {
		if !seen[ct.ID] {
			tile := Tile{Type: ct.ID, Label: ct.Label, Width: ct.DefaultWidth, Hidden: ct.Hidden}
			if ct.First && onLoad {
				first = append(first, tile)
			} else {
				out.Tiles = append(out.Tiles, tile)
			}
		}
	}
	out.Tiles = append(first, out.Tiles...)
	return out
}

func (s *Service) validateCustom(c *Custom) error {
	if c == nil {
		return fmt.Errorf("%w: custom tile without a view", ErrInvalidTile)
	}
	c.Title = strings.TrimSpace(c.Title)
	if c.Title == "" || utf8.RuneCountInString(c.Title) > 60 {
		return fmt.Errorf("%w: custom tile title must be 1-60 characters", ErrInvalidTile)
	}
	if c.Source == "" || (s.KnownSource != nil && !s.KnownSource(c.Source)) {
		return fmt.Errorf("%w: unknown source %q", ErrInvalidTile, c.Source)
	}
	if c.Display != "list" && c.Display != "count" {
		return fmt.Errorf("%w: display must be list or count", ErrInvalidTile)
	}
	if c.Query.Limit < 0 || c.Query.Limit > hub.MaxViewLimit {
		return fmt.Errorf("%w: limit must be 1-%d", ErrInvalidTile, hub.MaxViewLimit)
	}
	if utf8.RuneCountInString(c.Query.Text) > 100 {
		return fmt.Errorf("%w: text filter too long", ErrInvalidTile)
	}
	return nil
}

func newTileID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Get returns the user's dashboard layout, reconciled against the current
// tile catalog.
func (s *Service) Get(ctx context.Context, u store.User) (Layout, error) {
	d, err := s.dashboard(ctx, u)
	if err != nil {
		return Layout{}, err
	}
	var raw Layout
	_ = json.Unmarshal(d.Layout, &raw) // empty/invalid -> zero value; reconcile fills in the catalog
	return reconcile(raw, true), nil
}

// Save validates and persists a layout, and records the change (without
// preview-and-confirm — see the package doc comment). The order of l.Tiles
// is the display order; a caller-supplied Label is ignored and recomputed
// from the catalog.
func (s *Service) Save(ctx context.Context, u store.User, l Layout) (Layout, error) {
	idx := catalogIndex()
	seen := make(map[string]bool, len(l.Tiles))
	customs := 0
	for i := range l.Tiles {
		t := &l.Tiles[i]
		if t.Type == TypeCustom {
			customs++
			if customs > MaxCustomTiles {
				return Layout{}, fmt.Errorf("%w: at most %d custom tiles", ErrInvalidTile, MaxCustomTiles)
			}
			if t.ID == "" {
				t.ID = newTileID() // a tile just created in the builder
			}
			if seen[t.key()] {
				return Layout{}, fmt.Errorf("%w: duplicate custom tile %q", ErrInvalidTile, t.ID)
			}
			seen[t.key()] = true
			if err := s.validateCustom(t.Custom); err != nil {
				return Layout{}, err
			}
			if t.Width < 1 || t.Width > 4 {
				return Layout{}, fmt.Errorf("%w: width must be 1-4, got %d for custom tile", ErrInvalidTile, t.Width)
			}
			continue
		}
		ct, ok := idx[t.Type]
		if !ok {
			return Layout{}, fmt.Errorf("%w: unknown type %q", ErrInvalidTile, t.Type)
		}
		if seen[t.Type] {
			return Layout{}, fmt.Errorf("%w: duplicate type %q", ErrInvalidTile, t.Type)
		}
		seen[t.Type] = true
		if t.Width < 1 || t.Width > 4 {
			return Layout{}, fmt.Errorf("%w: width must be 1-4, got %d for %q", ErrInvalidTile, t.Width, ct.ID)
		}
	}

	d, err := s.dashboard(ctx, u)
	if err != nil {
		return Layout{}, err
	}
	reconciled := reconcile(l, false)
	enc, err := json.Marshal(reconciled)
	if err != nil {
		return Layout{}, err
	}
	if err := s.store.SaveDashboardLayout(ctx, u.ID, d.ID, enc); err != nil {
		return Layout{}, err
	}
	if err := s.store.AppendChange(ctx, store.ChangeEntry{
		UserID: u.ID, Action: "dashboard.update", Summary: "Updated dashboard layout",
	}); err != nil {
		return Layout{}, err
	}
	return reconciled, nil
}
