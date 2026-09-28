package dmi

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// A database shaped like the one on the reference box (its older schema:
// no poster columns).
func fixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "dreammediainfo.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`CREATE TABLE media_cache (id INTEGER PRIMARY KEY AUTOINCREMENT, key TEXT UNIQUE, title TEXT, type TEXT, data TEXT, ts INTEGER)`,
		`INSERT INTO media_cache (key, title, type, data, ts) VALUES ('das familiengericht_tv', 'Das Familiengericht', 'tv', '{"_type":"tv","id":121796,"name":"Das Familiengericht","overview":" Gerichtsshow. ","genres":[{"id":10764,"name":"Reality"}],"first_air_date":"2001-05-02","vote_average":6.1,"fsk":"6"}', 1)`,
		`INSERT INTO media_cache (key, title, type, data, ts) VALUES ('der bestseller - mord auf italienisch_2002_movie', 'Der Bestseller', 'movie', '{"_type":"movie","id":555,"title":"Der Bestseller - Mord auf Italienisch","release_date":"2002-11-08","fsk":"12","genres":[{"name":"Komödie"},{"name":"Krimi"}]}', 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestLookup(t *testing.T) {
	r := &Reader{Path: fixture(t)}
	ctx := context.Background()
	e, ok := r.Lookup(ctx, "Das Familiengericht", "")
	if !ok || e.TMDBID != 121796 || e.Kind != "tv" || e.Overview != "Gerichtsshow." || e.Year != 2001 || e.FSK != "6" || len(e.Genres) != 1 {
		t.Fatalf("tv: %+v %v", e, ok)
	}
	e, ok = r.Lookup(ctx, "Der Bestseller - Mord auf Italienisch", "movie")
	if !ok || e.TMDBID != 555 || e.Year != 2002 {
		t.Fatalf("a film stored with its year: %+v %v", e, ok)
	}
	if _, ok := r.Lookup(ctx, "Das Familiengericht", "movie"); ok {
		t.Error("kind movie must not find a tv entry")
	}
	if _, ok := (&Reader{Path: filepath.Join(t.TempDir(), "none.db")}).Lookup(ctx, "x", ""); ok {
		t.Error("without the plugin nothing is found")
	}
}

func TestLookup_ColonSpelling(t *testing.T) {
	p := fixture(t)
	db, _ := sql.Open("sqlite", p)
	db.Exec(`INSERT INTO media_cache (key, title, type, data, ts) VALUES ('fbi - special crime unit_tv', 'FBI', 'tv', '{"id":77,"name":"FBI - Special Crime Unit"}', 1)`)
	db.Close()
	if e, ok := (&Reader{Path: p}).Lookup(context.Background(), "FBI: Special Crime Unit", "tv"); !ok || e.TMDBID != 77 {
		t.Errorf("a colon is stored as \" -\": %+v %v", e, ok)
	}
}
