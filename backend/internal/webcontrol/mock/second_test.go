package mock

import (
	"context"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// The second box must differ from the reference box everywhere a view
// could mix them up.
func TestSecondDiffersFromReference(t *testing.T) {
	ctx := context.Background()
	a, b := New(), NewSecond()
	da, _ := a.DeviceInfo(ctx)
	db, _ := b.DeviceInfo(ctx)
	if da.DeviceName == db.DeviceName || len(db.Frontends) != 2 {
		t.Fatalf("device info: %q vs %q, %d tuners", da.DeviceName, db.DeviceName, len(db.Frontends))
	}
	ta, _ := a.TimerList(ctx)
	tb, _ := b.TimerList(ctx)
	if len(ta) == len(tb) || tb[0].Name == ta[0].Name {
		t.Fatalf("timers equal: %v / %v", ta, tb)
	}
	fa, _ := a.Services(ctx, favouritesTV.ServiceReference)
	fb, _ := b.Services(ctx, favouritesTV.ServiceReference)
	if fa[0].ServiceName == fb[0].ServiceName {
		t.Fatalf("favourites equal: %v", fb)
	}
	ma, _ := a.MovieList(ctx)
	mb, _ := b.MovieList(ctx)
	if ma[0].Title == mb[0].Title {
		t.Fatal("recordings equal")
	}
	// Its own guide, and its recordings are on its own file system.
	ev, _ := b.EPGSearch(ctx, "Terra")
	if len(ev) != 1 {
		t.Fatalf("search in the second box's guide: %v", ev)
	}
	if ev, _ := a.EPGSearch(ctx, "Terra"); len(ev) != 0 {
		t.Fatalf("reference box knows Terra X: %v", ev)
	}
	dir, _ := b.MovieDir(ctx, Root)
	if len(dir.Movies) != 2 {
		t.Fatalf("second box's folder: %+v", dir)
	}
	var _ webcontrol.Client = b
}
