package logosearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func entries(paths ...string) []Entry {
	var out []Entry
	for _, p := range paths {
		parts := strings.Split(p, "/")
		name := nameOf(parts[len(parts)-1])
		out = append(out, Entry{Path: p, Name: name, Country: parts[1], norm: Normalize(name)})
	}
	return out
}

func TestRank_FindsAChannelWhateverTheSpelling(t *testing.T) {
	all := entries(
		"countries/austria/orf-sport-plus-at.png",
		"countries/austria/atv-at.png",
		"countries/austria/atv2-at.png",
		"countries/germany/rtl-de.png",
		"countries/germany/hd/rtl-hd-de.png",
		"countries/germany/rtl-crime-de.png",
		"countries/italy/rai-1-it.png",
		"countries/italy/rai-2-it.png",
	)
	cases := []struct{ name, want string }{
		{"RTL Television", "countries/germany/rtl-de.png"},
		{"RTL HD", "countries/germany/rtl-de.png"},
		{"ATV", "countries/austria/atv-at.png"},
		{"RAI 1", "countries/italy/rai-1-it.png"},
		{"Rai1", "countries/italy/rai-1-it.png"},
		{"RAI 1 HD", "countries/italy/rai-1-it.png"},
	}
	for _, c := range cases {
		got := Rank(all, c.name, 3)
		if len(got) == 0 || got[0].Path != c.want {
			t.Errorf("%q: got %v, want first %s", c.name, got, c.want)
		}
	}
	// A channel the collection doesn't have gets nothing, not a wrong logo.
	if got := Rank(all, "ORF eins", 3); len(got) != 0 {
		t.Errorf("ORF eins: %v", got)
	}
	if got := Rank(all, "A", 3); len(got) != 0 {
		t.Errorf("a one-letter name matched %v", got)
	}
}

func TestValidPath(t *testing.T) {
	for _, p := range []string{"countries/austria/atv-at.png", "countries/germany/hd/rtl-hd-de.png"} {
		if !ValidPath(p) {
			t.Errorf("%s refused", p)
		}
	}
	for _, p := range []string{"../etc/passwd", "countries/../x.png", "countries/a/b.jpg", "other/x.png", "countries/a/b.png?x=1", "/countries/a/b.png", "countries/a b.png"} {
		if ValidPath(p) {
			t.Errorf("%s accepted", p)
		}
	}
}

func TestHTTP_SearchAndFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/tree"):
			w.Write([]byte(`{"tree":[{"path":"countries/austria/atv-at.png","type":"blob"},{"path":"countries/austria","type":"tree"},{"path":"README.md","type":"blob"}]}`))
		case r.URL.Path == "/raw/countries/austria/atv-at.png":
			w.Write([]byte("PNGDATA"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	h := &HTTP{TreeURL: srv.URL + "/tree", RawBase: srv.URL + "/raw/"}
	got, err := h.Search(context.Background(), "ATV", 5)
	if err != nil || len(got) != 1 || got[0].Country != "austria" {
		t.Fatalf("search: %v %v", got, err)
	}
	b, err := h.Fetch(context.Background(), got[0].Path)
	if err != nil || string(b) != "PNGDATA" {
		t.Fatalf("fetch: %q %v", b, err)
	}
	if _, err := h.Fetch(context.Background(), "../secret"); err != ErrInvalidPath {
		t.Fatalf("invalid path: %v", err)
	}
	// An unreachable collection is a plain error, not a crash.
	if _, err := (&HTTP{TreeURL: "http://127.0.0.1:1/x"}).Search(context.Background(), "ATV", 5); err != ErrUnavailable {
		t.Fatalf("unreachable: %v", err)
	}
}
