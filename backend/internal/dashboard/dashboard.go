// Package dashboard implements the personal dashboard (CONCEPT.md,
// "Dashboard (decided: tiles)"): a per-user layout of tiles, saved directly
// rather than through preview-and-confirm — like DreamHub's own settings,
// it's the app's own state, not a device change (PRINCIPLES.md, "Preview
// and confirm").
//
// v1 has a fixed catalog of built-in tile types (see catalog below); custom
// tiles as saved views over a hub's search results are a later addition
// once hubs contribute search providers (ROADMAP.md, hub contract).
package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

// Tile is one tile on the dashboard. Label is always the catalog's current
// value for Type — never trust or store a caller-supplied label.
type Tile struct {
	Type   string `json:"type"`
	Label  string `json:"label"`
	Width  int    `json:"width"`
	Hidden bool   `json:"hidden"`
}

// Layout is a dashboard's tiles, in display order.
type Layout struct {
	Tiles []Tile `json:"tiles"`
}

type tileType struct {
	ID           string
	Label        string
	DefaultWidth int
}

// catalog is the fixed v1 set of built-in tile types, each backed by data
// an existing endpoint already provides — no new data sources, only the
// layout (this type, its width, whether it's hidden, its position) is new.
var catalog = []tileType{
	{ID: "timers.upcoming", Label: "dashboard.tile.timersUpcoming", DefaultWidth: 2},
	{ID: "hubs.status", Label: "dashboard.tile.hubsStatus", DefaultWidth: 2},
	{ID: "media.diskSpace", Label: "dashboard.tile.mediaDiskSpace", DefaultWidth: 1},
	{ID: "series.upcoming", Label: "dashboard.tile.seriesUpcoming", DefaultWidth: 2},
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
func reconcile(l Layout) Layout {
	idx := catalogIndex()
	seen := make(map[string]bool, len(l.Tiles))
	out := Layout{Tiles: []Tile{}}
	for _, t := range l.Tiles {
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
	for _, ct := range catalog {
		if !seen[ct.ID] {
			out.Tiles = append(out.Tiles, Tile{Type: ct.ID, Label: ct.Label, Width: ct.DefaultWidth, Hidden: false})
		}
	}
	return out
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
	return reconcile(raw), nil
}

// Save validates and persists a layout, and records the change (without
// preview-and-confirm — see the package doc comment). The order of l.Tiles
// is the display order; a caller-supplied Label is ignored and recomputed
// from the catalog.
func (s *Service) Save(ctx context.Context, u store.User, l Layout) (Layout, error) {
	idx := catalogIndex()
	seen := make(map[string]bool, len(l.Tiles))
	for _, t := range l.Tiles {
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
	reconciled := reconcile(l)
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
