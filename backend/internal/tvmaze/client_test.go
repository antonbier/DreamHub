package tvmaze

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchAndEpisodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/shows":
			if r.URL.Query().Get("q") != "Tatort" {
				t.Errorf("query: %s", r.URL.RawQuery)
			}
			w.Write([]byte(`[{"score":0.9,"show":{"id":502,"name":"Tatort","premiered":"1970-11-29","language":"German"}}]`))
		case "/shows/502/episodes":
			w.Write([]byte(`[{"season":2026,"number":30,"name":"Haus am Meer","airdate":"2026-09-20","airstamp":"2026-09-20T18:15:00+00:00"},{"season":2026,"number":null,"name":"Special"},{"season":2026,"number":31,"name":"Gute Mensch","airdate":"","airstamp":null}]`))
		case "/shows/9/episodes":
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &HTTPClient{HTTP: srv.Client(), BaseURL: srv.URL}

	shows, err := c.SearchShows(context.Background(), "Tatort")
	if err != nil || len(shows) != 1 || shows[0].ID != 502 || shows[0].Name != "Tatort" {
		t.Fatalf("search: %+v %v", shows, err)
	}
	eps, err := c.Episodes(context.Background(), 502)
	if err != nil || len(eps) != 2 || eps[0].Number != 30 || eps[0].Airstamp != 1789928100 || eps[1].Airstamp != 0 {
		t.Fatalf("episodes (specials dropped, airstamp parsed): %+v %v", eps, err)
	}
	if _, err := c.Episodes(context.Background(), 9); !errors.Is(err, ErrRateLimited) {
		t.Errorf("429: want ErrRateLimited, got %v", err)
	}
}
