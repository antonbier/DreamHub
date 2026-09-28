package hubs

import (
	"context"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/store"
	tvmazemock "github.com/antonbier/DreamHub/backend/internal/tvmaze/mock"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

func TestMatchesSeries(t *testing.T) {
	cases := []struct {
		title, series string
		want          bool
	}{
		{"Tatort", "Tatort", true},
		{"Tatort: Borowski und das Haus am Meer", "Tatort", true},
		{"Bones - Die Knochenjägerin", "Bones", true},
		{"The Rookie (5)", "The Rookie", true},
		{"Die Verräter – Vertraue Niemandem!", "Die Verräter - Vertraue Niemandem!", true},
		{"die verrater vertraue niemandem", "Die Verräter - Vertraue Niemandem!", true},
		{"Tatort-Reiniger Spezial", "Tatort", false},
		{"Tatort X-Files", "Tatort", false},
		{"The Rookie: Feds", "The Rookie", true}, // a spin-off reads like an episode title; accepted
		{"Bauer sucht Frau", "Tatort", false},
		{"Tatort", "", false},
	}
	for _, c := range cases {
		if got := matchesSeries(c.title, c.series); got != c.want {
			t.Errorf("matchesSeries(%q, %q) = %v, want %v", c.title, c.series, got, c.want)
		}
	}
}

func TestParseEpisodeCode(t *testing.T) {
	for in, want := range map[string][2]int{
		"S01E02 - Pilot":         {1, 2},
		"Folge s2026e31":         {2026, 31},
		"S3 E7":                  {3, 7},
		"Borowski und das Haus":  {0, 0},
		"ASS01E02 is not a code": {0, 0},
	} {
		s, e := parseEpisodeCode(in)
		if s != want[0] || e != want[1] {
			t.Errorf("parseEpisodeCode(%q) = %d/%d, want %d/%d", in, s, e, want[0], want[1])
		}
	}
}

func TestDedupeTitles(t *testing.T) {
	got := dedupeTitles([]string{"Tatort: Borowski", "tatort – Der Fall", "", "Bones - Folge 3", "Bones"}, true)
	if len(got) != 2 || got[0] != "Tatort: Borowski"[:6] || got[1] != "Bones" {
		t.Errorf("episodes collapse to their series: %q", got)
	}
	got = dedupeTitles([]string{"Die Verräter - Vertraue Niemandem!"}, false)
	if len(got) != 1 || got[0] != "Die Verräter - Vertraue Niemandem!" {
		t.Errorf("series names stay whole: %q", got)
	}
}

// The three ways SerienRecorder files recordings on the reference box.
func TestSeriesRecordings_RealLayouts(t *testing.T) {
	box := mock.New()
	meta := func(title, desc string) string {
		return "1:0:1:445C:453:1:C00000:0:0:0:\n" + title + "\n" + desc + "\n1745960400\n"
	}
	put := func(p, title, desc string) {
		box.FS.Put(mock.Root+p+".ts", "rec")
		box.FS.Put(mock.Root+p+".ts.meta", meta(title, desc))
	}
	// A series' own record folder, with season folders below.
	put("serien/FBI Most Wanted/Season 5/20250429 2300 - SAT_1 - FBI_ Most Wanted - S05E07 - Gefangenenübergabe", "FBI: Most Wanted - S05E07 - Gefangenenübergabe", "S05E07 - Gefangenenübergabe")
	put("serien/FBI Most Wanted/Season 4/x", "FBI: Most Wanted - S04E01 - Pilot", "S04E01 - Pilot")
	// A series folder with its start year, inside a recording folder.
	put("Gute Zeiten, schlechte Zeiten (1992)/Season 0/y", "Gute Zeiten, schlechte Zeiten", "S00E8615")
	// Directly in the recording folder, by title.
	put("20251001 1940 - RTL - GZSZ", "Gute Zeiten, schlechte Zeiten", "Folge 8616")

	h := &Series{WC: box, FS: box.FS}
	ctx := context.Background()

	fbi, err := h.seriesRecordings(ctx, serienrecorder.Marker{Name: "FBI: Most Wanted", RecordFolder: mock.Root + "serien/FBI Most Wanted"})
	if err != nil || len(fbi) != 2 || fbi[0].Season == 0 || fbi[0].Episode == 0 {
		t.Fatalf("everything below the series' own folder, with codes: %+v %v", fbi, err)
	}
	gzsz, err := h.seriesRecordings(ctx, serienrecorder.Marker{Name: "Gute Zeiten, schlechte Zeiten", RecordFolder: mock.Root})
	if err != nil || len(gzsz) != 2 {
		t.Fatalf("the year-suffixed series folder and the title match: %+v %v", gzsz, err)
	}
	// A marker pointing at a folder that's gone finds nothing, not an error.
	gone, err := h.seriesRecordings(ctx, serienrecorder.Marker{Name: "Grey's Anatomy", RecordFolder: mock.Root + "serien/Grey"})
	if err != nil || len(gone) != 0 {
		t.Errorf("missing folder: %+v %v", gone, err)
	}
	// Off the box (no file system), the same through /web/movielist.
	off := &Series{WC: box}
	fbi2, err := off.seriesRecordings(ctx, serienrecorder.Marker{Name: "FBI: Most Wanted", RecordFolder: mock.Root + "serien/FBI Most Wanted"})
	if err != nil || len(fbi2) != 2 {
		t.Errorf("off the box: %+v %v", fbi2, err)
	}
	if !seriesFolder("Gute Zeiten, schlechte Zeiten (1992)", "Gute Zeiten, schlechte Zeiten") || seriesFolder("Tatort-Reiniger", "Tatort") {
		t.Error("seriesFolder")
	}
}

func TestFindShow_SubtitleFallbackNeedsTheYear(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h := &Series{TVmaze: tvmazemock.New(), Store: st}
	ctx := context.Background()
	show, ok, err := h.findShow(ctx, "FBI: Special Crime Unit", 2018)
	if err != nil || !ok || show.ID != 601 {
		t.Fatalf("the German subtitle falls back to the original title when the year agrees: %+v %v %v", show, ok, err)
	}
	if _, ok, _ := h.findShow(ctx, "FBI: Irgendwas Anderes", 2021); ok {
		t.Error("a different start year must not match")
	}
	if _, ok, _ := h.findShow(ctx, "FBI: Noch Etwas", 0); ok {
		t.Error("without a year the fallback isn't tried")
	}
}
