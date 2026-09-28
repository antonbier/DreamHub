package enrich

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/tmdb/mock"
)

func newService(t *testing.T) (*Service, *mock.Client) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m := mock.New()
	return &Service{TMDB: m, Store: st, Images: &ImageCache{Dir: t.TempDir(), MaxBytes: 1 << 20}}, m
}

func TestLookup_MatchesExactTitlesAndCaches(t *testing.T) {
	s, m := newService(t)
	ctx := context.Background()
	got := s.Lookup(ctx, "key", "de-DE", []Query{
		{Title: "Tatort", Kind: "tv"},
		{Title: "Tatort: Borowski und der gute Mensch"}, // an episode: found as its series
		{Title: "Bones Spezial"},                        // TMDB only knows "Bones" here: no guess
		{Title: "Bones"},                                // the original title matches
	})
	if got[0] == nil || got[0].ID != 1002 || got[0].Poster != "/mock-tatort.png" || len(got[0].Genres) != 2 || got[0].Genres[0] != "Krimi" {
		t.Errorf("Tatort: %+v", got[0])
	}
	if got[1] == nil || got[1].ID != 1002 {
		t.Errorf("an episode title falls back to its series: %+v", got[1])
	}
	if got[2] != nil {
		t.Errorf("a near-miss must not be used — a wrong poster is worse than none: %+v", got[2])
	}
	if got[3] == nil || got[3].Title != "Bones - Die Knochenjägerin" || got[3].OriginalTitle != "Bones" {
		t.Errorf("original title match: %+v", got[3])
	}

	before := m.Searches.Load()
	again := s.Lookup(ctx, "key", "de-DE", []Query{{Title: "Tatort", Kind: "tv"}, {Title: "Bones Spezial"}})
	if m.Searches.Load() != before {
		t.Errorf("hits and misses are cached: %d more searches", m.Searches.Load()-before)
	}
	if again[0] == nil || again[0].ID != 1002 || again[1] != nil {
		t.Errorf("cached answers: %+v", again)
	}

	// Without a key, only the cache answers.
	noKey := s.Lookup(ctx, "", "de-DE", []Query{{Title: "Tatort", Kind: "tv"}, {Title: "Bauer sucht Frau International"}})
	if noKey[0] == nil || noKey[1] != nil {
		t.Errorf("no key: cached only: %+v", noKey)
	}
}

func TestLookup_RefetchesStaleEntries(t *testing.T) {
	s, m := newService(t)
	ctx := context.Background()
	now := time.Now()
	s.Now = func() time.Time { return now }
	s.Lookup(ctx, "key", "de-DE", []Query{{Title: "Tatort"}})
	n := m.Searches.Load()
	s.Now = func() time.Time { return now.Add(31 * 24 * time.Hour) }
	if got := s.Lookup(ctx, "key", "de-DE", []Query{{Title: "Tatort"}}); got[0] == nil || m.Searches.Load() == n {
		t.Errorf("a month-old entry is looked up again: %+v", got)
	}
}

func TestImage_CachesOnDiskAndPrunes(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	b, ct, err := s.Image(ctx, "w185", "/mock-tatort.png")
	if err != nil || len(b) == 0 || ct != "image/png" {
		t.Fatalf("image: %d bytes, %q, %v", len(b), ct, err)
	}
	if _, err := os.Stat(filepath.Join(s.Images.Dir, "w185", "mock-tatort.png")); err != nil {
		t.Errorf("not cached on disk: %v", err)
	}
	if _, _, err := s.Image(ctx, "w185", "/../../etc/passwd"); err == nil {
		t.Error("a path outside TMDB's image names must be refused")
	}

	s.Images.MaxBytes = int64(len(b)) // room for one
	s.Images.Put("w92", "/other.png", b)
	s.Images.Prune()
	var files int
	filepath.WalkDir(s.Images.Dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files++
		}
		return nil
	})
	if files != 1 {
		t.Errorf("prune should keep the cache under its cap, %d files left", files)
	}
}
