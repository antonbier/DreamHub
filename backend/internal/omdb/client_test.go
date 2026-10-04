package omdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRatings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("i") {
		case "tt0133093":
			w.Write([]byte(`{"Response":"True","imdbRating":"8.7","imdbVotes":"2,123,456","Metascore":"73","Ratings":[{"Source":"Internet Movie Database","Value":"8.7/10"},{"Source":"Rotten Tomatoes","Value":"83%"}]}`))
		case "tt0000001":
			w.Write([]byte(`{"Response":"False","Error":"Incorrect IMDb ID."}`))
		default:
			w.Write([]byte(`{"Response":"False","Error":"Request limit reached!"}`))
		}
	}))
	defer srv.Close()
	c := &HTTPClient{HTTP: srv.Client(), BaseURL: srv.URL + "/"}
	ctx := context.Background()

	r, err := c.Ratings(ctx, "k", "tt0133093")
	if err != nil || r != (Ratings{IMDb: 8.7, IMDbVotes: 2123456, RottenTomatoes: 83, Metascore: 73}) {
		t.Errorf("ratings: %+v %v", r, err)
	}
	if r, err := c.Ratings(ctx, "k", "tt0000001"); err != nil || r != (Ratings{}) {
		t.Errorf("an unknown title is no ratings, no error: %+v %v", r, err)
	}
	if _, err := c.Ratings(ctx, "k", "tt9999999"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("the daily limit: %v", err)
	}
	if _, err := c.Ratings(ctx, "k", "../etc"); !errors.Is(err, ErrInvalidID) {
		t.Errorf("only IMDb IDs: %v", err)
	}
	if _, err := c.Ratings(ctx, "", "tt0133093"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("no key: %v", err)
	}
}
