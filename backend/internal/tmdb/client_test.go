package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchAndEpisode_BothKeyForms(t *testing.T) {
	var gotKey, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotAuth = r.URL.Query().Get("api_key"), r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/search/multi":
			w.Write([]byte(`{"results":[{"media_type":"movie","id":949,"title":"Heat","release_date":"1995-12-15"},{"media_type":"person","id":1,"name":"Al Pacino"},{"media_type":"tv","id":7,"name":"Heat","first_air_date":""}]}`))
		case "/tv/7/season/1/episode/2":
			w.Write([]byte(`{"name":"Pilot, Teil 2"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &HTTPClient{HTTP: srv.Client(), BaseURL: srv.URL}

	res, err := c.Search(context.Background(), "v3key", "Heat", "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0] != (Result{Kind: "movie", ID: 949, Title: "Heat", Year: 1995}) || res[1].Kind != "tv" {
		t.Errorf("unexpected results (people must be skipped): %+v", res)
	}
	if gotKey != "v3key" || gotAuth != "" {
		t.Errorf("a v3 key goes in the query, got key=%q auth=%q", gotKey, gotAuth)
	}

	name, err := c.EpisodeTitle(context.Background(), "eyJhbGciOi.token", 7, 1, 2, "de-DE")
	if err != nil || name != "Pilot, Teil 2" {
		t.Fatalf("episode: %q, %v", name, err)
	}
	if gotKey != "" || gotAuth != "Bearer eyJhbGciOi.token" {
		t.Errorf("a v4 token goes in the Authorization header, got key=%q auth=%q", gotKey, gotAuth)
	}
	if name, err := c.EpisodeTitle(context.Background(), "k", 7, 9, 9, "de-DE"); err != nil || name != "" {
		t.Errorf("an unknown episode is no title, not an error: %q, %v", name, err)
	}
	if _, err := c.Search(context.Background(), "", "x", "de-DE"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("no key: want ErrNotConfigured, got %v", err)
	}
}
