package dashboard

import (
	"context"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

func newTestService(t *testing.T) (*Service, store.User) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	u, err := st.EnsureDefaultUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(st), u
}

func TestGet_FreshUserGetsTheFullCatalog(t *testing.T) {
	s, u := newTestService(t)
	l, err := s.Get(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Tiles) != len(catalog) {
		t.Fatalf("got %d tiles, want %d (the full catalog)", len(l.Tiles), len(catalog))
	}
	for _, tile := range l.Tiles {
		if want := tile.Type == "plex.continue" || tile.Type == "stream.channels" || tile.Type == "media.recent"; tile.Hidden != want {
			t.Errorf("tile %q: hidden %v, want %v (only the optional tiles start hidden)", tile.Type, tile.Hidden, want)
		}
		if tile.Label == "" {
			t.Errorf("tile %q has no label", tile.Type)
		}
	}
}

func TestSave_RoundTripsOrderWidthAndHidden(t *testing.T) {
	s, u := newTestService(t)
	ctx := context.Background()

	saved, err := s.Save(ctx, u, Layout{Tiles: []Tile{
		{Type: "hubs.status", Width: 4, Hidden: true},
		{Type: "timers.upcoming", Width: 1, Hidden: false},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Tiles) != len(catalog) || saved.Tiles[0].Type != "hubs.status" || saved.Tiles[1].Type != "timers.upcoming" {
		t.Fatalf("order not preserved: %+v", saved.Tiles)
	}
	if !saved.Tiles[0].Hidden || saved.Tiles[0].Width != 4 {
		t.Fatalf("hidden/width not preserved: %+v", saved.Tiles[0])
	}

	got, err := s.Get(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tiles) != len(catalog) || got.Tiles[0].Type != "hubs.status" || !got.Tiles[0].Hidden || got.Tiles[0].Width != 4 {
		t.Fatalf("Get after Save = %+v", got.Tiles)
	}
}

func TestSave_RejectsUnknownType(t *testing.T) {
	s, u := newTestService(t)
	if _, err := s.Save(context.Background(), u, Layout{Tiles: []Tile{{Type: "made.up", Width: 2}}}); err == nil {
		t.Fatal("want an error for an unknown tile type")
	}
}

func TestSave_RejectsDuplicateType(t *testing.T) {
	s, u := newTestService(t)
	l := Layout{Tiles: []Tile{
		{Type: "timers.upcoming", Width: 2},
		{Type: "timers.upcoming", Width: 1},
	}}
	if _, err := s.Save(context.Background(), u, l); err == nil {
		t.Fatal("want an error for a duplicate tile type")
	}
}

func TestSave_RejectsWidthOutOfRange(t *testing.T) {
	s, u := newTestService(t)
	for _, width := range []int{0, 5, -1} {
		l := Layout{Tiles: []Tile{{Type: "timers.upcoming", Width: width}}}
		if _, err := s.Save(context.Background(), u, l); err == nil {
			t.Errorf("width %d: want an error", width)
		}
	}
}

func TestSave_LabelIsAlwaysRecomputedFromCatalog(t *testing.T) {
	s, u := newTestService(t)
	saved, err := s.Save(context.Background(), u, Layout{Tiles: []Tile{
		{Type: "timers.upcoming", Label: "not the real label", Width: 2},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Tiles[0].Label != "dashboard.tile.timersUpcoming" {
		t.Fatalf("label = %q, want the catalog label, not the caller-supplied one", saved.Tiles[0].Label)
	}
}

func TestReconcile_DropsUnknownTypeAndAppendsMissingCatalogEntries(t *testing.T) {
	l := reconcile(Layout{Tiles: []Tile{
		{Type: "timers.upcoming", Width: 3, Hidden: true},
		{Type: "removed.feature", Width: 2}, // simulates a tile type removed from a later catalog
	}}, true)
	types := map[string]Tile{}
	for _, t := range l.Tiles {
		types[t.Type] = t
	}
	if _, ok := types["removed.feature"]; ok {
		t.Error("an unknown/removed tile type must be dropped, not carried forward")
	}
	if tile := types["timers.upcoming"]; tile.Width != 3 || !tile.Hidden {
		t.Errorf("known tile's saved width/hidden must be preserved: %+v", tile)
	}
	if _, ok := types["hubs.status"]; !ok {
		t.Error("a catalog type missing from the saved layout must be appended, not lost")
	}
}

func TestReconcile_ClampsOutOfRangeWidthToDefault(t *testing.T) {
	l := reconcile(Layout{Tiles: []Tile{{Type: "timers.upcoming", Width: 99}}}, false)
	if want := catalogIndex()["timers.upcoming"].DefaultWidth; l.Tiles[0].Width != want {
		t.Errorf("width = %d, want the catalog default %d", l.Tiles[0].Width, want)
	}
}

// A tile type added by an upgrade shows up in an existing layout — on top
// for "Now on the box" (First) when loaded; when a layout is saved without
// it, after the caller's own tiles.
func TestReconcile_NewFirstTileGoesOnTopOnLoad(t *testing.T) {
	old := Layout{Tiles: []Tile{{Type: "timers.upcoming", Width: 2}, {Type: "hubs.status", Width: 2}}}
	got := reconcile(old, true)
	if got.Tiles[0].Type != "control.nowPlaying" || got.Tiles[1].Type != "timers.upcoming" || got.Tiles[2].Type != "hubs.status" {
		t.Fatalf("on load: %+v", got.Tiles)
	}
	n := 0
	for _, tile := range got.Tiles {
		if tile.Type == "control.nowPlaying" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected it once, got %d: %+v", n, got.Tiles)
	}
	saved := reconcile(old, false)
	if saved.Tiles[0].Type != "timers.upcoming" || saved.Tiles[1].Type != "hubs.status" || saved.Tiles[2].Type != "control.nowPlaying" {
		t.Fatalf("on save the caller's order stays first: %+v", saved.Tiles)
	}
}
